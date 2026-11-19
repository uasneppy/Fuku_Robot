//go:build testtools

package chatprompt

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestChatpromptPackageIsLeaf keeps the package free of fuku/config and fuku/db,
// whose init functions connect to PostgreSQL before main: it may import the
// standard library and the xenopersona leaf only.
func TestChatpromptPackageIsLeaf(t *testing.T) {
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
			path, _ := strconv.Unquote(spec.Path.Value)
			first, _, _ := strings.Cut(path, "/")
			if strings.Contains(first, ".") && path != "github.com/uasneppy/fuku_robot/fuku/utils/xenopersona" {
				t.Errorf("%s imports %q", name, path)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no source files checked")
	}
}
