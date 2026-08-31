package db

import (
	"os"
	"testing"
)

func TestIsCliModeActive(t *testing.T) {
	originalArgs := os.Args
	t.Cleanup(func() { os.Args = originalArgs })

	tests := []struct {
		name         string
		args         []string
		testDatabase string
		want         bool
	}{
		{name: "normal binary", args: []string{"/usr/local/bin/fuku"}, want: false},
		{name: "go run binary", args: []string{"/tmp/go-build123/b001/exe/fuku"}, want: false},
		{name: "test binary", args: []string{"/tmp/go-build123/b001/db.test"}, want: true},
		{name: "Windows test binary", args: []string{`C:\\Temp\\db.test.exe`}, want: true},
		{name: "database test binary", args: []string{"/tmp/go-build123/b001/db.test"}, testDatabase: "true", want: false},
		{name: "version flag", args: []string{"fuku", "--version"}, want: true},
		{name: "health flag", args: []string{"fuku", "--health"}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Args = tt.args
			t.Setenv("FUKU_TEST_DATABASE", tt.testDatabase)
			if got := isCliModeActive(); got != tt.want {
				t.Fatalf("isCliModeActive() = %v, want %v", got, tt.want)
			}
		})
	}
}
