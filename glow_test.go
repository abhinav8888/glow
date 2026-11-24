package main

import (
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestGlowFlags(t *testing.T) {
	tt := []struct {
		args  []string
		check func() bool
	}{
		{
			args: []string{"-p"},
			check: func() bool {
				return pager
			},
		},
		{
			args: []string{"-s", "light"},
			check: func() bool {
				return style == "light"
			},
		},
		{
			args: []string{"-w", "40"},
			check: func() bool {
				return width == 40
			},
		},
	}

	for _, v := range tt {
		err := rootCmd.ParseFlags(v.args)
		if err != nil {
			t.Fatal(err)
		}
		if !v.check() {
			t.Errorf("Parsing flag failed: %s", v.args)
		}
	}
}

func TestCLIReadsMermaidFromStdin(t *testing.T) {
	previousPager, previousStyle := pager, style
	t.Cleanup(func() { pager, style = previousPager, previousStyle })
	pager, style = false, "ascii"
	src := &source{reader: io.NopCloser(strings.NewReader("```mermaid\nflowchart LR\nA[Start] --> B[Done]\n```\n"))}
	var out strings.Builder
	if err := executeCLI(&cobra.Command{}, src, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Start") || !strings.Contains(out.String(), "▶") || strings.Contains(out.String(), "flowchart LR") {
		t.Errorf("expected a rendered flowchart on stdout, got %q", out.String())
	}
}
