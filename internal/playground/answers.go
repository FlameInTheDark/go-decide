package playground

import (
	"strconv"

	decide "github.com/FlameInTheDark/go-decide"
)

// answerViews flattens a result into display rows. Every number comes from a
// library method, so the UI cannot drift from Go semantics: Margin is
// ChoiceAnswer.Margin, Level is ScoreAnswer.Level, True is NoulAnswer.True.
func answerViews(result *decide.Result, questions []QuestionView) []AnswerView {
	if result == nil {
		return nil
	}

	// Index the display metadata by name so an answer can pick up its option
	// descriptions.
	views := make(map[string]QuestionView, len(questions))
	for _, view := range questions {
		views[view.Name] = view
	}

	out := make([]AnswerView, 0, len(questions))
	for _, question := range questions {
		answer, err := result.Answer(question.Name)
		if err != nil {
			continue
		}
		out = append(out, buildAnswer(answer, views[question.Name]))
	}
	return out
}

func buildAnswer(answer decide.Answer, question QuestionView) AnswerView {
	view := AnswerView{
		Name:     answer.Name(),
		Type:     answer.Type(),
		Summary:  answer.String(),
		Question: question,
	}

	switch typed := answer.(type) {
	case decide.ChoiceAnswer:
		text := ""
		for _, option := range question.Options {
			if option.Key == typed.Key {
				text = option.Text
			}
		}
		view.Key = typed.Key
		view.Text = text
		view.Confidence = typed.Confidence
		view.Margin = typed.Margin()
		view.Label = typed.Key
		view.Probabilities = barsFor(typed.Probabilities, typed.Ranked(), question.optionKeys(), optionTexts(question))
	case decide.NoulAnswer:
		view.Probability = typed.Probability
		view.True = typed.True()
		view.TrueLabel = "Yes"
		view.FalseLabel = "No"
		if question.Outcomes != nil {
			view.TrueLabel = question.Outcomes.True
			view.FalseLabel = question.Outcomes.False
		}
		view.Label = view.TrueLabel
		if !view.True {
			view.Label = view.FalseLabel
		}
	case decide.ScoreAnswer:
		view.Score = typed.Score
		view.Level = typed.Level()
		view.Max = typed.Max()
		view.Confidence = typed.Confidence
		view.Label = typed.Description(typed.Level())
		if view.Label == "" {
			view.Label = "level " + strconv.Itoa(view.Level)
		}
		view.Probabilities = barsFor(typed.Probabilities, typed.Ranked(), levelOrder(question), levelTexts(typed.Legend))
	}

	return view
}

// barsFor builds the probability rows in the order the request declared, so
// the panel reads the way the author wrote the question rather than reordering
// it behind their back. Any key the order does not mention is appended in
// ranked order, so nothing is silently dropped. When no order is known the
// ranking is used, which is what the CLI shows.
func barsFor(probabilities map[string]float64, ranked []decide.Ranked, order []string, texts map[string]string) []Bar {
	byKey := make(map[string]decide.Ranked, len(ranked))
	for _, entry := range ranked {
		byKey[entry.Key] = entry
	}

	keys := order
	if len(keys) == 0 {
		keys = make([]string, 0, len(ranked))
		for _, entry := range ranked {
			keys = append(keys, entry.Key)
		}
	} else {
		seen := make(map[string]bool, len(keys))
		for _, key := range keys {
			seen[key] = true
		}
		for _, entry := range ranked {
			if !seen[entry.Key] {
				keys = append(keys, entry.Key)
			}
		}
	}

	best := ""
	for _, entry := range ranked {
		if best == "" || entry.Probability > byKey[best].Probability {
			best = entry.Key
		}
	}

	bars := make([]Bar, 0, len(probabilities))
	for _, key := range keys {
		entry, ok := byKey[key]
		if !ok {
			continue
		}
		bars = append(bars, newBar(key, texts[key], entry.Probability, key == best))
	}
	return bars
}

func newBar(key, text string, probability float64, best bool) Bar {
	return Bar{
		Key:         key,
		Text:        text,
		Probability: probability,
		Percent:     probability * 100,
		Best:        best,
	}
}

// levelOrder returns the scale levels from lowest to highest, keyed the way
// ScoreAnswer stores them. A score reads as a scale, so it is shown in scale
// order: ranking the levels by probability puts the strongest one on top and
// scrambles the rest into something like 1, 2, 0. Any level the scale does not
// mention is appended in ranked order so nothing is dropped.
func levelOrder(question QuestionView) []string {
	keys := make([]string, 0, len(question.Scale)+4)
	for i := range question.Scale {
		keys = append(keys, strconv.Itoa(i))
	}
	return keys
}

// optionKeys returns the option keys in display order, so the probability
// rows line up with the options shown above them.
func (q QuestionView) optionKeys() []string {
	keys := make([]string, 0, len(q.Options))
	for _, option := range q.Options {
		keys = append(keys, option.Key)
	}
	return keys
}

func optionTexts(question QuestionView) map[string]string {
	texts := make(map[string]string, len(question.Options))
	for _, option := range question.Options {
		texts[option.Key] = option.Text
	}
	return texts
}

// levelTexts keys the legend by the string form of the level index, matching
// how ScoreAnswer stores it.
func levelTexts(legend map[string]string) map[string]string {
	return legend
}
