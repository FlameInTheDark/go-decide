package playground

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	decide "github.com/FlameInTheDark/go-decide"
	"github.com/FlameInTheDark/go-decide/internal/cliconfig"
	"github.com/FlameInTheDark/go-decide/providers/ollama"
)

// handleConfig reports the current settings. The API key is never included:
// only whether one is set.
func (s *Server) handleConfig(c fiber.Ctx) error {
	settings := s.currentSettings()

	view := ConfigView{
		Provider:  settings.Provider,
		BaseURL:   s.baseURLFor(settings),
		Model:     settings.Model,
		Timeout:   secondsOf(settings.Timeout),
		Retries:   settings.Retries,
		APIKeySet: settings.APIKey != "",
		Providers: decide.Registered(),
		Limits: LimitView{
			MinCriteria:   decide.MinCriteria,
			MaxCriteria:   decide.MaxCriteria,
			MaxStateBytes: decide.MaxStateBytes,
			MaxImageBytes: decide.MaxImageBytes,
		},
	}
	if settings.Provider == ollama.Name {
		view.DefaultModel = ollama.DefaultModel
	}

	return c.JSON(view)
}

// baseURLFor reports the effective address of the selected provider, filling in
// the provider's own default so the UI shows what is really being used.
func (s *Server) baseURLFor(settings cliconfig.Settings) string {
	if settings.BaseURL != "" {
		return settings.BaseURL
	}
	if settings.Provider == ollama.Name {
		return ollama.DefaultBaseURL
	}
	return openRouterDefaultBaseURL
}

// handleModels lists the installed decision models for the current settings.
func (s *Server) handleModels(c fiber.Ctx) error {
	settings := s.currentSettings()
	if provider := c.Query("provider"); provider != "" {
		settings.Provider = provider
	}
	if baseURL := c.Query("base_url"); baseURL != "" {
		settings.BaseURL = baseURL
	}

	ctx, cancel := s.decisionContext(c.Context(), settings)
	defer cancel()

	view, err := modelList(ctx, settings.WithDefaults())
	if err != nil {
		failure := newErrorView(err)
		return c.Status(failure.Status()).JSON(errorResponse{Error: failure})
	}
	if view.Models == nil {
		view.Models = []ModelView{}
	}
	return c.JSON(view)
}

// handleValidate checks a request locally, without contacting the provider. It
// returns the library's own problems so the UI can highlight fields before a
// run, and the provider capabilities so it can warn about an impossible
// request. It always replies 200: a failed validation is a successful answer
// to the question that was asked.
func (s *Server) handleValidate(c fiber.Ctx) error {
	var in DecideRequest
	if err := bindJSON(c, &in); err != nil {
		return err
	}

	settings := s.applyPatch(in.Config)
	req, err := in.toRequest()

	response := ValidateResponse{}

	if provider, perr := providerFor(settings); perr == nil {
		response.Capabilities = capabilitiesOf(provider)
	}

	if err != nil {
		view := newErrorView(err)
		response.Problems = view.Problems
		response.Fields = view.Fields
		// Questions still come back, so the UI can render what it did send
		// and highlight the offending row.
		response.Questions = questionViews(req.Questions, in.optionOrders())
		return c.JSON(response)
	}

	// The library's own capability check runs inside the client, so the UI
	// gets the same answer a real run would produce before spending a token.
	if err := s.capabilityProblem(settings, req); err != nil {
		view := newErrorView(err)
		response.Problems = view.Problems
		response.Fields = view.Fields
		response.Questions = questionViews(req.Questions, in.optionOrders())
		return c.JSON(response)
	}

	response.OK = true
	response.Questions = questionViews(req.Questions, in.optionOrders())
	return c.JSON(response)
}

// capabilityProblem runs the same capability check the client performs, without
// sending anything. Providers that do not implement decide.Capable are assumed
// to accept anything, matching the library.
func (s *Server) capabilityProblem(settings cliconfig.Settings, req decide.Request) error {
	provider, err := providerFor(settings)
	if err != nil {
		return nil
	}
	return decide.CheckCapabilities(provider, req)
}

// handleDecide runs a decision and records it in the history.
func (s *Server) handleDecide(c fiber.Ctx) error {
	body, err := readBody(c)
	if err != nil {
		return err
	}

	var in DecideRequest
	if err := decodeJSON(body, &in); err != nil {
		return err
	}

	settings := s.applyPatch(in.Config)
	s.remember(settings, in)

	req, err := in.toRequest()
	if err != nil {
		s.record(settings, body, nil, newErrorView(err), 0)
		view := newErrorView(err)
		return c.Status(view.Status()).JSON(errorResponse{Error: view})
	}

	ctx, cancel := s.decisionContext(c.Context(), settings)
	defer cancel()

	client := clientFor(settings)
	started := time.Now()
	result, err := client.DecideWith(ctx, settings.Provider, req)
	elapsed := time.Since(started)

	if err != nil {
		view := newErrorView(err)
		s.record(settings, body, nil, view, elapsed)
		return c.Status(view.Status()).JSON(errorResponse{Error: view})
	}

	response := DecideResponse{
		Result:     result,
		Raw:        result.Raw,
		Questions:  questionViews(req.Questions, in.optionOrders()),
		Missing:    result.Missing(req.Questions),
		DurationMS: elapsed.Milliseconds(),
		Provider:   settings.Provider,
		Model:      req.Model,
	}
	if response.Model == "" {
		response.Model = result.Model
	}
	response.Answers = answerViews(result, response.Questions)
	// The prompt is rendered from the request that was actually sent, so a
	// stored run carries what it was asked alongside what it answered.
	response.Prompt = renderPrompt(req.State)

	s.record(settings, body, &response, nil, elapsed)
	return c.JSON(response)
}

// record stores a run in the in-memory history.
func (s *Server) record(settings cliconfig.Settings, body json.RawMessage, response *DecideResponse, view *ErrorView, elapsed time.Duration) {
	entry := historyEntry{
		provider: settings.Provider,
		model:    settings.Model,
		ok:       view == nil,
		duration: elapsed,
		body:     append(json.RawMessage(nil), body...),
		err:      view,
	}
	if response != nil {
		if response.Model != "" {
			entry.model = response.Model
		}
		if raw, err := json.Marshal(response); err == nil {
			entry.response = raw
		}
		entry.summary = summarize(response)
		// The labels come from the response, so a run that never reached the
		// provider keeps whatever it managed to parse.
		for _, question := range response.Questions {
			entry.questions = append(entry.questions, question.Name)
		}
	}

	s.history.add(entry)
}

// remember applies settings the user changed in the UI so a later request
// reuses them. The settings are never written to disk.
func (s *Server) remember(settings cliconfig.Settings, in DecideRequest) {
	patch := in.Config
	if patch.Provider == "" && patch.BaseURL == "" && patch.APIKey == "" &&
		patch.Model == "" && patch.Timeout == 0 && patch.Retries == nil {
		return
	}
	s.mu.Lock()
	s.current = settings
	s.mu.Unlock()
}

// summarize renders a one-line description of a run for the history list.
func summarize(response *DecideResponse) string {
	if response == nil {
		return ""
	}
	parts := make([]string, 0, len(response.Answers))
	for _, answer := range response.Answers {
		parts = append(parts, answer.Name+"="+answer.Label)
	}
	return joinWith(parts, "  ")
}

func joinWith(parts []string, sep string) string {
	return strings.Join(parts, sep)
}

// handleHistory returns the recorded runs, newest first.
func (s *Server) handleHistory(c fiber.Ctx) error {
	return c.JSON(fiber.Map{"runs": s.history.list()})
}

// handleHistoryClear empties the history.
func (s *Server) handleHistoryClear(c fiber.Ctx) error {
	s.history.clear()
	return c.SendStatus(fiber.StatusNoContent)
}
