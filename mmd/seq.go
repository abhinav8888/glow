package mmd

// Sequence diagram renderer: participants become boxed headers with
// lifelines; messages span lifelines as labelled arrow rows; notes draw as
// boxes anchored left/right/over; alt/loop/opt/par-style blocks draw as
// labelled frames with dotted separators for else/and.

import (
	"strings"
)

type seqChart struct {
	actors []*seqActor
	byID   map[string]*seqActor
	events []seqEvent
}

type seqActor struct {
	id, name string
	cx       int // lifeline column
	boxL     int // header box left edge
	boxW     int
}

type evKind int

const (
	evMsg evKind = iota
	evNote
	evFrameOpen
	evFrameSep
	evFrameClose
)

type headKind int

const (
	headOpen  headKind = iota // >
	headFill                  // ▶
	headCross                 // x
)

type seqEvent struct {
	kind  evKind
	top   int // first canvas row occupied
	h     int // rows occupied
	msg   *seqMsg
	note  *seqNote
	label string // separator label (evFrameSep)
	frame string // opener keyword (evFrameOpen)
	flbl  string // opener label
}

type seqMsg struct {
	from, to *seqActor
	label    string
	st       stroke
	head     headKind
}

type noteAnchor int

const (
	noteLeft noteAnchor = iota
	noteRight
	noteOver
)

type seqNote struct {
	anchor noteAnchor
	a, b   *seqActor // b only for "Note over A,B"
	lines  []string
}

// drawn frame (recorded during layout).
type seqFrame struct {
	kw, label string
	top, bot  int
	depth     int
	seps      []seqSep
}

type seqSep struct {
	y     int
	label string
}

// --- parser ---------------------------------------------------------------

var seqArrows = []struct {
	tok  string
	st   stroke
	head headKind
}{
	{"-->>", stDots, headFill},
	{"->>", stLight, headFill},
	{"--x", stDots, headCross},
	{"-x", stLight, headCross},
	{"--)", stDots, headOpen},
	{"-)", stLight, headOpen},
	{"-->", stDots, headOpen},
	{"->", stLight, headOpen},
}

var frameOpeners = []string{"alt", "loop", "opt", "par", "critical", "break"}

// frameState tracks an open frame while parsing.
type frameState struct {
	kw string
}

func (sc *seqChart) actorRef(id string) *seqActor {
	a, ok := sc.byID[id]
	if !ok {
		a = &seqActor{id: id, name: id}
		sc.byID[id] = a
		sc.actors = append(sc.actors, a)
	}
	return a
}

func (sc *seqChart) declareActor(id, name string) {
	a := sc.actorRef(id)
	if name != "" {
		a.name = name
	}
}

func parseSeq(lines []string) (*seqChart, error) {
	sc := &seqChart{byID: map[string]*seqActor{}}
	header := false
	var stack []frameState
	for _, raw := range lines {
		stmt := strings.TrimSpace(cutComment(raw))
		if stmt == "" {
			continue
		}
		if !header {
			if !strings.EqualFold(stmt, "sequenceDiagram") {
				return nil, errUnknown
			}
			header = true
			continue
		}
		if err := sc.parseStmt(stmt, &stack); err != nil {
			return nil, err
		}
	}
	if !header || len(sc.events) == 0 {
		return nil, errUnknown
	}
	if len(stack) != 0 {
		return nil, errUnsupported // unclosed frame
	}
	return sc, nil
}

func (sc *seqChart) parseStmt(stmt string, stack *[]frameState) error {
	tok, rest := splitTok(stmt)
	lowTok := strings.ToLower(tok)
	for _, kw := range frameOpeners {
		if lowTok == kw {
			*stack = append(*stack, frameState{kw: kw})
			sc.events = append(sc.events, seqEvent{kind: evFrameOpen, frame: kw, flbl: strings.TrimSpace(rest)})
			return nil
		}
	}
	switch lowTok {
	case "end":
		if len(*stack) == 0 {
			return errUnsupported
		}
		*stack = (*stack)[:len(*stack)-1]
		sc.events = append(sc.events, seqEvent{kind: evFrameClose})
		return nil
	case "else", "and":
		if len(*stack) == 0 {
			return errUnsupported
		}
		fr := (*stack)[len(*stack)-1]
		if (fr.kw == "alt") != (lowTok == "else") && (fr.kw == "par") != (lowTok == "and") {
			return errUnsupported
		}
		if fr.kw != "alt" && fr.kw != "par" {
			return errUnsupported // loop/opt/critical/break take no separator
		}
		sc.events = append(sc.events, seqEvent{kind: evFrameSep, label: strings.TrimSpace(rest)})
		return nil
	case "participant", "actor":
		id, name := parseParticipant(strings.TrimSpace(rest))
		if id == "" {
			return errUnsupported
		}
		sc.declareActor(id, name)
		return nil
	case "note":
		return sc.parseNote(strings.TrimSpace(rest))
	case "autonumber", "title", "activate", "deactivate":
		return nil // render-neutral in terminal art
	case "box", "link", "links", "properties", "details", "create", "destroy", "rect", "ref":
		return errUnsupported
	}
	return sc.parseMessage(stmt)
}

func parseParticipant(rest string) (string, string) {
	if i := strings.Index(strings.ToLower(rest), " as "); i >= 0 {
		return strings.TrimSpace(rest[:i]), strings.TrimSpace(rest[i+4:])
	}
	return strings.TrimSpace(rest), ""
}

func (sc *seqChart) parseNote(rest string) error {
	low := strings.ToLower(rest)
	var anchor noteAnchor
	var spec string
	switch {
	case strings.HasPrefix(low, "left of "):
		anchor, spec = noteLeft, rest[len("left of "):]
	case strings.HasPrefix(low, "right of "):
		anchor, spec = noteRight, rest[len("right of "):]
	case strings.HasPrefix(low, "over "):
		anchor, spec = noteOver, rest[len("over "):]
	default:
		return errUnsupported
	}
	colon := strings.Index(spec, ":")
	if colon < 0 {
		return errUnsupported
	}
	ids, text := spec[:colon], spec[colon+1:]
	note := &seqNote{anchor: anchor, lines: splitNoteText(text)}
	switch anchor {
	case noteLeft, noteRight:
		note.a = sc.actorRef(strings.TrimSpace(ids))
	case noteOver:
		parts := strings.Split(ids, ",")
		note.a = sc.actorRef(strings.TrimSpace(parts[0]))
		switch len(parts) {
		case 1:
		case 2:
			note.b = sc.actorRef(strings.TrimSpace(parts[1]))
		default:
			return errUnsupported
		}
	}
	sc.events = append(sc.events, seqEvent{kind: evNote, note: note})
	return nil
}

// splitNoteText expands <br> line breaks.
func splitNoteText(s string) []string {
	s = strings.ReplaceAll(s, "<br/>", "\n")
	s = strings.ReplaceAll(s, "<br>", "\n")
	out := strings.Split(s, "\n")
	for i := range out {
		out[i] = strings.TrimSpace(out[i])
	}
	return out
}

func (sc *seqChart) parseMessage(stmt string) error {
	best := -1
	bestTok := seqArrows[0]
	for _, t := range seqArrows {
		if idx := strings.Index(stmt, t.tok); idx >= 0 && (best < 0 || idx < best || (idx == best && len(t.tok) > len(bestTok.tok))) {
			best, bestTok = idx, t
		}
	}
	if best < 0 {
		return errUnsupported
	}
	fromID := strings.TrimSpace(stmt[:best])
	rest := stmt[best+len(bestTok.tok):]
	colon := strings.Index(rest, ":")
	if colon < 0 {
		return errUnsupported
	}
	toID := strings.TrimRight(strings.TrimSpace(rest[:colon]), "+-")
	label := strings.TrimSpace(rest[colon+1:])
	if fromID == "" || toID == "" {
		return errUnsupported
	}
	sc.events = append(sc.events, seqEvent{kind: evMsg, msg: &seqMsg{
		from: sc.actorRef(fromID), to: sc.actorRef(toID),
		label: label, st: bestTok.st, head: bestTok.head,
	}})
	return nil
}

// --- layout + render ------------------------------------------------------

func idxOf(sc *seqChart, a *seqActor) int {
	for i, x := range sc.actors {
		if x == a {
			return i
		}
	}
	return -1
}

func renderSeq(sc *seqChart) []string {
	n := len(sc.actors)
	if n == 0 {
		return nil
	}
	gaps := make([]int, max(n-1, 0))
	for i := range gaps {
		gaps[i] = 6
	}
	rightPad := 0
	widen := func(gapIdx, w int) {
		if gapIdx >= len(gaps) {
			rightPad = max(rightPad, w)
			return
		}
		gaps[gapIdx] = max(gaps[gapIdx], w)
	}
	for _, ev := range sc.events {
		if ev.kind != evMsg || ev.msg.label == "" {
			continue
		}
		m := ev.msg
		ia, ib := idxOf(sc, m.from), idxOf(sc, m.to)
		lo, hi := min(ia, ib), max(ia, ib)
		if lo == hi {
			widen(lo, textW(m.label)+6) // self-label sits in the right span
			continue
		}
		// slack is distributed evenly across the crossed gaps; add up-front
		per := (textW(m.label) + 2 + (hi - lo) - 1) / (hi - lo)
		for k := lo; k < hi; k++ {
			widen(k, per+4)
		}
	}
	x := 1
	for i, a := range sc.actors {
		a.boxW = textW(a.name) + 4
		a.boxL = x
		a.cx = x + a.boxW/2
		x += a.boxW
		if i < len(gaps) {
			x += gaps[i]
		}
	}
	x += rightPad

	// event rows
	y := 3
	for i := range sc.events {
		ev := &sc.events[i]
		ev.top = y
		switch ev.kind {
		case evMsg:
			ev.h = 2
			if ev.msg.from == ev.msg.to {
				ev.h = 3
			}
		case evNote:
			ev.h = 2 + len(ev.note.lines)
		case evFrameOpen, evFrameSep, evFrameClose:
			ev.h = 1
		}
		y += ev.h
	}

	c := newCanvas()
	drawSeq(c, sc, y-1, x-1)
	lines := c.lines()
	if len(lines) == 0 {
		return nil
	}
	return lines
}

// drawSeq paints frames and lifelines as strokes first; boxes, heads, and
// text are literals and land on top.
func drawSeq(c *canvas, sc *seqChart, lastRow, rightEdge int) {
	var stack []*seqFrame
	for i := range sc.events {
		ev := &sc.events[i]
		switch ev.kind {
		case evFrameOpen:
			stack = append(stack, &seqFrame{kw: ev.frame, label: ev.flbl, top: ev.top, depth: len(stack)})
		case evFrameSep:
			fr := stack[len(stack)-1]
			fr.seps = append(fr.seps, seqSep{y: ev.top, label: ev.label})
		case evFrameClose:
			fr := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			fr.bot = ev.top
			drawFrame(c, fr, rightEdge)
		case evMsg, evNote:
		}
	}
	for _, a := range sc.actors {
		c.vline(a.cx, 3, lastRow, stLight)
	}
	for i := range sc.events {
		switch ev := &sc.events[i]; ev.kind {
		case evMsg:
			drawMsg(c, ev)
		case evNote:
			drawNote(c, ev)
		case evFrameOpen, evFrameSep, evFrameClose:
		}
	}
	for _, a := range sc.actors {
		drawActorHead(c, a)
	}
}

func drawFrame(c *canvas, fr *seqFrame, rightEdge int) {
	d := fr.depth
	x1, x2 := d, rightEdge-d
	c.hline(x1+1, x2-1, fr.top, stLight)
	c.corner(x1, fr.top, pR, pD, stLight)
	c.corner(x2, fr.top, pL, pD, stLight)
	c.hline(x1+1, x2-1, fr.bot, stLight)
	c.corner(x1, fr.bot, pR, pU, stLight)
	c.corner(x2, fr.bot, pL, pU, stLight)
	c.vline(x1, fr.top+1, fr.bot-1, stLight)
	c.vline(x2, fr.top+1, fr.bot-1, stLight)
	label := fr.kw
	if fr.label != "" {
		label += " " + fr.label
	}
	c.text(x1+2, fr.top, " "+label+" ")
	for _, s := range fr.seps {
		c.hline(x1+1, x2-1, s.y, stDots)
		if s.label != "" {
			c.text(x1+2, s.y, " "+s.label+" ")
		}
	}
}

func drawActorHead(c *canvas, a *seqActor) {
	r := a.boxL + a.boxW - 1
	c.put(a.boxL, 0, '┌')
	c.put(r, 0, '┐')
	c.put(a.boxL, 1, '│')
	c.put(r, 1, '│')
	c.put(a.boxL, 2, '└')
	c.put(r, 2, '┘')
	for xx := a.boxL + 1; xx < r; xx++ {
		c.put(xx, 0, '─')
		c.put(xx, 1, ' ')
		c.put(xx, 2, '─')
	}
	c.text(a.boxL+2, 1, a.name)
}

func drawMsg(c *canvas, ev *seqEvent) {
	m := ev.msg
	y := ev.top
	if m.from == m.to {
		cx := m.from.cx
		if m.label != "" {
			c.text(cx+2, y, " "+m.label)
		}
		c.corner(cx+3, y+1, pL, pD, m.st)
		c.hline(cx+1, cx+2, y+1, m.st)
		c.corner(cx+3, y+2, pU, pL, m.st)
		c.hline(cx+1, cx+2, y+2, m.st)
		c.put(cx+1, y+2, leftHead(m.head))
		return
	}
	fx, tx := m.from.cx, m.to.cx
	lo, hi := min(fx, tx), max(fx, tx)
	if m.label != "" {
		lw := textW(m.label)
		c.text(max((lo+hi-lw)/2, lo+1), y, " "+m.label+" ")
	}
	c.hline(lo+1, hi-1, y+1, m.st)
	if tx > fx {
		c.put(hi-1, y+1, rightHead(m.head))
	} else {
		c.put(lo+1, y+1, leftHead(m.head))
	}
}

func rightHead(h headKind) rune {
	switch h {
	case headFill:
		return '▶'
	case headCross:
		return 'x'
	case headOpen:
		return '>'
	}
	return '>'
}

func leftHead(h headKind) rune {
	switch h {
	case headFill:
		return '◀'
	case headCross:
		return 'x'
	case headOpen:
		return '<'
	}
	return '<'
}

func drawNote(c *canvas, ev *seqEvent) {
	note := ev.note
	w := 0
	for _, l := range note.lines {
		w = max(w, textW(l))
	}
	w += 4 // borders + pad
	var x1 int
	switch note.anchor {
	case noteLeft:
		x1 = note.a.cx - 1 - w
	case noteRight:
		x1 = note.a.cx + 2
	case noteOver:
		lo, hi := note.a.cx, note.a.cx
		if note.b != nil {
			lo, hi = min(note.a.cx, note.b.cx), max(note.a.cx, note.b.cx)
			w = max(w, hi-lo+3)
		}
		x1 = (lo + hi - w) / 2
	}
	x1 = max(x1, 0)
	y, bot := ev.top, ev.top+ev.h-1
	c.put(x1, y, '┌')
	c.put(x1+w-1, y, '┐')
	c.put(x1, bot, '└')
	c.put(x1+w-1, bot, '┘')
	for xx := x1 + 1; xx < x1+w-1; xx++ {
		c.put(xx, y, '─')
		c.put(xx, bot, '─')
	}
	for r := y + 1; r < bot; r++ {
		c.put(x1, r, '│')
		c.put(x1+w-1, r, '│')
		for xx := x1 + 1; xx < x1+w-1; xx++ {
			c.put(xx, r, ' ')
		}
	}
	for i, l := range note.lines {
		c.text(x1+2, y+1+i, l)
	}
}
