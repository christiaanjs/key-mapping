package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/christiaanswanepoel/key-mapping/core"
)

// modeOrder fixes the left-to-right tab order shown in the header, matching
// the trainer/index.html tab bar (mirror, nav, scratchpad, reference).
var modeOrder = []struct {
	mode  core.Mode
	label string
}{
	{core.ModeMirror, "Mirror typing"},
	{core.ModeNav, "Nav layer"},
	{core.ModeScratch, "Scratchpad"},
	{core.ModeReference, "Reference"},
}

func renderView(m model) string {
	var b strings.Builder

	b.WriteString(renderTabs(m.state.Mode))
	b.WriteString("\n\n")

	switch m.state.Mode {
	case core.ModeMirror:
		b.WriteString(renderMirror(m.state))
	case core.ModeNav:
		b.WriteString(renderNav(m.state))
	case core.ModeScratch:
		b.WriteString(renderScratch(m.scratch))
	case core.ModeReference:
		b.WriteString(renderReference(m.state.Reference))
	}

	b.WriteString("\n\n")
	b.WriteString(renderCorpusStatus(m.state.Corpus, m.tick))
	b.WriteString("\n")
	b.WriteString(renderHelp(m.state.Mode))
	b.WriteString("\n")

	return b.String()
}

// spinnerFrames animates the "still generating" state. Braille dots render on
// any modern terminal and take one cell, so the status line never reflows.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// renderCorpusStatus shows where practice text is coming from and, crucially,
// whether more of it is still arriving. A streaming corpus fills in the
// background, so without this line the user would have no idea the trainer had
// silently started them on fallback text — or that it had quietly upgraded.
func renderCorpusStatus(cs core.CorpusStatus, tick int) string {
	source := cs.Source
	if source == "" {
		source = "static"
	}
	prefix := styleMuted.Render("corpus: ")

	switch cs.Phase {
	case core.CorpusWarming:
		spin := spinnerFrames[((tick%len(spinnerFrames))+len(spinnerFrames))%len(spinnerFrames)]
		return prefix + styleAccent.Render(source) + " " + styleCurrent.Render(spin) +
			styleMuted.Render(" generating"+detailSuffix(cs)+" — drilling on static text meanwhile")

	case core.CorpusStreaming:
		return prefix + styleAccent.Render(source) + " " + styleDone.Render("●") +
			styleMuted.Render(fmt.Sprintf(" streaming%s — %d words, %d sentences so far",
				detailSuffix(cs), cs.Words, cs.Sentences))

	case core.CorpusFailed:
		detail := cs.Detail
		if detail == "" {
			detail = "unavailable"
		}
		return prefix + styleDanger.Render(source+" failed") +
			styleMuted.Render(" — drilling on static text; retrying. ("+detail+")")

	default:
		return prefix + styleMuted.Render(fmt.Sprintf("%s — %d words, %d sentences", source, cs.Words, cs.Sentences))
	}
}

// detailSuffix parenthesizes the status detail (model name, error) when set.
func detailSuffix(cs core.CorpusStatus) string {
	if cs.Detail == "" {
		return ""
	}
	return " (" + cs.Detail + ")"
}

func renderTabs(active core.Mode) string {
	parts := make([]string, 0, len(modeOrder))
	for _, mo := range modeOrder {
		if mo.mode == active {
			parts = append(parts, styleTabActive.Render(mo.label))
		} else {
			parts = append(parts, styleTabInactive.Render(mo.label))
		}
	}
	return strings.Join(parts, " ")
}

// ---- mirror mode ----

func renderMirror(st core.State) string {
	d := st.Drill
	if d == nil {
		return styleMuted.Render("loading…")
	}

	var chars strings.Builder
	for _, c := range d.Chars {
		disp := c.Char
		if disp == " " {
			disp = "·"
		}
		switch c.Status {
		case core.CharDone:
			chars.WriteString(styleDone.Render(disp))
		case core.CharCurrent:
			chars.WriteString(styleCurrent.Render(disp))
		default:
			chars.WriteString(stylePending.Render(disp))
		}
	}

	contentChips := chip("words", st.Content == core.ContentWords) + " " + chip("sentences", st.Content == core.ContentSentences)
	lengthChips := ""
	if st.Content == core.ContentWords {
		lengthChips = "   " +
			chip("short", st.Length == core.LengthShort) + " " +
			chip("any", st.Length == core.LengthAny) + " " +
			chip("long", st.Length == core.LengthLong)
	}

	hint := renderHint(d)

	box := styleBox.Render(chars.String() + "\n\n" + hint)

	stats := renderStatCards([]statCard{
		{"Correct", fmt.Sprint(d.Stats.Hits), styleDone},
		{"Misses", fmt.Sprint(d.Stats.Misses), styleDanger},
		{"Accuracy", fmt.Sprintf("%d%%", d.Stats.Accuracy), styleStatVal},
		{"WPM", statOrDash(d.Stats.WPM), styleStatVal},
	})

	feedback := ""
	if d.Feedback != "" {
		feedback = feedbackStyle(d.Feedback).Render(d.Feedback)
	}

	return contentChips + lengthChips + "\n\n" + box + "\n\n" + stats + "\n\n" + feedback
}

func renderHint(d *core.DrillState) string {
	switch {
	case d.Complete:
		return styleDone.Render("Complete!")
	case !d.Hint.Mapped:
		return styleMuted.Render("skipping unsupported char…")
	case d.Hint.IsSpace:
		return "Next: tap " + keycap("space")
	case d.Hint.HoldSpace:
		return fmt.Sprintf("Next: %s — hold %s + %s", styleCurrent.Render(d.Hint.Output), keycap("space"), keycap(d.Hint.Key))
	default:
		return fmt.Sprintf("Next: %s — press %s", styleCurrent.Render(d.Hint.Output), keycap(d.Hint.Key))
	}
}

// feedbackStyle guesses success vs. miss coloring from the feedback text:
// the core only ever sets a "complete"/"done" style message on success, and
// a Diagnose(...) sentence on a miss (see core/app.go handleType).
func feedbackStyle(feedback string) lipgloss.Style {
	lower := strings.ToLower(feedback)
	if strings.Contains(lower, "complete") || strings.Contains(lower, "done") {
		return styleDone
	}
	return styleDanger
}

// ---- nav mode ----

var arrowGlyph = map[string]string{
	core.ArrowLeft:  "←",
	core.ArrowDown:  "↓",
	core.ArrowUp:    "↑",
	core.ArrowRight: "→",
}

func renderNav(st core.State) string {
	n := st.Nav
	if n == nil {
		return styleMuted.Render("loading…")
	}

	var seq strings.Builder
	for i, dir := range n.Sequence {
		g := arrowGlyph[dir]
		if g == "" {
			g = "?"
		}
		switch {
		case i < n.Index:
			seq.WriteString(styleDone.Render(g))
		case i == n.Index:
			seq.WriteString(styleCurrent.Render(g))
		default:
			seq.WriteString(stylePending.Render(g))
		}
		seq.WriteString(" ")
	}

	box := styleBox.Render(seq.String())

	stats := renderStatCards([]statCard{
		{"Correct", fmt.Sprint(n.Hits), styleDone},
		{"Misses", fmt.Sprint(n.Misses), styleDanger},
		{"Progress", fmt.Sprintf("%d/%d", n.Index, len(n.Sequence)), styleStatVal},
	})

	feedback := ""
	if n.Feedback != "" {
		feedback = styleDanger.Render(n.Feedback)
	}

	return box + "\n\n" + stats + "\n\n" + feedback
}

// ---- scratch mode ----

func renderScratch(buf []rune) string {
	text := string(buf)
	if text == "" {
		text = styleMuted.Render("(start typing — this shows exactly what your keymap produces)")
	}
	return styleBox.Render(text)
}

// ---- reference mode ----

func renderReference(rows []core.RefRow) string {
	if len(rows) == 0 {
		return styleMuted.Render("loading…")
	}

	var left strings.Builder
	left.WriteString(styleBold.Render("Left key + space produces") + "\n\n")
	for _, r := range rows {
		left.WriteString(fmt.Sprintf("%s  %s  %s\n", keycap(r.Key), styleMuted.Render("→"), styleAccent.Render(r.Output)))
	}

	var right strings.Builder
	right.WriteString(styleBold.Render("Layer keys") + "\n\n")
	layerRows := [][2]string{
		{"caps", "return"},
		{"tab", "delete"},
		{"hold b", "arm nav (no space)"},
		{"a s d f", "← ↓ ↑ →"},
	}
	for _, lr := range layerRows {
		right.WriteString(fmt.Sprintf("%s  %s  %s\n", keycap(lr[0]), styleMuted.Render("→"), lr[1]))
	}

	cols := lipgloss.JoinHorizontal(lipgloss.Top, left.String(), "     ", right.String())
	return styleBox.Render(cols)
}

// ---- help line ----

func renderHelp(mode core.Mode) string {
	global := "ctrl+c/esc quit  ·  F1-F4 switch mode (mirror/nav/scratch/reference)"
	var modeHelp string
	switch mode {
	case core.ModeMirror:
		modeHelp = "type to drill  ·  ctrl+n skip  ·  ctrl+w toggle words/sentences  ·  ctrl+l cycle length"
	case core.ModeNav:
		modeHelp = "arrow keys to navigate the sequence"
	case core.ModeScratch:
		modeHelp = "type freely  ·  backspace deletes  ·  shows exactly what the keymap sends"
	case core.ModeReference:
		modeHelp = "read-only key -> output table"
	}
	return styleHelp.Render(global) + "\n" + styleHelp.Render(modeHelp)
}

// ---- small render helpers ----

type statCard struct {
	label string
	value string
	style lipgloss.Style
}

func renderStatCards(cards []statCard) string {
	parts := make([]string, 0, len(cards))
	for _, c := range cards {
		parts = append(parts, styleStatLabel.Render(c.label+": ")+c.style.Render(c.value))
	}
	return strings.Join(parts, "    ")
}

func statOrDash(wpm int) string {
	if wpm <= 0 {
		return "—"
	}
	return fmt.Sprint(wpm)
}
