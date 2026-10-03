// Command intent-router picks a support intent, and refuses to guess when
// the model is torn between two candidates.
//
// Run it:
//
//	go run ./examples/intent-router -text "how do I rotate my API key"
//
// A classifier that always answers is dangerous: routing a billing
// question to the refunds team because nothing fit better looks like a
// working system while quietly doing the wrong thing. Every intent here
// carries its own probability, so the honest move is to ask again when
// the top two are close.
package main

import (
	"context"
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

// clarifyMargin is the smallest gap between the top two intents that
// still justifies picking one. Below it the bot asks a question.
const (
	clarifyMargin = 0.30

	// minConfidence is the absolute floor: an intent nobody believes is not
	// routed even if it happens to lead.
	minConfidence = 0.50
)

func main() {
	model := flag.String("model", "nimble", "decision model")
	text := flag.String("text", "", "user message; read from stdin when empty")
	flag.Parse()

	state, err := readMessage(*text)
	if err != nil {
		fatal(err)
	}

	client := decide.New(
		decide.WithProvider(ollama.New(ollama.WithDefaultModel(*model))),
		decide.WithDefault(ollama.Name),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	result, err := client.Decide(ctx, decide.Request{
		State: decide.Text(state),
		Questions: []decide.Question{
			decide.Choice("intent", "What is the user trying to do?", decide.Options{
				"billing":   "Charges, invoices, refunds or plans",
				"technical": "An error, an outage or something not working",
				"account":   "Login, password, permissions or profile",
				"how_to":    "Step by step help using a feature",
				"sales":     "Pricing, purchasing or a sales question",
				"feedback":  "Praise, complaints or a feature request",
				"other":     "None of the above",
			}),
		},
	})
	if err != nil {
		fatal(describe(err))
	}

	intent, err := result.Choice("intent")
	if err != nil {
		fatal(fmt.Errorf("intent was not answered: %w", err))
	}

	confidence := intent.Probability(intent.Key)
	ranked := intent.Ranked()

	switch {
	case confidence < minConfidence:
		fmt.Println("route:   clarify")
		fmt.Println("reason:  no intent stood out")
		printRanking(ranked)
	case intent.Margin() < clarifyMargin:
		fmt.Println("route:   clarify")
		fmt.Printf("reason:  %q only just beat %q, %.0f%% vs %.0f%%\n",
			ranked[0].Key, ranked[1].Key,
			ranked[0].Probability*100, ranked[1].Probability*100)
		printRanking(ranked)
		fmt.Printf("\nsuggest: \"Just to check, is this about %s?\"\n", ranked[0].Key)
	default:
		fmt.Printf("route:   %s\n", ranked[0].Key)
		fmt.Printf("reason:  %.0f%%, %.0f%% ahead of the runner-up\n",
			confidence*100, intent.Margin()*100)
	}
}

func printRanking(ranked []decide.Ranked) {
	for i, entry := range ranked {
		if i >= 3 {
			break
		}
		fmt.Printf("    %-12s %6.2f%%\n", entry.Key, entry.Probability*100)
	}
}

// readMessage returns the message from the flag or stdin.
func readMessage(inline string) (string, error) {
	if text := strings.TrimSpace(inline); text != "" {
		return text, nil
	}

	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("read stdin: %w", err)
	}

	text := strings.TrimSpace(string(data))
	if text == "" {
		return "", errors.New("no message: pass -text or pipe it in")
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
	fmt.Fprintln(os.Stderr, "intent-router:", err)
	os.Exit(1)
}
