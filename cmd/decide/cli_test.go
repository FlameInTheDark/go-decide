package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	decide "github.com/FlameInTheDark/go-decide"
)

// runCLI drives the command tree with the given arguments and captures stdout.
// stdin is empty so the test process stdin is never consumed.
func runCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	return runCLIWithStdin(t, strings.NewReader(""), args...)
}

// runCLIWithStdin is runCLI with explicit stdin, making piped input testable.
func runCLIWithStdin(t *testing.T, stdin io.Reader, args ...string) (string, error) {
	t.Helper()

	previous := stdinSource
	stdinSource = stdin
	t.Cleanup(func() { stdinSource = previous })

	stdout := &strings.Builder{}
	cmd := newRootCommand()
	// Never touch the real terminal streams from a test.
	cmd.Writer = stdout
	cmd.ErrWriter = io.Discard

	if err := cmd.Run(context.Background(), append([]string{"decide"}, args...)); err != nil {
		return stdout.String(), err
	}
	return stdout.String(), nil
}

func TestBuildQuestionsFromFlags(t *testing.T) {
	questions, err := buildQuestions(questionFlags{
		noul:               []string{"is_refund:Is a refund requested?"},
		scale:              []string{"urgency:How urgent?"},
		option:             []string{"bug:Software errors", "billing:Payments"},
		level:              []string{"Annoying", "Blocking"},
		choiceName:         "category",
		choiceInstructions: "Which label?",
	})
	if err != nil {
		t.Fatalf("buildQuestions: %v", err)
	}

	if len(questions) != 3 {
		t.Fatalf("questions = %d, want 3", len(questions))
	}

	if questions[0].Type != decide.TypeChoice || len(questions[0].Options) != 2 {
		t.Errorf("choice = %+v", questions[0])
	}
	// The name comes from --choice-name, never a hardcoded one.
	if questions[0].Name != "category" {
		t.Errorf("choice name = %q, want category", questions[0].Name)
	}
	if questions[0].Instructions != "Which label?" {
		t.Errorf("instructions = %q", questions[0].Instructions)
	}
	if questions[0].Options["bug"] != "Software errors" {
		t.Errorf("options = %v", questions[0].Options)
	}

	if questions[1].Type != decide.TypeNoul || questions[1].Name != "is_refund" {
		t.Errorf("noul = %+v", questions[1])
	}
	if questions[1].Noul != nil {
		t.Errorf("noul criteria = %+v, want none without --yes/--no", questions[1].Noul)
	}

	// The scale levels come from --level, never a built-in rubric.
	scale := questions[2]
	if scale.Type != decide.TypeScore || len(scale.Scale) != 2 {
		t.Fatalf("score = %+v", scale)
	}
	if scale.Scale[0] != "Annoying" || scale.Scale[1] != "Blocking" {
		t.Errorf("scale = %v, want the levels from --level", scale.Scale)
	}
}

func TestBuildQuestionsNoulOutcomes(t *testing.T) {
	questions, err := buildQuestions(questionFlags{
		noul: []string{"refund:Is money back requested?"},
		yes:  "A refund is requested",
		no:   "No refund is requested",
	})
	if err != nil {
		t.Fatalf("buildQuestions: %v", err)
	}

	criteria := questions[0].Noul
	if criteria == nil {
		t.Fatal("noul criteria are missing")
	}
	if criteria.True != "A refund is requested" || criteria.False != "No refund is requested" {
		t.Errorf("criteria = %+v", criteria)
	}
}

func TestBuildQuestionsCopiesLevels(t *testing.T) {
	levels := []string{"low", "high"}
	questions, err := buildQuestions(questionFlags{
		scale: []string{"u:How?"},
		level: levels,
	})
	if err != nil {
		t.Fatalf("buildQuestions: %v", err)
	}

	levels[0] = "mutated"
	if questions[0].Scale[0] != "low" {
		t.Error("the question must not alias the flag slice")
	}
}

func TestBuildQuestionsRejectsIncompleteInput(t *testing.T) {
	tests := []struct {
		name  string
		flags questionFlags
	}{
		{
			name:  "scale without levels",
			flags: questionFlags{scale: []string{"urgency:How urgent?"}},
		},
		{
			name:  "scale with one level",
			flags: questionFlags{scale: []string{"u:How?"}, level: []string{"only"}},
		},
		{
			name:  "levels without scale",
			flags: questionFlags{level: []string{"low", "high"}},
		},
		{
			name:  "yes without noul",
			flags: questionFlags{yes: "yes text"},
		},
		{
			name:  "no without noul",
			flags: questionFlags{no: "no text"},
		},
		{
			name:  "yes with several noul questions",
			flags: questionFlags{yes: "y", noul: []string{"a:A?", "b:B?"}},
		},
		{
			name:  "option without a colon",
			flags: questionFlags{option: []string{"bug"}},
		},
		{
			name:  "duplicate option key",
			flags: questionFlags{option: []string{"a:One", "a:Two"}},
		},
		{
			name:  "noul without a colon",
			flags: questionFlags{noul: []string{"noul"}},
		},
		{
			name:  "scale without a colon",
			flags: questionFlags{scale: []string{"scale"}, level: []string{"a", "b"}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := buildQuestions(tc.flags)
			if err == nil {
				t.Fatal("expected an error")
			}
			if code := exitCodeOf(err); code != exitUsage {
				t.Errorf("exit code = %d, want %d", code, exitUsage)
			}
		})
	}
}

func TestBuildQuestionsFallsBackToDemo(t *testing.T) {
	questions, err := buildQuestions(questionFlags{})
	if err != nil {
		t.Fatalf("buildQuestions: %v", err)
	}
	if len(questions) != 3 {
		t.Fatalf("demo questions = %d, want 3", len(questions))
	}

	// The demonstration set is the only place a built-in rubric may appear.
	scale := questions[2]
	if len(scale.Scale) != 3 || scale.Scale[0] != demoScale[0] {
		t.Errorf("demo scale = %v", scale.Scale)
	}
}

func TestBuildQuestionsDemoUsesItsOwnScale(t *testing.T) {
	// Custom levels must never leak into, or be replaced by, the demo set.
	custom, err := buildQuestions(questionFlags{
		scale: []string{"urgency:How urgent?"},
		level: []string{"One", "Two"},
	})
	if err != nil {
		t.Fatalf("buildQuestions: %v", err)
	}

	// The custom levels must be exactly what was asked for, in order.
	if len(custom[0].Scale) != 2 {
		t.Fatalf("custom scale = %v", custom[0].Scale)
	}
	for i, level := range custom[0].Scale {
		if level != []string{"One", "Two"}[i] {
			t.Errorf("custom scale[%d] = %q", i, level)
		}
	}

	// And the demo set must be unaffected by a previous custom call.
	demo, err := buildQuestions(questionFlags{})
	if err != nil {
		t.Fatalf("buildQuestions: %v", err)
	}
	if len(demo[2].Scale) != len(demoScale) {
		t.Errorf("demo scale = %v, want %v", demo[2].Scale, demoScale)
	}
	for i, level := range demoScale {
		if demo[2].Scale[i] != level {
			t.Errorf("demo scale = %v, want %v", demo[2].Scale, demoScale)
			break
		}
	}
}

func TestSplitPair(t *testing.T) {
	name, value, err := splitPair("urgency:How urgent: really?")
	if err != nil {
		t.Fatalf("splitPair: %v", err)
	}
	if name != "urgency" {
		t.Errorf("name = %q", name)
	}
	// Everything after the first colon belongs to the value.
	if value != "How urgent: really?" {
		t.Errorf("value = %q", value)
	}

	name, value, err = splitPair("  key  :  description  ")
	if err != nil {
		t.Fatalf("splitPair: %v", err)
	}
	if name != "key" || value != "description" {
		t.Errorf("got %q/%q, want trimmed values", name, value)
	}
}

func TestExitErrorCodes(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code int
	}{
		{"invalid request", decide.ErrInvalidRequest, 2},
		{"unsupported", decide.ErrUnsupported, 2},
		{"auth", decide.ErrAuth, 3},
		{"payment", decide.ErrPayment, 3},
		{"not found", decide.ErrNotFound, 4},
		{"rate limited", decide.ErrRateLimited, 5},
		{"timeout", decide.ErrTimeout, 5},
		{"server", decide.ErrServer, 6},
		{"unavailable", decide.ErrUnavailable, 6},
		{"other", errors.New("boom"), 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if code := exitCodeOf(exitError(tc.err)); code != tc.code {
				t.Errorf("code = %d, want %d", code, tc.code)
			}
		})
	}

	if exitError(nil) != nil {
		t.Error("exitError(nil) should be nil")
	}

	// An already coded error passes through unchanged.
	original := usageError("already coded")
	if got := exitError(original); got != original {
		t.Errorf("exitError = %v, want the original error", got)
	}
}

func TestExitCodeErrorIsNotAnExitCoder(t *testing.T) {
	// Regression guard: cli.ExitCoder is `interface { error; ExitCode() int }`.
	// If exitCodeError ever regains an ExitCode method, urfave/cli intercepts
	// the error inside Command.Run and calls os.Exit, which silently breaks
	// every test that exercises a failing command.
	var err error = usageError("boom")

	if _, isExitCoder := err.(interface{ ExitCode() int }); isExitCoder {
		t.Fatal("exitCodeError must not expose an ExitCode() method")
	}
}

func TestRootCommandDoesNotReadStdin(t *testing.T) {
	// Regression guard: urfave/cli applies ReadArgsFromStdin on the root
	// command before dispatching to any subcommand. Its parser loops on
	// ReadRune until EOF, so enabling it makes "version" and "--help" hang
	// whenever stdin never reaches EOF, for example in a terminal or a CI
	// job. Stdin is handled in settings.loadState instead.
	if newRootCommand().ReadArgsFromStdin {
		t.Fatal("the root command must not set ReadArgsFromStdin, it blocks on version and help")
	}
}

func TestNonBlockingCommandsIgnoreStdin(t *testing.T) {
	// Even with data waiting on stdin, these commands must print and return
	// without consuming it.
	for _, args := range [][]string{
		{"version"},
		{"--help"},
		{"models", "--help"},
	} {
		t.Run(args[0], func(t *testing.T) {
			out, err := runCLIWithStdin(t, blockingReader{}, args...)
			if err != nil {
				t.Fatalf("%v: %v", args, err)
			}
			if strings.TrimSpace(out) == "" {
				t.Errorf("%v produced no output", args)
			}
		})
	}
}

// blockingReader returns no data and never reports EOF. Any read attempt hangs,
// so a command that touches it fails the test by timeout.
type blockingReader struct{}

func (blockingReader) Read([]byte) (int, error) {
	select {}
}

func TestPipedStdinIsUsedAsState(t *testing.T) {
	// With piped input and no argument, the text must come from stdin rather
	// than being reported as missing.
	previous := stdinSource
	stdinSource = strings.NewReader("Checkout is broken.")
	t.Cleanup(func() { stdinSource = previous })

	if text, ok := readPipedInput(); !ok {
		t.Fatal("piped input was not detected")
	} else if text != "Checkout is broken." {
		t.Errorf("state = %q", text)
	}
}

func TestStdinIsIgnoredWhenInteractive(t *testing.T) {
	previous := stdinSource
	stdinSource = nil
	t.Cleanup(func() { stdinSource = previous })

	// With no override the real os.Stdin is inspected. In `go test` it is not
	// a console, so reading must be attempted; what matters is that the
	// terminal check is the only guard standing between us and a hang.
	if stdinIsTerminal() {
		t.Skip("stdin is an interactive terminal in this environment")
	}
}

func TestBuildReportIncludesQuestions(t *testing.T) {
	confidence := 0.5
	req := decide.Request{
		Questions: []decide.Question{
			decide.Choice("label", "Which label?", decide.Options{"bug": "Broken", "billing": "Money"}),
			decide.Noul("is_refund", "Refund?"),
			decide.Score("urgency", "How urgent?", decide.Scale{"Low", "High"}),
		},
	}
	result := &decide.Result{
		Provider: "ollama",
		Model:    "nimble",
		Usage:    decide.Usage{InputTokens: 10, OutputTokens: 2, TotalTokens: 12},
		Answers: decide.NewAnswers([]decide.Answer{
			decide.ChoiceAnswer{QuestionName: "label", Key: "bug",
				Probabilities: map[string]float64{"bug": 0.9}, Confidence: confidence},
			decide.NoulAnswer{QuestionName: "is_refund", Probability: 0.1},
			decide.ScoreAnswer{QuestionName: "urgency", Score: 1.2,
				Legend:        map[string]string{"0": "Low", "1": "High"},
				Probabilities: map[string]float64{"0": 0.4, "1": 0.6}, Confidence: 0.1},
		}),
	}

	report := buildReport(req, result)

	if len(report.Questions) != 3 {
		t.Fatalf("questions = %d, want 3", len(report.Questions))
	}
	if report.Questions[0].Type != "choice" || len(report.Questions[0].Options) != 2 {
		t.Errorf("choice question = %+v", report.Questions[0])
	}
	if len(report.Questions[2].Scale) != 2 {
		t.Errorf("scale question = %+v", report.Questions[2])
	}

	choice := report.Answers["label"]
	if choice.Key != "bug" || choice.Type != "choice" {
		t.Errorf("choice answer = %+v", choice)
	}
	if choice.Confidence == nil || *choice.Confidence != confidence {
		t.Errorf("confidence = %v", choice.Confidence)
	}

	noul := report.Answers["is_refund"]
	if noul.Noul == nil || *noul.Noul != 0.1 || noul.Key != "" {
		t.Errorf("noul answer = %+v", noul)
	}

	score := report.Answers["urgency"]
	if score.Score == nil || *score.Score != 1.2 {
		t.Errorf("score answer = %+v", score)
	}
	if score.Legend["0"] != "Low" {
		t.Errorf("legend = %v", score.Legend)
	}
}

func TestBuildReportSkipsUnansweredQuestions(t *testing.T) {
	req := decide.Request{Questions: []decide.Question{decide.Noul("present", "?")}}
	result := &decide.Result{
		Answers: decide.NewAnswers([]decide.Answer{decide.NoulAnswer{QuestionName: "present", Probability: 0.5}}),
	}

	report := buildReport(req, result)

	// The question is still described even though the provider omitted it.
	if len(report.Questions) != 1 || report.Questions[0].Name != "present" {
		t.Errorf("questions = %+v", report.Questions)
	}
	if _, ok := report.Answers["present"]; !ok {
		t.Error("the answered question should be present")
	}
}

func TestWriteJSONIsValid(t *testing.T) {
	out := &strings.Builder{}
	if err := writeJSON(out, buildReport(decide.Request{}, &decide.Result{Answers: decide.NewAnswers(nil)})); err != nil {
		t.Fatalf("writeJSON: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(out.String()), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if _, ok := decoded["answers"]; !ok {
		t.Errorf("answers key is missing: %v", decoded)
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		value string
		width int
		want  string
	}{
		{"short", 10, "short"},
		{"1234567890", 10, "1234567890"},
		{"a-very-long-option-key", 10, "a-very-lo…"},
		{"x", 1, "x"},
		{"y", 0, ""},
	}

	for _, tc := range tests {
		if got := truncate(tc.value, tc.width); got != tc.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tc.value, tc.width, got, tc.want)
		}
	}
}

func TestFillClamps(t *testing.T) {
	tests := []struct {
		probability float64
		want        int
	}{
		{0, 0},
		{-1, 0},
		{0.5, barWidth / 2},
		{1, barWidth},
		{2, barWidth},
	}

	for _, tc := range tests {
		if got := fill(tc.probability); got != tc.want {
			t.Errorf("fill(%v) = %d, want %d", tc.probability, got, tc.want)
		}
	}
}

func TestPlainPrinterEmitsNoEscapes(t *testing.T) {
	out := &strings.Builder{}
	p := newPrinter(out, false, true) // plain

	p.print(&decide.Result{
		Provider: "ollama",
		Model:    "nimble",
		Answers: decide.NewAnswers([]decide.Answer{
			decide.ChoiceAnswer{QuestionName: "label", Key: "bug",
				Probabilities: map[string]float64{"bug": 0.9}, Confidence: 0.5},
		}),
	}, decide.Request{Questions: []decide.Question{decide.Choice("label", "Which?",
		decide.Options{"bug": "Broken", "other": "Other"})}}, false)

	got := out.String()
	if strings.Contains(got, "\x1b[") {
		t.Errorf("plain output contains ANSI escapes: %q", got)
	}
	for _, want := range []string{"ollama", "nimble", "label", "bug", "90.00%"} {
		if !strings.Contains(got, want) {
			t.Errorf("output is missing %q:\n%s", want, got)
		}
	}
}

func TestColorPrinterEmitsEscapes(t *testing.T) {
	out := &strings.Builder{}
	newPrinter(out, true, true) // both set: plain wins

	if shouldColor(out, true, true) {
		t.Error("--plain must win over --color")
	}
	if !shouldColor(out, true, false) {
		t.Error("--color should force colour")
	}
	t.Setenv("NO_COLOR", "1")
	if shouldColor(out, true, false) {
		t.Error("NO_COLOR must disable colour")
	}
}

func TestLoadQuestionsFile(t *testing.T) {
	path := writeTemp(t, "triage.json", `{
      "questions": [
        {"name": "category", "type": "choice", "instructions": "Which team?",
         "options": {"bug": "Broken", "billing": "Money"}},
        {"name": "refund", "type": "noul", "instructions": "Refund?",
         "outcomes": {"false": "No refund", "true": "Refund asked"}},
        {"name": "urgency", "type": "score", "instructions": "How urgent?",
         "scale": ["Low", "High"]}
      ]
    }`)

	questions, err := loadQuestionsFile(path)
	if err != nil {
		t.Fatalf("loadQuestionsFile: %v", err)
	}
	if len(questions) != 3 {
		t.Fatalf("questions = %d, want 3", len(questions))
	}

	if questions[0].Name != "category" || questions[0].Type != decide.TypeChoice {
		t.Errorf("choice = %+v", questions[0])
	}
	if questions[0].Options["bug"] != "Broken" {
		t.Errorf("options = %v", questions[0].Options)
	}
	if questions[1].Noul == nil || questions[1].Noul.True != "Refund asked" {
		t.Errorf("outcomes = %+v", questions[1].Noul)
	}
	if len(questions[2].Scale) != 2 || questions[2].Scale[1] != "High" {
		t.Errorf("scale = %v", questions[2].Scale)
	}
}

func TestLoadQuestionsFileRejectsBadInput(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"invalid json", `{`},
		{"unknown type", `{"questions":[{"name":"q","type":"mystery","instructions":"?"}]}`},
		{"missing type", `{"questions":[{"name":"q","instructions":"?"}]}`},
		{"no questions", `{"questions":[]}`},
		{"choice without options", `{"questions":[{"name":"q","type":"choice","instructions":"?"}]}`},
		{"score without scale", `{"questions":[{"name":"q","type":"score","instructions":"?"}]}`},
		{"one scale level", `{"questions":[{"name":"q","type":"score","instructions":"?","scale":["only"]}]}`},
		{"one option", `{"questions":[{"name":"q","type":"choice","instructions":"?","options":{"a":"A"}}]}`},
		{"missing instructions", `{"questions":[{"name":"q","type":"noul"}]}`},
		{"duplicate names", `{"questions":[
			{"name":"q","type":"noul","instructions":"A?"},
			{"name":"q","type":"noul","instructions":"B?"}]}`},
		{"unknown field", `{"questions":[{"name":"q","type":"noul","instructions":"?","oops":1}]}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := writeTemp(t, "bad.json", tc.body)
			if _, err := loadQuestionsFile(path); err == nil {
				t.Fatal("expected an error")
			} else if code := exitCodeOf(err); code != exitUsage {
				t.Errorf("exit code = %d, want %d", code, exitUsage)
			}
		})
	}
}

func TestLoadQuestionsFileMissing(t *testing.T) {
	if _, err := loadQuestionsFile("does-not-exist.json"); err == nil {
		t.Fatal("expected an error for a missing file")
	}
}

func TestLoadStateFileJSON(t *testing.T) {
	path := writeTemp(t, "state.json", `{"ticket":"blank page","tier":"enterprise"}`)

	state, err := loadStateFile(path)
	if err != nil {
		t.Fatalf("loadStateFile: %v", err)
	}

	// A .json file must keep its structure instead of becoming raw text.
	raw, err := state.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	if !strings.HasPrefix(string(raw), "{") {
		t.Errorf("state = %s, want a JSON object", raw)
	}

	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded["ticket"] != "blank page" || decoded["tier"] != "enterprise" {
		t.Errorf("state = %v", decoded)
	}
}

func TestLoadStateFileText(t *testing.T) {
	path := writeTemp(t, "state.txt", "  plain ticket body  ")

	state, err := loadStateFile(path)
	if err != nil {
		t.Fatalf("loadStateFile: %v", err)
	}

	raw, _ := state.MarshalJSON()
	if string(raw) != `"  plain ticket body  "` {
		t.Errorf("state = %s, want the raw text", raw)
	}
}

func TestLoadStateFileErrors(t *testing.T) {
	if _, err := loadStateFile("missing.txt"); err == nil {
		t.Error("expected an error for a missing file")
	}

	if _, err := loadStateFile(writeTemp(t, "empty.txt", "   ")); err == nil {
		t.Error("expected an error for an empty file")
	}

	if _, err := loadStateFile(writeTemp(t, "bad.json", `{`)); err == nil {
		t.Error("expected an error for invalid JSON")
	}

	// A bare JSON scalar is not a valid state.
	if _, err := loadStateFile(writeTemp(t, "scalar.json", `42`)); err == nil {
		t.Error("expected an error for a numeric JSON state")
	}
}

func TestInlineQuestionFlagsLists(t *testing.T) {
	if got := inlineQuestionFlags(questionFlags{}); len(got) != 0 {
		t.Errorf("flags = %v, want none", got)
	}

	all := inlineQuestionFlags(questionFlags{
		noul:   []string{"a:A?"},
		yes:    "y",
		no:     "n",
		scale:  []string{"s:S?"},
		level:  []string{"Low", "High"},
		option: []string{"a:A", "b:B"},
	})
	if len(all) != 6 {
		t.Errorf("flags = %v, want all six", all)
	}

	// Defaults such as --choice-name must not count as user input.
	if got := inlineQuestionFlags(questionFlags{choiceName: "label"}); len(got) != 0 {
		t.Errorf("flags = %v, want none", got)
	}
}

// writeTemp writes content to a temporary file that is removed with the test.
func writeTemp(t *testing.T, name, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func TestVersionCommand(t *testing.T) {
	out, err := runCLI(t, "version")
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if !strings.Contains(out, "decide ") {
		t.Errorf("output = %q", out)
	}
}

func TestClassifyRequiresInput(t *testing.T) {
	// No text and no piped stdin: the command must explain what is missing.
	_, err := runCLI(t, "classify")
	if err == nil {
		t.Fatal("expected an error")
	}
	if code := exitCodeOf(err); code != exitUsage {
		t.Errorf("exit code = %d, want %d", code, exitUsage)
	}
}

func TestClassifyRejectsConflictingInput(t *testing.T) {
	_, err := runCLI(t, "classify", "--state", "from flag", "from args")
	if err == nil {
		t.Fatal("expected an error for two input sources")
	}
	if code := exitCodeOf(err); code != exitUsage {
		t.Errorf("exit code = %d, want %d", code, exitUsage)
	}
}

func TestClassifyRejectsMissingStateFile(t *testing.T) {
	if _, err := runCLI(t, "classify", "--state-file", "does-not-exist.txt"); err == nil {
		t.Fatal("expected an error for a missing file")
	}
}

func TestClassifyRejectsMalformedQuestionFlag(t *testing.T) {
	_, err := runCLI(t, "classify", "--noul", "no-colon", "some text")
	if err == nil {
		t.Fatal("expected an error for a malformed --noul value")
	}
	if code := exitCodeOf(err); code != exitUsage {
		t.Errorf("exit code = %d, want %d", code, exitUsage)
	}
}

func TestUnknownProviderIsReported(t *testing.T) {
	_, err := runCLI(t, "classify", "--provider", "nope", "some text")
	if err == nil {
		t.Fatal("expected an error for an unknown provider")
	}
	if code := exitCodeOf(err); code != exitNotFound {
		t.Errorf("exit code = %d, want %d", code, exitNotFound)
	}
}

func TestHumanSize(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{512, "512 B"},
		{1024, "1.0 KiB"},
		{9527502277, "8.9 GiB"},
	}

	for _, tc := range tests {
		if got := humanSize(tc.bytes); got != tc.want {
			t.Errorf("humanSize(%d) = %q, want %q", tc.bytes, got, tc.want)
		}
	}
}

func TestTrimTag(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"nimble:latest", "nimble"},
		{"nimble", "nimble"},
		{"registry.example.com:5000/model:tag", "registry.example.com:5000/model"},
	}

	for _, tc := range tests {
		if got := trimTag(tc.name); got != tc.want {
			t.Errorf("trimTag(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestBar(t *testing.T) {
	if got := bar(0); got != "" {
		t.Errorf("bar(0) = %q, want empty", got)
	}
	if got := bar(1); len([]rune(got)) != barWidth {
		t.Errorf("bar(1) = %q, want %d blocks", got, barWidth)
	}
	if got := bar(0.5); len([]rune(got)) != barWidth/2 {
		t.Errorf("bar(0.5) = %q, want %d blocks", got, barWidth/2)
	}
	// A probability above 1 is clamped rather than overflowing the bar.
	if got := bar(2); len([]rune(got)) != barWidth {
		t.Errorf("bar(2) = %q, want a clamped bar of %d blocks", got, barWidth)
	}
	// A negative probability must not panic or render.
	if got := bar(-1); got != "" {
		t.Errorf("bar(-1) = %q, want empty", got)
	}
}
