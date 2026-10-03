package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/urfave/cli/v3"

	decide "github.com/FlameInTheDark/go-decide"
	"github.com/FlameInTheDark/go-decide/providers/ollama"
	"github.com/FlameInTheDark/go-decide/providers/openrouter"
)

// stdinSource overrides where piped input is read from. Tests replace it so
// they never consume the real process stdin. When nil, os.Stdin is used, and
// it is only read when it is genuinely a pipe rather than a terminal.
var stdinSource io.Reader

// stdinIsTerminal reports whether stdin is an interactive console. Supplying an
// override implies piped input, so this only inspects the real os.Stdin when no
// override is present.
func stdinIsTerminal() bool {
	if stdinSource != nil {
		return false
	}

	info, err := os.Stdin.Stat()
	if err != nil {
		// When in doubt, assume interactive so we never block.
		return true
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// settings is the resolved configuration for one invocation. Global flags live
// on the root command, so subcommands read them through cmd.Root().
type settings struct {
	provider string
	baseURL  string
	apiKey   string
	timeout  time.Duration
	retries  int
	verbose  bool
	stdout   io.Writer
	stderr   io.Writer
}

// resolveSettings reads the global flags from whichever command is running.
func resolveSettings(cmd *cli.Command) settings {
	root := cmd.Root()

	out := root.Writer
	if out == nil {
		out = os.Stdout
	}
	errOut := root.ErrWriter
	if errOut == nil {
		errOut = os.Stderr
	}

	retries := root.Int("retries")
	if retries < 0 {
		retries = 0
	}

	return settings{
		provider: root.String("provider"),
		baseURL:  root.String("base-url"),
		apiKey:   root.String("api-key"),
		timeout:  root.Duration("timeout"),
		retries:  retries,
		verbose:  root.Bool("verbose"),
		stdout:   out,
		stderr:   errOut,
	}
}

// context applies the configured timeout unless the parent already has an
// earlier deadline.
func (s settings) context(parent context.Context) (context.Context, context.CancelFunc) {
	if s.timeout <= 0 {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, s.timeout)
}

// logger builds the stderr logger used when --verbose is set.
func (s settings) logger() *slog.Logger {
	return slog.New(slog.NewTextHandler(s.stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// httpClientFor gives the transport its own deadline so a hung connection cannot
// outlive the command timeout.
func httpClientFor(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	return &http.Client{Timeout: timeout}
}

// client builds a decide.Client for the selected provider. Both adapters are
// always registered so switching backends costs a single flag.
func (s settings) client() *decide.Client {
	ollamaOpts := []ollama.Option{
		ollama.WithBaseURL(or(s.baseURL, ollama.DefaultBaseURL)),
		ollama.WithHTTPClient(httpClientFor(s.timeout)),
	}

	openrouterOpts := []openrouter.Option{
		openrouter.WithBaseURL(or(s.baseURL, openrouter.DefaultBaseURL)),
		openrouter.WithHTTPClient(httpClientFor(s.timeout)),
	}
	if s.apiKey != "" {
		openrouterOpts = append(openrouterOpts, openrouter.WithAPIKey(s.apiKey))
	}
	if s.verbose {
		openrouterOpts = append(openrouterOpts,
			openrouter.WithReferer("github.com/FlameInTheDark/go-decide"),
			openrouter.WithTitle("go-decide"),
		)
	}

	options := []decide.Option{
		decide.WithProvider(ollama.New(ollamaOpts...)),
		decide.WithProvider(openrouter.New(openrouterOpts...)),
		decide.WithDefault(s.provider),
		decide.WithRetry(decide.RetryPolicy{
			MaxAttempts: s.retries + 1,
			BaseDelay:   300 * time.Millisecond,
			MaxDelay:    3 * time.Second,
			Jitter:      0.2,
		}),
	}
	if s.verbose {
		options = append(options, decide.WithMiddleware(decide.WithLogging(s.logger())))
	}

	return decide.New(options...)
}

// loadState resolves the state to evaluate from the positional argument, the
// --state flag, --state-file, or stdin when the input is piped. A .json
// state file keeps its structure instead of being sent as raw text.
func (s settings) loadState(cmd *cli.Command, inline, file string) (decide.State, error) {
	provided := make([]string, 0, 3)

	if fromArgs := strings.Join(positionalArgs(cmd), " "); strings.TrimSpace(fromArgs) != "" {
		provided = append(provided, "arguments")
		inline = strings.TrimSpace(fromArgs)
	}
	if cmd.IsSet("state") {
		provided = append(provided, "--state")
	}
	if cmd.IsSet("state-file") {
		provided = append(provided, "--state-file")
	}

	switch len(provided) {
	case 0:
		// Nothing given: fall back to stdin when something is piped in.
		if text, ok := readPipedInput(); ok {
			return decide.Text(text), nil
		}
		return decide.State{}, usageError("no input: pass text as an argument, with --state, with --state-file, or pipe it to stdin")
	case 1:
		// Exactly one source, carry on.
	default:
		return decide.State{}, usageError("conflicting input: use only one of %s", strings.Join(provided, ", "))
	}

	if file != "" {
		return loadStateFile(file)
	}

	if strings.TrimSpace(inline) == "" {
		return decide.State{}, usageError("the state must not be empty")
	}
	return decide.Text(inline), nil
}

// positionalArgs returns the non-blank positional arguments of a command.
// Reading stdin can contribute an empty argument, which is never meaningful.
func positionalArgs(cmd *cli.Command) []string {
	raw := cmd.Args().Slice()
	args := make([]string, 0, len(raw))
	for _, arg := range raw {
		if strings.TrimSpace(arg) != "" {
			args = append(args, arg)
		}
	}
	return args
}

// readPipedInput reads stdin when it is a pipe rather than a terminal. It never
// blocks on an interactive terminal, which would hang the command.
func readPipedInput() (string, bool) {
	if stdinIsTerminal() {
		return "", false
	}

	reader := io.Reader(os.Stdin)
	if stdinSource != nil {
		reader = stdinSource
	}

	data, err := io.ReadAll(reader)
	if err != nil || strings.TrimSpace(string(data)) == "" {
		return "", false
	}
	return strings.TrimSpace(string(data)), true
}

// exitCodeError is an error that carries the process exit code the CLI should
// terminate with.
//
// It deliberately does NOT implement cli.ExitCoder. That interface is
// `interface { error; ExitCode() int }`, so a method named ExitCode would make
// this type satisfy it by structural typing; urfave/cli would then call
// os.Exit itself from inside Command.Run, killing the process and making the
// command untestable. Actions return this type instead, and main maps it.
type exitCodeError struct {
	message string
	code    int
}

// Error implements the error interface.
func (e *exitCodeError) Error() string { return e.message }

// status returns the process exit code. The unexported, differently named
// method keeps this type from matching cli.ExitCoder.
func (e *exitCodeError) status() int { return e.code }

// Exit codes used by the command. They let scripts branch on the failure kind.
const (
	exitFailure     = 1 // unexpected or unclassified failure
	exitUsage       = 2 // bad flags, bad input, invalid request
	exitAuth        = 3 // missing or rejected credentials, insufficient credits
	exitNotFound    = 4 // unknown provider or model
	exitRateLimited = 5 // rate limited or timed out
	exitUpstream    = 6 // provider or transport failure
)

// usageError reports a misuse of the command line or of the request.
func usageError(format string, args ...any) error {
	return &exitCodeError{message: fmt.Sprintf(format, args...), code: exitUsage}
}

// exitError converts a library failure into an actionable CLI error carrying a
// meaningful exit code.
func exitError(err error) error {
	if err == nil {
		return nil
	}

	var coded *exitCodeError
	if errors.As(err, &coded) {
		return err
	}

	switch {
	case errors.Is(err, decide.ErrInvalidRequest), errors.Is(err, decide.ErrUnsupported):
		return &exitCodeError{message: err.Error(), code: exitUsage}
	case errors.Is(err, decide.ErrAuth), errors.Is(err, decide.ErrPayment):
		return &exitCodeError{message: err.Error(), code: exitAuth}
	case errors.Is(err, decide.ErrNotFound):
		return &exitCodeError{message: err.Error(), code: exitNotFound}
	case errors.Is(err, decide.ErrRateLimited), errors.Is(err, decide.ErrTimeout):
		return &exitCodeError{message: err.Error(), code: exitRateLimited}
	case errors.Is(err, decide.ErrServer), errors.Is(err, decide.ErrUnavailable):
		return &exitCodeError{message: err.Error(), code: exitUpstream}
	default:
		return &exitCodeError{message: err.Error(), code: exitFailure}
	}
}

// exitCodeOf reports the exit code carried by err, defaulting to exitFailure.
func exitCodeOf(err error) int {
	var coded *exitCodeError
	if errors.As(err, &coded) {
		return coded.status()
	}
	return exitFailure
}

func or(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}
