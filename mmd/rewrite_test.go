package mmd

import (
	"strings"
	"testing"
)

func TestRewrite(t *testing.T) {
	flow := "```mermaid\nflowchart TD\n    A --> B\n```"
	flowArt := "```\n ┌───┐\n │ A │\n └───┘\n   │\n   ▼\n ┌───┐\n │ B │\n └───┘\n```"

	cases := []struct {
		name, in, want string
	}{
		{
			name: "flowchart replaced by art",
			in:   "before\n\n" + flow + "\n\nafter",
			want: "before\n\n" + flowArt + "\n\nafter",
		},
		{
			name: "LR direction renders horizontally",
			in:   "```mermaid\ngraph LR\n    A --> B\n```",
			want: "```\n ┌───┐  ┌───┐\n │ A │─▶│ B │\n └───┘  └───┘\n```",
		},
		{
			name: "BT direction keeps source",
			in:   "```mermaid\ngraph BT\n    A --> B\n```",
			want: "```mermaid\ngraph BT\n    A --> B\n```",
		},
		{
			name: "unsupported diagram kind keeps source",
			in:   "```mermaid\ngantt\n    title T\n```",
			want: "```mermaid\ngantt\n    title T\n```",
		},
		{
			name: "unsupported construct keeps source",
			in:   "```mermaid\nflowchart TD\n    subgraph s\n    end\n```",
			want: "```mermaid\nflowchart TD\n    subgraph s\n    end\n```",
		},
		{
			name: "parse error keeps source",
			in:   "```mermaid\nflowchart TD\n    A --> B & C\n```",
			want: "```mermaid\nflowchart TD\n    A --> B & C\n```",
		},
		{
			name: "empty mermaid block keeps source",
			in:   "```mermaid\n\n```",
			want: "```mermaid\n\n```",
		},
		{
			name: "tilde fence renders",
			in:   "~~~mermaid\nflowchart TD\n    A --> B\n~~~",
			want: flowArt,
		},
		{
			name: "mermaid info with attributes",
			in:   "```mermaid title=chart\nflowchart TD\n    A --> B\n```",
			want: flowArt,
		},
		{
			name: "non-mermaid fences untouched",
			in:   "```go\nfmt.Println(\"graph TD\")\n```",
			want: "```go\nfmt.Println(\"graph TD\")\n```",
		},
		{
			name: "fence after mermaid block unaffected",
			in:   flow + "\n\n```\nnot mermaid\n```",
			want: flowArt + "\n\n```\nnot mermaid\n```",
		},
		{
			name: "blockquoted diagram keeps prefix",
			in:   "> ```mermaid\n> flowchart TD\n>     A --> B\n> ```\nplain",
			want: "> ```\n>  ┌───┐\n>  │ A │\n>  └───┘\n>    │\n>    ▼\n>  ┌───┐\n>  │ B │\n>  └───┘\n> ```\nplain",
		},
		{
			name: "indented code lines never start a fence",
			in:   "    ```mermaid\n    flowchart TD\n        A --> B",
			want: "    ```mermaid\n    flowchart TD\n        A --> B",
		},
		{
			name: "unterminated mermaid fence keeps source",
			in:   "intro\n\n```mermaid\nflowchart TD\n    A --> B",
			want: "intro\n\n```mermaid\nflowchart TD\n    A --> B",
		},
		{
			name: "sequence diagram renders",
			in:   "```mermaid\nsequenceDiagram\n    A->>B: hi\n```",
			want: "```\n ┌───┐        ┌───┐\n │ A │        │ B │\n └───┘        └───┘\n   │     hi     │\n   │───────────▶│\n```",
		},
		{
			name: "math-like dollars in labels survive",
			in:   "```mermaid\nflowchart TD\n    A -->|cost $5| B\n```",
			want: "```\n ┌───┐\n │ A │\n └───┘\n   │ cost $5\n   ▼\n ┌───┐\n │ B │\n └───┘\n```",
		},
		{
			name: "backtick label cannot close the art fence",
			in:   "```mermaid\nsequenceDiagram\n    A->>B: ```\n```",
			want: "````\n ┌───┐         ┌───┐\n │ A │         │ B │\n └───┘         └───┘\n   │     ```     │\n   │────────────▶│\n````",
		},
		{
			name: "surrounding markdown untouched",
			in:   "a $x$ b\n\n" + flow + "\n\nplain `code` tail",
			want: "a $x$ b\n\n" + flowArt + "\n\nplain `code` tail",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Rewrite(tc.in); got != tc.want {
				t.Errorf("\nin:\n%s\n\ngot:\n%s\n\nwant:\n%s", tc.in, got, tc.want)
			}
		})
	}
}

// TestRewriteFenceSafety pins the fence-injection invariant: no art line —
// user labels included — may form a closing fence for the art block (a
// line of only backticks and spaces whose run reaches the opener's).
func TestRewriteFenceSafety(t *testing.T) {
	for _, in := range []string{
		"```mermaid\nflowchart TD\n    A --> B --> C\n```",
		"```mermaid\nsequenceDiagram\n    A->>B: `````\n```",
		"```mermaid\nflowchart TD\n    A -->|``code``| B\n```",
	} {
		got := Rewrite(in)
		lines := strings.Split(got, "\n")
		fence := lines[0]
		for _, l := range lines[1 : len(lines)-1] {
			if trimmed := strings.TrimLeft(l, " "); strings.Trim(trimmed, "` ") == "" && len(trimmed) >= len(fence) {
				t.Errorf("art line %q closes its own fence %q in:\n%s", l, fence, got)
			}
		}
	}
}
