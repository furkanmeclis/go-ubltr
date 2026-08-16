package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

func cmdGenerate(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("generate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprint(stderr, `Usage: ubltr generate

Regenerates Go types from vendored XSD and GİB code lists.
Run from a go-ubltr repository checkout (or any subdirectory).
`)
	}
	if err := fs.Parse(flagsFirst(args)); err != nil {
		return 1
	}
	if fs.NArg() != 0 {
		fs.Usage()
		return 1
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "ubltr generate: %v\n", err)
		return 1
	}
	root, err := findRepoRoot(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "ubltr generate: %v\n", err)
		return 1
	}

	targets := []string{
		"./schema/ubltr121",
		"./schema/earchive",
		"./codelist",
	}
	for _, t := range targets {
		cmd := exec.Command("go", "generate", t)
		cmd.Dir = root
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(stderr, "ubltr generate: go generate %s: %v\n", t, err)
			return 1
		}
	}
	return 0
}

func findRepoRoot(start string) (string, error) {
	dir := start
	for {
		gen := filepath.Join(dir, "schema", "ubltr121", "generate.go")
		tool := filepath.Join(dir, "cmd", "ubltr-gen", "main.go")
		if fileExists(gen) && fileExists(tool) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("not a go-ubltr checkout; run from a clone of the repository")
		}
		dir = parent
	}
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}
