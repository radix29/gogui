// Command gentokens generates the tokens file of internal/aqua/theme and its
// Markdown listing from a theme spec (PLAN.md S3.1a). It is run by
// "go generate ./internal/aqua/theme":
//
//	gentokens -spec spec/sequoia.json -out tokens.go -doc ../../../docs/tokens.md
//
// Paths are relative to the working directory (the theme package under go
// generate). The spec path is written into both headers as given.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/radix29/gogui/internal/aqua/theme/gen"
)

func main() {
	specPath := flag.String("spec", "", "theme spec (JSON)")
	out := flag.String("out", "", "generated Go file")
	doc := flag.String("doc", "", "generated Markdown listing")
	flag.Parse()
	if *specPath == "" || *out == "" || *doc == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: gentokens -spec FILE -out FILE -doc FILE")
		os.Exit(2)
	}
	if err := run(*specPath, *out, *doc); err != nil {
		fmt.Fprintln(os.Stderr, "gentokens:", err)
		os.Exit(1)
	}
}

func run(specPath, out, doc string) error {
	data, err := os.ReadFile(specPath)
	if err != nil {
		return err
	}
	s, err := gen.Parse(data)
	if err != nil {
		return fmt.Errorf("%s: %w", specPath, err)
	}
	goSrc, md, err := gen.Generate(s, filepath.ToSlash(specPath))
	if err != nil {
		return err
	}
	if err := os.WriteFile(out, goSrc, 0o644); err != nil {
		return err
	}
	return os.WriteFile(doc, md, 0o644)
}
