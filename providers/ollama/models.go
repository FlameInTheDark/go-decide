package ollama

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	decide "github.com/FlameInTheDark/go-decide"
)

// Model is an entry of the model list returned by /api/tags.
type Model struct {
	Name         string    `json:"name"`
	Model        string    `json:"model"`
	ModifiedAt   time.Time `json:"modified_at"`
	Size         int64     `json:"size"`
	Digest       string    `json:"digest"`
	Capabilities []string  `json:"capabilities"`
	Details      Details   `json:"details"`
	RemoteHost   string    `json:"remote_host,omitempty"`
	RemoteModel  string    `json:"remote_model,omitempty"`
}

// Details carries the model metadata nested under "details".
type Details struct {
	ParentModel       string `json:"parent_model"`
	Format            string `json:"format"`
	Family            string `json:"family"`
	ParameterSize     string `json:"parameter_size"`
	QuantizationLevel string `json:"quantization_level"`
	ContextLength     int    `json:"context_length"`
}

// ID returns the name to use in a request.
func (m Model) ID() string {
	if m.Name != "" {
		return m.Name
	}
	return m.Model
}

// Family returns the model family, e.g. "qwen35".
func (m Model) Family() string { return m.Details.Family }

// ContextLength returns the context window in tokens.
func (m Model) ContextLength() int { return m.Details.ContextLength }

// HasCapability reports whether the model advertises a capability, e.g.
// "decision", "vision" or "tools".
func (m Model) HasCapability(capability string) bool {
	for _, candidate := range m.Capabilities {
		if candidate == capability {
			return true
		}
	}
	return false
}

// Vision reports whether the model can judge images.
func (m Model) Vision() bool { return m.HasCapability("vision") }

// Decision reports whether the model can answer decision questions.
func (m Model) Decision() bool { return isDecisionModel(m) }

// Cloud reports whether the model is served by ollama.com rather than locally.
// Cloud models are rejected by the System One endpoint.
func (m Model) Cloud() bool { return m.RemoteHost != "" }

type tagsResponse struct {
	Models []Model `json:"models"`
}

// Models lists the models installed on the server.
func (p *Provider) Models(ctx context.Context) ([]Model, error) {
	p.mu.Lock()
	sender := *p.tags
	p.mu.Unlock()

	var response tagsResponse
	if err := sender.GetJSON(ctx, &response); err != nil {
		return nil, err
	}
	return response.Models, nil
}

// DecisionModelNames lists the installed models that can serve decision
// requests.
func (p *Provider) DecisionModelNames(ctx context.Context) ([]string, error) {
	models, err := p.Models(ctx)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(models))
	for _, model := range FilterDecisionModels(models) {
		names = append(names, model.ID())
	}
	return names, nil
}

// FilterDecisionModels returns the subset of models that support decisions:
// anything tagged with the "decision" capability, plus the nimble, tev, clef
// and jev families that older servers do not tag.
func FilterDecisionModels(models []Model) []Model {
	filtered := make([]Model, 0, len(models))
	for _, model := range models {
		if isDecisionModel(model) {
			filtered = append(filtered, model)
		}
	}
	return filtered
}

func isDecisionModel(model Model) bool {
	if model.HasCapability("decision") {
		return true
	}

	name := strings.ToLower(model.ID())
	for _, family := range []string{"nimble", "tev", "clef", "jev"} {
		if strings.Contains(name, family) {
			return true
		}
	}
	return false
}

// Version is the server version reported by /api/version.
type Version struct {
	Version string `json:"version"`
}

// ServerVersion reports the running Ollama version.
func (p *Provider) ServerVersion(ctx context.Context) (string, error) {
	p.mu.Lock()
	sender := *p.version
	p.mu.Unlock()

	var version Version
	if err := sender.GetJSON(ctx, &version); err != nil {
		return "", err
	}
	return version.Version, nil
}

// SupportsSystemOne reports whether the server is new enough for the decision
// endpoint, which requires Ollama v0.35.0 or later.
func (p *Provider) SupportsSystemOne(ctx context.Context) (bool, error) {
	raw, err := p.ServerVersion(ctx)
	if err != nil {
		return false, err
	}

	parsed, err := parseVersion(raw)
	if err != nil {
		return false, decide.NewError(Name, decide.KindDecode, fmt.Errorf("parse version %q: %w", raw, err))
	}
	return parsed.atLeast(0, 35, 0), nil
}

// version is a small comparable version triple.
type version struct {
	major, minor, patch int
}

func (v version) atLeast(major, minor, patch int) bool {
	switch {
	case v.major != major:
		return v.major > major
	case v.minor != minor:
		return v.minor > minor
	default:
		return v.patch >= patch
	}
}

func parseVersion(raw string) (version, error) {
	trimmed := strings.TrimPrefix(strings.TrimSpace(raw), "v")
	parts := strings.Split(trimmed, ".")
	if len(parts) == 0 {
		return version{}, fmt.Errorf("empty version")
	}

	out := version{}
	targets := []*int{&out.major, &out.minor, &out.patch}
	for i, part := range parts {
		if i >= len(targets) {
			break
		}
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			return version{}, err
		}
		*targets[i] = n
	}
	return out, nil
}
