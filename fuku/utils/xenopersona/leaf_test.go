//go:build testtools

package xenopersona

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestXenopersonaPackageIsLeaf keeps the package a standard-library leaf. The
// spike tool imports it, and an import of fuku/config or fuku/db would run their
// init functions, which connect to PostgreSQL before main.
func TestXenopersonaPackageIsLeaf(t *testing.T) {
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
			first, _, _ := strings.Cut(path, "/")
			if strings.Contains(first, ".") {
				t.Errorf("%s imports %q: xenopersona may import the standard library only", name, path)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no non-test source files found to check")
	}
}
