package decide

import "encoding/json"

// Usage reports the token accounting returned by a provider. Cost is only
// populated by providers that meter requests.
type Usage struct {
	// InputTokens is the number of prompt tokens consumed.
	InputTokens int `json:"input_tokens,omitempty"`
	// OutputTokens is the number of generated tokens.
	OutputTokens int `json:"output_tokens,omitempty"`
	// TotalTokens is the combined token count when reported.
	TotalTokens int `json:"total_tokens,omitempty"`
	// Cost is the request cost in USD when reported.
	Cost float64 `json:"cost,omitempty"`
}

// Result is the outcome of a decision request.
type Result struct {
	// Provider is the adapter that produced the result.
	Provider string `json:"provider,omitempty"`
	// Model is the model that answered. Providers that resolve a concrete
	// snapshot report it here.
	Model string `json:"model,omitempty"`
	// UpstreamModel is the resolved model identifier when it differs from
	// the requested one, e.g. "typesafe/jev-1.13-20260917".
	UpstreamModel string `json:"upstream_model,omitempty"`
	// Upstream is the vendor that served the request, when known.
	Upstream string `json:"upstream,omitempty"`
	// ID is the provider generation identifier, when the provider issues one.
	ID string `json:"id,omitempty"`
	// Answers holds one entry per question, in question order.
	Answers Answers `json:"answers"`
	// Usage reports token consumption.
	Usage Usage `json:"usage,omitzero"`
	// Raw is the unmodified provider response body.
	Raw json.RawMessage `json:"-"`
}

// UnmarshalJSON implements [json.Unmarshaler]. It reads the document written
// by [Result.MarshalJSON], so a JSON export can be read back into Go.
func (r *Result) UnmarshalJSON(data []byte) error {
	type alias Result // avoids recursing into this method

	var out alias
	if err := json.Unmarshal(data, &out); err != nil {
		return err
	}
	*r = Result(out)
	return nil
}

// Answer returns the answer for the named question.
func (r *Result) Answer(name string) (Answer, error) {
	if r == nil {
		return nil, ErrNotFound
	}
	return r.Answers.Get(name)
}

// Choice returns the choice answer for the named question. It reports a
// [KindDecode] error when the question was answered with another type.
func (r *Result) Choice(name string) (ChoiceAnswer, error) {
	if r == nil {
		return ChoiceAnswer{}, ErrNotFound
	}
	return r.Answers.Choice(name)
}

// Noul returns the noul answer for the named question. It reports a
// [KindDecode] error when the question was answered with another type.
func (r *Result) Noul(name string) (NoulAnswer, error) {
	if r == nil {
		return NoulAnswer{}, ErrNotFound
	}
	return r.Answers.Noul(name)
}

// Score returns the score answer for the named question. It reports a
// [KindDecode] error when the question was answered with another type.
func (r *Result) Score(name string) (ScoreAnswer, error) {
	if r == nil {
		return ScoreAnswer{}, ErrNotFound
	}
	return r.Answers.Score(name)
}

// Missing lists question names that the provider did not answer.
func (r *Result) Missing(questions []Question) []string {
	var missing []string
	for _, q := range questions {
		if _, err := r.Answers.Get(q.Name); err != nil {
			missing = append(missing, q.Name)
		}
	}
	return missing
}
