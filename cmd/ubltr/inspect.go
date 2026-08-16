package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/furkanmeclis/go-ubltr"
)

func cmdInspect(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprint(stderr, "Usage: ubltr inspect FILE...\n")
	}
	if err := fs.Parse(flagsFirst(args)); err != nil {
		return 1
	}
	files, err := expandFiles(fs.Args())
	if err != nil {
		fmt.Fprintf(stderr, "ubltr inspect: %v\n", err)
		return 1
	}
	if len(files) == 0 {
		fs.Usage()
		return 1
	}
	failed := false
	for i, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(stderr, "ubltr inspect: %v\n", err)
			failed = true
			continue
		}
		sum, err := ubltr.Inspect(data)
		if err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", path, err)
			failed = true
			continue
		}
		if i > 0 {
			fmt.Fprintln(stdout)
		}
		if len(files) > 1 {
			fmt.Fprintf(stdout, "%s\n", path)
		}
		fmt.Fprintln(stdout, sum.String())
	}
	if failed {
		return 1
	}
	return 0
}
