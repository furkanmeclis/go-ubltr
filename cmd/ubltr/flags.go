package main

import "strings"

// flagsFirst moves flag tokens before positional arguments so stdlib flag.Parse
// accepts both `ubltr render --pdf file.xml` and `ubltr render file.xml --pdf`.
func flagsFirst(args []string, valueNames ...string) []string {
	takes := make(map[string]bool, len(valueNames)*2)
	for _, n := range valueNames {
		takes["-"+n] = true
		takes["--"+n] = true
	}
	var flags, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			pos = append(pos, args[i:]...)
			break
		}
		if len(a) > 1 && a[0] == '-' && a != "-" {
			name, _, hasEq := strings.Cut(a, "=")
			flags = append(flags, a)
			if !hasEq && takes[name] && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		pos = append(pos, a)
	}
	out := make([]string, 0, len(flags)+len(pos))
	out = append(out, flags...)
	out = append(out, pos...)
	return out
}
