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
	format := fs.String("format", formatText, "output format: text or json")
	fs.Usage = func() {
		fmt.Fprint(stderr, `Usage: ubltr inspect [--format=text|json] FILE...

  --format=text   Output format: text or json (one object with every file)
`)
	}
	if err := fs.Parse(flagsFirst(args, "format")); err != nil {
		return 1
	}
	if err := checkFormat(*format); err != nil {
		fmt.Fprintf(stderr, "ubltr inspect: %v\n", err)
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
	if *format == formatJSON {
		return inspectJSON(files, stdout, stderr)
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

// inspectReport is the --format=json output of ubltr inspect.
type inspectReport struct {
	Files []inspectFileReport `json:"files"`
}

type inspectFileReport struct {
	Path    string         `json:"path"`
	Summary *ubltr.Summary `json:"summary,omitempty"`
	Error   string         `json:"error,omitempty"`
}

func inspectJSON(files []string, stdout, stderr io.Writer) int {
	report := inspectReport{Files: make([]inspectFileReport, 0, len(files))}
	failed := false
	for _, path := range files {
		r := inspectFileReport{Path: path}
		data, err := os.ReadFile(path)
		if err == nil {
			var sum ubltr.Summary
			if sum, err = ubltr.Inspect(data); err == nil {
				r.Summary = &sum
			}
		}
		if err != nil {
			r.Error = err.Error()
			failed = true
		}
		report.Files = append(report.Files, r)
	}
	if err := writeJSON(stdout, report); err != nil {
		fmt.Fprintf(stderr, "ubltr inspect: %v\n", err)
		return 1
	}
	if failed {
		return 1
	}
	return 0
}
