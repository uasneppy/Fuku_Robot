//go:build testtools

package helpers

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
)

// commandUpdate builds a message update whose text starts with a bot command entity.
func commandUpdate(bot *gotgbot.Bot, text string) *ext.Context {
	msg := &gotgbot.Message{
		MessageId: 1,
		Date:      1,
		Chat:      gotgbot.Chat{Id: -1001234, Type: "supergroup"},
		From:      &gotgbot.User{Id: 77, FirstName: "Member"},
		Text:      text,
		Entities:  []gotgbot.MessageEntity{{Type: "bot_command", Offset: 0, Length: int64(len(text))}},
	}
	return ext.NewContext(bot, &gotgbot.Update{UpdateId: 1, Message: msg}, nil)
}

func TestBotCommandRegistry(t *testing.T) {
	bot := &gotgbot.Bot{Token: "999:test", User: gotgbot.User{Id: 999, IsBot: true, Username: "FukuTestBot"}}
	noop := func(*gotgbot.Bot, *ext.Context) error { return nil }

	handler := NewBotCommand("Probe17", noop)
	reference := handlers.NewCommand("Probe17", noop)

	for _, text := range []string{"/probe17", "/Probe17", "/probe17@FukuTestBot", "/Probe17@fukutestbot", "/probe17@OtherBot", "/probe18"} {
		ctx := commandUpdate(bot, text)
		if got, want := handler.CheckUpdate(bot, ctx), reference.CheckUpdate(bot, ctx); got != want {
			t.Errorf("CheckUpdate(%q) = %v, the library's own handler says %v", text, got, want)
		}
	}
	if !handler.CheckUpdate(bot, commandUpdate(bot, "/Probe17@FukuTestBot")) {
		t.Error("the shim's handler does not match /Probe17@FukuTestBot")
	}
	if handler.Command != "probe17" {
		t.Errorf("handler command = %q, want the lower-cased name", handler.Command)
	}

	for name, want := range map[string]bool{"probe17": true, "PROBE17": true, "Probe17": true, "probe18": false, "": false} {
		if got := IsBotCommand(name); got != want {
			t.Errorf("IsBotCommand(%q) = %v, want %v", name, got, want)
		}
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := fmt.Sprintf("Race17x%d", i)
			NewBotCommand(name, noop)
			if !IsBotCommand(name) {
				t.Errorf("IsBotCommand(%q) = false right after registering it", name)
			}
			_ = IsBotCommand("probe18")
		}(i)
	}
	wg.Wait()
}

// handlersImportPath is the package whose command constructor and Command type only the
// shim may use.
const handlersImportPath = "github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"

// rawCommandUses returns "file:line" for every use of the library's command constructor
// or a composite literal of its Command type in the parsed file, under every name the
// file imports the package by.
func rawCommandUses(fset *token.FileSet, file *ast.File) []string {
	names := map[string]bool{}
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil || path != handlersImportPath {
			continue
		}
		name := "handlers"
		if imp.Name != nil {
			name = imp.Name.Name
		}
		names[name] = true
	}
	if len(names) == 0 {
		return nil
	}
	isHandlers := func(x ast.Expr) bool {
		id, ok := x.(*ast.Ident)
		return ok && names[id.Name]
	}
	var found []string
	ast.Inspect(file, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.SelectorExpr:
			if isHandlers(v.X) && v.Sel.Name == "NewCommand" {
				found = append(found, fset.Position(v.Pos()).String())
			}
		case *ast.CompositeLit:
			if sel, ok := v.Type.(*ast.SelectorExpr); ok && isHandlers(sel.X) && sel.Sel.Name == "Command" {
				found = append(found, fset.Position(v.Pos()).String())
			}
		}
		return true
	})
	return found
}

// TestNoRawNewCommand fails when a non-test file builds a command handler without the
// recording shim, so a future command cannot escape the registry that command deletion
// reads. It checks only where a construction may appear, never what a file says.
func TestNoRawNewCommand(t *testing.T) {
	roots := []string{filepath.Join("..", ".."), filepath.Join("..", "..", "..", "main.go")}
	shim := filepath.Join("..", "..", "utils", "helpers", "command_registry.go")
	checked := 0
	fset := token.NewFileSet()
	check := func(path string) {
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Errorf("parse %s: %v", path, err)
			return
		}
		checked++
		if filepath.Clean(path) == filepath.Clean(shim) {
			return
		}
		for _, at := range rawCommandUses(fset, file) {
			t.Errorf("%s builds a command handler outside helpers.NewBotCommand (fuku/utils/helpers/command_registry.go)", at)
		}
	}
	for _, root := range roots {
		info, err := os.Stat(root)
		if err != nil {
			t.Fatalf("stat %s: %v", root, err)
		}
		if !info.IsDir() {
			check(root)
			continue
		}
		err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				check(path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
	if checked < 100 {
		t.Fatalf("the guard parsed only %d files: the roots are wrong", checked)
	}
}

// TestRawCommandUsesFindsEveryForm proves the guard sees the constructor call, a function
// value, an aliased import and a composite literal, so it cannot pass by looking at nothing.
func TestRawCommandUsesFindsEveryForm(t *testing.T) {
	const src = `package x

import (
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	hd "github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
)

var (
	a = handlers.NewCommand("a", nil)
	b = hd.NewCommand
	c = handlers.Command{Command: "c"}
	d = hd.Command{}
	e = handlers.NewMessage
)
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "x.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(rawCommandUses(fset, file)); got != 4 {
		t.Fatalf("rawCommandUses found %d uses, want the 4 command constructions", got)
	}
}
