package main

import (
	"encoding/json"
	"fmt"
	"io"

	decide "github.com/FlameInTheDark/go-decide"
)

// jsonReport is the machine readable form of a classification, written by
// `decide classify --json`. It repeats the questions alongside the answers so
// the document is self describing: a consumer can tell a choice question from
// a score question, and recover option keys and scale labels, without also
// having to keep the original command line.
type jsonReport struct {
	Provider      string                `json:"provider"`
	Model         string                `json:"model"`
	UpstreamModel string                `json:"upstream_model,omitempty"`
	Upstream      string                `json:"upstream,omitempty"`
	ID            string                `json:"id,omitempty"`
	Questions     []jsonQuestion        `json:"questions"`
	Answers       map[string]jsonAnswer `json:"answers"`
	Usage         decide.Usage          `json:"usage"`
}

// jsonQuestion describes one asked question.
type jsonQuestion struct {
	Name         string            `json:"name"`
	Type         string            `json:"type"`
	Instructions string            `json:"instructions,omitempty"`
	Options      map[string]string `json:"options,omitempty"`
	Scale        []string          `json:"scale,omitempty"`
}

// jsonAnswer is one answer in its natural shape.
type jsonAnswer struct {
	Type          string             `json:"type"`
	Key           string             `json:"key,omitempty"`
	Noul          *float64           `json:"noul,omitempty"`
	Score         *float64           `json:"score,omitempty"`
	Legend        map[string]string  `json:"legend,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Confidence    *float64           `json:"confidence,omitempty"`
}

// jsonServer describes the Ollama server behind `decide models`.
type jsonServer struct {
	BaseURL           string `json:"base_url"`
	Version           string `json:"version,omitempty"`
	SupportsSystemOne bool   `json:"supports_system_one"`
}

// jsonModels is the machine readable form of `decide models --json`.
type jsonModels struct {
	Server  jsonServer  `json:"server"`
	Models  []jsonModel `json:"models"`
	Missing []string    `json:"missing,omitempty"`
}

// jsonModel is one installed model.
type jsonModel struct {
	Name          string   `json:"name"`
	Decision      bool     `json:"decision"`
	Vision        bool     `json:"vision"`
	Cloud         bool     `json:"cloud"`
	SizeBytes     int64    `json:"size_bytes,omitempty"`
	Family        string   `json:"family,omitempty"`
	ContextLength int      `json:"context_length,omitempty"`
	Capabilities  []string `json:"capabilities,omitempty"`
	Found         bool     `json:"found"`
}

// writeJSON encodes v as indented JSON.
func writeJSON(out io.Writer, v any) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(v); err != nil {
		return &exitCodeError{message: fmt.Sprintf("encode result: %v", err), code: exitFailure}
	}
	return nil
}

// buildReport turns a request and its result into the JSON document.
func buildReport(req decide.Request, result *decide.Result) jsonReport {
	report := jsonReport{
		Provider:      result.Provider,
		Model:         result.Model,
		UpstreamModel: result.UpstreamModel,
		Upstream:      result.Upstream,
		ID:            result.ID,
		Usage:         result.Usage,
		Answers:       map[string]jsonAnswer{},
	}

	for _, question := range req.Questions {
		report.Questions = append(report.Questions, jsonQuestion{
			Name:         question.Name,
			Type:         string(question.Type),
			Instructions: question.Instructions,
			Options:      question.Options,
			Scale:        question.Scale,
		})

		answer, err := result.Answers.Get(question.Name)
		if err != nil {
			continue
		}

		switch typed := answer.(type) {
		case decide.ChoiceAnswer:
			confidence := typed.Confidence
			report.Answers[question.Name] = jsonAnswer{
				Type:          string(decide.TypeChoice),
				Key:           typed.Key,
				Probabilities: typed.Probabilities,
				Confidence:    &confidence,
			}
		case decide.NoulAnswer:
			probability := typed.Probability
			report.Answers[question.Name] = jsonAnswer{
				Type: string(decide.TypeNoul),
				Noul: &probability,
			}
		case decide.ScoreAnswer:
			score := typed.Score
			confidence := typed.Confidence
			report.Answers[question.Name] = jsonAnswer{
				Type:          string(decide.TypeScore),
				Score:         &score,
				Legend:        typed.Legend,
				Probabilities: typed.Probabilities,
				Confidence:    &confidence,
			}
		}
	}

	return report
}

// buildModelsReport converts an installed model list for JSON output.
func buildModelsReport(server jsonServer, models []ollamaModelView, missing []string) jsonModels {
	report := jsonModels{Server: server, Missing: missing}
	for _, model := range models {
		report.Models = append(report.Models, jsonModel{
			Name:          model.id,
			Decision:      model.decision,
			Vision:        model.vision,
			Cloud:         model.cloud,
			SizeBytes:     model.size,
			Family:        model.family,
			ContextLength: model.contextLength,
			Capabilities:  model.capabilities,
			Found:         true,
		})
	}
	for _, name := range missing {
		report.Models = append(report.Models, jsonModel{Name: name, Found: false})
	}
	return report
}
