package mmd

import "strings"

func splitLines(s string) []string {
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

func joinLines(ls []string) string {
	return strings.Join(ls, "\n")
}
