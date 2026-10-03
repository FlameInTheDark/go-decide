package decide

import (
	"encoding/json"
	"sort"
	"strings"
)

// Type enumerates the question types supported by System One decision models.
type Type string

const (
	// TypeChoice picks one of several named options and reports each
	// option's probability.
	TypeChoice Type = "choice"
	// TypeNoul answers a yes/no question with the probability of "true".
	TypeNoul Type = "noul"
	// TypeScore places the state on an ordered scale and reports a
	// probability-weighted average of the zero-based levels.
	TypeScore Type = "score"
)

// Valid reports whether the type is one of the supported question types.
func (t Type) Valid() bool {
	switch t {
	case TypeChoice, TypeNoul, TypeScore:
		return true
	default:
		return false
	}
}

// MinCriteria and MaxCriteria bound the number of options or scale levels
// accepted by choice and score questions. The upper bound is the System One
// wire limit, one entry per letter of the alphabet.
const (
	MinCriteria = 2
	MaxCriteria = 26
)

// Question is a single typed question about a [State]. Build one with
// [Choice], [Noul] or [Score]; the builder methods return copies, so a
// partially built Question can be shared as a template.
type Question struct {
	// Name is the key the answer is returned under.
	Name string `json:"-"`
	// Type is the question type.
	Type Type `json:"type"`
	// Instructions tells the model what to decide.
	Instructions string `json:"instructions"`
	// Options maps option keys to their descriptions. Used by choice
	// questions.
	Options map[string]string `json:"-"`
	// Scale lists descriptions ordered from the lowest level to the
	// highest. Used by score questions.
	Scale []string `json:"-"`
	// Noul optionally overrides the "false" and "true" descriptions.
	Noul *NoulCriteria `json:"-"`
}

// NoulCriteria customises the descriptions of the two yes/no outcomes.
// Empty fields fall back to "No" and "Yes".
type NoulCriteria struct {
	False string
	True  string
}

// Options are the named options of a choice question.
type Options map[string]string

// Scale is the ordered level list of a score question, lowest level first.
type Scale []string

// Choice builds a choice question. Options are optional at the call site so
// that a template can be defined first and filled in later:
//
//	decide.Choice("label", "Which label fits?")
//	decide.Choice("label", "Which label fits?", decide.Options{
//		"bug": "Software errors",
//		"billing": "Payments and refunds",
//	})
//
// Repeated Options arguments are merged.
func Choice(name, instructions string, options ...Options) Question {
	question := Question{
		Name:         name,
		Type:         TypeChoice,
		Instructions: instructions,
		Options:      map[string]string{},
	}
	for _, set := range options {
		question = question.WithOptions(set)
	}
	return question
}

// Noul builds a yes/no question, optionally describing the two outcomes:
//
//	decide.Noul("is_urgent", "Does this block revenue?")
//	decide.Noul("is_urgent", "Does this block revenue?", decide.NoulCriteria{
//		False: "Revenue continues",
//		True:  "Revenue is blocked",
//	})
//
// Repeated criteria arguments are merged.
func Noul(name, instructions string, outcomes ...NoulCriteria) Question {
	question := Question{Name: name, Type: TypeNoul, Instructions: instructions}
	for _, criteria := range outcomes {
		question = question.WithOutcomes(criteria.False, criteria.True)
	}
	return question
}

// Score builds a score question over an ordered scale, lowest level first:
//
//	decide.Score("urgency", "How urgent?", decide.Scale{"Soon", "Now"})
//
// Repeated scale arguments are appended in order.
func Score(name, instructions string, scale ...Scale) Question {
	question := Question{Name: name, Type: TypeScore, Instructions: instructions}
	for _, levels := range scale {
		question = question.WithScale(levels...)
	}
	return question
}

// WithOption appends a single option to a choice question.
func (q Question) WithOption(key, description string) Question {
	options := make(map[string]string, len(q.Options)+1)
	for k, v := range q.Options {
		options[k] = v
	}
	options[key] = description
	q.Options = options
	return q
}

// WithOptions appends every option to a choice question.
func (q Question) WithOptions(options map[string]string) Question {
	merged := make(map[string]string, len(q.Options)+len(options))
	for k, v := range q.Options {
		merged[k] = v
	}
	for k, v := range options {
		merged[k] = v
	}
	q.Options = merged
	return q
}

// WithOutcomes attaches the descriptions of the "false" and "true" outcomes to
// a noul question. Repeated calls merge field by field, so a partial override
// such as NoulCriteria{True: "yes"} keeps the existing false description.
func (q Question) WithOutcomes(no, yes string) Question {
	criteria := NoulCriteria{}
	if q.Noul != nil {
		criteria = *q.Noul
	}
	if no != "" {
		criteria.False = no
	}
	if yes != "" {
		criteria.True = yes
	}
	q.Noul = &criteria
	return q
}

// WithScale appends ordered levels to a score question, lowest first.
func (q Question) WithScale(levels ...string) Question {
	q.Scale = append(append([]string(nil), q.Scale...), levels...)
	return q
}

// WithLevels is an alias of [Question.WithScale] for slice based call sites.
func (q Question) WithLevels(levels []string) Question {
	return q.WithScale(levels...)
}

// OptionKeys returns the option keys of a choice question in sorted order.
func (q Question) OptionKeys() []string {
	keys := make([]string, 0, len(q.Options))
	for k := range q.Options {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Validate reports every problem that prevents the question from being sent.
func (q Question) Validate(v *ValidationError) {
	name := q.Name
	if name == "" {
		name = "<unnamed>"
	}

	if strings.TrimSpace(q.Name) == "" {
		v.Add("question name must not be empty")
	}
	if strings.TrimSpace(q.Instructions) == "" {
		v.Add("question %q: instructions must not be empty", name)
	}
	if !q.Type.Valid() {
		v.Add("question %q: unknown type %q", name, q.Type)
		return
	}

	switch q.Type {
	case TypeChoice:
		if len(q.Scale) > 0 {
			v.Add("question %q: choice questions use Options, not Scale", name)
		}
		if q.Noul != nil {
			v.Add("question %q: choice questions do not take noul criteria", name)
		}
		if n := len(q.Options); n < MinCriteria || n > MaxCriteria {
			v.Add("question %q: choice needs between %d and %d options, got %d", name, MinCriteria, MaxCriteria, n)
		}
		for key, description := range q.Options {
			if strings.TrimSpace(key) == "" {
				v.Add("question %q: option keys must not be empty", name)
			}
			if strings.TrimSpace(description) == "" {
				v.Add("question %q: option %q needs a description", name, key)
			}
		}
	case TypeScore:
		if len(q.Options) > 0 {
			v.Add("question %q: score questions use Scale, not Options", name)
		}
		if q.Noul != nil {
			v.Add("question %q: score questions do not take noul criteria", name)
		}
		if n := len(q.Scale); n < MinCriteria || n > MaxCriteria {
			v.Add("question %q: score needs between %d and %d levels, got %d", name, MinCriteria, MaxCriteria, n)
		}
		for i, level := range q.Scale {
			if strings.TrimSpace(level) == "" {
				v.Add("question %q: scale level %d must not be empty", name, i)
			}
		}
	case TypeNoul:
		if len(q.Options) > 0 {
			v.Add("question %q: noul questions take NoulCriteria, not Options", name)
		}
		if len(q.Scale) > 0 {
			v.Add("question %q: noul questions do not take a Scale", name)
		}
		if q.Noul != nil && strings.TrimSpace(q.Noul.False) == "" && strings.TrimSpace(q.Noul.True) == "" {
			v.Add("question %q: noul outcomes must not be both empty", name)
		}
	}
}

// wireQuestion is the JSON shape shared by the System One endpoints.
type wireQuestion struct {
	Type         Type   `json:"type"`
	Instructions string `json:"instructions"`
	Criteria     any    `json:"criteria,omitempty"`
}

// MarshalJSON implements [json.Marshaler]. Choice criteria are emitted as an
// object, score criteria as an array and noul criteria as an optional object
// holding the "false" and "true" descriptions.
func (q Question) MarshalJSON() ([]byte, error) {
	wire := wireQuestion{Type: q.Type, Instructions: q.Instructions}

	switch q.Type {
	case TypeChoice:
		wire.Criteria = q.Options
	case TypeScore:
		wire.Criteria = q.Scale
	case TypeNoul:
		if q.Noul != nil && (q.Noul.False != "" || q.Noul.True != "") {
			wire.Criteria = map[string]string{"false": q.Noul.False, "true": q.Noul.True}
		}
	}

	return json.Marshal(wire)
}

// UnmarshalJSON implements [json.Unmarshaler]. The question name comes from
// the map key the answer is stored under, so it must be set by the caller.
func (q *Question) UnmarshalJSON(data []byte) error {
	var wire wireQuestion
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}

	q.Type = wire.Type
	q.Instructions = wire.Instructions
	q.Options = nil
	q.Scale = nil
	q.Noul = nil

	switch typed := wire.Criteria.(type) {
	case map[string]any:
		if q.Type == TypeNoul {
			q.Noul = &NoulCriteria{
				False: stringOf(typed["false"]),
				True:  stringOf(typed["true"]),
			}
			break
		}
		options := make(map[string]string, len(typed))
		for k, v := range typed {
			options[k] = stringOf(v)
		}
		q.Options = options
	case []any:
		levels := make([]string, 0, len(typed))
		for _, item := range typed {
			levels = append(levels, stringOf(item))
		}
		q.Scale = levels
	}

	return nil
}

func stringOf(v any) string {
	s, _ := v.(string)
	return s
}
