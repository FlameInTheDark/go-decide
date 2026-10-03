package openrouter

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"

	decide "github.com/FlameInTheDark/go-decide"
)

// request is the decisions request body: the shared System One payload plus the
// OpenRouter specific observability and routing fields.
type request struct {
	decide.SystemOnePayload
	// Provider holds routing preferences such as allow_fallbacks.
	Provider json.RawMessage `json:"provider,omitempty"`
	// SessionID groups related requests for observability.
	SessionID string `json:"session_id,omitempty"`
	// Trace carries observability metadata.
	Trace map[string]string `json:"trace,omitempty"`
	// User identifies the end user.
	User string `json:"user,omitempty"`
}

// response is the decisions response body.
type response struct {
	Answers map[string]answer `json:"answers"`
	Model   string            `json:"model"`
	Usage   struct {
		Cost         float64 `json:"cost"`
		InputTokens  int     `json:"input_tokens"`
		OutputTokens int     `json:"output_tokens"`
	} `json:"usage"`
	ID       string `json:"id"`
	Provider string `json:"provider"`
}

// answer is a single typed answer; the shape depends on Type.
type answer struct {
	Type          decide.Type        `json:"type"`
	Choice        string             `json:"choice"`
	Noul          *float64           `json:"noul"`
	Score         *float64           `json:"score"`
	Probabilities map[string]float64 `json:"probabilities"`
	Confidence    *float64           `json:"confidence"`
	Legend        map[string]string  `json:"legend"`
}

// result converts the wire response into a [decide.Result]. The requested model
// is kept in Model while the resolved snapshot lands in UpstreamModel.
func (r *response) result(provider, requestedModel string) (*decide.Result, error) {
	if len(r.Answers) == 0 {
		return nil, decide.NewError(provider, decide.KindDecode, fmt.Errorf("response carries no answers"))
	}

	names := make([]string, 0, len(r.Answers))
	for name := range r.Answers {
		names = append(names, name)
	}
	sort.Strings(names)

	answers := make([]decide.Answer, 0, len(names))
	for _, name := range names {
		converted, err := r.Answers[name].convert(name)
		if err != nil {
			return nil, decide.NewError(provider, decide.KindDecode, err)
		}
		answers = append(answers, converted)
	}

	model := requestedModel
	upstream := r.Model
	if model == "" {
		model = r.Model
	}

	return &decide.Result{
		Provider:      provider,
		Model:         model,
		UpstreamModel: upstream,
		Upstream:      r.Provider,
		ID:            r.ID,
		Answers:       decide.NewAnswers(answers),
		Usage: decide.Usage{
			InputTokens:  r.Usage.InputTokens,
			OutputTokens: r.Usage.OutputTokens,
			TotalTokens:  r.Usage.InputTokens + r.Usage.OutputTokens,
			Cost:         r.Usage.Cost,
		},
	}, nil
}

// convert turns a wire answer into its typed representation.
func (a answer) convert(name string) (decide.Answer, error) {
	switch a.Type {
	case decide.TypeChoice:
		if a.Choice == "" {
			return nil, fmt.Errorf("answer %q: choice is missing", name)
		}
		return decide.ChoiceAnswer{
			QuestionName:  name,
			Key:           a.Choice,
			Probabilities: nonNilProbabilities(a.Probabilities),
			Confidence:    value(a.Confidence),
		}, nil

	case decide.TypeNoul:
		if a.Noul == nil {
			return nil, fmt.Errorf("answer %q: noul probability is missing", name)
		}
		return decide.NoulAnswer{QuestionName: name, Probability: *a.Noul}, nil

	case decide.TypeScore:
		if a.Score == nil {
			return nil, fmt.Errorf("answer %q: score is missing", name)
		}
		return decide.ScoreAnswer{
			QuestionName:  name,
			Score:         *a.Score,
			Legend:        nonNilStringMap(a.Legend),
			Probabilities: nonNilProbabilities(a.Probabilities),
			Confidence:    value(a.Confidence),
		}, nil

	default:
		return nil, fmt.Errorf("answer %q: unknown type %q", name, a.Type)
	}
}

func nonNilProbabilities(p map[string]float64) map[string]float64 {
	if p == nil {
		return map[string]float64{}
	}
	return p
}

func nonNilStringMap(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

func value(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

// errorBody is the documented OpenRouter error envelope.
type errorBody struct {
	Error struct {
		Code     int            `json:"code"`
		Message  string         `json:"message"`
		Metadata map[string]any `json:"metadata,omitempty"`
	} `json:"error"`
}

// decodeError maps the OpenRouter {"error": {"code", "message"}} envelope onto
// a [decide.Error], falling back to the status code when the body is empty or
// unrecognised.
func decodeError(provider string, status int, body []byte, header http.Header) error {
	var envelope errorBody
	if err := json.Unmarshal(body, &envelope); err == nil && envelope.Error.Message != "" {
		errOut := decide.NewHTTPError(provider, status, envelope.Error.Message, string(body), header)
		// OpenRouter sometimes reports a code that differs from the status.
		if envelope.Error.Code != 0 && envelope.Error.Code != status {
			errOut.Kind = decide.ClassifyStatus(envelope.Error.Code)
		}
		return errOut
	}

	message := http.StatusText(status)
	if len(body) > 0 {
		message = string(body)
	}
	return decide.NewHTTPError(provider, status, message, string(body), header)
}
