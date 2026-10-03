package ollama

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"

	decide "github.com/FlameInTheDark/go-decide"
)

// response is the body of a successful System One call.
type response struct {
	Model   string            `json:"model"`
	Answers map[string]answer `json:"answers"`
	Usage   struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// answer is a single typed answer. Every field is optional because the shape
// depends on Type.
type answer struct {
	Type          decide.Type        `json:"type"`
	Choice        string             `json:"choice"`
	Noul          *float64           `json:"noul"`
	Score         *float64           `json:"score"`
	Probabilities map[string]float64 `json:"probabilities"`
	Confidence    *float64           `json:"confidence"`
	Legend        map[string]string  `json:"legend"`
}

// result converts the wire response into a [decide.Result], keeping answers in
// a stable, name-sorted order.
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

	model := r.Model
	if model == "" {
		model = requestedModel
	}

	return &decide.Result{
		Provider: provider,
		Model:    model,
		Answers:  decide.NewAnswers(answers),
		Usage: decide.Usage{
			InputTokens:  r.Usage.InputTokens,
			OutputTokens: r.Usage.OutputTokens,
			TotalTokens:  r.Usage.InputTokens + r.Usage.OutputTokens,
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

// decodeError maps an Ollama error body, which is always {"error": "..."},
// onto a [decide.Error].
func decodeError(provider string, status int, body []byte, header http.Header) error {
	var envelope struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil || envelope.Error == "" {
		message := http.StatusText(status)
		if err == nil && len(body) > 0 {
			message = string(body)
		}
		return decide.NewHTTPError(provider, status, message, string(body), header)
	}
	return decide.NewHTTPError(provider, status, envelope.Error, string(body), header)
}
