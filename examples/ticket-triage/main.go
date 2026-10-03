// Command ticket-triage turns a support ticket into a routing decision using a
// local decision model.
//
// Run it:
//
//	go run ./examples/ticket-triage -text "Checkout returns 500 for everyone"
//
// This is the shape most support queues actually need: a few typed questions,
// combined with explicit thresholds, into an operational decision. The
// thresholds are the interesting part. A decision model hands you
// probabilities; what counts as "confident enough to act on alone" is a
// business decision, and it belongs in your code rather than in a prompt.
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

// Thresholds decide when the model is trusted to act without review.
const (
	// autoAssignMargin is the smallest gap between the top two categories
	// that still counts as a clear answer. Below this a human looks at
	// the ticket instead of it being auto-routed.
	autoAssignMargin = 0.35

	// pageOncallScore is the urgency score at which the on-call engineer
	// is paged rather than the ticket simply being queued.
	pageOncallScore = 1.5

	// refundProbability is how sure the model must be before a refund is
	// flagged for the billing team.
	refundProbability = 0.80
)

func main() {
	model := flag.String("model", "nimble", "decision model")
	text := flag.String("text", "", "ticket text; read from stdin when empty")
	asJSON := flag.Bool("json", false, "print the routing decision as JSON")
	flag.Parse()

	state, err := readState(*text)
	if err != nil {
		fatal(err)
	}

	client := decide.New(
		decide.WithProvider(ollama.New(ollama.WithDefaultModel(*model))),
		decide.WithDefault(ollama.Name),
		decide.WithRetry(decide.RetryPolicy{MaxAttempts: 3, BaseDelay: 250 * time.Millisecond}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	result, err := client.Decide(ctx, decide.Request{
		State: decide.Text(state),
		Questions: []decide.Question{
			decide.Choice("category", "Which team owns this ticket?", decide.Options{
				"payments": "Payments, invoices or refunds",
				"product":  "The product is broken or unavailable",
				"account":  "Login, permissions or profile",
			}),
			decide.Noul("refund", "Is the customer explicitly asking for money back?",
				decide.NoulCriteria{
					False: "No refund is requested",
					True:  "A refund is explicitly requested",
				}),
			decide.Score("urgency", "How urgent is this ticket?", decide.Scale{
				"Can wait for the next release",
				"Should be fixed this week",
				"Blocking customers right now",
			}),
		},
	})
	if err != nil {
		fatal(describe(err))
	}

	outcome := route(result)

	if *asJSON {
		fmt.Printf("%+v\n", outcome)
		return
	}

	fmt.Printf("team:        %s\n", outcome.Team)
	fmt.Printf("priority:    %s\n", outcome.Priority)
	fmt.Printf("refund:      %t\n", outcome.NeedsRefund)
	fmt.Printf("auto-route:  %t\n", outcome.AutoAssigned)
	for _, reason := range outcome.Reasons {
		fmt.Printf("  - %s\n", reason)
	}
}

// routing is the operational outcome of a ticket.
type routing struct {
	Team         string
	Priority     string
	NeedsRefund  bool
	AutoAssigned bool
	Reasons      []string
}

// route turns probabilities into an operational decision.
func route(result *decide.Result) routing {
	out := routing{}

	category, err := result.Choice("category")
	if err != nil {
		out.Reasons = append(out.Reasons, "category was not answered")
		out.Team = "unrouted"
		return out
	}

	refund, err := result.Noul("refund")
	if err == nil && refund.Probability >= refundProbability {
		out.NeedsRefund = true
		out.Reasons = append(out.Reasons,
			fmt.Sprintf("refund requested with %.0f%% confidence", refund.Probability*100))
	}

	urgency, err := result.Score("urgency")
	if err == nil {
		switch {
		case urgency.Score >= pageOncallScore:
			out.Priority = "page"
			out.Reasons = append(out.Reasons,
				fmt.Sprintf("urgency %.2f is at or above the page threshold %.2f", urgency.Score, pageOncallScore))
		case urgency.Level() >= 1:
			out.Priority = "high"
		default:
			out.Priority = "normal"
		}
	}

	// Only auto-route when the model is not torn between two categories.
	margin := category.Margin()
	if margin < autoAssignMargin {
		out.Team = category.Key + " (review)"
		out.Reasons = append(out.Reasons,
			fmt.Sprintf("top two categories are only %.0f%% apart, below the %.0f%% auto-route threshold",
				margin*100, autoAssignMargin*100))
		return out
	}

	out.Team = category.Key
	out.AutoAssigned = true
	out.Reasons = append(out.Reasons,
		fmt.Sprintf("routed to %s with %.0f%% confidence", category.Key, category.Probability(category.Key)*100))

	return out
}

// readState returns the ticket text from the flag or stdin.
func readState(inline string) (string, error) {
	if text := strings.TrimSpace(inline); text != "" {
		return text, nil
	}

	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("read stdin: %w", err)
	}

	text := strings.TrimSpace(string(data))
	if text == "" {
		return "", errors.New("no ticket text: pass -text or pipe it in")
	}
	return text, nil
}

// describe turns a library error into an actionable message.
func describe(err error) error {
	switch {
	case errors.Is(err, decide.ErrNotFound):
		return fmt.Errorf("%w: is the decision model installed? try: ollama pull nimble", err)
	case errors.Is(err, decide.ErrRateLimited):
		return fmt.Errorf("%w: the local server is busy, try again shortly", err)
	default:
		return err
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "ticket-triage:", err)
	os.Exit(1)
}
