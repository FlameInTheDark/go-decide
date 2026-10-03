package decide

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestQuestionMarshalChoice(t *testing.T) {
	question := Choice("label", "Which label?", Options{"bug": "Software errors", "billing": "Payments"})

	raw, err := json.Marshal(question)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if got["type"] != "choice" {
		t.Errorf("type = %v", got["type"])
	}
	if got["instructions"] != "Which label?" {
		t.Errorf("instructions = %v", got["instructions"])
	}
	criteria, ok := got["criteria"].(map[string]any)
	if !ok || criteria["bug"] != "Software errors" {
		t.Errorf("criteria = %v", got["criteria"])
	}
	if _, present := got["name"]; present {
		t.Error("name must not appear in the wire format, it is the map key")
	}
}

func TestQuestionMarshalScore(t *testing.T) {
	question := Score("urgency", "How urgent?", Scale{"Soon", "Now"})

	raw, err := json.Marshal(question)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	criteria, ok := got["criteria"].([]any)
	if !ok || len(criteria) != 2 || criteria[0] != "Soon" || criteria[1] != "Now" {
		t.Errorf("criteria = %v, want an ordered array", got["criteria"])
	}
}

func TestQuestionMarshalNoul(t *testing.T) {
	// Without outcomes the criteria key must be omitted entirely.
	raw, err := json.Marshal(Noul("ok", "Is it fine?"))
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(raw), "criteria") {
		t.Errorf("criteria should be omitted when unset: %s", raw)
	}

	raw, err = json.Marshal(Noul("ok", "Is it fine?", NoulCriteria{False: "Broken", True: "Fine"}))
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	criteria, ok := got["criteria"].(map[string]any)
	if !ok || criteria["false"] != "Broken" || criteria["true"] != "Fine" {
		t.Errorf("criteria = %v", got["criteria"])
	}
}

func TestQuestionRoundTrip(t *testing.T) {
	original := Noul("ok", "Is it fine?", NoulCriteria{False: "Broken", True: "Fine"})

	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded Question
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.Name != "" {
		t.Errorf("name = %q, want empty because it lives in the map key", decoded.Name)
	}
	if decoded.Noul == nil || decoded.Noul.True != "Fine" {
		t.Errorf("noul = %+v", decoded.Noul)
	}
}

func TestConstructorsMergeAndStayImmutable(t *testing.T) {
	base := Choice("label", "Which label?", Options{"a": "A"})
	extended := base.WithOption("b", "B")

	if len(base.Options) != 1 {
		t.Errorf("base was mutated: %v", base.Options)
	}
	if len(extended.Options) != 2 {
		t.Errorf("extended = %v", extended.Options)
	}

	merged := Choice("label", "Which label?", Options{"a": "A1"}, Options{"b": "B"})
	if merged.Options["a"] != "A1" || merged.Options["b"] != "B" {
		t.Errorf("merged = %v", merged.Options)
	}

	scaled := Score("s", "Scale?", Scale{"a"}, Scale{"b", "c"})
	if len(scaled.Scale) != 3 || scaled.Scale[2] != "c" {
		t.Errorf("scale = %v", scaled.Scale)
	}

	noul := Noul("n", "Noul?", NoulCriteria{False: "no"}, NoulCriteria{True: "yes"})
	if noul.Noul.False != "no" || noul.Noul.True != "yes" {
		t.Errorf("noul = %+v", noul.Noul)
	}

	// Empty fields must not wipe out previously set criteria.
	partial := Noul("n", "Noul?", NoulCriteria{False: "no", True: "yes"}).WithOutcomes("", "still yes")
	if partial.Noul.False != "no" || partial.Noul.True != "still yes" {
		t.Errorf("partial override = %+v", partial.Noul)
	}
}

func TestOptionKeysSorted(t *testing.T) {
	question := Choice("label", "Which?", Options{"c": "C", "a": "A", "b": "B"})

	got := question.OptionKeys()
	want := []string{"a", "b", "c"}

	if len(got) != len(want) {
		t.Fatalf("keys = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("keys = %v, want %v", got, want)
		}
	}
}

func TestSystemOnePayload(t *testing.T) {
	req := Request{
		Model:     "nimble",
		State:     Text("hello"),
		KeepAlive: 5 * time.Minute,
		Questions: []Question{Noul("a", "A?"), Noul("b", "B?")},
	}

	payload := req.SystemOnePayload()
	if len(payload.Questions) != 2 {
		t.Fatalf("questions = %v", payload.Questions)
	}
	if payload.Questions["a"].Instructions != "A?" {
		t.Errorf("question a = %+v", payload.Questions["a"])
	}
	if payload.KeepAlive != "5m0s" {
		t.Errorf("keep_alive = %q", payload.KeepAlive)
	}
	if payload.Model != "nimble" {
		t.Errorf("model = %q", payload.Model)
	}
}

func TestKeepAliveDuration(t *testing.T) {
	tests := []struct {
		name   string
		hint   time.Duration
		wantOK bool
		want   string
	}{
		{"unset", 0, false, ""},
		{"five minutes", 5 * time.Minute, true, "5m0s"},
		{"seconds", 300 * time.Second, true, "5m0s"},
		{"one minute", time.Minute, true, "1m0s"},
		{"negative keeps loaded", -1 * time.Second, true, "-1s"},
		{"sub second keeps its precision", 1500 * time.Millisecond, true, "1.5s"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := Request{KeepAlive: tc.hint}

			got, ok := req.KeepAliveDuration()
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if ok && got != tc.hint {
				t.Errorf("duration = %v, want %v", got, tc.hint)
			}
			if hint := keepAliveHint(tc.hint); hint != tc.want {
				t.Errorf("keep_alive = %q, want %q", hint, tc.want)
			}
		})
	}
}

func TestExtraIsMergedIntoPayload(t *testing.T) {
	req := Request{
		Model:     "nimble",
		State:     Text("hello"),
		Extra:     map[string]json.RawMessage{"temperature": json.RawMessage(`0.25`)},
		Questions: []Question{Noul("a", "A?")},
	}

	raw, err := json.Marshal(req.SystemOnePayload())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["temperature"] != 0.25 {
		t.Errorf("temperature = %v, want 0.25", got["temperature"])
	}
	if got["model"] != "nimble" {
		t.Errorf("model = %v, want nimble", got["model"])
	}
	if _, ok := got["questions"]; !ok {
		t.Error("questions missing from payload")
	}
}

func TestExtraOverridesKnownField(t *testing.T) {
	req := Request{
		Model:     "nimble",
		State:     Text("hello"),
		Extra:     map[string]json.RawMessage{"model": json.RawMessage(`"override"`)},
		Questions: []Question{Noul("a", "A?")},
	}

	raw, err := json.Marshal(req.SystemOnePayload())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["model"] != "override" {
		t.Errorf("model = %v, want override", got["model"])
	}
}
