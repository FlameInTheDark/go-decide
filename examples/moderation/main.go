// Command moderation applies a content policy using a decision model.
// Run it:
//
//	echo "buy cheap followers now" | go run ./examples/moderation
//
// Each policy check is its own question with its own threshold. Different harms
// deserve different sensitivity, and a single blended judgement makes that
// impossible to tune. Keeping the checks separate means you can change the
// thresholds without changing the model or re-evaluating your policy.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	decide "github.com/FlameInTheDark/go-decide"
	"github.com/FlameInTheDark/go-decide/providers/ollama"
)

// Per-check thresholds. Block early on serious harm, escalate on anything
// ambiguous, and be permissive about the rest.
const (
	blockThreats    = 0.75 // credible threats of violence
	blockSelfHarm   = 0.85 // self-harm content is routed to trained staff
	blockHarassment = 0.90 // harassment: high volume, low severity
	blockSpam       = 0.95 // spam: highest volume, lowest cost of a false positive
	escalateAny     = 0.50 // anything above this is worth a human look
)

// check is one policy rule.
type check struct {
	name      string
	question  string
	threshold float64
}

// checks are asked together in a single request, so one model load answers all
// of them.
var checks = []check{
	{"threats", "Does this text contain a threat of violence?", blockThreats},
	{"self-harm", "Does this text describe or encourage self-harm?", blockSelfHarm},
	{"harassment", "Is this text targeting or harassing a person or group?", blockHarassment},
	{"spam", "Is this text unsolicited promotion or a scam?", blockSpam},
}

// verdict is the outcome of a moderation pass.
type verdict struct {
	Action   string             `json:"action"`
	Checks   map[string]float64 `json:"checks"`
	Blocking []string           `json:"blocking,omitempty"`
	Flagged  []string           `json:"flagged,omitempty"`
}

func main() {
	model := flag.String("model", "nimble", "decision model")
	text := flag.String("text", "", "content to review; read from stdin when empty")
	asJSON := flag.Bool("json", false, "print the verdict as JSON")
	flag.Parse()

	state, err := readContent(*text)
	if err != nil {
		fatal(err)
	}

	questions := make([]decide.Question, 0, len(checks))
	for _, c := range checks {
		questions = append(questions, decide.Noul(c.name, c.question,
			decide.NoulCriteria{False: "No", True: "Yes"}))
	}

	client := decide.New(
		decide.WithProvider(ollama.New(ollama.WithDefaultModel(*model))),
		decide.WithDefault(ollama.Name),
		decide.WithRetry(decide.RetryPolicy{MaxAttempts: 2, BaseDelay: 250 * time.Millisecond}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	result, err := client.Decide(ctx, decide.Request{
		State:     decide.Text(state),
		Questions: questions,
	})
	if err != nil {
		fatal(describe(err))
	}

	outcome := review(result)

	if *asJSON {
		printJSON(outcome)
		return
	}

	fmt.Printf("action: %s\n", outcome.Action)
	for _, name := range orderedNames(outcome.Checks) {
		probability := outcome.Checks[name]

		marker := "ok"
		switch {
		case isBlocking(outcome, name):
			marker = "BLOCK"
		case probability >= escalateAny:
			marker = "review"
		}

		fmt.Printf("  %-10s %5.1f%%  %s\n", name, probability*100, marker)
	}
}

func printJSON(v verdict) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(v); err != nil {
		fatal(err)
	}
}

// review applies the thresholds to the model probabilities.
func review(result *decide.Result) verdict {
	out := verdict{
		Action: "allow",
		Checks: map[string]float64{},
	}

	for _, c := range checks {
		answer, err := result.Noul(c.name)
		if err != nil {
			// A missing answer is not evidence of safety, so it escalates.
			out.Flagged = append(out.Flagged, c.name+" (unanswered)")
			out.Action = "escalate"
			continue
		}

		out.Checks[c.name] = answer.Probability

		if answer.Probability >= c.threshold {
			out.Blocking = append(out.Blocking, c.name)
			out.Action = "block"
		} else if answer.Probability >= escalateAny {
			out.Flagged = append(out.Flagged, c.name)
			if out.Action != "block" {
				out.Action = "escalate"
			}
		}
	}

	return out
}

// isBlocking reports whether a named check is currently blocking.
func isBlocking(v verdict, name string) bool {
	for _, blocked := range v.Blocking {
		if blocked == name {
			return true
		}
	}
	return false
}

// orderedNames returns the check names in their declared order.
func orderedNames(values map[string]float64) []string {
	names := make([]string, 0, len(values))
	for _, c := range checks {
		if _, ok := values[c.name]; ok {
			names = append(names, c.name)
		}
	}
	return names
}

// readContent returns the text to review from the flag or stdin.
func readContent(inline string) (string, error) {
	if text := strings.TrimSpace(inline); text != "" {
		return text, nil
	}

	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("read stdin: %w", err)
	}

	text := strings.TrimSpace(string(data))
	if text == "" {
		return "", errors.New("no content: pass -text or pipe it in")
	}
	return text, nil
}

func describe(err error) error {
	if errors.Is(err, decide.ErrNotFound) {
		return fmt.Errorf("%w: is the decision model installed? try: ollama pull nimble", err)
	}
	return err
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "moderation:", err)
	os.Exit(1)
}
