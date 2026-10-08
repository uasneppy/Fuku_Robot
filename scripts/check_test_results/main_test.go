package main

import (
	"io"
	"strings"
	"testing"
)

func TestCheckResultsRejectsFailuresAndUnexpectedSkips(t *testing.T) {
	for _, input := range []string{
		`{"Action":"skip","Package":"repo/approvals","Test":"TestPersistApproval"}`,
		`{"Action":"fail","Package":"repo/approvals"}`,
		`{"Action":"build-fail","ImportPath":"repo/approvals"}`,
		``,
	} {
		if err := checkResults(strings.NewReader(input), io.Discard); err == nil {
			t.Errorf("accepted unsuccessful test stream %q", input)
		}
	}
	if err := checkResults(strings.NewReader(`{"Action":"pass","Package":"repo/approvals"}`), io.Discard); err != nil {
		t.Fatal(err)
	}
}

func TestCheckResultsAcceptsStaffTriggerSkipOnlyWithoutPostgres(t *testing.T) {
	const skip = `{"Action":"skip","Package":"github.com/divkix/Alita_Robot/alita/db/staff","Test":"TestStaffExclusivityTrigger"}
{"Action":"pass","Package":"repo/approvals"}`
	const otherStaffSkip = `{"Action":"skip","Package":"github.com/divkix/Alita_Robot/alita/db/staff","Test":"TestStaffConstraintsRejectSelfLink"}
{"Action":"pass","Package":"repo/approvals"}`

	t.Setenv("ALITA_TEST_DATABASE", "")
	if err := checkResults(strings.NewReader(skip), io.Discard); err != nil {
		t.Errorf("rejected the staff trigger skip with ALITA_TEST_DATABASE unset: %v", err)
	}
	if err := checkResults(strings.NewReader(otherStaffSkip), io.Discard); err == nil {
		t.Error("accepted an unrelated skip in the staff package")
	}

	t.Setenv("ALITA_TEST_DATABASE", "true")
	if err := checkResults(strings.NewReader(skip), io.Discard); err == nil {
		t.Error("accepted the staff trigger skip with ALITA_TEST_DATABASE=true")
	}
}
