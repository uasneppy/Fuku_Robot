package main

import (
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/multichecker"
	"honnef.co/go/tools/quickfix"
)

func main() {
	var as []*analysis.Analyzer
	for _, a := range quickfix.Analyzers {
		as = append(as, a.Analyzer)
	}
	multichecker.Main(as...)
}
