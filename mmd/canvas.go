package mmd

// canvas is a display-cell grid. Each cell holds either a literal glyph
// (box borders, arrowheads, text) or a set of line ports (strokes entering
// from the four sides). Strokes merge: orthogonal strokes become corners,
// tees, and crosses automatically, and a stroke landing on a box border
// demotes the border to ports first, so lines attach to boxes cleanly.
//
// Rendering order matters: strokes are drawn first, box borders and
// interiors overwrite stray strokes (occlusion), and text is written last.

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

// stroke style per port.
type stroke uint8

const (
	stNil stroke = iota
	stLight
	stDouble
	stDots
)

// port indexes into cell.ports: up, right, down, left.
const (
	pU = 0
	pR = 1
	pD = 2
	pL = 3
)

// portMask holds per-side stroke styles: up, right, down, left.
type portMask [4]stroke

type cell struct {
	ports portMask
	lit   string // literal cell text ("" = unset); combining marks are appended
	wide  bool   // continuation cell of a double-width rune
}

type canvas struct {
	cells map[[2]int]*cell
	w, h  int // used extents (cells)
}

func newCanvas() *canvas {
	return &canvas{cells: map[[2]int]*cell{}}
}

func (c *canvas) at(x, y int) *cell {
	k := [2]int{x, y}
	cl, ok := c.cells[k]
	if !ok {
		cl = &cell{}
		c.cells[k] = cl
	}
	if x >= c.w {
		c.w = x + 1
	}
	if y >= c.h {
		c.h = y + 1
	}
	return cl
}

func (c *canvas) peek(x, y int) *cell { return c.cells[[2]int{x, y}] }

// lit2ports demotes a box-drawing literal to its port mask so a crossing
// stroke can merge with it. Returns false when the literal is not a line
// glyph (text, arrowhead, slant): the literal wins and the stroke is lost.
func lit2ports(s string) (portMask, bool) {
	mk := func(u, r, d, l stroke) portMask { return portMask{u, r, d, l} }
	L, D := stLight, stDouble
	switch s {
	case "─":
		return mk(0, L, 0, L), true
	case "│":
		return mk(L, 0, L, 0), true
	case "┌", "╭":
		return mk(0, L, L, 0), true
	case "┐", "╮":
		return mk(0, 0, L, L), true
	case "└", "╰":
		return mk(L, L, 0, 0), true
	case "┘", "╯":
		return mk(L, 0, 0, L), true
	case "├":
		return mk(L, L, L, 0), true
	case "┤":
		return mk(L, 0, L, L), true
	case "┬":
		return mk(0, L, L, L), true
	case "┴":
		return mk(L, L, 0, L), true
	case "┼":
		return mk(L, L, L, L), true
	case "═":
		return mk(0, D, 0, D), true
	case "║":
		return mk(D, 0, D, 0), true
	case "╔":
		return mk(0, D, D, 0), true
	case "╗":
		return mk(0, 0, D, D), true
	case "╚":
		return mk(D, D, 0, 0), true
	case "╝":
		return mk(D, 0, 0, D), true
	case "╠":
		return mk(D, D, D, 0), true
	case "╣":
		return mk(D, 0, D, D), true
	case "╦":
		return mk(0, D, D, D), true
	case "╩":
		return mk(D, D, 0, D), true
	case "╬":
		return mk(D, D, D, D), true
	case "┄":
		return mk(0, stDots, 0, stDots), true
	case "┆":
		return mk(stDots, 0, stDots, 0), true
	}
	return [4]stroke{}, false
}

func (c *canvas) port(x, y, p int, st stroke) {
	cl := c.at(x, y)
	if cl.wide {
		return
	}
	if cl.lit != "" {
		ports, ok := lit2ports(cl.lit)
		if !ok {
			return // literal wins
		}
		cl.ports = ports
		cl.lit = ""
	}
	if cl.ports[p] < st {
		cl.ports[p] = st
	}
}

func (p portMask) count() int {
	n := 0
	for _, s := range p {
		if s != stNil {
			n++
		}
	}
	return n
}

// glyph resolves a port mask to a box-drawing rune. Dotted ports only
// render dotted on straight segments; corners, tees and crosses render
// light because Unicode has no dotted forms. Mixed light/double masks
// fall back to double only when uniform, else light (fine at this size).
func glyph(p portMask) rune {
	uniform := func(st stroke) bool {
		for _, s := range p {
			if s != stNil && s != st {
				return false
			}
		}
		return p.count() > 0
	}
	mask := 0
	for i, s := range p {
		if s != stNil {
			mask |= 1 << i
		}
	}
	// dotted straight runs have dedicated glyphs; corners/tees/crosses have
	// no dotted forms, so they fall back to light
	if uniform(stDots) {
		switch mask {
		case 1, 4, 1 | 4:
			return '┆'
		case 2, 8, 2 | 8:
			return '┄'
		}
	}
	// normalize dotted ports
	for i, s := range p {
		if s == stDots {
			p[i] = stLight
		}
	}
	// port-bit layout: up=1, right=2, down=4, left=8
	var table [16]rune
	switch {
	case uniform(stDouble):
		table = [16]rune{
			0: ' ', 1: '║', 2: '═', 3: '╚', 4: '║', 5: '║', 6: '╔', 7: '╠',
			8: '═', 9: '╝', 10: '═', 11: '╩', 12: '╗', 13: '╣', 14: '╦', 15: '╬',
		}
	default: // light
		table = [16]rune{
			0: ' ', 1: '│', 2: '─', 3: '└', 4: '│', 5: '│', 6: '┌', 7: '├',
			8: '─', 9: '┘', 10: '─', 11: '┴', 12: '┐', 13: '┤', 14: '┬', 15: '┼',
		}
	}
	return table[mask]
}

// lines assembles the visible grid as one string per row, trimming trailing
// whitespace.
func (c *canvas) lines() []string {
	out := make([]string, c.h)
	for y := range c.h {
		var b strings.Builder
		last := -1 // last x with visible content
		for x := range c.w {
			if cl := c.peek(x, y); cl != nil && (cl.lit != "" || cl.ports.count() > 0) {
				last = x
			}
		}
		for x := range last + 1 {
			if cl := c.peek(x, y); cl != nil {
				switch {
				case cl.lit != "":
					b.WriteString(cl.lit)
					if runewidth.StringWidth(cl.lit) == 0 {
						b.WriteByte(' ') // combining-only cell still takes a column
					}
				case cl.wide:
					// continuation cell of a wide rune: emit nothing; the
					// rune already fills this terminal column
				case cl.ports.count() > 0:
					b.WriteRune(glyph(cl.ports))
				default:
					b.WriteByte(' ')
				}
			} else {
				b.WriteByte(' ')
			}
		}
		out[y] = strings.TrimRight(b.String(), " ")
	}
	// drop fully empty rows at the edges
	first, lastRow := 0, len(out)-1
	for first <= lastRow && strings.TrimSpace(out[first]) == "" {
		first++
	}
	for lastRow >= first && strings.TrimSpace(out[lastRow]) == "" {
		lastRow--
	}
	return out[first : lastRow+1]
}

// --- drawing primitives -------------------------------------------------

// hline draws a stroke from (x1,y) to (x2,y) inclusive. Interior cells get
// both horizontal ports; each endpoint gets only its inward port, so a line
// ending at a corner merges into a proper corner/tee glyph.
func (c *canvas) hline(x1, x2, y int, st stroke) {
	if x1 > x2 {
		x1, x2 = x2, x1
	}
	if x1 == x2 {
		c.port(x1, y, pL, st)
		c.port(x1, y, pR, st)
		return
	}
	for x := x1; x <= x2; x++ {
		if x > x1 {
			c.port(x, y, pL, st)
		}
		if x < x2 {
			c.port(x, y, pR, st)
		}
	}
}

// vline draws a stroke from (x,y1) to (x,y2) inclusive (see hline).
func (c *canvas) vline(x, y1, y2 int, st stroke) {
	if y1 > y2 {
		y1, y2 = y2, y1
	}
	if y1 == y2 {
		c.port(x, y1, pU, st)
		c.port(x, y1, pD, st)
		return
	}
	for y := y1; y <= y2; y++ {
		if y > y1 {
			c.port(x, y, pU, st)
		}
		if y < y2 {
			c.port(x, y, pD, st)
		}
	}
}

// put writes a literal glyph (arrow head, corner of a box, slant edge).
func (c *canvas) put(x, y int, r rune) {
	cl := c.at(x, y)
	cl.lit = string(r)
	cl.ports = [4]stroke{}
}

// putStr writes literal text.
func (c *canvas) putStr(x, y int, s string) {
	for _, r := range s {
		c.put(x, y, r)
		x += max(runewidth.RuneWidth(r), 1)
	}
}

// text writes label text, one rune per cell, erasing any stroke under it.
// Wide runes mark the following cell as a continuation.
func (c *canvas) text(x, y int, s string) {
	prev := -1 // x of last non-combining cell
	for _, r := range s {
		w := runewidth.RuneWidth(r)
		if w == 0 && prev >= 0 {
			cl := c.at(prev, y)
			cl.lit += string(r)
			continue
		}
		cl := c.at(x, y)
		cl.lit = string(r)
		cl.ports = [4]stroke{}
		if w == 2 {
			cl.wide = true
			nx := c.at(x+1, y)
			nx.wide = true
			nx.lit = ""
			nx.ports = [4]stroke{}
		}
		prev = x
		x += max(w, 1)
	}
}

// textW measures display width of label text.
func textW(s string) int { return runewidth.StringWidth(s) }

func (c *canvas) String() string { return strings.Join(c.lines(), "\n") }
