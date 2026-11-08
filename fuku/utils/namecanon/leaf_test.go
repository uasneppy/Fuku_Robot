//go:build testtools

package namecanon

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestCanonPackageIsLeaf keeps the package a leaf: the standard library and
// golang.org/x/text only. scripts/claude-spike imports it, and an import of
// fuku/config or fuku/db would run their init functions, which connect to
// PostgreSQL before main.
func TestCanonPackageIsLeaf(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}

	checked := 0
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		checked++
		for _, spec := range parsed.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatalf("%s: bad import %s", name, spec.Path.Value)
			}
			// Standard library paths have no dot in their first element.
			first, _, _ := strings.Cut(path, "/")
			if strings.Contains(first, ".") && path != "golang.org/x/text" && !strings.HasPrefix(path, "golang.org/x/text/") {
				t.Errorf("%s imports %q: namecanon may import the standard library and golang.org/x/text only", name, path)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no non-test source files found to check")
	}
}
