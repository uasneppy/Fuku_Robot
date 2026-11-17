//go:build testtools

package i18n

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLocaleLoaderUsesSlashPaths holds I1: the locale files are read from an embed.FS, whose
// paths are always slash-separated, while filepath.Join uses the operating system's separator
// and builds "locales\en.yml" on Windows, which the embedded FS cannot open. A Windows build
// cannot run here, so the guard walks the package's own source (tests excluded) with go/parser
// and fails on any call of filepath.Join, the way TestNoRawNewCommand guards the command shim.
func TestLocaleLoaderUsesSlashPaths(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the package directory: %v", err)
	}
	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(".", name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "filepath" && sel.Sel.Name == "Join" {
				t.Errorf("%s calls filepath.Join; locale paths name embed.FS entries and must use path.Join",
					fset.Position(call.Pos()))
			}
			return true
		})
	}
}
