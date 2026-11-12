//go:build testtools

package modules

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Plan 17-04 (D-23, C-03): CommandDeletionProcessor deletes a command after every handler group has
// run, so no handler may reply to the command message from a goroutine that outlives the handler:
// that reply would target a message the wrapper has already deleted (Telegram answers "message to
// be replied not found", or the reply loses its quote). This guard is structural: it parses every
// non-test file of the package, finds each `go` statement whose function literal, or whose named
// same-package callee (followed up to repliesGuardDepth calls deep), calls a method named Reply or
// builds ReplyParameters, and compares the (file, enclosing function) pairs with the allow-list
// below. The list was recorded from the tree when plan 17-04 ran; each entry says why its reply
// never targets a member's command after the handler returned. A new pair fails the test: reply
// inside the handler, or add the pair here with a reason that plan 17-11 reviews.

// repliesGuardDepth bounds how many same-package calls deep a goroutine's callees are followed.
const repliesGuardDepth = 4

// repliesGuardAllowed is the recorded set of goroutines that reply, keyed "file.go: Function". It is
// empty: when plan 17-04 ran, no goroutine of the package replied (research S-01), so every reply to
// a command happens inside its handler, before the wrapper's delete. The scan resolves calls by
// name (a plain function) and by the caller's own receiver only, so a reply reached through another
// object's method is outside what it sees. Plan 17-11 reviewed that limit (C-03, D-23): a throwaway
// scan that followed every selector call by bare method name, to depth 4, found three more
// candidates and no real reply. Two are the method name `start` (raidAIFireAsync reaches
// raidPostTracker.start in raid_trigger.go, which a bare name confuses with help.go's `start`, a
// handler that replies); the third is startJoinedStaffRun in staff_action_run.go, the Staff Group
// run's coordinator, which can reply to a staff command in a Staff Group, a chat no member rule
// ever deletes in (deletionAct refuses it, D-17). The allow-list therefore stays empty and the
// guard stays as it is.
var repliesGuardAllowed = map[string]string{}

// repliesGuardFinding is one goroutine of the package that replies.
type repliesGuardFinding struct {
	File     string
	Function string
}

func (f repliesGuardFinding) key() string { return f.File + ": " + f.Function }

// repliesGuardDecls indexes the package's functions by "name" and its methods by "Type.name".
type repliesGuardDecls map[string][]*ast.FuncDecl

// repliesGuardScope is the receiver of the function a body belongs to: a call `r.name()` on that
// receiver resolves to the method Type.name of the same type; any other selector call is not
// followed, because without type information a bare method name cannot be told from another type's.
type repliesGuardScope struct {
	recvName string
	recvType string
}

func repliesGuardScopeOf(fn *ast.FuncDecl) repliesGuardScope {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return repliesGuardScope{}
	}
	field := fn.Recv.List[0]
	typ := field.Type
	if star, ok := typ.(*ast.StarExpr); ok {
		typ = star.X
	}
	ident, ok := typ.(*ast.Ident)
	if !ok || len(field.Names) == 0 {
		return repliesGuardScope{}
	}
	return repliesGuardScope{recvName: field.Names[0].Name, recvType: ident.Name}
}

func repliesGuardDeclKey(fn *ast.FuncDecl) string {
	if scope := repliesGuardScopeOf(fn); scope.recvType != "" {
		return scope.recvType + "." + fn.Name.Name
	}
	return fn.Name.Name
}

// repliesGuardScan parses the non-test files of dir and returns every goroutine that replies.
func repliesGuardScan(t *testing.T, dir string) []repliesGuardFinding {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("glob %s: %v", dir, err)
	}
	fset := token.NewFileSet()
	type parsedFile struct {
		name string
		file *ast.File
	}
	var files []parsedFile
	decls := repliesGuardDecls{}
	for _, path := range paths {
		base := filepath.Base(path)
		if strings.HasSuffix(base, "_test.go") {
			continue
		}
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		parsed, err := parser.ParseFile(fset, path, src, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		files = append(files, parsedFile{name: base, file: parsed})
		for _, decl := range parsed.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
				decls[repliesGuardDeclKey(fn)] = append(decls[repliesGuardDeclKey(fn)], fn)
			}
		}
	}

	var found []repliesGuardFinding
	for _, pf := range files {
		for _, decl := range pf.file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			scope := repliesGuardScopeOf(fn)
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				goStmt, ok := node.(*ast.GoStmt)
				if !ok {
					return true
				}
				if repliesGuardCallReplies(goStmt.Call, decls, scope, repliesGuardDepth, map[string]bool{}) {
					found = append(found, repliesGuardFinding{File: pf.name, Function: fn.Name.Name})
				}
				return true
			})
		}
	}
	return found
}

// repliesGuardCallReplies reports whether the called function replies: a function literal is
// inspected in place, a named same-package function or a method on the caller's own receiver by
// every declaration of that name.
func repliesGuardCallReplies(call *ast.CallExpr, decls repliesGuardDecls, scope repliesGuardScope, depth int, seen map[string]bool) bool {
	if lit, ok := call.Fun.(*ast.FuncLit); ok {
		return repliesGuardBodyReplies(lit.Body, decls, scope, depth, seen)
	}
	key := repliesGuardCalleeKey(call, scope)
	if key == "" || depth == 0 || seen[key] {
		return false
	}
	seen[key] = true
	for _, fn := range decls[key] {
		if repliesGuardBodyReplies(fn.Body, decls, repliesGuardScopeOf(fn), depth-1, seen) {
			return true
		}
	}
	return false
}

// repliesGuardCalleeKey names the declaration a call resolves to, or "" when it cannot be told:
// `name()` is the function name, `recv.name()` on the caller's own receiver is Type.name.
func repliesGuardCalleeKey(call *ast.CallExpr, scope repliesGuardScope) string {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name
	case *ast.SelectorExpr:
		if x, ok := fun.X.(*ast.Ident); ok && scope.recvName != "" && x.Name == scope.recvName {
			return scope.recvType + "." + fun.Sel.Name
		}
	}
	return ""
}

// repliesGuardBodyReplies reports whether the body calls a method named Reply or names
// ReplyParameters, or calls a same-package function that does.
func repliesGuardBodyReplies(body *ast.BlockStmt, decls repliesGuardDecls, scope repliesGuardScope, depth int, seen map[string]bool) bool {
	replies := false
	ast.Inspect(body, func(node ast.Node) bool {
		if replies {
			return false
		}
		switch n := node.(type) {
		case *ast.Ident:
			if n.Name == "ReplyParameters" {
				replies = true
			}
		case *ast.SelectorExpr:
			if n.Sel.Name == "ReplyParameters" {
				replies = true
			}
		case *ast.CallExpr:
			if sel, ok := n.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Reply" {
				replies = true
			} else if depth > 0 && repliesGuardCallReplies(n, decls, scope, depth, seen) {
				replies = true
			}
		}
		return !replies
	})
	return replies
}

func TestCommandRepliesStayInsideTheHandler(t *testing.T) {
	found := repliesGuardScan(t, ".")

	got := map[string]bool{}
	for _, f := range found {
		got[f.key()] = true
	}
	var unexpected, stale []string
	for key := range got {
		if _, ok := repliesGuardAllowed[key]; !ok {
			unexpected = append(unexpected, key)
		}
	}
	for key, reason := range repliesGuardAllowed {
		if !got[key] {
			stale = append(stale, key)
		}
		if strings.TrimSpace(reason) == "" {
			t.Errorf("allow-list entry %q has no reason", key)
		}
	}
	sort.Strings(unexpected)
	sort.Strings(stale)
	if len(unexpected) > 0 {
		t.Errorf("a goroutine that replies was added: %s\n"+
			"A command is deleted after its handler returned (CommandDeletionProcessor), so a reply sent from a goroutine can target a deleted message. "+
			"Reply inside the handler, or add the pair to repliesGuardAllowed with the reason it never replies to a member's command after the handler returned.",
			strings.Join(unexpected, ", "))
	}
	if len(stale) > 0 {
		t.Errorf("allow-list entries no longer found (remove them): %s", strings.Join(stale, ", "))
	}
}

// TestRepliesGuardFindsAReplyingGoroutine proves the scanner sees what it claims to: a goroutine
// literal that calls Reply, one that builds ReplyParameters one reaching Reply through a named
// callee and one through a method of its own receiver are found, and a goroutine that only sends, or
// that calls another type's method of the same name (which cannot be resolved without types), is not.
func TestRepliesGuardFindsAReplyingGoroutine(t *testing.T) {
	dir := t.TempDir()
	src := `package modules

func literal(msg Msg)   { go func() { msg.Reply(nil, "x", nil) }() }
func params()           { go func() { _ = ReplyParameters{} }() }
func named(msg Msg)     { go replyLater(msg) }
func replyLater(m Msg)  { m.Reply(nil, "x", nil) }
func sendOnly(b Bot)    { go func() { b.SendMessage(1, "x", nil) }() }

type mod struct{}

func (m mod) viaMethod()     { go m.later() }
func (m mod) later()         { m.msg.Reply(nil, "x", nil) }
func (m mod) collide(o Other) { go o.start() }
func (o Other) start()       { o.msg.Reply(nil, "x", nil) }
func (m mod) start()         {}
`
	if err := os.WriteFile(filepath.Join(dir, "sample.go"), []byte(src), 0o600); err != nil {
		t.Fatalf("write sample: %v", err)
	}
	var keys []string
	for _, f := range repliesGuardScan(t, dir) {
		keys = append(keys, f.Function)
	}
	sort.Strings(keys)
	if got, want := strings.Join(keys, ","), "literal,named,params,viaMethod"; got != want {
		t.Fatalf("goroutines found = %q, want %q", got, want)
	}
}
