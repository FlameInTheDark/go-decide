package decide

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
)

// Answer is a single decision result. The concrete types are [ChoiceAnswer],
// [NoulAnswer] and [ScoreAnswer]; use a type switch or the typed accessors on
// [Result] to consume them.
type Answer interface {
	// Name is the question this answer belongs to.
	Name() string
	// Type is the question type of the answer.
	Type() Type
	// String renders a short human readable summary.
	String() string
}

// ChoiceAnswer is the result of a choice question.
type ChoiceAnswer struct {
	QuestionName string
	// Key is the selected option key.
	Key string
	// Probabilities maps every option key to its probability.
	Probabilities map[string]float64
	// Confidence measures how strongly one option dominates, from 0
	// (uniform) to 1 (one option takes almost all mass).
	Confidence float64
}

// Name implements [Answer].
func (a ChoiceAnswer) Name() string { return a.QuestionName }

// Type implements [Answer].
func (a ChoiceAnswer) Type() Type { return TypeChoice }

// String implements [Answer].
func (a ChoiceAnswer) String() string {
	return fmt.Sprintf("%s: choice=%s (%.4f, confidence %.4f)", a.QuestionName, a.Key, a.Probability(a.Key), a.Confidence)
}

// Probability returns the probability of the named option, or 0.
func (a ChoiceAnswer) Probability(key string) float64 { return a.Probabilities[key] }

// Best returns the selected key.
func (a ChoiceAnswer) Best() string { return a.Key }

// Ranked returns the options ordered from most to least probable.
func (a ChoiceAnswer) Ranked() []Ranked {
	return rank(a.Probabilities)
}

// Margin returns the gap between the two most probable options, which is a
// more stable signal than raw confidence.
func (a ChoiceAnswer) Margin() float64 {
	ranked := a.Ranked()
	if len(ranked) < 2 {
		return 0
	}
	return ranked[0].Probability - ranked[1].Probability
}

// NoulAnswer is the result of a yes/no question.
type NoulAnswer struct {
	QuestionName string
	// Probability is the probability of "true", from 0 to 1.
	Probability float64
}

// Name implements [Answer].
func (a NoulAnswer) Name() string { return a.QuestionName }

// Type implements [Answer].
func (a NoulAnswer) Type() Type { return TypeNoul }

// String implements [Answer].
func (a NoulAnswer) String() string {
	return fmt.Sprintf("%s: noul=%.4f", a.QuestionName, a.Probability)
}

// True reports whether the probability of "true" exceeds the false threshold
// of 0.5.
func (a NoulAnswer) True() bool { return a.Probability >= 0.5 }

// False reports the complement of [NoulAnswer.True].
func (a NoulAnswer) False() bool { return !a.True() }

// ScoreAnswer is the result of a score question.
type ScoreAnswer struct {
	QuestionName string
	// Score is the probability-weighted average of the zero-based
	// criterion indices. It is neither rounded to a level nor normalised
	// to the 0-1 range.
	Score float64
	// Legend maps zero-based criterion indices to their descriptions.
	Legend map[string]string
	// Probabilities maps zero-based criterion indices to their
	// probabilities.
	Probabilities map[string]float64
	// Confidence measures distribution concentration, from 0 to 1.
	Confidence float64
}

// Name implements [Answer].
func (a ScoreAnswer) Name() string { return a.QuestionName }

// Type implements [Answer].
func (a ScoreAnswer) Type() Type { return TypeScore }

// String implements [Answer].
func (a ScoreAnswer) String() string {
	return fmt.Sprintf("%s: score=%.4f (confidence %.4f, level %d)", a.QuestionName, a.Score, a.Confidence, a.Level())
}

// Max returns the highest level index of the scale, which is the number of
// criteria minus one.
func (a ScoreAnswer) Max() int {
	if len(a.Probabilities) == 0 {
		return 0
	}
	max := 0
	for key := range a.Probabilities {
		if idx := indexOf(key); idx > max {
			max = idx
		}
	}
	return max
}

// Level returns the nearest integer level for the score.
func (a ScoreAnswer) Level() int { return int(math.Round(a.Score)) }

// Description returns the legend text of a level, or an empty string.
func (a ScoreAnswer) Description(level int) string {
	return a.Legend[strconv.Itoa(level)]
}

// Best returns the level with the highest probability.
func (a ScoreAnswer) Best() int {
	best, bestProb := 0, math.Inf(-1)
	for key, prob := range a.Probabilities {
		if prob > bestProb {
			best, bestProb = indexOf(key), prob
		}
	}
	return best
}

// Ranked returns the levels ordered from most to least probable.
func (a ScoreAnswer) Ranked() []Ranked {
	return rank(a.Probabilities)
}

// Ranked is one entry of a probability ranking.
type Ranked struct {
	// Key is the option key or the zero-based level index as a string.
	Key string
	// Probability is the probability assigned to the key.
	Probability float64
}

// Answers holds the answers of a request, indexed by question name and kept in
// question order.
type Answers struct {
	list  []Answer
	index map[string]Answer
}

// NewAnswers builds an Answers collection from a slice of answers.
func NewAnswers(answers []Answer) Answers {
	list := append([]Answer(nil), answers...)
	index := make(map[string]Answer, len(list))
	for _, answer := range answers {
		index[answer.Name()] = answer
	}
	return Answers{list: list, index: index}
}

// Len returns the number of answers.
func (a Answers) Len() int { return len(a.list) }

// All returns the answers in question order.
func (a Answers) All() []Answer { return append([]Answer(nil), a.list...) }

// Get returns the answer for the named question.
func (a Answers) Get(name string) (Answer, error) {
	answer, ok := a.index[name]
	if !ok {
		return nil, &Error{Kind: KindNotFound, Message: fmt.Sprintf("decide: no answer for question %q", name)}
	}
	return answer, nil
}

// MarshalJSON implements [json.Marshaler]. Answers are emitted as an object
// keyed by question name, where the value is the answer in its natural shape:
//
//	{"label": {"type": "choice", "key": "bug", "probabilities": {...}}}
//
// Answers that were never populated are omitted.
func (a Answers) MarshalJSON() ([]byte, error) {
	out := make(map[string]any, len(a.list))
	for _, answer := range a.list {
		switch typed := answer.(type) {
		case ChoiceAnswer:
			out[typed.QuestionName] = map[string]any{
				"type":          string(TypeChoice),
				"key":           typed.Key,
				"probabilities": typed.Probabilities,
				"confidence":    typed.Confidence,
			}
		case NoulAnswer:
			out[typed.QuestionName] = map[string]any{
				"type": string(TypeNoul),
				"noul": typed.Probability,
			}
		case ScoreAnswer:
			out[typed.QuestionName] = map[string]any{
				"type":          string(TypeScore),
				"score":         typed.Score,
				"legend":        typed.Legend,
				"probabilities": typed.Probabilities,
				"confidence":    typed.Confidence,
			}
		default:
			if answer != nil {
				out[answer.Name()] = answer
			}
		}
	}
	return json.Marshal(out)
}

// UnmarshalJSON implements [json.Unmarshaler]. It reads the same document
// [Answers.MarshalJSON] writes, so a JSON export round-trips back into Go:
//
//	{"label": {"type": "choice", "key": "bug", "probabilities": {...}}}
//
// Keys are visited in sorted order, which makes the resulting [Answers.All]
// order deterministic. The "noul" field is also accepted as an alias of
// "probability", because that is how the wire format spells it.
func (a *Answers) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}

	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)

	list := make([]Answer, 0, len(names))
	for _, name := range names {
		answer, err := decodeAnswer(name, fields[name])
		if err != nil {
			return err
		}
		list = append(list, answer)
	}

	*a = NewAnswers(list)
	return nil
}

// decodeAnswer rebuilds one answer from its JSON object.
func decodeAnswer(name string, raw json.RawMessage) (Answer, error) {
	var head struct {
		Type Type `json:"type"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return nil, fmt.Errorf("decide: decode answer %q: %w", name, err)
	}

	switch head.Type {
	case TypeChoice:
		var body struct {
			Key           string             `json:"key"`
			Probabilities map[string]float64 `json:"probabilities"`
			Confidence    float64            `json:"confidence"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			return nil, fmt.Errorf("decide: decode choice answer %q: %w", name, err)
		}
		if body.Probabilities == nil {
			body.Probabilities = map[string]float64{}
		}
		return ChoiceAnswer{
			QuestionName:  name,
			Key:           body.Key,
			Probabilities: body.Probabilities,
			Confidence:    body.Confidence,
		}, nil

	case TypeNoul:
		var body struct {
			Probability *float64 `json:"probability"`
			Noul        *float64 `json:"noul"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			return nil, fmt.Errorf("decide: decode noul answer %q: %w", name, err)
		}
		probability := 0.0
		switch {
		case body.Probability != nil:
			probability = *body.Probability
		case body.Noul != nil:
			probability = *body.Noul
		}
		return NoulAnswer{QuestionName: name, Probability: probability}, nil

	case TypeScore:
		var body struct {
			Score         float64            `json:"score"`
			Legend        map[string]string  `json:"legend"`
			Probabilities map[string]float64 `json:"probabilities"`
			Confidence    float64            `json:"confidence"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			return nil, fmt.Errorf("decide: decode score answer %q: %w", name, err)
		}
		if body.Probabilities == nil {
			body.Probabilities = map[string]float64{}
		}
		if body.Legend == nil {
			body.Legend = map[string]string{}
		}
		return ScoreAnswer{
			QuestionName:  name,
			Score:         body.Score,
			Legend:        body.Legend,
			Probabilities: body.Probabilities,
			Confidence:    body.Confidence,
		}, nil

	default:
		return nil, fmt.Errorf("decide: answer %q has unknown type %q", name, head.Type)
	}
}

// Choice returns the choice answer for the named question.
func (a Answers) Choice(name string) (ChoiceAnswer, error) {
	return typedAnswer[ChoiceAnswer](a, name, TypeChoice)
}

// Noul returns the noul answer for the named question.
func (a Answers) Noul(name string) (NoulAnswer, error) {
	return typedAnswer[NoulAnswer](a, name, TypeNoul)
}

// Score returns the score answer for the named question.
func (a Answers) Score(name string) (ScoreAnswer, error) {
	return typedAnswer[ScoreAnswer](a, name, TypeScore)
}

func typedAnswer[T Answer](a Answers, name string, want Type) (T, error) {
	var zero T

	answer, err := a.Get(name)
	if err != nil {
		return zero, err
	}

	typed, ok := answer.(T)
	if !ok {
		return zero, &Error{
			Kind:    KindDecode,
			Message: fmt.Sprintf("decide: question %q answered with type %q, want %q", name, answer.Type(), want),
		}
	}
	return typed, nil
}

func indexOf(key string) int {
	idx, err := strconv.Atoi(key)
	if err != nil {
		return 0
	}
	return idx
}

func rank(probabilities map[string]float64) []Ranked {
	ranked := make([]Ranked, 0, len(probabilities))
	for key, prob := range probabilities {
		ranked = append(ranked, Ranked{Key: key, Probability: prob})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		return ranked[i].Probability > ranked[j].Probability
	})
	return ranked
}
