// Package mmd rewrites mermaid diagrams in markdown source before it is
// handed to glamour:
//
//	```mermaid
//	graph TD
//	    A --> B
//	```
//
// becomes a plain fenced code block holding rendered unicode box-drawing
// art. Diagrams that fail to parse or render — or that use unsupported
// diagram types and constructs — keep their original source, untouched.
//
// Supported: flowcharts (`graph`/`flowchart`, TD/TB/LR/RL directions) with common
// node shapes and labelled/chained edges, and sequence diagrams with
// participants, notes, self messages, and alt/loop/opt/par-style frames.
package mmd

import (
	"strings"
)

// Rewrite returns markdown with renderable mermaid blocks replaced by
// unicode art. Unrenderable blocks are returned verbatim.
func Rewrite(s string) string {
	return (&rewriter{}).run(s)
}

type rewriter struct {
	// fenced code block state
	fenceCh  byte // '`' or '~'; 0 = not in a fence
	fenceLen int
	mermaid  bool // current fence is a mermaid block
	prefix   string
	body     []string // collected mermaid source lines (prefix stripped, for the parser)
	rawBody  []string // the same lines verbatim (for keep-source fallback)
	open     string   // the opening fence line, verbatim
}

func (r *rewriter) run(s string) string {
	lines := strings.Split(s, "\n")
	var out strings.Builder
	i := 0
	flush := func(txt string, last bool) {
		out.WriteString(txt)
		if !last {
			out.WriteByte('\n')
		}
	}
	for i < len(lines) {
		line := lines[i]
		last := i == len(lines)-1

		if r.fenceCh != 0 {
			r.handleFenceLine(line, last, flush)
			i++
			continue
		}

		prefix, body := splitBlockPrefix(line)
		trim := strings.TrimLeft(body, " ")
		// Indented code blocks hold no fences.
		if len(body)-len(trim) > 3 {
			flush(line, last)
			i++
			continue
		}
		if ch, n, info, ok := fenceOpenInfo(trim); ok {
			r.fenceCh, r.fenceLen = ch, n
			r.prefix = prefix
			r.open = line
			r.mermaid = strings.EqualFold(strings.TrimSpace(strings.SplitN(info, " ", 2)[0]), "mermaid")
			if !r.mermaid {
				flush(line, last)
			}
			i++
			continue
		}
		flush(line, last)
		i++
	}
	// Unterminated mermaid fence: re-emit the buffered source verbatim.
	if r.mermaid {
		flush(r.open, len(r.rawBody) == 0)
		for i, l := range r.rawBody {
			flush(l, i == len(r.rawBody)-1)
		}
	}
	r.resetFence()
	return out.String()
}

// handleFenceLine processes a line while inside a fenced block: closes the
// fence (rendering mermaid art or re-emitting the source block on failure)
// or collects mermaid body lines.
func (r *rewriter) handleFenceLine(line string, last bool, flush func(string, bool)) {
	content := r.stripPrefix(line)
	if isFenceClose(content, r.fenceCh, r.fenceLen) {
		defer r.resetFence()
		if !r.mermaid {
			flush(line, last)
			return
		}
		indent := r.open[:strings.IndexByte(r.open, r.fenceCh)]
		if art := renderBlock(r.body, indent); art != "" {
			flush(art, last)
			return
		}
		// keep-source: open line, body, and this closing line verbatim
		flush(r.open, false)
		for _, l := range r.rawBody {
			flush(l, false)
		}
		flush(line, last)
		return
	}
	if r.mermaid {
		r.body = append(r.body, content)
		r.rawBody = append(r.rawBody, line)
		return
	}
	flush(line, last)
}

func (r *rewriter) resetFence() {
	r.fenceCh, r.fenceLen, r.mermaid = 0, 0, false
	r.prefix, r.open, r.body, r.rawBody = "", "", nil, nil
}

// stripPrefix removes the block's blockquote prefix from a collected body
// line when present.
func (r *rewriter) stripPrefix(line string) string {
	if r.prefix != "" && strings.HasPrefix(line, r.prefix) {
		return strings.TrimPrefix(line, r.prefix)
	}
	return line
}

// renderBlock renders collected mermaid source to a plain fenced code
// block of unicode art, re-applying the blockquote prefix line by line.
// Returns "" on any parse or render failure; the caller then emits the
// original source block instead.
func renderBlock(body []string, prefix string) string {
	block, err := renderDiagram(body)
	if err != nil {
		return ""
	}
	// The fence must outrun any backtick run inside the art — CommonMark
	// only closes a fence on a run at least as long as the opener — so a
	// label of literal backticks can never terminate it early.
	fence := strings.Repeat("`", 3)
	for _, l := range block {
		for run, n := 0, 0; run < len(l); run++ {
			if l[run] == '`' {
				n++
			} else {
				n = 0
			}
			if n >= len(fence) {
				fence = strings.Repeat("`", n+1)
			}
		}
	}
	var b strings.Builder
	b.WriteString(prefix + fence + "\n")
	for _, l := range block {
		b.WriteString(prefix + l + "\n")
	}
	b.WriteString(prefix + fence)
	return b.String()
}

// renderDiagram parses a mermaid body and returns the rendered art lines.
func renderDiagram(body []string) ([]string, error) {
	chart, kind, err := parse(stringOf(body))
	if err != nil {
		return nil, err
	}
	switch kind {
	case kindFlow:
		return renderFlow(chart.(*flowChart)), nil
	case kindSeq:
		return renderSeq(chart.(*seqChart)), nil
	}
	return nil, errUnknown
}

func stringOf(lines []string) string { return strings.Join(lines, "\n") }

// splitBlockPrefix separates a blockquote prefix (e.g. "> ", "> > ") from
// the content body.
func splitBlockPrefix(s string) (string, string) {
	i := 0
	for {
		j := i
		for j < len(s) && s[j] == '>' {
			j++
		}
		if j > i {
			if j < len(s) && s[j] == ' ' {
				j++
			}
			i = j
			continue
		}
		break
	}
	return s[:i], s[i:]
}

// fenceOpenInfo recognizes an opening code fence ("```" or "~~~", at least
// three chars) and returns the fence character, run length, and info
// string.
func fenceOpenInfo(trim string) (byte, int, string, bool) {
	if len(trim) < 3 {
		return 0, 0, "", false
	}
	ch := trim[0]
	if ch != '`' && ch != '~' {
		return 0, 0, "", false
	}
	n := 1
	for n < len(trim) && trim[n] == ch {
		n++
	}
	if n < 3 {
		return 0, 0, "", false
	}
	info := trim[n:]
	if ch == '`' && strings.ContainsRune(info, '`') {
		// CommonMark: info strings on backtick fences contain no backticks;
		// this is inline code, not a fence.
		return 0, 0, "", false
	}
	return ch, n, strings.TrimSpace(info), true
}

// isFenceClose reports a closing fence: a run of at least n of the fence
// character followed only by whitespace.
func isFenceClose(line string, ch byte, n int) bool {
	trim := strings.TrimLeft(line, " ")
	if len(line)-len(trim) > 3 {
		return false
	}
	k := 0
	for k < len(trim) && trim[k] == ch {
		k++
	}
	if k < n {
		return false
	}
	return strings.TrimSpace(trim[k:]) == ""
}
