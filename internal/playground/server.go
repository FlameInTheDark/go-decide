package playground

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/recover"

	decide "github.com/FlameInTheDark/go-decide"
	"github.com/FlameInTheDark/go-decide/internal/cliconfig"
	"github.com/FlameInTheDark/go-decide/providers/ollama"
)

// guardHeader is required on every /api request. A no-auth server bound to
// localhost is still reachable from any web page the user visits, because a
// cross-origin fetch can be sent without a preflight only for simple requests.
// Requiring a custom header forces a preflight, and the API never answers one,
// so a hostile page cannot read or trigger anything.
const guardHeader = "X-Decide-Playground"

// assetPrefix is where the bundler writes hashed, immutable assets.
const assetPrefix = "/assets/"

// Server serves the playground UI and its API.
type Server struct {
	app      *fiber.App
	settings cliconfig.Settings
	logger   *slog.Logger
	assets   fs.FS
	history  *historyStore

	// mu guards the settings the UI last applied, so a change made in the
	// browser persists across requests without a restart.
	mu      sync.RWMutex
	current cliconfig.Settings
}

// Options configures a playground [Server].
type Options struct {
	// Settings is the configuration the server starts with. The UI may
	// change it at runtime; nothing is ever written to disk.
	Settings cliconfig.Settings
	// Logger receives request logs. A nil logger discards them.
	Logger *slog.Logger
	// Assets is the built single-page app. Required.
	Assets fs.FS
	// HistoryLimit caps the in-memory history. Zero uses the default.
	HistoryLimit int
	// BodyLimit caps an API request body. Zero uses maxRequestBytes.
	BodyLimit int
}

// New builds a server. It does not listen; call [Server.App] and use
// [fiber.App.Listen], or [Server.Test] in tests.
func New(opts Options) (*Server, error) {
	if opts.Assets == nil {
		return nil, errors.New("playground: an assets filesystem is required")
	}

	settings := opts.Settings.WithDefaults()
	logger := opts.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	bodyLimit := opts.BodyLimit
	if bodyLimit <= 0 {
		bodyLimit = maxRequestBytes
	}

	server := &Server{
		settings: settings,
		logger:   logger,
		assets:   opts.Assets,
		history:  newHistoryStore(opts.HistoryLimit),
		current:  settings,
	}

	app := fiber.New(fiber.Config{
		AppName:      "decide-playground",
		BodyLimit:    bodyLimit,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // a decision can take as long as the timeout allows
		// ServerHeader is empty so fasthttp does not advertise itself. The
		// playground only ever talks to a local browser, so identifying the server
		// stack buys nothing.
		ServerHeader: "",
		ErrorHandler: server.onError,
	})

	app.Use(recover.New())
	server.registerAPI(app)
	server.registerAssets(app)

	server.app = app
	return server, nil
}

// App exposes the Fiber application so the caller controls the listener.
func (s *Server) App() *fiber.App { return s.app }

// Listen starts serving. The Fiber startup banner is suppressed because this
// command prints its own one-line URL first; two banners would be noise.
func (s *Server) Listen(addr string) error {
	return s.app.Listen(addr, fiber.ListenConfig{DisableStartupMessage: true})
}

// Shutdown stops the server, giving in-flight decisions time to finish.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.app.ShutdownWithContext(ctx)
}

// Test serves a single request without opening a socket. The timeout defaults
// to [TestTimeout] because a decision can legitimately take as long as the
// configured provider timeout; pass a config to override it.
func (s *Server) Test(req *http.Request, config ...fiber.TestConfig) (*http.Response, error) {
	if len(config) == 0 {
		config = []fiber.TestConfig{{Timeout: TestTimeout}}
	}
	return s.app.Test(req, config...)
}

// TestTimeout is the default deadline for a request served through [Server.Test].
const TestTimeout = 30 * time.Second

func (s *Server) registerAPI(app *fiber.App) {
	api := app.Group("/api", s.guard)

	api.Get("/config", s.handleConfig)
	api.Get("/models", s.handleModels)
	api.Post("/validate", s.handleValidate)
	api.Post("/decide", s.handleDecide)
	api.Get("/history", s.handleHistory)
	api.Post("/history/clear", s.handleHistoryClear)
}

// guard requires the custom header on every API request and rejects oversized
// bodies before they are read.
func (s *Server) guard(c fiber.Ctx) error {
	if c.Get(guardHeader) == "" {
		return &fiber.Error{
			Code:    fiber.StatusForbidden,
			Message: "missing " + guardHeader + " header",
		}
	}
	return c.Next()
}

func (s *Server) registerAssets(app *fiber.App) {
	assets := newAssetHandler(s.assets)

	// Hashed assets are immutable, so they can be cached hard. The shell must
	// never be cached, or a rebuilt binary would keep serving the old app.
	app.Use(func(c fiber.Ctx) error {
		if strings.HasPrefix(c.Path(), "/api/") {
			return c.Next()
		}
		if served, err := assets.handle(c); err != nil {
			return err
		} else if served {
			return nil
		}
		// Anything that is not a file is a client-side route, so it gets the
		// app shell and the router takes over from there.
		return serveIndex(c, s.assets)
	})
}

// onError turns any error returned by a handler into the shared error shape.
func (s *Server) onError(c fiber.Ctx, err error) error {
	var fiberErr *fiber.Error
	if errors.As(err, &fiberErr) {
		view := &ErrorView{Message: fiberErr.Message}
		if view.Message == "" {
			view.Message = http.StatusText(fiberErr.Code)
		}
		return c.Status(fiberErr.Code).JSON(errorResponse{Error: view})
	}

	// A typed client error carries its own status so a malformed payload is
	// never reported as a server fault.
	var status interface{ Status() int }
	if errors.As(err, &status) {
		view := &ErrorView{Message: err.Error()}
		return c.Status(status.Status()).JSON(errorResponse{Error: view})
	}

	view := newErrorView(err)
	if view == nil {
		view = &ErrorView{Message: err.Error()}
	}
	return c.Status(view.Status()).JSON(errorResponse{Error: view})
}

// currentSettings returns the settings the UI last applied, falling back to the
// ones the server started with.
func (s *Server) currentSettings() cliconfig.Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

// applyPatch merges a request-scoped patch over the server settings and, when
// the patch came from a real user action, stores it for later requests. The API
// key is never read back out of the server, so a patch that omits it keeps the
// one the server already has.
func (s *Server) applyPatch(patch ConfigPatch) cliconfig.Settings {
	base := s.currentSettings()

	out := base
	if patch.Provider != "" {
		out.Provider = patch.Provider
	}
	if patch.BaseURL != "" {
		out.BaseURL = patch.BaseURL
	}
	if patch.APIKey != "" {
		out.APIKey = patch.APIKey
	}
	if patch.Timeout > 0 {
		out.Timeout = time.Duration(patch.Timeout) * time.Second
	}
	if patch.Retries != nil && *patch.Retries >= 0 {
		out.Retries = *patch.Retries
	}
	if patch.Model != "" {
		out.Model = patch.Model
	}
	return out
}

// clientFor builds a decide.Client for the given settings. Every decision the
// playground runs goes through the library, so validation, capability checks,
// retries and error classification behave exactly as they do in the CLI.
func clientFor(settings cliconfig.Settings) *decide.Client {
	return settings.NewClient()
}

// capabilitiesOf reports what a provider advertises.
func capabilitiesOf(provider decide.Provider) CapabilityView {
	view := CapabilityView{Provider: provider.Name()}
	capable, ok := provider.(decide.Capable)
	if !ok {
		return view
	}
	caps := capable.Capabilities()
	view.Images = caps.Images
	view.MaxQuestions = caps.MaxQuestions
	view.MaxChoices = caps.MaxChoices
	view.MaxStateBytes = caps.MaxStateBytes
	view.Known = true
	return view
}

// providerFor builds the provider named in settings so its capabilities can be
// read without running a decision.
func providerFor(settings cliconfig.Settings) (decide.Provider, error) {
	settings = settings.WithDefaults()
	client := settings.NewClient()
	return client.Provider(settings.Provider)
}

// decisionContext bounds a decision with the configured timeout.
func (s *Server) decisionContext(parent context.Context, settings cliconfig.Settings) (context.Context, context.CancelFunc) {
	timeout := settings.Timeout
	if timeout <= 0 {
		timeout = cliconfig.DefaultTimeout
	}
	return context.WithTimeout(parent, timeout)
}

// modelList describes the installed decision models for the given provider.
// Only Ollama exposes a model listing; other providers return an empty list
// with no error so the UI can fall back to free-text entry.
func modelList(ctx context.Context, settings cliconfig.Settings) (*ModelsView, error) {
	view := &ModelsView{Provider: settings.Provider, BaseURL: settings.BaseURL}

	if settings.Provider != ollama.Name {
		return view, nil
	}

	provider, err := providerFor(settings)
	if err != nil {
		return nil, err
	}
	local, ok := provider.(*ollama.Provider)
	if !ok {
		return view, nil
	}

	models, err := local.Models(ctx)
	if err != nil {
		return nil, err
	}

	for _, model := range ollama.FilterDecisionModels(models) {
		view.Models = append(view.Models, ModelView{
			Name:     model.ID(),
			Decision: model.Decision(),
			Vision:   model.Vision(),
			Family:   model.Family(),
			Size:     model.Size,
			Context:  model.ContextLength(),
		})
	}

	if version, err := local.ServerVersion(ctx); err == nil {
		view.Version = version
		if supported, err := local.SupportsSystemOne(ctx); err == nil && !supported {
			view.Warnings = append(view.Warnings,
				"this Ollama is older than v0.35.0, which introduced decision models")
		}
	} else {
		view.Warnings = append(view.Warnings, "could not read the Ollama version: "+err.Error())
	}

	return view, nil
}

func secondsOf(d time.Duration) int { return int(d.Seconds()) }
