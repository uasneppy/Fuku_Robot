//go:build testtools

package captchaweb

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestCaptchaWebPackageIsLeaf keeps the package free of the module's database init. An
// import of alita/config or alita/db runs their init functions, which connect to PostgreSQL
// before main (D-29), so the non-test files may import the standard library, go-redis and
// logrus only.
func TestCaptchaWebPackageIsLeaf(t *testing.T) {
	allowed := map[string]bool{
		"github.com/redis/go-redis/v9": true,
		"github.com/sirupsen/logrus":   true,
	}

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
			if strings.Contains(first, ".") && !allowed[path] {
				t.Errorf("%s imports %q: the captchaweb package may import the standard library, go-redis and logrus only", name, path)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no non-test source files found to check")
	}
}
