package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/urfave/cli/v3"

	decide "github.com/FlameInTheDark/go-decide"
)

func classifyCommand() *cli.Command {
	return &cli.Command{
		Name:      "classify",
		Aliases:   []string{"c"},
		Usage:     "Ask the decision model a set of typed questions about some text",
		ArgsUsage: "[text]",
		Description: "Classify text with choice, noul and score questions.\n\n" +
			"The text can be given as an argument, with --state, with --state-file or on stdin.\n\n" +
			"Build a choice question with repeated --option key:description.\n" +
			"Build a yes/no question with --noul name:question, refined by --yes and --no.\n" +
			"Build a score question with --scale name:question and repeated --level values, lowest first.\n\n" +
			"With no question flags at all, a demonstration question set is used instead.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "model",
				Aliases: []string{"m"},
				Usage:   "decision model, e.g. nimble or typesafe/jev-1.13",
			},
			&cli.StringFlag{
				Name:  "state",
				Usage: "the text to evaluate (alternative to the positional argument)",
			},
			&cli.StringFlag{
				Name:  "state-file",
				Usage: "read the state from a file; a .json file is decoded into a structured state",
			},
			&cli.StringFlag{
				Name:    "questions",
				Aliases: []string{"q"},
				Usage:   "load the questions from a JSON file instead of the question flags",
			},
			&cli.StringSliceFlag{
				Name:  "noul",
				Usage: "ask a yes/no question, name:question text (repeatable)",
			},
			&cli.StringFlag{
				Name:  "yes",
				Usage: "description of the true outcome of the yes/no question",
			},
			&cli.StringFlag{
				Name:  "no",
				Usage: "description of the false outcome of the yes/no question",
			},
			&cli.StringSliceFlag{
				Name:  "scale",
				Usage: "ask a score question, name:question text (repeatable, needs --level)",
			},
			&cli.StringSliceFlag{
				Name:    "level",
				Aliases: []string{"l"},
				Usage:   "one level of the score question, lowest first, 2 or more (repeatable, needs --scale)",
			},
			&cli.StringSliceFlag{
				Name:    "option",
				Aliases: []string{"o"},
				Usage:   "option for the choice question, key:description (repeatable, needs 2 or more)",
			},
			&cli.StringFlag{
				Name:  "choice-name",
				Value: "label",
				Usage: "name of the question built from --option",
			},
			&cli.StringFlag{
				Name:  "choice-instructions",
				Value: "Which label fits this text?",
				Usage: "instructions for the question built from --option",
			},
			&cli.StringSliceFlag{
				Name:  "image",
				Usage: "attach an image file, needs a vision decision model (repeatable)",
			},
			&cli.StringFlag{
				Name:  "keep-alive",
				Usage: "how long Ollama keeps the model loaded, e.g. 5m",
			},
			&cli.StringFlag{
				Name:  "session-id",
				Usage: "group this request with others in provider observability",
			},
			&cli.BoolFlag{
				Name:    "json",
				Aliases: []string{"j"},
				Usage:   "print the result as JSON",
			},
			&cli.BoolFlag{
				Name:  "no-legend",
				Usage: "hide probability bars",
			},
			&cli.BoolFlag{
				Name:  "plain",
				Usage: "disable colour and styling",
			},
			&cli.BoolFlag{
				Name:  "color",
				Usage: "force colour even when not writing to a terminal",
			},
		},
		Action: runClassify,
	}
}

func runClassify(ctx context.Context, cmd *cli.Command) error {
	cfg := resolveSettings(cmd)

	file := cmd.String("state-file")
	state, err := cfg.loadState(cmd, cmd.String("state"), file)
	if err != nil {
		return err
	}

	questions, err := resolveQuestions(cmd)
	if err != nil {
		return err
	}

	req := decide.Request{
		Model:     cmd.String("model"),
		State:     state,
		Questions: questions,
		SessionID: cmd.String("session-id"),
		KeepAlive: cmd.String("keep-alive"),
	}

	for _, path := range cmd.StringSlice("image") {
		image, err := decide.ImageFromFile(path)
		if err != nil {
			return usageError("%v", err)
		}
		req.Images = append(req.Images, image)
	}

	reqCtx, cancel := cfg.context(ctx)
	defer cancel()

	result, err := cfg.client().Decide(reqCtx, req)
	if err != nil {
		return exitError(err)
	}

	if cmd.Bool("json") {
		return writeJSON(cfg.stdout, buildReport(req, result))
	}

	out := newPrinter(cfg.stdout, cmd.Bool("color"), cmd.Bool("plain"))
	out.print(result, req, cmd.Bool("no-legend"))
	return nil
}

// resolveQuestions loads the questions either from --questions or from the
// inline question flags. The two sources are mutually exclusive so a file can
// never be silently half overridden by stray flags.
func resolveQuestions(cmd *cli.Command) ([]decide.Question, error) {
	flags := questionFlags{
		noul:               cmd.StringSlice("noul"),
		yes:                cmd.String("yes"),
		no:                 cmd.String("no"),
		scale:              cmd.StringSlice("scale"),
		level:              cmd.StringSlice("level"),
		option:             cmd.StringSlice("option"),
		choiceName:         cmd.String("choice-name"),
		choiceInstructions: cmd.String("choice-instructions"),
	}

	path := cmd.String("questions")
	if path == "" {
		return buildQuestions(flags)
	}

	if inline := inlineQuestionFlags(flags); len(inline) > 0 {
		return nil, usageError("--questions cannot be combined with %s, the file already defines the questions", inline)
	}

	return loadQuestionsFile(path)
}

// inlineQuestionFlags lists the question flags the caller actually set, so the
// conflict message can name them.
func inlineQuestionFlags(qf questionFlags) []string {
	var set []string
	if len(qf.noul) > 0 {
		set = append(set, "--noul")
	}
	if qf.yes != "" {
		set = append(set, "--yes")
	}
	if qf.no != "" {
		set = append(set, "--no")
	}
	if len(qf.scale) > 0 {
		set = append(set, "--scale")
	}
	if len(qf.level) > 0 {
		set = append(set, "--level")
	}
	if len(qf.option) > 0 {
		set = append(set, "--option")
	}
	return set
}

// questionFlags holds the parsed question-related flags.
type questionFlags struct {
	noul               []string
	yes                string
	no                 string
	scale              []string
	level              []string
	option             []string
	choiceName         string
	choiceInstructions string
}

// buildQuestions turns the question flags into request questions.
//
// Every part of a question must come from the command line: there are no
// implicit criteria. In particular --scale requires --level, because silently
// substituting a built-in rubric would answer a question the caller never
// asked. When no question flags are given at all, the demonstration set is used
// and reported as such.
func buildQuestions(qf questionFlags) ([]decide.Question, error) {
	var questions []decide.Question

	if len(qf.level) > 0 && len(qf.scale) == 0 {
		return nil, usageError("--level needs a question to apply to, add --scale name:question")
	}

	if len(qf.option) > 0 {
		options := make(decide.Options, len(qf.option))
		for _, raw := range qf.option {
			key, description, err := splitPair(raw)
			if err != nil {
				return nil, usageError("--option %q: %v", raw, err)
			}
			if _, duplicate := options[key]; duplicate {
				return nil, usageError("--option %q: duplicate option key %q", raw, key)
			}
			options[key] = description
		}
		questions = append(questions, decide.Choice(qf.choiceName, qf.choiceInstructions, options))
	}

	if (qf.yes != "" || qf.no != "") && len(qf.noul) == 0 {
		return nil, usageError("--yes and --no need a yes/no question, add --noul name:question")
	}
	if (qf.yes != "" || qf.no != "") && len(qf.noul) > 1 {
		return nil, usageError("--yes and --no apply to a single question, but %d --noul questions were given", len(qf.noul))
	}

	for _, raw := range qf.noul {
		name, text, err := splitPair(raw)
		if err != nil {
			return nil, usageError("--noul %q: %v", raw, err)
		}
		question := decide.Noul(name, text)
		if qf.yes != "" || qf.no != "" {
			question = question.WithOutcomes(qf.no, qf.yes)
		}
		questions = append(questions, question)
	}

	if len(qf.scale) > 0 {
		if len(qf.level) < decide.MinCriteria {
			return nil, usageError("--scale needs at least %d --level values, got %d", decide.MinCriteria, len(qf.level))
		}
		levels := make(decide.Scale, len(qf.level))
		copy(levels, qf.level)

		for _, raw := range qf.scale {
			name, text, err := splitPair(raw)
			if err != nil {
				return nil, usageError("--scale %q: %v", raw, err)
			}
			questions = append(questions, decide.Score(name, text, levels))
		}
	}

	if len(questions) > 0 {
		return questions, nil
	}
	return demoQuestions(), nil
}

// demoScale is the rubric used only by the demonstration question set.
var demoScale = decide.Scale{
	"Can wait for the next release",
	"Should be fixed this week",
	"Blocking revenue right now",
}

// demoQuestions exercises every supported question type. It is used when the
// caller supplies no question flags at all.
func demoQuestions() []decide.Question {
	return []decide.Question{
		decide.Choice("label", "Which label fits this text?", decide.Options{
			"billing": "Payments, invoices or refunds",
			"bug":     "Software errors or outages",
			"account": "Login, permissions or profile issues",
		}),
		decide.Noul("is_urgent", "Does this need immediate attention?"),
		decide.Score("urgency", "How urgent is this?", demoScale),
	}
}

// splitPair parses a "name:value" flag value, requiring a non-empty name and a
// non-empty remainder. The remainder may itself contain colons.
func splitPair(raw string) (name, value string, err error) {
	name, value, found := strings.Cut(raw, ":")
	name = strings.TrimSpace(name)
	value = strings.TrimSpace(value)

	switch {
	case !found:
		return "", "", fmt.Errorf("expected `name:value`")
	case name == "":
		return "", "", fmt.Errorf("the name before the colon must not be empty")
	case value == "":
		return "", "", fmt.Errorf("the value after the colon must not be empty")
	}
	return name, value, nil
}
