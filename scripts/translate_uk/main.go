// Command translate_uk is a local editor for writing locales/uk.yml by hand.
// See README.md in this directory and the design spec under docs/superpowers/specs.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is main with its inputs and outputs passed in, so the tests can drive every
// mode. Exit codes: 0 fine, 1 -check found work left, 2 usage or environment error.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("translate_uk", flag.ContinueOnError)
	fs.SetOutput(stderr)
	doInit := fs.Bool("init", false, "create the draft locales/uk.yml from ru.yml")
	force := fs.Bool("force", false, "with -init: overwrite an existing uk.yml and clear confirmed.txt")
	doSync := fs.Bool("sync", false, "append keys that en.yml has and uk.yml lacks")
	doCheck := fs.Bool("check", false, "report progress; exit 1 until every key is confirmed")
	addr := fs.String("addr", "127.0.0.1:8080", "address of the editor (loopback only)")
	locales := fs.String("locales", "locales", "directory with en.yml, ru.yml and uk.yml")
	data := fs.String("data", filepath.Join("scripts", "translate_uk"), "directory with confirmed.txt and glossary.yml")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	modes := 0
	for _, on := range []bool{*doInit, *doSync, *doCheck} {
		if on {
			modes++
		}
	}
	if modes > 1 {
		fmt.Fprintln(stderr, "give only one of -init, -sync and -check")
		return 2
	}

	p := &Project{
		EnPath:       filepath.Join(*locales, "en.yml"),
		RuPath:       filepath.Join(*locales, "ru.yml"),
		UkPath:       filepath.Join(*locales, "uk.yml"),
		ProgressPath: filepath.Join(*data, "confirmed.txt"),
		GlossaryPath: filepath.Join(*data, "glossary.yml"),
	}
	if _, err := os.Stat(p.EnPath); err != nil {
		fmt.Fprintf(stderr, "cannot find %s: run this from the repository root (or pass -locales)\n", p.EnPath)
		return 2
	}

	switch {
	case *doInit:
		if err := p.Init(*force); err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		fmt.Fprintf(stdout, "created %s from %s, plus %s and %s\n", p.UkPath, p.RuPath, p.ProgressPath, p.GlossaryPath)
		return 0
	case *doSync:
		n, err := p.Sync()
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		switch n {
		case 0:
			fmt.Fprintln(stdout, "nothing to add: uk.yml has every key of en.yml")
		case 1:
			fmt.Fprintln(stdout, "added 1 key to uk.yml (unconfirmed)")
		default:
			fmt.Fprintf(stdout, "added %d keys to uk.yml (unconfirmed)\n", n)
		}
		return 0
	case *doCheck:
		report, ok := p.Check()
		fmt.Fprint(stdout, report)
		if !ok {
			return 1
		}
		return 0
	}
	if err := serve(*addr, p, stdout); err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	return 0
}
