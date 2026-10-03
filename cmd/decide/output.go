package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	decide "github.com/FlameInTheDark/go-decide"
)

// barWidth is the number of cells in a probability bar.
const barWidth = 20

const (
	glyphFull  = "█" // solid block
	glyphEmpty = "░" // light shade for the unused part of a track
)

// theme bundles the styles used across the CLI output so the look stays
// consistent and is easy to tweak in one place.
type theme struct {
	meta     lipgloss.Style
	question lipgloss.Style
	answer   lipgloss.Style
	key      lipgloss.Style
	muted    lipgloss.Style
	track    lipgloss.Style
	// bars is a calm gradient from the most likely option to the least.
	bars    []lipgloss.Style
	colored bool
}

func newTheme(colored bool) theme {
	t := theme{colored: colored}

	base := lipgloss.NewStyle()
	plain := lipgloss.NewStyle()
	style := func(s lipgloss.Style) lipgloss.Style {
		if colored {
			return s
		}
		return plain
	}

	t.meta = style(base.Faint(true))
	t.question = style(base.Foreground(lipgloss.Color("6")))
	t.answer = style(base.Bold(true).Foreground(lipgloss.Color("2")))
	t.key = style(base.Foreground(lipgloss.Color("7")))
	t.muted = style(base.Faint(true))
	t.track = style(base.Foreground(lipgloss.Color("8")))

	if colored {
		t.bars = []lipgloss.Style{
			base.Foreground(lipgloss.Color("42")),
			base.Foreground(lipgloss.Color("41")),
			base.Foreground(lipgloss.Color("214")),
			base.Foreground(lipgloss.Color("220")),
			base.Foreground(lipgloss.Color("240")),
		}
	}

	return t
}

// barStyle picks a gradient style by rank, staying in range for long lists.
func (t theme) barStyle(rank int) lipgloss.Style {
	if !t.colored || len(t.bars) == 0 {
		return lipgloss.NewStyle()
	}
	if rank >= len(t.bars) {
		rank = len(t.bars) - 1
	}
	return t.bars[rank]
}

// printer renders a result to a writer using a theme.
type printer struct {
	out   io.Writer
	theme theme
}

// newPrinter builds a printer. Colour is disabled when the destination cannot
// render it, when NO_COLOR is set, or when --plain is used.
func newPrinter(out io.Writer, forceColor, forcePlain bool) *printer {
	return &printer{out: out, theme: newTheme(shouldColor(out, forceColor, forcePlain))}
}

// shouldColor reports whether ANSI escapes may be written to out.
func shouldColor(out io.Writer, forceColor, forcePlain bool) bool {
	switch {
	case forcePlain, os.Getenv("NO_COLOR") != "":
		return false
	case forceColor:
		return true
	case lipgloss.ColorProfile() == termenv.Ascii:
		return false
	default:
		return true
	}
}

// fill returns how many cells of a bar a probability occupies, clamped to the
// track width.
func fill(probability float64) int {
	if probability <= 0 {
		return 0
	}
	cells := int(probability*barWidth + 0.5)
	switch {
	case cells > barWidth:
		return barWidth
	case cells < 1:
		return 1
	default:
		return cells
	}
}

// bar renders a probability as a solid bar of the given length, used where a
// track is not wanted.
func bar(probability float64) string {
	cells := fill(probability)
	if cells == 0 {
		return ""
	}
	return strings.Repeat(glyphFull, cells)
}

// barRow renders one aligned probability row: a coloured track followed by the
// percentage.
func (p printer) barRow(rank int, probability float64) string {
	cells := fill(probability)
	style := p.theme.barStyle(rank)

	var chart string
	if p.theme.colored {
		chart = style.Render(strings.Repeat(glyphFull, cells)) +
			p.theme.track.Render(strings.Repeat(glyphEmpty, barWidth-cells))
	} else {
		chart = bar(probability)
	}

	return fmt.Sprintf("%s  %s", chart, p.theme.muted.Render(fmt.Sprintf("%6.2f%%", probability*100)))
}

// summary prints the one-line header describing the call, its model and cost.
func (p printer) summary(result *decide.Result) {
	parts := []string{result.Provider, result.Model}
	if result.UpstreamModel != "" && result.UpstreamModel != result.Model {
		parts = append(parts, fmt.Sprintf("%s via %s", result.UpstreamModel, result.Upstream))
	}

	fmt.Fprintf(p.out, "%s\n", p.theme.meta.Render(strings.Join(parts, "  ·  ")))
	fmt.Fprintf(p.out, "%s\n", p.theme.muted.Render(fmt.Sprintf(
		"%d in / %d out", result.Usage.InputTokens, result.Usage.OutputTokens)))

	if result.Usage.Cost > 0 {
		fmt.Fprintf(p.out, "%s\n", p.theme.muted.Render(fmt.Sprintf("$%.9f", result.Usage.Cost)))
	}
	if result.ID != "" {
		fmt.Fprintf(p.out, "%s\n", p.theme.muted.Render(result.ID))
	}
}

// heading prints a question name and its resolved answer.
func (p printer) heading(name, answer string) {
	fmt.Fprintf(p.out, "\n%s  %s\n", p.theme.question.Render(name), p.theme.answer.Render(answer))
}

// legend prints the per-option probability rows.
func (p printer) legend(ranked []decide.Ranked) {
	for rank, entry := range ranked {
		fmt.Fprintf(p.out, "  %s  %s\n",
			p.theme.key.Render(fmt.Sprintf("%-10s", truncate(entry.Key, 10))),
			p.barRow(rank, entry.Probability))
	}
}

// footer prints the diagnostics line under a question.
func (p printer) footer(format string, args ...any) {
	fmt.Fprintf(p.out, "  %s\n", p.theme.muted.Render(fmt.Sprintf(format, args...)))
}

// print renders the whole result: a header, then one block per question in the
// order asked.
func (p printer) print(result *decide.Result, req decide.Request, noLegend bool) {
	p.summary(result)

	for _, question := range req.Questions {
		answer, err := result.Answers.Get(question.Name)
		if err != nil {
			// A provider may skip a question; say so instead of failing.
			p.heading(question.Name, "no answer")
			continue
		}

		switch typed := answer.(type) {
		case decide.ChoiceAnswer:
			p.heading(question.Name, typed.Key)
			if !noLegend {
				p.legend(typed.Ranked())
			}
			p.footer("confidence %.4f  ·  margin %.4f", typed.Confidence, typed.Margin())

		case decide.NoulAnswer:
			p.heading(question.Name, fmt.Sprintf("%s  %s",
				fmt.Sprintf("%.2f%%", typed.Probability*100), yesNo(typed.True())))
			p.barLine(0, typed.Probability)

		case decide.ScoreAnswer:
			p.heading(question.Name, fmt.Sprintf("%.4f  %s", typed.Score, levelLabel(typed)))
			if !noLegend {
				p.legend(typed.Ranked())
			}
			p.footer("confidence %.4f", typed.Confidence)

		default:
			p.heading(question.Name, answer.String())
		}
	}

	fmt.Fprintln(p.out)
}

// barLine prints a single probability bar, used for yes/no answers.
func (p printer) barLine(rank int, probability float64) {
	fmt.Fprintf(p.out, "  %s\n", p.barRow(rank, probability))
}

// levelLabel describes a score's rounded level, falling back to its index.
func levelLabel(score decide.ScoreAnswer) string {
	if description := score.Description(score.Level()); description != "" {
		return fmt.Sprintf("%s (%d/%d)", description, score.Level(), score.Max())
	}
	return fmt.Sprintf("level %d/%d", score.Level(), score.Max())
}

// truncate shortens a string to at most width runes.
func truncate(value string, width int) string {
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	if width <= 1 {
		return string(runes[:width])
	}
	return string(runes[:width-1]) + "…"
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}
