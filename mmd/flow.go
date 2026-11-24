package mmd

// Flowchart renderer: parses the common flowchart subset of mermaid and
// lays it out top-down (TD/TB only) with longest-path ranks and barycenter
// ordering. Edges route through the channels between ranks as box-drawing
// strokes; labels sit on the routing rows beside or on the line.

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

type shape int

const (
	shSquare   shape = iota // [label]
	shRound                 // (label)
	shDouble                // [[label]]
	shCircle                // ((label))
	shStadium               // ([label])
	shCylinder              // [(label)]
	shRhombus               // {label}
	shHexagon               // {{label}}
)

type fNode struct {
	id    string
	lines []string // label rows (labels split on <br/>)
	shape shape

	init  int // insertion index (kept for deterministic tie-breaks)
	rank  int
	x, y  int     // box top-left
	w, h  int     // box size
	extra int     // reserved cells right of the box (self-loop space)
	grp   *fGroup // innermost subgraph containing this node
}

func (n *fNode) cx() int  { return n.x + n.w/2 }
func (n *fNode) my() int  { return n.y + n.h/2 }   // middle row
func (n *fNode) bot() int { return n.y + n.h - 1 } // bottom border row
func (n *fNode) rx() int  { return n.x + n.w }     // first cell right of box

type fEdge struct {
	from, to *fNode
	label    string
	st       stroke
	arrow    bool
}

// fGroup is a subgraph: a titled frame around its member nodes.
type fGroup struct {
	id, title string
	parent    *fGroup
	members   []*fNode
}

type flowChart struct {
	nodes map[string]*fNode
	order []*fNode
	edges []*fEdge
	dir   string // TD, LR or RL

	groups []*fGroup // declaration order
	gstack []*fGroup // open subgraphs during parsing

	// rank band columns (LR/RL): set by placeAcross
	bandX []int
	bandW []int
}

var errUnsupported = errors.New("unsupported construct")

func (fc *flowChart) node(id, label string, sh shape, hasShape bool) *fNode {
	n, ok := fc.nodes[id]
	if !ok {
		n = &fNode{id: id, lines: []string{id}, shape: shSquare, rank: -1, init: len(fc.order)}
		fc.nodes[id] = n
		fc.order = append(fc.order, n)
	}
	if hasShape {
		n.lines, n.shape = labelLines(label), sh
	}
	return n
}

// labelLines splits a label into rows on <br/> or <br>. Entities are
// already decoded by the parser.
func labelLines(s string) []string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "<br/>", "\n"), "<br>", "\n")
	out := strings.Split(s, "\n")
	for i := range out {
		out[i] = strings.Trim(out[i], " ")
	}
	return out
}

// --- parser ---------------------------------------------------------------

type fscan struct {
	s string
	i int
}

func (p *fscan) rest() string { return p.s[p.i:] }

func (p *fscan) skipWS() {
	for p.i < len(p.s) && (p.s[p.i] == ' ' || p.s[p.i] == '\t') {
		p.i++
	}
}

func (p *fscan) eof() bool { return p.i >= len(p.s) }

func (p *fscan) err() error { return fmt.Errorf("flowchart: unexpected %q", p.rest()) }

// nodeSpec parses an id with an optional shape payload.
func (p *fscan) nodeSpec(fc *flowChart) (*fNode, bool) {
	p.skipWS()
	start := p.i
	for p.i < len(p.s) && isIDByte(p.s[p.i]) {
		p.i++
	}
	id := p.s[start:p.i]
	if id == "" {
		return nil, false
	}
	label, sh, hasShape := "", shSquare, false
	if p.i < len(p.s) {
		switch p.s[p.i] {
		case '[', '(', '{':
			l, shape, n, ok := bracketLabel(p.rest())
			if !ok {
				return nil, false
			}
			label, sh, hasShape = l, shape, true
			p.i += n
		}
	}
	return fc.node(id, label, sh, hasShape), true
}

func isIDByte(b byte) bool {
	return b >= 0x80 || b == '_' || b == '.' ||
		unicode.IsLetter(rune(b)) || unicode.IsDigit(rune(b))
}

// shapes lists the open/close payload delimiters for every supported
// shape, longest openers first.
var shapes = []struct {
	opener, closer string
	sh             shape
}{
	{"([", "])", shStadium},
	{"[(", ")]", shCylinder},
	{"[[", "]]", shDouble},
	{"{{", "}}", shHexagon},
	{"((", "))", shCircle},
	{"[", "]", shSquare},
	{"(", ")", shRound},
	{"{", "}", shRhombus},
}

// bracketLabel reads a shape payload at s[0]. Quoted labels ("...") may
// contain any character; unquoted labels end at the closer. Returns the
// label, shape, and bytes consumed.
func bracketLabel(s string) (string, shape, int, bool) {
	for _, d := range shapes {
		if !strings.HasPrefix(s, d.opener) {
			continue
		}
		body := d.opener
		rest := s[len(d.opener):]
		if strings.HasPrefix(rest, "\"") {
			end := strings.IndexByte(rest[1:], '"')
			if end < 0 {
				return "", 0, 0, false
			}
			label := rest[1 : 1+end]
			tail := rest[1+end+1:]
			if !strings.HasPrefix(tail, d.closer) {
				return "", 0, 0, false
			}
			return decodeEntities(label), d.sh, len(body) + 1 + end + 1 + len(d.closer), true
		}
		end := strings.Index(rest, d.closer)
		if end < 0 || strings.ContainsAny(rest[:end], "[](){}") {
			return "", 0, 0, false
		}
		return decodeEntities(rest[:end]), d.sh, len(body) + end + len(d.closer), true
	}
	return "", 0, 0, false // [/x/], >x], ...: unsupported
}

// decodeEntities resolves mermaid's #num; and #quot; label escapes.
func decodeEntities(s string) string {
	if !strings.ContainsRune(s, '#') {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '#' && i+1 < len(s) {
			if j := strings.IndexByte(s[i:], ';'); j > 1 && j <= 8 {
				switch code := s[i+1 : i+j]; code {
				case "quot":
					b.WriteByte('"')
					i += j
					continue
				default:
					if n, err := strconv.Atoi(code); err == nil && n >= 0 && n <= unicode.MaxRune {
						b.WriteRune(rune(n))
						i += j
						continue
					}
				}
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// linkTokens are the complete link forms, longest dotted first.
var linkTokens = []struct {
	tok   string
	st    stroke
	arrow bool
}{
	{"-.->", stDots, true},
	{"-.-", stDots, false},
	{"-->", stLight, true},
	{"---", stLight, false},
	{"==>", stDouble, true},
	{"===", stDouble, false},
}

// linkSpec parses a link token plus an optional label.
func (p *fscan) linkSpec() (string, stroke, bool, error) {
	p.skipWS()
	// complete tokens first
	for _, t := range linkTokens {
		if strings.HasPrefix(p.rest(), t.tok) {
			p.i += len(t.tok)
			label := strings.TrimSpace(p.pipeLabel())
			return label, t.st, t.arrow, nil
		}
	}
	// bare-label forms: -- t -->, == t ==>, -. t .->
	for _, start := range []struct {
		opener  string
		closers []string
		st      stroke
	}{
		{"--", []string{"-->", "--", "---"}, stLight},
		{"==", []string{"==>", "==", "==="}, stDouble},
		{"-.", []string{".->", ".-"}, stDots},
	} {
		if !strings.HasPrefix(p.rest(), start.opener) {
			continue
		}
		after := p.rest()[len(start.opener):]
		bestIdx, bestTok := -1, ""
		for _, c := range start.closers {
			if idx := strings.Index(after, c); idx >= 0 && (bestIdx < 0 || idx < bestIdx || (idx == bestIdx && len(c) > len(bestTok))) {
				bestIdx, bestTok = idx, c
			}
		}
		if bestIdx < 0 || strings.TrimSpace(after[:bestIdx]) == "" {
			return "", 0, false, errUnsupported
		}
		label := strings.TrimSpace(after[:bestIdx])
		p.i += len(start.opener) + bestIdx + len(bestTok)
		return label, start.st, strings.HasSuffix(bestTok, ">"), nil
	}
	return "", 0, false, p.err()
}

// pipeLabel reads a trailing |label| after a link token, decoding entities
// and stripping surrounding quotes.
func (p *fscan) pipeLabel() string {
	p.skipWS()
	if p.eof() || p.s[p.i] != '|' {
		return ""
	}
	j := strings.IndexByte(p.s[p.i+1:], '|')
	if j < 0 {
		return ""
	}
	label := p.s[p.i+1 : p.i+1+j]
	p.i += j + 2
	label = strings.TrimSpace(decodeEntities(label))
	if len(label) >= 2 && label[0] == '"' && label[len(label)-1] == '"' {
		label = label[1 : len(label)-1]
	}
	return label
}

// parseFlow parses `graph`/`flowchart` statement lines.
func parseFlow(lines []string) (*flowChart, error) {
	fc := &flowChart{nodes: map[string]*fNode{}}
	header := false
	for _, raw := range lines {
		stmt := strings.TrimSpace(cutComment(raw))
		if stmt == "" {
			continue
		}
		if !header {
			header = true
			tok, rest := splitTok(strings.ToLower(stmt))
			if tok != "graph" && tok != "flowchart" {
				return nil, errUnknown
			}
			dir, _ := splitTok(rest)
			switch strings.ToUpper(dir) {
			case "TD", "TB", "":
				fc.dir = "TD"
			case "LR":
				fc.dir = "LR"
			case "RL":
				fc.dir = "RL"
			case "BT":
				return nil, errUnsupported
			default:
				return nil, errUnknown
			}
			continue
		}
		if err := parseFlowStmt(fc, stmt); err != nil {
			return nil, err
		}
	}
	if !header || len(fc.edges) == 0 && len(fc.order) == 0 {
		return nil, errUnknown
	}
	if len(fc.gstack) != 0 {
		return nil, errUnsupported // unclosed subgraph
	}
	return fc, nil
}

func splitTok(s string) (string, string) {
	s = strings.TrimSpace(s)
	i := strings.IndexAny(s, " \t")
	if i < 0 {
		return s, ""
	}
	return s[:i], strings.TrimSpace(s[i:])
}

func parseFlowStmt(fc *flowChart, stmt string) error {
	lower := strings.ToLower(stmt)
	switch {
	case strings.HasPrefix(lower, "subgraph "):
		id, title := parseSubgraph(stmt[len("subgraph "):])
		if id == "" {
			return errUnsupported
		}
		g := &fGroup{id: id, title: title}
		if len(fc.gstack) > 0 {
			g.parent = fc.gstack[len(fc.gstack)-1]
		}
		fc.groups = append(fc.groups, g)
		fc.gstack = append(fc.gstack, g)
		return nil
	case lower == "end":
		if len(fc.gstack) == 0 {
			return errUnsupported
		}
		fc.gstack = fc.gstack[:len(fc.gstack)-1]
		return nil
	case strings.HasPrefix(lower, "direction "):
		return errUnsupported
	case strings.HasPrefix(lower, "class "), strings.HasPrefix(lower, "classdef "),
		strings.HasPrefix(lower, "style "), strings.HasPrefix(lower, "linkstyle "),
		strings.HasPrefix(lower, "click "):
		return nil // styling and links don't apply to terminal art
	}
	p := &fscan{s: stmt}
	from, ok := p.nodeSpec(fc)
	if !ok {
		return errUnsupported
	}
	if err := fc.attach(from); err != nil {
		return err
	}
	for {
		p.skipWS()
		label, st, arrow, err := p.linkSpec()
		if err != nil {
			if p.eof() {
				return nil // standalone node declaration
			}
			return err
		}
		to, ok := p.nodeSpec(fc)
		if !ok {
			return errUnsupported
		}
		if err := fc.attach(to); err != nil {
			return err
		}
		fc.edges = append(fc.edges, &fEdge{from: from, to: to, label: label, st: st, arrow: arrow})
		from = to
	}
}

// attach records node membership in the innermost open subgraph. Referring
// to a subgraph itself as an edge endpoint or node is unsupported.
func (fc *flowChart) attach(n *fNode) error {
	for _, g := range fc.groups {
		if g.id == n.id {
			return errUnsupported
		}
	}
	if n.grp == nil && len(fc.gstack) > 0 {
		n.grp = fc.gstack[len(fc.gstack)-1]
		n.grp.members = append(n.grp.members, n)
	}
	return nil
}

// parseSubgraph reads `ID["Title"]` or bare `ID`. Brackets required for
// titles with spaces.
func parseSubgraph(rest string) (string, string) {
	rest = strings.TrimSpace(rest)
	if i := strings.IndexByte(rest, '['); i > 0 {
		id := strings.TrimSpace(rest[:i])
		label, _, _, ok := bracketLabel(rest[i:])
		if !ok || id == "" {
			return "", ""
		}
		return id, strings.TrimSpace(label)
	}
	if strings.ContainsAny(rest, " \t") {
		return "", "" // unbracketed multi-word titles unsupported
	}
	return rest, rest
}

// --- layout ---------------------------------------------------------------

// rankAssign computes longest-path ranks. Cycles are broken by a DFS in
// insertion order: an edge reaching a node on the DFS stack is a back edge
// and is excluded from ranking (it renders upwards later), keeping entry
// nodes at the top.
func (fc *flowChart) rankAssign() {
	succs := map[*fNode][]*fEdge{}
	for _, e := range fc.edges {
		if e.from != e.to {
			succs[e.from] = append(succs[e.from], e)
		}
	}
	const (
		white = iota
		grey
		black
	)
	color := map[*fNode]int{}
	back := map[*fEdge]bool{}
	var dfs func(u *fNode)
	dfs = func(u *fNode) {
		color[u] = grey
		for _, e := range succs[u] {
			switch color[e.to] {
			case grey:
				back[e] = true
			case white:
				dfs(e.to)
			}
		}
		color[u] = black
	}
	for _, n := range fc.order {
		if color[n] == white {
			dfs(n)
		}
	}

	preds := map[*fNode][]*fNode{}
	for _, e := range fc.edges {
		if e.from != e.to && !back[e] {
			preds[e.to] = append(preds[e.to], e.from)
		}
	}
	memo := map[*fNode]int{}
	var walk func(n *fNode) int
	walk = func(n *fNode) int {
		if r, ok := memo[n]; ok {
			return r
		}
		r := 0
		for _, pr := range preds[n] {
			r = max(r, walk(pr)+1)
		}
		memo[n] = r
		return r
	}
	for _, n := range fc.order {
		n.rank = walk(n)
	}
}

// ranks groups nodes by rank, insertion order.
func (fc *flowChart) ranks() [][]*fNode {
	maxR := 0
	for _, n := range fc.order {
		maxR = max(maxR, n.rank)
	}
	out := make([][]*fNode, maxR+1)
	for _, n := range fc.order {
		out[n.rank] = append(out[n.rank], n)
	}
	return out
}

// barycenter reorders nodes inside ranks to reduce edge crossings: eight
// alternating sweeps, neighbours' average position as sort key.
func barycenter(rk [][]*fNode, edges []*fEdge) {
	posOf := func() map[*fNode]int {
		m := map[*fNode]int{}
		for _, row := range rk {
			for i, n := range row {
				m[n] = i
			}
		}
		return m
	}
	for sweep := range 8 {
		down := sweep%2 == 0
		from, to, step := 0, len(rk), 1
		if !down {
			from, to, step = len(rk)-1, -1, -1
		}
		for i := from; i != to; i += step {
			pos := posOf()
			type keyed struct {
				n   *fNode
				key float64
			}
			ks := make([]keyed, 0, len(rk[i]))
			for idx, n := range rk[i] {
				k := keyed{n: n, key: float64(idx)}
				sum, cnt := 0.0, 0
				for _, e := range edges {
					var other *fNode
					switch {
					case down && e.to == n:
						other = e.from
					case !down && e.from == n:
						other = e.to
					}
					if other != nil && other != n {
						sum += float64(pos[other])
						cnt++
					}
				}
				if cnt > 0 {
					k.key = sum / float64(cnt)
				}
				ks = append(ks, k)
			}
			stableSort(ks, func(a, b keyed) bool { return a.key < b.key })
			for j, k := range ks {
				rk[i][j] = k.n
			}
		}
	}
}

// stableSort is insertion sort (small rows; keeps ties in place).
func stableSort[T any](s []T, less func(a, b T) bool) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && less(s[j], s[j-1]); j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// --- placement + channel routing -----------------------------------------

const (
	nodePadX   = 1 // spaces between border and label
	rankGapX   = 3 // horizontal gap between sibling boxes (TD)
	rankGapY   = 2 // vertical gap between sibling boxes (LR/RL)
	loopMargin = 2 // cells a self-loop extends past the box
)

// place assigns node geometry: ranks flow downward (TD) or rightward
// (LR/RL), reserving the channels between them.
func (fc *flowChart) place(rk [][]*fNode) {
	fc.sizeNodes()
	if fc.dir == "LR" || fc.dir == "RL" {
		fc.placeAcross(rk)
		return
	}
	fc.placeDown(rk)
}

// sizeNodes computes box size from label lines, reserving self-loop space.
func (fc *flowChart) sizeNodes() {
	loops := map[*fNode]int{}
	for _, e := range fc.edges {
		if e.from == e.to {
			w := loopMargin
			if e.label != "" {
				w += 1 + textW(e.label)
			}
			loops[e.from] = max(loops[e.from], w)
		}
	}
	for _, n := range fc.order {
		lw := 1
		for _, l := range n.lines {
			lw = max(lw, textW(l))
		}
		n.w = lw + 2*nodePadX + 2
		n.h = len(n.lines) + 2
		n.extra = loops[n]
	}
}

// placeDown packs nodes per-rank horizontally; ranks stack down the canvas.
// Top-level groups form disjoint columns: the first pass packs rows and
// records each group's natural column start; the second anchors every
// occurrence of a group to the widest such start, so frames never
// interleave horizontally with sibling frames.
func (fc *flowChart) placeDown(rk [][]*fNode) {
	start := map[*fGroup]int{}
	for _, row := range rk {
		x := 1
		var prevG *fGroup
		for _, n := range row {
			g := n.grp
			if prevG != nil && g != prevG {
				x += 2 * rankGapX // sibling frames need clear columns
			}
			if g != nil && g != prevG {
				// natural start of this group's column in this row
				if s, ok := start[g]; !ok || x > s {
					start[g] = x
				}
			}
			prevG = g
			n.x = x
			x += n.w + n.extra + rankGapX
		}
	}
	for _, row := range rk {
		x := 1
		var prevG *fGroup
		for _, n := range row {
			g := n.grp
			if prevG != nil && g != prevG {
				x += 2 * rankGapX
			}
			if g != nil && g != prevG {
				x = max(x, start[g]) // anchor to the group's widest start
			}
			prevG = g
			n.x = x
			x += n.w + n.extra + rankGapX
		}
	}
	y := 0
	for g, row := range rk {
		for _, n := range row {
			n.y = y
		}
		y += rankHeight(row)
		if g < len(rk)-1 {
			y += fc.channelHeight(g)
		}
	}
}

// channelHeight reserves the rows between rank g and g+1: stub + arrow
// rows, plus rows for same-rank dips and bent elbows when needed.
func (fc *flowChart) channelHeight(g int) int {
	h := 2
	if fc.hasSameRankEdge(g) {
		h++
	}
	if fc.hasBentCrossing(g) {
		h++
	}
	return h
}

// placeAcross stacks nodes per-rank vertically; ranks flow right (LR).
// RL reverses the band order at the end so rank 0 lands rightmost.
func (fc *flowChart) placeAcross(rk [][]*fNode) {
	for _, row := range rk {
		y := 1
		var prevG *fGroup
		for _, n := range row {
			g := n.grp
			if prevG != nil && g != prevG {
				y += 2 * rankGapY // sibling frames need clear rows
			}
			prevG = g
			n.y = y
			y += n.h + rankGapY
		}
	}
	x := 1
	fc.bandX = make([]int, len(rk))
	fc.bandW = make([]int, len(rk))
	for g, row := range rk {
		w := 1
		for _, n := range row {
			w = max(w, n.w+n.extra)
			n.x = x
		}
		fc.bandX[g], fc.bandW[g] = x, w
		x += w
		if g < len(rk)-1 {
			x += fc.channelWidth(g)
		}
	}
	if fc.dir == "RL" {
		// mirror the whole layout horizontally: flip every node x about
		// the total width (nodes keep their sizes)
		total := x - 1
		for _, n := range fc.order {
			n.x = total - n.x - n.w + 2
		}
	}
}

// channelWidth reserves the columns between rank band g and g+1: stub +
// arrow columns, a dip column when same-rank/back edges need it, a bend
// column for vertical elbows, and room for edge labels.
func (fc *flowChart) channelWidth(g int) int {
	w := 2
	for _, e := range fc.edges {
		if e.from == e.to {
			continue
		}
		if e.from.rank == g && e.to.rank <= g {
			w++ // same-rank / back edge dip column
			break
		}
	}
	lw := 0
	for _, e := range fc.edges {
		if e.from == e.to {
			continue
		}
		if e.from.rank <= g && e.to.rank > g {
			if e.from.my() != e.to.my() {
				lw = max(lw, 3) // bend column
			}
			if e.label != "" {
				lw = max(lw, textW(e.label)+2)
			}
		}
	}
	return w + lw
}

// gapCols resolves channel column numbers between rank bands g and g+1,
// mirroring gapRows: stub column, optional dip column (same-rank/back
// edges), optional bend column (elbows). -1 marks "not reserved". Order
// follows direction: left-to-right for LR, mirrored for RL.
func gapCols(fc *flowChart, g int) (stub, dip, bend int) {
	dip, bend = -1, -1
	lo := fc.bandX[g] + fc.bandW[g] // first col after band g
	// sub-column positions along flow direction
	pos := make([]int, 0, 3)
	pos = append(pos, lo) // stub
	needDip := false
	for _, e := range fc.edges {
		if e.from != e.to && e.from.rank == g && e.to.rank <= g {
			needDip = true
			break
		}
	}
	needBend := false
	for _, e := range fc.edges {
		if e.from != e.to && e.from.rank <= g && e.to.rank > g && e.from.my() != e.to.my() {
			needBend = true
			break
		}
	}
	if needDip {
		pos = append(pos, lo+1)
	}
	if needBend {
		pos = append(pos, lo+len(pos))
	}
	stub = pos[0]
	if needDip && needBend {
		dip, bend = pos[1], pos[2]
	} else if needDip {
		dip = pos[1]
	} else if needBend {
		bend = pos[1]
	}
	if fc.dir == "RL" {
		// mirror col positions about the gap center
		total := fc.bandX[g] + fc.bandW[g] + fc.channelWidth(g) - 1
		mirror := func(c int) int {
			if c < 0 {
				return c
			}
			return total - (c - lo)
		}
		stub, dip, bend = mirror(stub), mirror(dip), mirror(bend)
	}
	return stub, dip, bend
}

// stackGroupOrphans: members of a subgraph that no edge touches would all
// land in one rank band side by side, making the frame absurdly wide. With
// no edges ordering them, stack the members into sequential ranks instead
// (a column grid for TD, a row for LR).
func (fc *flowChart) stackGroupOrphans() {
	touched := map[*fGroup]bool{}
	for _, e := range fc.edges {
		touched[e.from.grp] = true
		touched[e.to.grp] = true
	}
	for _, g := range fc.groups {
		if touched[g] || len(g.members) < 2 {
			continue
		}
		base := g.members[0].rank
		for i, n := range g.members {
			n.rank = base + i
		}
	}
}

// rankHeight is the tallest node in the rank (ranks share one y band).
func rankHeight(row []*fNode) int {
	h := 3
	for _, n := range row {
		h = max(h, n.h)
	}
	return h
}

func (fc *flowChart) hasSameRankEdge(g int) bool {
	for _, e := range fc.edges {
		if e.from != e.to && e.from.rank == g && e.to.rank == g {
			return true
		}
	}
	return false
}

func (fc *flowChart) hasBentCrossing(g int) bool {
	for _, e := range fc.edges {
		if e.from != e.to && e.from.rank <= g && e.to.rank > g && e.from.cx() != e.to.cx() {
			return true
		}
	}
	return false
}

// gapRows resolves channel row numbers for the gap below rank g.
// Layout: stub row, optional same-rank elbow row, optional bend row, arrow
// row (directly above the next rank's boxes).
func gapRows(fc *flowChart, rk [][]*fNode, g int) (stub, same, bend int) {
	same, bend = -1, -1
	// channel starts below the rank's tallest box
	base := 0
	for _, n := range rk[g] {
		base = max(base, n.bot())
	}
	row := base + 1
	stub = row
	row++
	if fc.hasSameRankEdge(g) {
		same = row
		row++
	}
	if fc.hasBentCrossing(g) {
		bend = row
	}
	return stub, same, bend
}

// --- rendering ------------------------------------------------------------

type arrowStamp struct {
	x, y int
	r    rune
}

func renderFlow(fc *flowChart) []string {
	fc.rankAssign()
	fc.stackGroupOrphans()
	rk := fc.ranks()
	barycenter(rk, fc.edges)
	fc.place(rk)
	var bboxes map[*fGroup]*groupRect
	if len(fc.groups) > 0 {
		bboxes = fc.groupBBoxes()
		fc.groupShift(bboxes)
	}
	c := newCanvas()
	heads := make([]arrowStamp, 0, len(fc.edges))
	for _, e := range fc.edges {
		heads = append(heads, drawFlowEdge(c, fc, rk, e)...)
	}
	drawGroups(c, fc, bboxes)
	for _, n := range fc.order {
		drawShape(c, n)
	}
	// arrowheads land inside box borders: stamp after shapes so the box
	// can't erase them
	for _, h := range heads {
		c.put(h.x, h.y, h.r)
	}
	return c.lines()
}

func drawFlowEdge(c *canvas, fc *flowChart, rk [][]*fNode, e *fEdge) []arrowStamp {
	if fc.dir == "LR" || fc.dir == "RL" {
		return drawEdgeAcross(c, fc, e)
	}
	return drawEdgeDown(c, fc, rk, e)
}

func drawEdgeDown(c *canvas, fc *flowChart, rk [][]*fNode, e *fEdge) []arrowStamp {
	from, to := e.from, e.to
	st := e.st
	var heads []arrowStamp
	head := func(x, y int, up bool) {
		if e.arrow {
			r := '▼'
			if up {
				r = '▲'
			}
			heads = append(heads, arrowStamp{x, y, r})
		}
	}

	switch {
	case from == to:
		// self-loop: right side exit, back up into the bottom border
		n := from
		lx := n.rx()      // corner column; rows my..bot+1
		y2 := n.bot() + 1 // loop return row
		c.corner(lx, n.my(), pL, pD, st)
		c.vline(lx, n.my()+1, y2, st)
		c.corner(lx, y2, pU, pL, st)
		c.hline(n.cx()+1, lx-1, y2, st)
		c.corner(n.cx(), y2, pU, pR, st)
		head(n.cx(), n.bot(), true)
		if e.label != "" {
			c.text(lx+1, n.my(), " "+e.label)
		}

	case to.rank < from.rank:
		// back edge: down into the channel, rise into the target's bottom
		stub, _, _ := gapRows(fc, rk, from.rank)
		fx, tx := from.cx(), to.cx()
		c.vline(tx, to.bot()+1, stub-1, st) // rising stem (if any rows)
		elbow(c, fx, tx, stub, st, pU, pU)
		head(tx, to.bot(), true)
		if e.label != "" {
			c.text(min(fx, tx)+1, stub, " "+e.label+" ")
		}

	case to.rank == from.rank:
		// same-rank sibling: dip and come back up
		_, same, _ := gapRows(fc, rk, from.rank)
		fx, tx := from.cx(), to.cx()
		top := from.bot() + 1
		c.vline(fx, top, same-1, st)
		c.vline(tx, top, same-1, st)
		elbow(c, fx, tx, same, st, pU, pU) // both stems rise: corners point up
		head(tx, to.bot(), true)
		if e.label != "" {
			lo := min(fx, tx)
			c.text((lo+max(fx, tx)-textW(e.label))/2, same, " "+e.label+" ")
		}

	default:
		// downward edge, possibly spanning several ranks
		stub, _, bend := gapRows(fc, rk, from.rank)
		fx, tx := from.cx(), to.cx()
		if fx == tx {
			c.vline(fx, stub, to.y-2, st)
			head(fx, to.y-1, false)
			if e.label != "" {
				c.text(fx+1, stub, " "+e.label+" ")
			}
			return heads
		}
		if bend < 0 {
			bend = stub + 1 // defensive; place() reserved this row
		}
		c.vline(fx, stub, bend-1, st)
		elbow(c, fx, tx, bend, st, pU, pD)
		c.vline(tx, bend+1, to.y-2, st)
		head(tx, to.y-1, false)
		if e.label != "" {
			lo, hi := min(fx, tx), max(fx, tx)
			lw := textW(e.label) + 2 // with the spaces around it
			if span := hi - lo; span >= lw+2 {
				// centered, clear of both corner cells
				x := lo + 1 + (span-lw)/2
				x = min(max(x, lo+2), hi-1-lw)
				c.text(x, bend, " "+e.label+" ")
			} else if fx < tx {
				c.text(fx+1, stub, " "+e.label+" ")
			} else {
				c.text(fx-1-lw, stub, " "+e.label+" ")
			}
		}
	}
	return heads
}

// drawEdgeAcross renders one edge in LR/RL layout: ranks are columns, the
// flow travels right (LR) or left (RL), and edges exit the flow-side border
// of the source's mid row and enter the target's entry border.
func drawEdgeAcross(c *canvas, fc *flowChart, e *fEdge) []arrowStamp {
	from, to := e.from, e.to
	st := e.st
	var heads []arrowStamp
	head := func(x, y int, r rune) {
		if e.arrow {
			heads = append(heads, arrowStamp{x, y, r})
		}
	}
	lr := fc.dir == "LR"

	// outCell: first cell outside the flow-side border
	outX := from.x + from.w
	if !lr {
		outX = from.x - 1
	}
	// inX/inR: entry cell (adjacent to target border) and head rune
	inX, inR := to.x-1, '▶'
	if !lr {
		inX, inR = to.x+to.w, '◀'
	}
	fy, ty := from.my(), to.my()

	switch {
	case from == to:
		// self-loop: right side exit, back up into the bottom border
		n := from
		lx := n.rx()
		y2 := n.bot() + 1
		c.corner(lx, n.my(), pL, pD, st)
		c.vline(lx, n.my()+1, y2, st)
		c.corner(lx, y2, pU, pL, st)
		c.hline(n.cx()+1, lx-1, y2, st)
		c.corner(n.cx(), y2, pU, pR, st)
		head(n.cx(), n.bot(), '▲')
		if e.label != "" {
			c.text(lx+1, n.my(), " "+e.label)
		}

	case to.rank <= from.rank:
		if to.rank < from.rank {
			// back edge: detour below the whole diagram, entering the
			// target's bottom border
			detour := fc.bottomRow() + 2
			side := outX
			c.hline(side, side+flowDir(fc), fy, st)        // exit stub
			c.vline(side+flowDir(fc), fy, detour, st)      // down
			c.hline(side+flowDir(fc), to.cx(), detour, st) // travel back
			c.vline(to.cx(), to.bot()+1, detour-1, st)     // rise
			head(to.cx(), to.bot(), '▲')
			if e.label != "" {
				c.text(min(side, to.cx())+2, detour, " "+e.label+" ")
			}
			return heads
		}
		// same-rank edge: exit side, dip into the channel of from.rank,
		// return into the target's flow-side border
		_, dip, _ := gapCols(fc, from.rank)
		if dip < 0 {
			dip = gapEnd(fc, from.rank) + 1
		}
		headX, headR := to.x+to.w-1, '◀'
		if !lr {
			headX, headR = to.x, '▶'
		}
		c.hline(outX, dip, fy, st)
		c.vline(dip, fy, ty, st)
		c.hline(dip, outX2(fc, to), ty, st)
		head(headX, ty, headR)
		if e.label != "" {
			x := outX + 1
			if !lr {
				x = outX - 1 - textW(e.label) - 2
			}
			c.text(x, fy, " "+e.label+" ")
		}

	default:
		// forward edge across rank bands
		stub, _, bend := gapCols(fc, from.rank)
		if fy == ty {
			c.hline(outX, inX, fy, st)
			head(inX, fy, inR)
		} else {
			if bend < 0 {
				bend = stub + 1 // defensive; channelWidth reserved it
			}
			c.hline(outX, bend, fy, st)
			c.vline(bend, fy, ty, st)
			c.hline(bend, inX, ty, st)
			head(inX, ty, inR)
		}
		if e.label != "" {
			x := outX + 1
			if !lr {
				x = outX - 1 - textW(e.label) - 2
			}
			c.text(x, fy, " "+e.label+" ")
		}
	}
	return heads
}

// groupRect is one frame's border rectangle.
type groupRect struct{ x0, y0, x1, y1 int }

// groupBBoxes resolves every subgraph's frame around members and nested
// frames, including the pad row and the title-carrying border row. The
// topmost frame may need rows above the first rank; groupShift applies
// that global offset before anything draws.
func (fc *flowChart) groupBBoxes() map[*fGroup]*groupRect {
	kids := map[*fGroup][]*fGroup{}
	for _, g := range fc.groups {
		if g.parent != nil {
			kids[g.parent] = append(kids[g.parent], g)
		}
	}
	bboxes := map[*fGroup]*groupRect{}
	var bbox func(g *fGroup) *groupRect
	bbox = func(g *fGroup) *groupRect {
		if r, ok := bboxes[g]; ok {
			return r
		}
		r := &groupRect{1 << 30, 1 << 30, -1, -1}
		have := false
		for _, n := range g.members {
			have = true
			r.x0 = min(r.x0, n.x-1)
			r.y0 = min(r.y0, n.y-1)
			r.x1 = max(r.x1, n.x+n.w+n.extra)
			r.y1 = max(r.y1, n.y+n.h)
		}
		for _, k := range kids[g] {
			if kr := bbox(k); kr != nil {
				have = true
				r.x0 = min(r.x0, kr.x0-1)
				r.y0 = min(r.y0, kr.y0-1)
				r.x1 = max(r.x1, kr.x1+1)
				r.y1 = max(r.y1, kr.y1+1)
			}
		}
		if !have {
			return nil
		}
		r.y0 -= 2 // one pad row plus the title-carrying border row
		bboxes[g] = r
		return r
	}
	for _, g := range fc.groups {
		bbox(g)
	}
	return bboxes
}

// groupShift lowers/widens every node (and frame) so the outermost frames
// fit within the canvas.
func (fc *flowChart) groupShift(bboxes map[*fGroup]*groupRect) {
	shiftY, shiftX := 0, 0
	for _, r := range bboxes {
		shiftY = max(shiftY, -r.y0)
		shiftX = max(shiftX, -r.x0)
	}
	if shiftY <= 0 && shiftX <= 0 {
		return
	}
	for _, n := range fc.order {
		n.y += shiftY
		n.x += shiftX
	}
	for _, r := range bboxes {
		r.y0 += shiftY
		r.x0 += shiftX
		r.y1 += shiftY
		r.x1 += shiftX
	}
}

// clipRunes truncates s to at most w display columns.
func clipRunes(s string, w int) string {
	if textW(s) <= w {
		return s
	}
	out := ""
	cum := 0
	for _, r := range s {
		rw := textW(string(r))
		if cum+rw > w {
			break
		}
		out += string(r)
		cum += rw
	}
	return out
}

// drawGroups frames each subgraph around its members. Borders are strokes
// (not literals), so crossing edge lines merge into the frame instead of
// being cut; titles are written over the top border.
func drawGroups(c *canvas, fc *flowChart, bboxes map[*fGroup]*groupRect) {
	order := append([]*fGroup(nil), fc.groups...)
	stableSort(order, func(a, b *fGroup) bool { return depthOfGroup(a) > depthOfGroup(b) })
	for _, g := range order {
		r := bboxes[g]
		if r == nil {
			continue
		}
		c.hline(r.x0+1, r.x1-1, r.y0, stLight)
		c.hline(r.x0+1, r.x1-1, r.y1, stLight)
		c.corner(r.x0, r.y0, pR, pD, stLight)
		c.corner(r.x1, r.y0, pL, pD, stLight)
		c.corner(r.x0, r.y1, pR, pU, stLight)
		c.corner(r.x1, r.y1, pL, pU, stLight)
		c.vline(r.x0, r.y0+1, r.y1-1, stLight)
		c.vline(r.x1, r.y0+1, r.y1-1, stLight)
		if g.title != "" {
			// titles live on the top border, clipped to the frame's width
			c.text(r.x0+2, r.y0, " "+clipRunes(g.title, r.x1-r.x0-4)+" ")
		}
	}
}

func depthOfGroup(g *fGroup) int {
	d := 0
	for p := g.parent; p != nil; p = p.parent {
		d++
	}
	return d
}

// gapEnd returns the last column of the channel following rank band g.
func gapEnd(fc *flowChart, g int) int {
	return fc.bandX[g] + fc.bandW[g] + fc.channelWidth(g) - 1
}

// flowDir is +1 for LR, -1 for RL.
func flowDir(fc *flowChart) int {
	if fc.dir == "LR" {
		return 1
	}
	return -1
}

// bottomRow is the lowest box row in the chart (for back-edge detours).
func (fc *flowChart) bottomRow() int {
	m := 0
	for _, n := range fc.order {
		m = max(m, n.bot())
	}
	return m
}

// outX2 returns the first cell outside n's flow-side border.
func outX2(fc *flowChart, n *fNode) int {
	if fc.dir == "LR" {
		return n.x + n.w
	}
	return n.x - 1
}

// elbow draws a horizontal connection between columns fx and tx at row y,
// joining vertical stems: the source stem approaches from srcDir (pU/pD),
// the target stem from dstDir. The stems themselves are drawn separately.
func elbow(c *canvas, fx, tx, y int, st stroke, srcDir, dstDir int) {
	c.hline(fx, tx, y, st)
	if srcDir >= 0 {
		c.port(fx, y, srcDir, st)
	}
	if dstDir >= 0 {
		c.port(tx, y, dstDir, st)
	}
}

// corner paints exactly the given two ports at one cell (a stroke turn),
// replacing degenerate two-port elbow output from line endpoints.
func (c *canvas) corner(x, y, p1, p2 int, st stroke) {
	c.port(x, y, p1, st)
	c.port(x, y, p2, st)
}

// --- node shapes ----------------------------------------------------------

// frameStyle is one shape's full border: corners, top/bottom caps, and
// side borders (side = 0 means no side border, rhombus style).
type frameStyle struct {
	tl, tr, bl, br rune
	capTop, capBot rune
	sideL, sideR   rune
}

func styleOf(sh shape) frameStyle {
	switch sh {
	case shRhombus:
		return frameStyle{'╱', '╲', '╲', '╱', '‾', '_', 0, 0}
	case shHexagon:
		return frameStyle{'╱', '╲', '╲', '╱', '‾', '_', '⟨', '⟩'}
	case shCircle:
		return frameStyle{'╭', '╮', '╰', '╯', '‾', '_', '│', '│'}
	case shCylinder:
		return frameStyle{'┌', '┐', '└', '┘', '‾', '─', '║', '║'}
	case shRound, shStadium:
		return frameStyle{'╭', '╮', '╰', '╯', '─', '─', '│', '│'}
	case shDouble:
		return frameStyle{'╔', '╗', '╚', '╝', '═', '═', '║', '║'}
	case shSquare:
		return frameStyle{'┌', '┐', '└', '┘', '─', '─', '│', '│'}
	}
	return frameStyle{'┌', '┐', '└', '┘', '─', '─', '│', '│'}
}

func drawShape(c *canvas, n *fNode) {
	x, y, w, h := n.x, n.y, n.w, n.h
	inner := w - 2
	s := styleOf(n.shape)
	c.put(x, y, s.tl)
	c.put(x+w-1, y, s.tr)
	c.put(x, y+h-1, s.bl)
	c.put(x+w-1, y+h-1, s.br)
	c.putStr(x+1, y, strings.Repeat(string(s.capTop), inner))
	c.putStr(x+1, y+h-1, strings.Repeat(string(s.capBot), inner))
	firstLabel := y + 1 + (h-2-len(n.lines))/2
	for dy := 1; dy < h-1; dy++ {
		for xx := x + 1; xx < x+w-1; xx++ {
			c.put(xx, y+dy, ' ')
		}
		if s.sideL != 0 {
			c.put(x, y+dy, s.sideL)
			c.put(x+w-1, y+dy, s.sideR)
		}
	}
	for i, l := range n.lines {
		c.text(x+1+(inner-textW(l))/2, firstLabel+i, l)
	}
}
