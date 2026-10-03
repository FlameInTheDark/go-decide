package decide

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStateMarshal(t *testing.T) {
	tests := []struct {
		name  string
		state State
		want  string
	}{
		{"text", Text("hello"), `"hello"`},
		{"object", Object(map[string]any{"a": "b"}), `{"a":"b"}`},
		{"list", List([]any{"a", 1.0}), `["a",1]`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(tc.state)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if string(raw) != tc.want {
				t.Errorf("got %s, want %s", raw, tc.want)
			}
		})
	}
}

func TestStateIsZero(t *testing.T) {
	tests := []struct {
		name  string
		state State
		want  bool
	}{
		{"unset", State{}, true},
		{"empty text", Text(""), true},
		{"blank text", Text("  \n "), true},
		{"text", Text("hi"), false},
		{"empty object", Object(map[string]any{}), false},
		{"empty list", List(nil), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.state.IsZero(); got != tc.want {
				t.Errorf("IsZero = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestStateUnmarshal(t *testing.T) {
	var state State
	if err := json.Unmarshal([]byte(`{"ticket":"broken"}`), &state); err != nil {
		t.Fatalf("Unmarshal object: %v", err)
	}
	if state.String() != `{"ticket":"broken"}` {
		t.Errorf("state = %s", state.String())
	}

	if err := json.Unmarshal([]byte(`"plain text"`), &state); err != nil {
		t.Fatalf("Unmarshal string: %v", err)
	}
	if state.String() != `"plain text"` {
		t.Errorf("state = %s", state.String())
	}

	if err := json.Unmarshal([]byte(`[1,2]`), &state); err != nil {
		t.Fatalf("Unmarshal array: %v", err)
	}
	if state.String() != `[1,2]` {
		t.Errorf("state = %s", state.String())
	}

	if err := json.Unmarshal([]byte(`null`), &state); err != nil {
		t.Fatalf("Unmarshal null: %v", err)
	}
	if !state.IsZero() {
		t.Error("null should reset the state to the zero value")
	}
}

func TestStateUnmarshalRejectsScalars(t *testing.T) {
	var state State
	err := json.Unmarshal([]byte(`42`), &state)
	if err == nil {
		t.Fatal("expected an error for a numeric state")
	}
	if !strings.Contains(err.Error(), "string, object or array") {
		t.Errorf("error = %v", err)
	}
}

func TestJSONHelper(t *testing.T) {
	state, err := JSON(struct {
		Ticket string `json:"ticket"`
	}{Ticket: "broken"})
	if err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if state.String() != `{"ticket":"broken"}` {
		t.Errorf("state = %s", state.String())
	}

	if _, err := JSON(map[string]string{"a": "b"}); err != nil {
		t.Errorf("map should be accepted: %v", err)
	}
	if _, err := JSON([]string{"a"}); err != nil {
		t.Errorf("slice should be accepted: %v", err)
	}
	if _, err := JSON("plain"); err != nil {
		t.Errorf("string should be accepted: %v", err)
	}
	if _, err := JSON(42); err == nil {
		t.Error("numbers must be rejected")
	}
	if _, err := JSON(nil); err == nil {
		t.Error("nil must be rejected")
	}
}

func TestImageFromBytes(t *testing.T) {
	image := ImageFromBytes([]byte("hello"))

	if image.Base64 != "aGVsbG8=" {
		t.Errorf("base64 = %q", image.Base64)
	}
	if image.MIMEType == "" {
		t.Error("mime type should be detected")
	}

	raw, err := json.Marshal(image)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(raw) != `"aGVsbG8="` {
		t.Errorf("wire format = %s", raw)
	}
}

func TestAnswerHelpers(t *testing.T) {
	answers := NewAnswers([]Answer{
		ChoiceAnswer{
			QuestionName:  "label",
			Key:           "bug",
			Probabilities: map[string]float64{"bug": 0.7, "billing": 0.2, "account": 0.1},
			Confidence:    0.6,
		},
		NoulAnswer{QuestionName: "ok", Probability: 0.96},
		ScoreAnswer{
			QuestionName:  "urgency",
			Score:         1.4,
			Legend:        map[string]string{"0": "low", "1": "mid", "2": "high"},
			Probabilities: map[string]float64{"0": 0.1, "1": 0.4, "2": 0.5},
			Confidence:    0.3,
		},
	})

	if answers.Len() != 3 {
		t.Errorf("len = %d", answers.Len())
	}

	if _, err := answers.Get("missing"); err == nil {
		t.Error("expected an error for a missing answer")
	}

	if _, err := answers.Choice("ok"); err == nil {
		t.Error("expected a type mismatch error")
	}
	if _, err := answers.Noul("label"); err == nil {
		t.Error("expected a type mismatch error")
	}

	choice, err := answers.Choice("label")
	if err != nil {
		t.Fatalf("Choice: %v", err)
	}
	if choice.Name() != "label" || choice.Type() != TypeChoice {
		t.Errorf("choice = %+v", choice)
	}
	if choice.Best() != "bug" {
		t.Errorf("best = %q", choice.Best())
	}
	if m := choice.Margin(); m < 0.49 || m > 0.51 {
		t.Errorf("margin = %v, want 0.5", m)
	}
	if ranked := choice.Ranked(); ranked[0].Key != "bug" || ranked[2].Key != "account" {
		t.Errorf("ranked = %+v", ranked)
	}
	if choice.String() == "" {
		t.Error("String is empty")
	}

	noul, err := answers.Noul("ok")
	if err != nil {
		t.Fatalf("Noul: %v", err)
	}
	if !noul.True() || noul.False() {
		t.Errorf("noul = %+v", noul)
	}

	score, err := answers.Score("urgency")
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if score.Max() != 2 || score.Best() != 2 {
		t.Errorf("score = %+v", score)
	}
	if score.Level() != 1 {
		t.Errorf("level = %d, want 1 for 1.4", score.Level())
	}
	if score.Description(score.Best()) != "high" {
		t.Errorf("description = %q", score.Description(score.Best()))
	}
	if score.Type() != TypeScore || score.Name() != "urgency" {
		t.Errorf("score identity = %s/%s", score.Name(), score.Type())
	}
}

func TestResultMissing(t *testing.T) {
	result := &Result{
		Answers: NewAnswers([]Answer{NoulAnswer{QuestionName: "present"}}),
	}

	missing := result.Missing([]Question{Noul("present", "?"), Noul("absent", "?")})
	if len(missing) != 1 || missing[0] != "absent" {
		t.Errorf("missing = %v", missing)
	}
}

func TestResultTypedAccessorsOnNil(t *testing.T) {
	var result *Result
	if _, err := result.Answer("x"); err == nil {
		t.Error("expected an error from a nil result")
	}
}
