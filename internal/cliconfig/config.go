// Package cliconfig turns a small set of connection settings into a working
// [decide.Client], and turns library errors back into something a program can
// present to a human.
//
// It exists so that the `decide` command and `decide-playground` configure the
// library in exactly the same way. Both binaries are thin shells over
// [decide.Client]; every decision they run goes through the library, including
// validation, capability checks, retries and error classification.
package cliconfig

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	decide "github.com/FlameInTheDark/go-decide"
	"github.com/FlameInTheDark/go-decide/providers/ollama"
	"github.com/FlameInTheDark/go-decide/providers/openrouter"
)

// Defaults shared by the command line and the playground UI.
const (
	// DefaultTimeout is how long a single decision may take before it is
	// aborted.
	DefaultTimeout = 2 * time.Minute
	// DefaultRetries is the number of extra attempts after the first one.
	DefaultRetries = 2
	// DefaultProvider is the provider used when none is selected.
	DefaultProvider = ollama.Name

	baseDelay = 300 * time.Millisecond
	maxDelay  = 3 * time.Second
	jitter    = 0.2
)

// Settings describes how to reach a provider. The zero value is usable: it
// selects Ollama on its default address with the default timeout and retry
// count.
type Settings struct {
	// Provider is the adapter name, e.g. "ollama" or "openrouter".
	Provider string
	// BaseURL overrides the provider's own default address.
	BaseURL string
	// APIKey is the credential for providers that need one. Ollama ignores
	// it.
	APIKey string
	// Model is the model used when a request does not name one. Ollama
	// applies it as the provider default.
	Model string
	// Timeout bounds a single decision. Zero means [DefaultTimeout].
	Timeout time.Duration
	// Retries is the number of attempts after the first one. Negative values
	// are treated as zero.
	Retries int
	// Verbose enables request logging through [decide.WithLogging].
	Verbose bool
	// Logger receives the log lines when Verbose is set. A nil logger uses
	// slog.Default.
	Logger *slog.Logger
	// Headers are extra HTTP headers sent with every request to the provider.
	Headers map[string]string
	// ReqLog receives one entry per decision when Verbose is set. It exists
	// so the playground can show request logs without replacing the
	// process-wide default logger.
	ReqLog func(provider, model string, questions []string, elapsed time.Duration)
}

// WithDefaults returns a copy of s with zero-valued fields filled in.
func (s Settings) WithDefaults() Settings {
	out := s
	if out.Provider == "" {
		out.Provider = DefaultProvider
	}
	if out.Timeout <= 0 {
		out.Timeout = DefaultTimeout
	}
	if out.Retries < 0 {
		out.Retries = 0
	}
	return out
}

// Context applies the timeout unless parent already has an earlier deadline.
func (s Settings) Context(parent context.Context) (context.Context, context.CancelFunc) {
	timeout := s.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return context.WithTimeout(parent, timeout)
}

// NewClient builds a [decide.Client] for the configured provider. Both
// adapters are always registered so switching backends costs a single flag.
func (s Settings) NewClient() *decide.Client {
	s = s.WithDefaults()

	httpClient := &http.Client{Timeout: s.Timeout}

	options := []decide.Option{
		decide.WithProvider(s.ollamaProvider(httpClient)),
		decide.WithProvider(s.openRouterProvider(httpClient)),
		decide.WithDefault(s.Provider),
		decide.WithRetry(decide.RetryPolicy{
			MaxAttempts: s.Retries + 1,
			BaseDelay:   baseDelay,
			MaxDelay:    maxDelay,
			Jitter:      jitter,
		}),
	}
	if s.Verbose {
		if middleware := s.logging(); middleware != nil {
			options = append(options, decide.WithMiddleware(middleware))
		}
	}

	return decide.New(options...)
}

func (s Settings) ollamaProvider(httpClient *http.Client) decide.Provider {
	options := []ollama.Option{
		ollama.WithBaseURL(or(s.BaseURL, ollama.DefaultBaseURL)),
		ollama.WithHTTPClient(httpClient),
	}
	if s.Model != "" {
		options = append(options, ollama.WithDefaultModel(s.Model))
	}
	if s.BaseURL != "" && s.APIKey != "" {
		// An explicit base URL may point at a non-Ollama-compatible backend,
		// so send the key there too.
		options = append(options, ollama.WithAPIKey(s.APIKey))
	}
	for key, value := range s.Headers {
		options = append(options, ollama.WithHeader(key, value))
	}
	return ollama.New(options...)
}

func (s Settings) openRouterProvider(httpClient *http.Client) decide.Provider {
	options := []openrouter.Option{
		openrouter.WithBaseURL(or(s.BaseURL, openrouter.DefaultBaseURL)),
		openrouter.WithHTTPClient(httpClient),
	}
	if s.APIKey != "" {
		options = append(options, openrouter.WithAPIKey(s.APIKey))
	}
	for key, value := range s.Headers {
		options = append(options, openrouter.WithHeader(key, value))
	}
	if s.Verbose {
		options = append(options,
			openrouter.WithReferer("github.com/FlameInTheDark/go-decide"),
			openrouter.WithTitle("go-decide"),
		)
	}
	return openrouter.New(options...)
}

// logging builds the request-logging middleware, or nil when there is nothing
// to log to.
func (s Settings) logging() decide.Middleware {
	if s.ReqLog == nil {
		if s.Logger == nil {
			return decide.WithLogging(nil)
		}
		return decide.WithLogging(s.Logger)
	}
	return func(next decide.Handler) decide.Handler {
		return func(ctx context.Context, req decide.Request) (*decide.Result, error) {
			started := time.Now()
			res, err := next(ctx, req)
			s.ReqLog(providerNameFrom(ctx), req.Model, questionNames(req), time.Since(started))
			return res, err
		}
	}
}

func providerNameFrom(ctx context.Context) string {
	name, _ := decide.ProviderFromContext(ctx)
	return name
}

func questionNames(req decide.Request) []string {
	names := make([]string, 0, len(req.Questions))
	for _, question := range req.Questions {
		names = append(names, question.Name)
	}
	return names
}

// HTTPClientFor gives a transport its own deadline so a hung connection cannot
// outlive a decision.
func HTTPClientFor(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &http.Client{Timeout: timeout}
}

// Detail describes a library error in a form that is convenient to display:
// a stable machine-readable kind, a readable message, and the individual
// validation problems when the library collected more than one.
type Detail struct {
	// Kind is the [decide.Kind] name, e.g. "not_found". It is empty for
	// errors the library did not classify.
	Kind string
	// Message is the full error string.
	Message string
	// Problems lists each validation problem separately. It is nil unless
	// the error was a *[decide.ValidationError].
	Problems []string
	// RetryAfter is the delay the provider requested, if any.
	RetryAfter time.Duration
	// Retryable reports whether retrying could succeed.
	Retryable bool
	// Provider is the adapter that failed, when known.
	Provider string
	// StatusCode is the HTTP status, or 0.
	StatusCode int
}

// Describe classifies err using the library's own error types. It returns nil
// for a nil error, so callers can use it directly on the result of a decision.
func Describe(err error) *Detail {
	if err == nil {
		return nil
	}

	detail := &Detail{Message: err.Error()}

	var validation *decide.ValidationError
	if errors.As(err, &validation) {
		detail.Kind = decide.KindInvalidRequest.String()
		detail.Problems = validation.Problems
	}

	var typed *decide.Error
	if errors.As(err, &typed) {
		detail.Kind = typed.Kind.String()
		detail.Provider = typed.Provider
		detail.StatusCode = typed.StatusCode
		detail.RetryAfter = typed.RetryAfter
		detail.Retryable = typed.Retryable()
	}

	return detail
}

func or(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}
