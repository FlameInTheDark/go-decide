// Command autotag derives a set of tags from structured content using a
// decision model.
//
// Run it:
//
//	go run ./examples/autotag -file examples/ticket.json
//
// Tags are independent binary questions rather than one multi-label question.
// Asking "is this a bug report?" separately from "is this a how-to?" gives each
// tag its own probability, which is what you need to place a threshold per tag
// and to explain why a tag was applied.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	decide "github.com/FlameInTheDark/go-decide"
	"github.com/FlameInTheDark/go-decide/providers/ollama"
)

// thresholds are per tag, because a noisy tag should not drag every other tag
// with it.
var thresholds = map[string]float64{
	"bug":             0.60,
	"how_to":          0.60,
	"question":        0.50,
	"feature_request": 0.55,
	"sales":           0.70,
	"needs_reply":     0.50,
}

// tag is one independent classification.
type tag struct {
	name      string
	question  string
	threshold float64
}

// tags are asked in one request.
var tags = []tag{
	{"bug", "Does this describe a defect or something not working as intended?", thresholds["bug"]},
	{"how_to", "Does this ask for step by step instructions to use a feature?", thresholds["how_to"]},
	{"question", "Is this asking for information or an explanation?", thresholds["question"]},
	{"feature_request", "Is this asking for a feature that does not exist yet?", thresholds["feature_request"]},
	{"sales", "Is this about pricing, purchasing or a commercial enquiry?", thresholds["sales"]},
	{"needs_reply", "Does this expect a human response?", thresholds["needs_reply"]},
}

func main() {
	model := flag.String("model", "nimble", "decision model")
	file := flag.String("file", "", "JSON file to tag; read from stdin when empty")
	flag.Parse()

	state, err := readContent(*file)
	if err != nil {
		fatal(err)
	}

	questions := make([]decide.Question, 0, len(tags))
	for _, t := range tags {
		questions = append(questions, decide.Noul(t.name, t.question,
			decide.NoulCriteria{False: "No", True: "Yes"}))
	}

	client := decide.New(
		decide.WithProvider(ollama.New(ollama.WithDefaultModel(*model))),
		decide.WithDefault(ollama.Name),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// A JSON file keeps its structure, so the model sees the whole object
	// rather than a flattened string.
	result, err := client.Decide(ctx, decide.Request{
		State:     state,
		Questions: questions,
	})
	if err != nil {
		fatal(describe(err))
	}

	applied, rejected := classify(result)

	fmt.Printf("applied (%d):\n", len(applied))
	for _, item := range applied {
		fmt.Printf("  %-16s %.0f%%\n", item.name, item.probability*100)
	}
	if len(applied) == 0 {
		fmt.Println("  (none cleared the threshold)")
	}

	fmt.Printf("\nbelow threshold (%d):\n", len(rejected))
	for _, item := range rejected {
		fmt.Printf("  %-16s %.0f%%  (needed %.0f%%)\n",
			item.name, item.probability*100, item.threshold*100)
	}
}

// scored is a tag with its probability.
type scored struct {
	name        string
	probability float64
	threshold   float64
}

// classify splits tags into applied and rejected.
func classify(result *decide.Result) (applied, rejected []scored) {
	for _, t := range tags {
		answer, err := result.Noul(t.name)
		if err != nil {
			continue
		}

		item := scored{name: t.name, probability: answer.Probability, threshold: t.threshold}
		if answer.Probability >= t.threshold {
			applied = append(applied, item)
			continue
		}
		rejected = append(rejected, item)
	}

	sort.Slice(applied, func(i, j int) bool {
		return applied[i].probability > applied[j].probability
	})

	return applied, rejected
}

// readContent loads the content to tag. A .json file is kept as a structured
// state so the model receives the object rather than raw JSON text.
func readContent(file string) (decide.State, error) {
	if strings.TrimSpace(file) == "" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return decide.State{}, fmt.Errorf("read stdin: %w", err)
		}
		text := strings.TrimSpace(string(data))
		if text == "" {
			return decide.State{}, errors.New("no content: pass -file or pipe it in")
		}
		return decide.Text(text), nil
	}

	data, err := os.ReadFile(file)
	if err != nil {
		return decide.State{}, fmt.Errorf("read %s: %w", file, err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return decide.State{}, fmt.Errorf("%s is empty", file)
	}

	if strings.EqualFold(filepath.Ext(file), ".json") {
		decoded, err := loadJSON(data)
		if err != nil {
			return decide.State{}, fmt.Errorf("parse %s: not valid JSON: %w", file, err)
		}
		state, err := decide.JSON(decoded)
		if err != nil {
			return decide.State{}, fmt.Errorf("parse %s: %w", file, err)
		}
		return state, nil
	}

	return decide.Text(string(data)), nil
}

// loadJSON parses data into an arbitrary value. It reports a syntax error
// instead of returning the raw text, so a malformed .json file fails with a
// clear message rather than being silently tagged as prose.
func loadJSON(data []byte) (any, error) {
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func describe(err error) error {
	if errors.Is(err, decide.ErrNotFound) {
		return fmt.Errorf("%w: is the decision model installed? try: ollama pull nimble", err)
	}
	return err
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "autotag:", err)
	os.Exit(1)
}
