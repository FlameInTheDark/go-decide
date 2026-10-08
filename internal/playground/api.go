package playground

import (
	"encoding/json"
	"strings"
	"time"

	decide "github.com/FlameInTheDark/go-decide"
)

// maxRequestBytes bounds a request body. The library caps the state at 64 KiB
// for text providers and 32 MiB when images are attached; this is set slightly
// above the text ceiling so an oversized payload is rejected by the library's
// own message rather than by a bare 413 with no explanation.
const maxRequestBytes = 4 << 20

// QuestionInput is one question as sent by the UI. It mirrors the shape of a
// questions file, so a body copied out of examples/triage.json works here.
type QuestionInput struct {
	Name         string      `json:"name"`
	Type         decide.Type `json:"type"`
	Instructions string      `json:"instructions"`
	// Options is keyed by option name. A JSON object has no order of its own,
	// so OptionsList carries the author's order when it matters. Both are
	// accepted; OptionsList wins where they overlap.
	Options     map[string]string `json:"options,omitempty"`
	OptionsList []OptionInput     `json:"options_list,omitempty"`
	Outcomes    *Outcomes         `json:"outcomes,omitempty"`
	Scale       []string          `json:"scale,omitempty"`
}

// OptionInput is one entry of an ordered option list.
type OptionInput struct {
	Key  string `json:"key"`
	Text string `json:"text"`
}

// Outcomes carries the two descriptions of a yes/no question.
type Outcomes struct {
	False string `json:"false"`
	True  string `json:"true"`
}

// DecideRequest is the body of POST /api/decide and POST /api/validate. It is
// deliberately the same shape as the CLI's questions file, so a payload that
// works with `decide --questions` works here unchanged:
//
//	{
//	  "state": "...",
//	  "questions": [ {...} ]
//	}
type DecideRequest struct {
	// Config overrides the server-side provider settings for this call. A
	// zero value uses the configuration the server started with.
	Config ConfigPatch `json:"config,omitempty"`
	// State is the content to evaluate: a string, object or array.
	State json.RawMessage `json:"state"`
	// Questions are the typed questions to answer.
	Questions []QuestionInput `json:"questions"`
	// Model overrides the server-side default model.
	Model string `json:"model,omitempty"`
	// KeepAlive is an Ollama residency hint in seconds. Negative keeps the
	// model loaded indefinitely.
	KeepAlive int `json:"keep_alive,omitempty"`
	// SessionID and User are provider observability fields.
	SessionID string `json:"session_id,omitempty"`
	User      string `json:"user,omitempty"`
	// Trace carries observability metadata such as "trace_id".
	Trace map[string]string `json:"trace,omitempty"`
	// Extra carries provider specific fields merged into the request body.
	Extra map[string]json.RawMessage `json:"extra,omitempty"`
}

// ConfigPatch overrides provider settings for one request. It mirrors the
// fields of the settings form; an empty field keeps the server-side value.
// The API key is only ever sent from the form, never returned by the API.
type ConfigPatch struct {
	Provider string `json:"provider,omitempty"`
	BaseURL  string `json:"base_url,omitempty"`
	APIKey   string `json:"api_key,omitempty"`
	Model    string `json:"model,omitempty"`
	Timeout  int    `json:"timeout,omitempty"`
	Retries  *int   `json:"retries,omitempty"`
}

// toRequest converts the wire body into a library request. It returns every
// structural problem at once, anchored to the part of the payload at fault, so
// the UI can highlight a field and list every other problem in one pass.
func (in DecideRequest) toRequest() (decide.Request, error) {
	questions := make([]decide.Question, 0, len(in.Questions))
	names := make([]string, len(in.Questions))
	orders := make([][]string, len(in.Questions))
	var problems []problem

	state, err := stateFromJSON(in.State)
	if err != nil {
		problems = append(problems, problem{Anchor: prefixState, Message: err.Error()})
	}

	for i, raw := range in.Questions {
		names[i] = raw.Name
		orders[i] = optionOrder(raw)
		question, err := raw.toQuestion()
		if err != nil {
			var validation *decide.ValidationError
			if asValidation(err, &validation) {
				for _, message := range validation.Problems {
					problems = append(problems, problem{
						Anchor:  "questions[" + itoa(i) + "]",
						Message: message,
					})
				}
			} else {
				problems = append(problems, problem{
					Anchor:  "questions[" + itoa(i) + "]",
					Message: err.Error(),
				})
			}
			continue
		}
		questions = append(questions, question)
	}

	req := decide.Request{
		Model:     in.Model,
		State:     state,
		Questions: questions,
		SessionID: in.SessionID,
		User:      in.User,
		Trace:     in.Trace,
		Extra:     in.Extra,
	}
	if in.KeepAlive != 0 {
		req.KeepAlive = time.Duration(in.KeepAlive) * time.Second
	}

	if err := requestProblems(req); err != nil {
		problems = append(problems, splitProblems(err, names)...)
	}

	if len(problems) > 0 {
		return req, &problemsError{Problems: problems}
	}
	return req, nil
}

// optionOrders returns, per question, the option keys in the order the payload
// declared them. It lines up with the questions that survived toRequest, which
// is why callers pass it rather than the raw payload.
func (in DecideRequest) optionOrders() [][]string {
	orders := make([][]string, 0, len(in.Questions))
	for _, question := range in.Questions {
		if question.Type == decide.TypeChoice {
			orders = append(orders, optionOrder(question))
		}
	}
	return orders
}

// requestProblems runs the library's request validation but only reports what
// it adds beyond the questions, which were already validated individually
// above. Without this a single bad question is reported twice, and a bad
// question leaves the request with no questions at all, which produces a
// misleading "at least one question is required".
func requestProblems(req decide.Request) error {
	v := &decide.ValidationError{}

	if req.State.IsZero() {
		v.Add("state must be a non-empty string, object or array")
	}
	if len(req.Questions) == 0 {
		v.Add("at least one question is required")
	}

	seen := make(map[string]struct{}, len(req.Questions))
	for _, question := range req.Questions {
		if name := strings.TrimSpace(question.Name); name != "" {
			if _, dup := seen[name]; dup {
				v.Add("duplicate question name %q", name)
			}
			seen[name] = struct{}{}
		}
	}

	return v.OrNil()
}

// toQuestion converts one wire question, validating it with the library so the
// UI reports the same messages a Go caller would see.
func (in QuestionInput) toQuestion() (decide.Question, error) {
	if in.Name == "" {
		return decide.Question{}, &decide.ValidationError{Problems: []string{"a question needs a name"}}
	}

	v := &decide.ValidationError{}
	question := decide.Question{
		Name:         in.Name,
		Type:         in.Type,
		Instructions: in.Instructions,
	}

	switch in.Type {
	case decide.TypeChoice:
		question.Options = in.options()
	case decide.TypeScore:
		question.Scale = in.Scale
	case decide.TypeNoul:
		if in.Outcomes != nil {
			question.Noul = &decide.NoulCriteria{False: in.Outcomes.False, True: in.Outcomes.True}
		}
	default:
		typed := in.Type
		return decide.Question{}, &decide.ValidationError{
			Problems: []string{"unknown type " + string(typed)},
		}
	}

	question.Validate(v)
	if err := v.OrNil(); err != nil {
		return decide.Question{}, err
	}
	return question, nil
}

// options builds the option map, preferring the ordered list when the payload
// carried one.
func (in QuestionInput) options() map[string]string {
	if len(in.OptionsList) == 0 {
		return in.Options
	}
	options := make(map[string]string, len(in.OptionsList))
	for _, option := range in.OptionsList {
		options[option.Key] = option.Text
	}
	return options
}

// optionOrder returns the keys in the order the request declared them, falling
// back to the sorted order for a payload that only sent an object. The library
// stores options in a map and ranks them, so this is the only place the
// author's order can survive.
func optionOrder(question QuestionInput) []string {
	if len(question.OptionsList) == 0 {
		return sortedKeys(question.Options)
	}
	keys := make([]string, 0, len(question.OptionsList))
	for _, option := range question.OptionsList {
		keys = append(keys, option.Key)
	}
	return keys
}

// stateFromJSON converts the state field into a library state, accepting a
// JSON string, object or array.
func stateFromJSON(raw json.RawMessage) (decide.State, error) {
	if len(raw) == 0 {
		return decide.State{}, &decide.ValidationError{
			Problems: []string{"state must be a string, object or array"},
		}
	}
	var state decide.State
	if err := state.UnmarshalJSON(raw); err != nil {
		return decide.State{}, err
	}
	return state, nil
}

// QuestionView describes a question for the UI: the type, its criteria and
// the display names the result pane needs. It is derived from the request, not
// from the result, so the UI can label bars before a run happens.
type QuestionView struct {
	Name         string       `json:"name"`
	Type         decide.Type  `json:"type"`
	Instructions string       `json:"instructions"`
	Options      []OptionView `json:"options,omitempty"`
	Scale        []string     `json:"scale,omitempty"`
	Outcomes     *Outcomes    `json:"outcomes,omitempty"`
}

// OptionView is one option of a choice question with its display name.
type OptionView struct {
	Key  string `json:"key"`
	Text string `json:"text"`
}

// questionViews converts the request into display metadata. Option order
// follows the sorted key order the library uses, so the UI and the result pane
// always agree.
func questionViews(questions []decide.Question, orders ...[][]string) []QuestionView {
	views := make([]QuestionView, 0, len(questions))
	for i, question := range questions {
		view := QuestionView{
			Name:         question.Name,
			Type:         question.Type,
			Instructions: question.Instructions,
			Scale:        question.Scale,
		}

		switch question.Type {
		case decide.TypeChoice:
			// The library stores options in a map, so the order the author
			// declared is only known from the request payload. Without one,
			// fall back to sorted keys, which is what the CLI shows.
			keys := sortedKeys(question.Options)
			if len(orders) > 0 && i < len(orders[0]) && len(orders[0][i]) > 0 {
				keys = orders[0][i]
			}
			for _, key := range keys {
				if text, ok := question.Options[key]; ok {
					view.Options = append(view.Options, OptionView{Key: key, Text: text})
				}
			}
		case decide.TypeNoul:
			outcomes := &Outcomes{False: "No", True: "Yes"}
			if question.Noul != nil {
				if question.Noul.False != "" {
					outcomes.False = question.Noul.False
				}
				if question.Noul.True != "" {
					outcomes.True = question.Noul.True
				}
			}
			view.Outcomes = outcomes
		}

		views = append(views, view)
	}
	return views
}

// AnswerView is one answer, flattened for display. Every numeric field is
// computed by the library, so the UI cannot disagree with Go semantics.
type AnswerView struct {
	Name     string       `json:"name"`
	Type     decide.Type  `json:"type"`
	Label    string       `json:"label"`
	Question QuestionView `json:"question"`

	// Choice answers.
	Key           string  `json:"key,omitempty"`
	Text          string  `json:"text,omitempty"`
	Probabilities []Bar   `json:"probabilities,omitempty"`
	Confidence    float64 `json:"confidence,omitempty"`
	Margin        float64 `json:"margin,omitempty"`

	// Score answers.
	Score float64 `json:"score,omitempty"`
	Level int     `json:"level,omitempty"`
	Max   int     `json:"max,omitempty"`

	// Noul answers.
	Probability float64 `json:"probability,omitempty"`
	// True is the outcome the model chose. It is always serialized: a noul
	// answer that came back false is the interesting half, and omitempty
	// would drop the key exactly then.
	True       bool   `json:"true"`
	TrueLabel  string `json:"true_label,omitempty"`
	FalseLabel string `json:"false_label,omitempty"`

	// Summary is the library's own one-line rendering of the answer.
	Summary string `json:"summary"`
}

// Bar is one probability row in the result pane.
type Bar struct {
	Key         string  `json:"key"`
	Text        string  `json:"text,omitempty"`
	Probability float64 `json:"probability"`
	Percent     float64 `json:"percent"`
	Best        bool    `json:"best"`
}

// DecideResponse is the body of a successful POST /api/decide.
type DecideResponse struct {
	// Result is the library result, encoded exactly as the CLI's --json
	// output encodes it.
	Result *decide.Result `json:"result"`
	// Raw is the unmodified provider response body.
	Raw json.RawMessage `json:"raw,omitempty"`
	// Answers flattens the result into display rows.
	Answers []AnswerView `json:"answers"`
	// Questions is the display metadata for the request's questions.
	Questions []QuestionView `json:"questions"`
	// Missing lists questions the provider did not answer.
	Missing []string `json:"missing,omitempty"`
	// Prompt is the state's text as it was sent, stored with the run so a
	// stored run can be read back as the content it was decided from. The
	// System One endpoints compose the model's real prompt server-side and
	// never return it, so this is not that prompt: it is the playground's
	// rendering of the request's state.
	Prompt string `json:"prompt,omitempty"`
	// Duration is the wall clock time the decision took.
	DurationMS int64 `json:"duration_ms"`
	// Provider and Model echo what was actually used.
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
}

// ValidateResponse is the body of POST /api/validate.
type ValidateResponse struct {
	// OK reports whether the request is within the provider's limits.
	OK bool `json:"ok"`
	// Problems lists every problem the library reported, one per line.
	Problems []string `json:"problems,omitempty"`
	// Fields lists the payload anchors with problems, e.g. "questions[1]",
	// so the UI can highlight the offending row.
	Fields []string `json:"fields,omitempty"`
	// Questions echoes the questions the UI sent, with their display metadata.
	Questions []QuestionView `json:"questions,omitempty"`
	// Capabilities describes what the selected provider accepts.
	Capabilities CapabilityView `json:"capabilities"`
}

// CapabilityView is the provider's advertised limits, sent to the UI so it can
// warn before a run instead of after one.
type CapabilityView struct {
	Provider      string `json:"provider,omitempty"`
	Images        bool   `json:"images"`
	MaxQuestions  int    `json:"max_questions,omitempty"`
	MaxChoices    int    `json:"max_choices,omitempty"`
	MaxStateBytes int    `json:"max_state_bytes,omitempty"`
	// Known reports whether the provider implements decide.Capable. When
	// false the limits above are unknown rather than unlimited.
	Known bool `json:"known"`
}

// ConfigView is the body of GET /api/config. The API key is never included;
// only whether one is set.
type ConfigView struct {
	Provider  string   `json:"provider"`
	BaseURL   string   `json:"base_url"`
	Model     string   `json:"model,omitempty"`
	Timeout   int      `json:"timeout"`
	Retries   int      `json:"retries"`
	APIKeySet bool     `json:"api_key_set"`
	Providers []string `json:"providers"`
	// DefaultModel is the provider's own suggested model.
	DefaultModel string `json:"default_model,omitempty"`
	// Limits are the library's compile-time ceilings, sent so the UI can
	// warn about an impossible request before it is sent.
	Limits LimitView `json:"limits"`
}

// LimitView reports the library's hard limits.
type LimitView struct {
	MinCriteria   int `json:"min_criteria"`
	MaxCriteria   int `json:"max_criteria"`
	MaxStateBytes int `json:"max_state_bytes"`
	MaxImageBytes int `json:"max_image_bytes"`
}

// ModelsView is the body of GET /api/models.
type ModelsView struct {
	Provider string      `json:"provider,omitempty"`
	BaseURL  string      `json:"base_url,omitempty"`
	Models   []ModelView `json:"models"`
	Version  string      `json:"version,omitempty"`
	Warnings []string    `json:"warnings,omitempty"`
}

// ModelView is one installed model.
type ModelView struct {
	Name     string `json:"name"`
	Decision bool   `json:"decision"`
	Vision   bool   `json:"vision"`
	Family   string `json:"family,omitempty"`
	Size     int64  `json:"size,omitempty"`
	Context  int    `json:"context,omitempty"`
}

// HistoryView is one past run.
type HistoryView struct {
	RequestID  string `json:"id"`
	At         int64  `json:"at"`
	Provider   string `json:"provider,omitempty"`
	Model      string `json:"model,omitempty"`
	OK         bool   `json:"ok"`
	DurationMS int64  `json:"duration_ms"`
	// Body is the exact POST /api/decide body that produced this run, so
	// the UI can restore the request without reconstructing it.
	Body json.RawMessage `json:"body"`
	// Response is the run's outcome, or its error when OK is false.
	Response json.RawMessage `json:"response,omitempty"`
	Error    *ErrorView      `json:"error,omitempty"`
	// Questions is a short label list for the history list.
	Questions []string `json:"questions,omitempty"`
	Summary   string   `json:"summary,omitempty"`
}
