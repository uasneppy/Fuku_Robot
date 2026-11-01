package main

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runIn runs the CLI against the temp project of newProject (all files in one dir).
func runIn(t *testing.T, p *Project, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	dir := filepath.Dir(p.EnPath)
	var out, errb bytes.Buffer
	full := append([]string{"-locales", dir, "-data", dir}, args...)
	code = run(full, &out, &errb)
	return code, out.String(), errb.String()
}

func TestCheckExitCodes(t *testing.T) {
	p := newProject(t)
	if code, _, errOut := runIn(t, p, "-init"); code != 0 {
		t.Fatalf("-init exit %d: %s", code, errOut)
	}
	code, out, _ := runIn(t, p, "-check")
	if code != 1 || !strings.Contains(out, "plain_yes") {
		t.Fatalf("-check with nothing confirmed: exit %d, output:\n%s", code, out)
	}
	confirmAll(t, p)
	if code, out, _ := runIn(t, p, "-check"); code != 0 {
		t.Fatalf("-check with everything confirmed: exit %d, output:\n%s", code, out)
	}
}

func TestInitRefusesToOverwrite(t *testing.T) {
	p := newProject(t)
	if code, _, _ := runIn(t, p, "-init"); code != 0 {
		t.Fatal("first -init should work")
	}
	code, _, errOut := runIn(t, p, "-init")
	if code != 2 || !strings.Contains(errOut, "-force") {
		t.Fatalf("second -init: exit %d, stderr %q", code, errOut)
	}
	if code, _, errOut := runIn(t, p, "-init", "-force"); code != 0 {
		t.Fatalf("-init -force: exit %d: %s", code, errOut)
	}
}

func TestSyncPrintsHowManyKeysWereAdded(t *testing.T) {
	p := newProject(t)
	runIn(t, p, "-init")
	appendTo(t, p.EnPath, "brand_new: Brand new\nanother_new: Another\n")
	code, out, errOut := runIn(t, p, "-sync")
	if code != 0 || !strings.Contains(out, "added 2 keys") {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	if _, out, _ := runIn(t, p, "-sync"); !strings.Contains(out, "nothing to add") {
		t.Fatalf("a second sync should say so, got %q", out)
	}
}

func TestRunFromWrongDirectoryExplainsItself(t *testing.T) {
	var out, errb bytes.Buffer
	code := run([]string{"-locales", filepath.Join(t.TempDir(), "nope"), "-check"}, &out, &errb)
	if code != 2 || !strings.Contains(errb.String(), "repository root") {
		t.Fatalf("exit %d, stderr %q", code, errb.String())
	}
}

func TestConflictingModeFlagsAreRefused(t *testing.T) {
	p := newProject(t)
	if code, _, errOut := runIn(t, p, "-init", "-check"); code != 2 || !strings.Contains(errOut, "one of") {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
}

func TestUnknownFlagIsAUsageError(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"-bogus"}, &out, &errb); code != 2 {
		t.Fatalf("exit %d", code)
	}
}

// The tool must run without a database: alita/config and alita/db connect to
// PostgreSQL in init(), before main, so importing either would break `go run`.
func TestToolDoesNotImportDatabasePackages(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", ".")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list failed: %v\n%s", err, out)
	}
	for _, dep := range strings.Fields(string(out)) {
		if strings.Contains(dep, "Alita_Robot/alita/") {
			t.Errorf("the tool depends on %s", dep)
		}
	}
}
