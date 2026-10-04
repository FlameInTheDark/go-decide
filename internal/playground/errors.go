package playground

import (
	"sort"

	decide "github.com/FlameInTheDark/go-decide"
	"github.com/FlameInTheDark/go-decide/internal/cliconfig"
)

// ErrorView is the body of every failed /api response. It carries the
// library's own classification so the UI can branch on the failure kind
// instead of matching on message text.
type ErrorView struct {
	// Message is the full library message, including the "decide: " prefix.
	Message string `json:"error"`
	// Kind is the decide.Kind name, e.g. "invalid_request", "rate_limited".
	// It is empty when the failure was not classified.
	Kind string `json:"kind,omitempty"`
	// Problems lists each validation problem separately, so the UI can show
	// one line per problem instead of one long message.
	Problems []string `json:"problems,omitempty"`
	// Fields lists the payload anchors with problems, e.g. "questions[1]", so
	// a form can highlight the offending field.
	Fields []string `json:"fields,omitempty"`
	// Provider is the adapter that failed, when known.
	Provider string `json:"provider,omitempty"`
	// StatusCode is the HTTP status the provider returned, or 0. The wire key
	// is "status" because the HTTP reply itself already carries a status; the
	// field is kept as StatusCode because that is what it holds.
	StatusCode int `json:"status,omitempty"`
	// RetryAfter is the delay the provider asked for, in seconds.
	RetryAfter float64 `json:"retry_after,omitempty"`
	// Retryable reports whether retrying could succeed.
	Retryable bool `json:"retryable,omitempty"`
}

// Status reports the HTTP status the API replies with. It mirrors the
// provider's own classification, so a rate-limited upstream is 429 here rather
// than a blanket 500.
func (e *ErrorView) Status() int {
	switch e.Kind {
	case "invalid_request", "unsupported":
		return 400
	case "authentication", "forbidden":
		return 401
	case "payment":
		return 402
	case "not_found":
		return 404
	case "payload_too_large":
		return 413
	case "rate_limited":
		return 429
	case "timeout":
		return 504
	case "server_error":
		return 502
	case "unavailable":
		return 503
	default:
		return 500
	}
}

// newErrorView classifies err with the library's error types.
func newErrorView(err error) *ErrorView {
	if err == nil {
		return nil
	}
	view := &ErrorView{Message: err.Error()}

	// A payload conversion failure knows which fields it is complaining about;
	// a library validation failure carries the library's own problem list.
	if typed, ok := err.(*problemsError); ok {
		view.Kind = decide.KindInvalidRequest.String()
		view.Fields = typed.Fields()
		view.Problems = make([]string, 0, len(typed.Problems))
		for _, p := range typed.Problems {
			view.Problems = append(view.Problems, p.String())
		}
		view.Message = typed.Error()
		return view
	}

	detail := cliconfig.Describe(err)
	if detail == nil {
		return view
	}
	view.Message = detail.Message
	view.Kind = detail.Kind
	view.Problems = detail.Problems
	view.Provider = detail.Provider
	view.StatusCode = detail.StatusCode
	view.RetryAfter = detail.RetryAfter.Seconds()
	view.Retryable = detail.Retryable
	return view
}

// errorResponse is the envelope for a failed request.
type errorResponse struct {
	Error *ErrorView `json:"error"`
}

// asValidation reports whether err is a *decide.ValidationError and assigns it.
func asValidation(err error, target **decide.ValidationError) bool {
	v, ok := err.(*decide.ValidationError)
	if ok {
		*target = v
	}
	return ok
}

// sortedKeys returns the option keys in sorted order, matching the order the
// library uses when it ranks options.
func sortedKeys(options map[string]string) []string {
	keys := make([]string, 0, len(options))
	for key := range options {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
