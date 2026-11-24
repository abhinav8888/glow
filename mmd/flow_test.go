package mmd

import (
	"strings"
	"testing"
)

func renderFlowArt(t *testing.T, src string) string {
	t.Helper()
	fc, err := parseFlow(splitLines(src))
	if err != nil {
		t.Fatalf("parseFlow: %v", err)
	}
	return joinLines(renderFlow(fc))
}

func TestFlowRender(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{
			name: "chained edges",
			src:  "flowchart TD\n    A --> B --> C\n",
			want: ` ┌───┐
 │ A │
 └───┘
   │
   ▼
 ┌───┐
 │ B │
 └───┘
   │
   ▼
 ┌───┐
 │ C │
 └───┘`,
		},
		{
			name: "decision with labelled branches and back edge",
			src:  "flowchart TD\n    A[Start] --> B{Safe to deploy?}\n    B -->|yes| C[Ship it]\n    B -->|no| D[Roll back]\n    C --> E((Done))\n    D --> A\n",
			want: ` ┌───────┐
 │ Start │
 └───▲───┘
     │
     ├────┐
     │    ▼
 ╱‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾╲
   Safe to deploy?
 ╲_________________╱
     yes  │
     │┌───┴─── no ───┐
     │▼              ▼
 ┌─────────┐   ┌───────────┐
 │ Ship it │   │ Roll back │
 └─────────┘   └───────────┘
     └┼──────────────┘
     ┌┘
     ▼
 ╭‾‾‾‾‾‾╮
 │ Done │
 ╰______╯`,
		},
		{
			name: "dotted thick and plain edges",
			src:  "flowchart TD\n    A -.-> B\n    C ==> D\n    E --- F\n",
			want: ` ┌───┐   ┌───┐   ┌───┐
 │ A │   │ C │   │ E │
 └───┘   └───┘   └───┘
   ┆       ║       │
   ▼       ▼
 ┌───┐   ┌───┐   ┌───┐
 │ B │   │ D │   │ F │
 └───┘   └───┘   └───┘`,
		},
		{
			name: "all node shapes",
			src:  "flowchart TD\n    q([quick]) --> r[rect]\n    r --> s(round)\n    s --> t[[double]]\n    t --> u((circle))\n    u --> v{rhombus}\n    v --> w{{hexagon}}\n    w --> x[(cylinder)]\n",
			want: ` ╭───────╮
 │ quick │
 ╰───────╯
     │
     ▼
 ┌──────┐
 │ rect │
 └──────┘
     │
     ▼
 ╭───────╮
 │ round │
 ╰───────╯
     │
     └┐
      ▼
 ╔════════╗
 ║ double ║
 ╚════════╝
      │
      ▼
 ╭‾‾‾‾‾‾‾‾╮
 │ circle │
 ╰________╯
      │
      ▼
 ╱‾‾‾‾‾‾‾‾‾╲
   rhombus
 ╲_________╱
      │
      ▼
 ╱‾‾‾‾‾‾‾‾‾╲
 ⟨ hexagon ⟩
 ╲_________╱
      │
      └┐
       ▼
 ┌‾‾‾‾‾‾‾‾‾‾┐
 ║ cylinder ║
 └──────────┘`,
		},
		{
			name: "self loop with label",
			src:  "flowchart TD\n    A[Queue] -->|tick| A\n",
			want: ` ┌───────┐
 │ Queue │┐ tick
 └───▲───┘│
     └────┘`,
		},
		{
			name: "quoted labels and entities",
			src:  "flowchart TD\n    A[\"start (now)\"] --> B(\"end #59;done\")\n",
			want: ` ┌─────────────┐
 │ start (now) │
 └─────────────┘
        │
       ┌┘
       ▼
 ╭───────────╮
 │ end ;done │
 ╰───────────╯`,
		},
		{
			name: "bare label form",
			src:  "flowchart TD\n    A -- go --> B\n",
			want: ` ┌───┐
 │ A │
 └───┘
   │ go
   ▼
 ┌───┐
 │ B │
 └───┘`,
		},
		{
			name: "multi-line labels",
			src:  "flowchart TD\n    A[\"first line<br/>second\"] --> B[(\"store<br/>here\")]\n",
			want: ` ┌────────────┐
 │ first line │
 │   second   │
 └────────────┘
        │
     ┌──┘
     ▼
 ┌‾‾‾‾‾‾‾┐
 ║ store ║
 ║ here  ║
 └───────┘`,
		},
		{
			name: "LR chain with dotted back edge",
			src:  "flowchart LR\n    A[x1] --> B[x2] --> C[x3]\n    C -.->|back| A\n",
			want: ` ┌────┐  ┌────┐  ┌────┐
 │ x1 │─▶│ x2 │─▶│ x3 │┄┐
 └──▲─┘  └────┘  └────┘ ┆
    ┆                   ┆
    ┄┄ back ┄┄┄┄┄┄┄┄┄┄┄┄┘`,
		},
		{
			name: "LR bent edges",
			src:  "flowchart LR\n    A[Ingest data from everywhere] --> B[queue]\n    A --> C[wal wal wal wal]\n    C --> D[store]\n",
			want: ` ┌─────────────────────────────┐     ┌───────┐               ┌───────┐
 │ Ingest data from everywhere │─┬──▶│ queue │           ┌──▶│ store │
 └─────────────────────────────┘ │   └───────┘           │   └───────┘
                                 │                       │
                                 │                       │
                                 │   ┌─────────────────┐ │
                                 └──▶│ wal wal wal wal │─┘
                                     └─────────────────┘`,
		},
		{
			name: "subgraph frames",
			src:  "flowchart TD\n    subgraph box[\"a box\"]\n        A[one]\n        B[two]\n    end\n    C[out]\n    A --> C\n",
			want: `┌─ a box ─────────┐
│                 │
│                 │
│┌─────┐   ┌─────┐│
││ one │   │ two ││
│└─────┘   └─────┘│
└───┼─────────────┘
    ▼
 ┌─────┐
 │ out │
 └─────┘`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := renderFlowArt(t, tc.src); got != tc.want {
				t.Errorf("\ngot:\n%s\n\nwant:\n%s", got, tc.want)
			}
		})
	}
}

// TestFlowWideRunes: labels with double-width runes occupy exactly one
// display cell pair per rune; everything right of them stays aligned.
func TestFlowWideRunes(t *testing.T) {
	art := renderFlowArt(t, "flowchart TD\n    中[中文] --> B\n")
	want := ` ┌──────┐
 │ 中文 │
 └──────┘
     │
   ┌─┘
   ▼
 ┌───┐
 │ B │
 └───┘`
	if art != want {
		t.Errorf("\ngot:\n%s\n\nwant:\n%s", art, want)
	}
}

// TestFlowParseErrors pins the fallback surface: everything here must fail
// parsing so the rewriter keeps the original markdown block.
func TestFlowParseErrors(t *testing.T) {
	for _, src := range []string{
		"graph BT\n    A --> B\n",                 // unsupported direction
		"flowchart TD\n    subgraph x\n    end\n", // subgraphs
		"flowchart TD\n    A --> B & C\n",         // multi-target
		"flowchart TD\n    A --> B; B --> C\n",    // statement separators
		"not a diagram\n    A --> B\n",            // missing header
		"",                                        // empty
	} {
		if _, err := parseFlow(splitLines(src)); err == nil {
			t.Errorf("expected parse error for %q", src)
		}
	}
}

// TestFlowParallelogramDegrades: [/.../] shapes aren't drawn as
// parallelograms; they degrade to a square with the literal slashes kept,
// rather than sinking the whole diagram back to source.
func TestFlowParallelogramDegrades(t *testing.T) {
	art := renderFlowArt(t, "flowchart TD\n    A[/in/] --> B\n")
	want := ` ┌──────┐
 │ /in/ │
 └──────┘
     │
   ┌─┘
   ▼
 ┌───┐
 │ B │
 └───┘`
	if art != want {
		t.Errorf("\ngot:\n%s\n\nwant:\n%s", art, want)
	}
}

// TestFlowIgnoresStyling: style directives don't affect art but must not
// break parsing.
func TestFlowIgnoresStyling(t *testing.T) {
	art := renderFlowArt(t, "flowchart TD\n    style A fill:#f9f\n    A --> B\n")
	want := ` ┌───┐
 │ A │
 └───┘
   │
   ▼
 ┌───┐
 │ B │
 └───┘`
	if art != want {
		t.Errorf("\ngot:\n%s\n\nwant:\n%s", art, want)
	}
}

func TestFlowControlEntityStaysLiteral(t *testing.T) {
	art := renderFlowArt(t, "flowchart TD\nA[Version #9;] --> B")
	if !strings.Contains(art, "Version #9;") {
		t.Errorf("expected literal entity in label, got %q", art)
	}
}
