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

// Answer returns the answer for the named question.
func (r *Result) Answer(name string) (Answer, error) {
	if r == nil {
		return nil, ErrNotFound
	}
	return r.Answers.Get(name)
}

// Choice returns the choice answer for the named question.
func (r *Result) Choice(name string) (ChoiceAnswer, error) {
	answer, err := r.Answer(name)
	if err != nil {
		return ChoiceAnswer{}, err
	}
	choice, ok := answer.(ChoiceAnswer)
	if !ok {
		return ChoiceAnswer{}, &Error{Kind: KindDecode, Message: "decide: question " + name + " is not a choice answer"}
	}
	return choice, nil
}

// Noul returns the noul answer for the named question.
func (r *Result) Noul(name string) (NoulAnswer, error) {
	answer, err := r.Answer(name)
	if err != nil {
		return NoulAnswer{}, err
	}
	noul, ok := answer.(NoulAnswer)
	if !ok {
		return NoulAnswer{}, &Error{Kind: KindDecode, Message: "decide: question " + name + " is not a noul answer"}
	}
	return noul, nil
}

// Score returns the score answer for the named question.
func (r *Result) Score(name string) (ScoreAnswer, error) {
	answer, err := r.Answer(name)
	if err != nil {
		return ScoreAnswer{}, err
	}
	score, ok := answer.(ScoreAnswer)
	if !ok {
		return ScoreAnswer{}, &Error{Kind: KindDecode, Message: "decide: question " + name + " is not a score answer"}
	}
	return score, nil
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
