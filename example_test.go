package decide_test

import (
	"context"
	"errors"
	"fmt"

	decide "github.com/FlameInTheDark/go-decide"
)

// Example builds a request and reads typed answers from the result.
func Example() {
	// A provider stands in for the real adapters; see providers/ollama and
	// providers/openrouter.
	provider := decide.ProviderFunc{
		ID: "demo",
		Fn: func(context.Context, decide.Request) (*decide.Result, error) {
			return &decide.Result{
				Provider: "demo",
				Model:    "nimble",
				Answers: decide.NewAnswers([]decide.Answer{
					decide.ChoiceAnswer{
						QuestionName:  "label",
						Key:           "bug",
						Probabilities: map[string]float64{"bug": 0.9781, "billing": 0.0125, "account": 0.0094},
						Confidence:    0.8906,
					},
					decide.NoulAnswer{QuestionName: "refund", Probability: 0.016},
					decide.ScoreAnswer{
						QuestionName:  "urgency",
						Score:         1.992,
						Legend:        map[string]string{"0": "Can wait", "1": "Soon", "2": "Immediate"},
						Probabilities: map[string]float64{"0": 0.0004, "1": 0.0072, "2": 0.9924},
						Confidence:    0.9589,
					},
				}),
				Usage: decide.Usage{InputTokens: 862, OutputTokens: 4, TotalTokens: 866},
			}, nil
		},
	}

	result, err := provider.Decide(context.Background(), decide.Request{
		Model: "nimble",
		State: decide.Text("Our checkout has returned 500 errors since 9am."),
		Questions: []decide.Question{
			decide.Choice("label", "Which label fits this ticket?", decide.Options{
				"billing": "Payments and refunds",
				"bug":     "Software errors",
				"account": "Login and account access",
			}),
			decide.Noul("refund", "Is the customer explicitly requesting a refund?"),
			decide.Score("urgency", "How urgent is this ticket?", decide.Scale{
				"Can wait", "Should be fixed this week", "Blocking revenue right now",
			}),
		},
	})
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	label, _ := result.Choice("label")
	fmt.Printf("label=%s (p=%.4f, confidence=%.4f)\n",
		label.Key, label.Probability(label.Key), label.Confidence)

	for _, entry := range label.Ranked() {
		fmt.Printf("  %-8s %.4f\n", entry.Key, entry.Probability)
	}

	refund, _ := result.Noul("refund")
	fmt.Printf("refund=%.4f (%t)\n", refund.Probability, refund.True())

	urgency, _ := result.Score("urgency")
	fmt.Printf("urgency=%.4f level=%d (%s) of %d\n",
		urgency.Score, urgency.Level(), urgency.Description(urgency.Level()), urgency.Max())

	fmt.Printf("usage: %d in / %d out\n", result.Usage.InputTokens, result.Usage.OutputTokens)

	// Output:
	// label=bug (p=0.9781, confidence=0.8906)
	//   bug      0.9781
	//   billing  0.0125
	//   account  0.0094
	// refund=0.0160 (false)
	// urgency=1.9920 level=2 (Immediate) of 2
	// usage: 862 in / 4 out
}

// ExampleProvider_customProvider shows the extension point: any type with a
// name and a Decide method is a provider.
func ExampleProvider_customProvider() {
	// Echo back the request so the example is deterministic.
	echo := decide.ProviderFunc{
		ID: "echo",
		Fn: func(_ context.Context, req decide.Request) (*decide.Result, error) {
			return &decide.Result{
				Provider: "echo",
				Model:    req.Model,
				Answers: decide.NewAnswers([]decide.Answer{
					decide.NoulAnswer{QuestionName: req.Questions[0].Name, Probability: 0.5},
				}),
			}, nil
		},
	}

	client := decide.New(decide.WithProvider(echo), decide.WithDefault("echo"))

	result, err := client.Decide(context.Background(), decide.Request{
		Model:     "any-model",
		State:     decide.Text("hello"),
		Questions: []decide.Question{decide.Noul("ok", "Is everything fine?")},
	})
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	answer, _ := result.Noul("ok")
	fmt.Printf("ok via %s: %.2f\n", result.Provider, answer.Probability)

	// Output:
	// ok via echo: 0.50
}

// ExampleError_classification shows how to branch on provider failures.
func ExampleError_classification() {
	provider := decide.ProviderFunc{
		ID: "flaky",
		Fn: func(context.Context, decide.Request) (*decide.Result, error) {
			return nil, decide.NewHTTPError("flaky", 429, "Rate limit exceeded", "", nil)
		},
	}

	_, err := provider.Decide(context.Background(), decide.Request{})

	switch {
	case errors.Is(err, decide.ErrRateLimited):
		fmt.Println("rate limited, back off and retry")
	case errors.Is(err, decide.ErrInvalidRequest):
		fmt.Println("fix the request")
	default:
		fmt.Println("unexpected:", err)
	}

	var decideErr *decide.Error
	if errors.As(err, &decideErr) {
		fmt.Printf("kind=%s retryable=%t\n", decideErr.Kind, decideErr.Retryable())
	}

	// Output:
	// rate limited, back off and retry
	// kind=rate_limited retryable=true
}

// ExampleRequest_Validate shows that invalid requests fail before any network
// call, listing every problem at once.
func ExampleRequest_Validate() {
	req := decide.Request{
		State: decide.Text("hello"),
		Questions: []decide.Question{
			decide.Choice("label", "Which label?"), // no options
			decide.Score("urgency", "How urgent?"), // no levels
		},
	}

	err := req.Validate()
	if errors.Is(err, decide.ErrInvalidRequest) {
		fmt.Println("rejected:", err)
	}

	// Output:
	// rejected: decide: invalid request: question "label": choice needs between 2 and 26 options, got 0; question "urgency": score needs between 2 and 26 levels, got 0
}

// ExampleState shows the three accepted state shapes.
func ExampleState() {
	text := decide.Text("a plain ticket body")
	object := decide.Object(map[string]any{"ticket": "blank page after Pay"})
	list := decide.List([]any{"first item", "second item"})

	fmt.Println(text)
	fmt.Println(object)
	fmt.Println(list)

	// Anything that marshals to a JSON object or array works too.
	state, err := decide.JSON(map[string]string{"kind": "incident"})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(state)

	// Output:
	// "a plain ticket body"
	// {"ticket":"blank page after Pay"}
	// ["first item","second item"]
	// {"kind":"incident"}
}
