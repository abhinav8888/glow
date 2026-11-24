package mmd

import "testing"

func renderSeqArt(t *testing.T, src string) string {
	t.Helper()
	sc, err := parseSeq(splitLines(src))
	if err != nil {
		t.Fatalf("parseSeq: %v", err)
	}
	return joinLines(renderSeq(sc))
}

func TestSeqRender(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{
			name: "activation shorthand preserves both lifelines",
			src:  "sequenceDiagram\n    A->>+B: hi\n    B-->>-A: yo\n",
			want: ` ┌───┐        ┌───┐
 │ A │        │ B │
 └───┘        └───┘
   │     hi     │
   │───────────▶│
   │     yo     │
   │◀┄┄┄┄┄┄┄┄┄┄┄│`,
		},
		{
			name: "self message",
			src:  "sequenceDiagram\n    A->A: think\n",
			want: ` ┌───┐
 │ A │
 └───┘
   │  think
   │──┐
   │<─┘`,
		},
		{
			name: "alt frame with else separator",
			src:  "sequenceDiagram\n    A->B: q\n    alt one\n        B->>A: r1\n    else two\n        B->>A: r2\n    end\n",
			want: ` ┌───┐        ┌───┐
 │ A │        │ B │
 └───┘        └───┘
   │      q     │
   │───────────>│
┌─ alt one ─────┼─┐
│  │     r1     │ │
│  │◀───────────│ │
│┄ two ┄┄┄┄┄┄┄┄┄┼┄│
│  │     r2     │ │
│  │◀───────────│ │
└──┴────────────┴─┘`,
		},
		{
			name: "notes left and over pair",
			src:  "sequenceDiagram\n    participant U as User\n    Note left of U: memo\n    Note over U,B: sync\n",
			want: ` ┌──────┐      ┌───┐
 │ User │      │ B │
 └──────┘      └───┘
┌──────┐         │
│ memo │         │
└──────┘         │
   ┌─────────────┐
   │ sync        │
   └─────────────┘`,
		},
		{
			name: "loop frame",
			src:  "sequenceDiagram\n    loop work\n        A->B: ping\n    end\n",
			want: ` ┌───┐          ┌───┐
 │ A │          │ B │
 └───┘          └───┘
┌─ loop work ─────┬─┐
│  │     ping     │ │
│  │─────────────>│ │
└──┴──────────────┴─┘`,
		},
		{
			name: "cross head and open head",
			src:  "sequenceDiagram\n    A-x B: cut\n    B->A: ok\n",
			want: ` ┌───┐         ┌───┐
 │ A │         │ B │
 └───┘         └───┘
   │     cut     │
   │────────────x│
   │      ok     │
   │<────────────│`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := renderSeqArt(t, tc.src); got != tc.want {
				t.Errorf("\ngot:\n%s\n\nwant:\n%s", got, tc.want)
			}
		})
	}
}

func TestSeqParseErrors(t *testing.T) {
	for _, src := range []string{
		"sequenceDiagram\n    rect rgb(0,0,0)\n        A->B: x\n    end\n", // rect blocks
		"sequenceDiagram\n    alt one\n        A->B: x\n",                  // unclosed frame
		"sequenceDiagram\n    else stray\n",                                // stray separator
		"sequenceDiagram\n    end\n",                                       // stray end
		"not a diagram\n    A->B: x\n",                                     // missing header
		"",                                                                 // empty
	} {
		if _, err := parseSeq(splitLines(src)); err == nil {
			t.Errorf("expected parse error for %q", src)
		}
	}
}

// TestSeqIgnoresNeutral: activation and autonumber parse fine even though
// they leave no mark on the art.
func TestSeqIgnoresNeutral(t *testing.T) {
	art := renderSeqArt(t, "sequenceDiagram\n    autonumber\n    activate A\n    deactivate A\n    A->B: hi\n")
	if art == "" {
		t.Error("expected art")
	}
}
