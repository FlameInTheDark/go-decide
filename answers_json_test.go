package decide

import (
	"encoding/json"
	"math"
	"testing"
)

func TestAnswersMarshalJSON(t *testing.T) {
	answers := NewAnswers([]Answer{
		ChoiceAnswer{
			QuestionName:  "label",
			Key:           "bug",
			Probabilities: map[string]float64{"bug": 0.97, "billing": 0.03},
			Confidence:    0.88,
		},
		NoulAnswer{QuestionName: "ok", Probability: 0.96},
		ScoreAnswer{
			QuestionName:  "urgency",
			Score:         1.99,
			Legend:        map[string]string{"0": "low", "1": "high"},
			Probabilities: map[string]float64{"0": 0.01, "1": 0.99},
			Confidence:    0.5,
		},
	})

	raw, err := json.Marshal(answers)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded map[string]map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if len(decoded) != 3 {
		t.Fatalf("answers = %v", decoded)
	}

	choice := decoded["label"]
	if choice["type"] != "choice" || choice["key"] != "bug" {
		t.Errorf("choice = %v", choice)
	}
	if math.Abs(choice["confidence"].(float64)-0.88) > 0.0001 {
		t.Errorf("confidence = %v", choice["confidence"])
	}
	probabilities, ok := choice["probabilities"].(map[string]any)
	if !ok || probabilities["bug"] == nil {
		t.Errorf("probabilities = %v", choice["probabilities"])
	}

	noul := decoded["ok"]
	if noul["type"] != "noul" || noul["noul"] != 0.96 {
		t.Errorf("noul = %v", noul)
	}

	score := decoded["urgency"]
	if score["type"] != "score" || score["score"] != 1.99 {
		t.Errorf("score = %v", score)
	}
	legend, ok := score["legend"].(map[string]any)
	if !ok || legend["0"] != "low" {
		t.Errorf("legend = %v", score["legend"])
	}
}

func TestAnswersMarshalEmpty(t *testing.T) {
	raw, err := json.Marshal(NewAnswers(nil))
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(raw) != "{}" {
		t.Errorf("got %s, want {}", raw)
	}
}

func TestResultMarshalJSON(t *testing.T) {
	result := &Result{
		Provider: "ollama",
		Model:    "nimble",
		Answers: NewAnswers([]Answer{
			NoulAnswer{QuestionName: "ok", Probability: 0.5},
		}),
		Usage: Usage{InputTokens: 10, OutputTokens: 2, TotalTokens: 12},
	}

	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded struct {
		Provider string                    `json:"provider"`
		Answers  map[string]map[string]any `json:"answers"`
		Usage    Usage                     `json:"usage"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.Provider != "ollama" {
		t.Errorf("provider = %q", decoded.Provider)
	}
	if decoded.Answers["ok"]["noul"] != 0.5 {
		t.Errorf("answers = %v", decoded.Answers)
	}
	if decoded.Usage.TotalTokens != 12 {
		t.Errorf("usage = %+v", decoded.Usage)
	}
}
