package mmd

// Diagram-kind dispatch: parse sniffs the first content line of a mermaid
// block (skipping blanks and %% comments) and picks a parser; unsupported
// diagram kinds fail here so the rewriter can keep the source block.

import (
	"errors"
	"strings"
)

type kind int

const (
	kindFlow kind = iota
	kindSeq
)

var errUnknown = errors.New("unsupported mermaid diagram")

// parse routes source to the diagram-kind parser.
func parse(src string) (any, kind, error) {
	for _, line := range strings.Split(src, "\n") {
		head := strings.TrimSpace(cutComment(line))
		if head == "" {
			continue
		}
		tok := head
		if i := strings.IndexAny(head, " \t"); i >= 0 {
			tok = head[:i]
		}
		switch strings.ToLower(tok) {
		case "graph", "flowchart":
			fc, err := parseFlow(strings.Split(src, "\n"))
			return fc, kindFlow, err
		case "sequencediagram":
			sc, err := parseSeq(strings.Split(src, "\n"))
			return sc, kindSeq, err
		}
		return nil, 0, errUnknown
	}
	return nil, 0, errUnknown
}

// cutComment removes a %% ... comment, respecting double-quoted spans.
func cutComment(s string) string {
	quoted := false
	for i := 0; i+1 < len(s); i++ {
		switch s[i] {
		case '"':
			quoted = !quoted
		case '%':
			if !quoted && s[i+1] == '%' {
				return s[:i]
			}
		}
	}
	return s
}
