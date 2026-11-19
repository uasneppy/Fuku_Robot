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

// TestPMCopyCallers holds PMSG-05 structurally (D-04, D-33): a lockdown's joiner bans, "Ban N recent
// joiners" and every Staff Group action must never send a private copy, so no raid-scale or staff path
// may reach the private copy engine. It parses every non-test file of the package and compares the
// files that use offerEventPM and registerEventPM (any reference but the declaration itself) with the
// two allow-lists below, and it fails when a file whose base name starts with staff, lockdown or raid
// names offerEventPM, registerEventPM, offerReplyPM, replyEvent or any identifier starting with pmCopy.
// Each allow-list entry carries the reason the file may use the engine, and an entry no longer found
// fails, so the lists stay exact. A later phase that gives its events a private copy adds its caller
// here with a reason, never a staff, lockdown or raid file.
func TestPMCopyCallers(t *testing.T) {
	allowed := map[string]map[string]string{
		"offerEventPM": {
			"event_message.go": "the moderation pipeline's reply step, after the punishment",
		},
		"registerEventPM": {
			"event_message_pm.go": "the manual events' specs, registered with the engine",
		},
	}
	forbiddenNames := map[string]bool{"offerEventPM": true, "registerEventPM": true, "offerReplyPM": true, "replyEvent": true}
	forbiddenPrefixes := []string{"staff", "lockdown", "raid"}

	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	fset := token.NewFileSet()
	users := map[string]map[string]bool{"offerEventPM": {}, "registerEventPM": {}}
	scanned := 0
	for _, path := range paths {
		base := filepath.Base(path)
		if strings.HasSuffix(base, "_test.go") {
			continue
		}
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		file, err := parser.ParseFile(fset, path, src, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		scanned++
		declared := map[*ast.Ident]bool{}
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok {
				declared[fn.Name] = true
			}
		}
		guarded := false
		for _, prefix := range forbiddenPrefixes {
			guarded = guarded || strings.HasPrefix(base, prefix)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			ident, ok := node.(*ast.Ident)
			if !ok {
				return true
			}
			if guarded && (forbiddenNames[ident.Name] || strings.HasPrefix(ident.Name, "pmCopy")) {
				t.Errorf("%s names %s: a staff, lockdown or raid path must never reach the private copy engine (PMSG-05)",
					base, ident.Name)
			}
			if set, tracked := users[ident.Name]; tracked && !declared[ident] {
				set[base] = true
			}
			return true
		})
	}
	if scanned == 0 {
		t.Fatal("no source file was scanned")
	}

	for name, files := range users {
		var got []string
		for file := range files {
			got = append(got, file)
		}
		sort.Strings(got)
		for _, file := range got {
			if reason, ok := allowed[name][file]; !ok || strings.TrimSpace(reason) == "" {
				t.Errorf("%s uses %s but is not in its allow-list (or has no reason): add it with a reason, never from a staff, lockdown or raid path", file, name)
			}
		}
		for file := range allowed[name] {
			if !files[file] {
				t.Errorf("the allow-list of %s names %s, which no longer uses it: remove the stale entry", name, file)
			}
		}
	}
}
