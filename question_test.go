package decide

import (
	"encoding/json"
	"strings"
	"testing"
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
		KeepAlive: "5m",
		Questions: []Question{Noul("a", "A?"), Noul("b", "B?")},
	}

	payload := req.SystemOnePayload()
	if len(payload.Questions) != 2 {
		t.Fatalf("questions = %v", payload.Questions)
	}
	if payload.Questions["a"].Instructions != "A?" {
		t.Errorf("question a = %+v", payload.Questions["a"])
	}
	if payload.KeepAlive != "5m" {
		t.Errorf("keep_alive = %q", payload.KeepAlive)
	}
	if payload.Model != "nimble" {
		t.Errorf("model = %q", payload.Model)
	}
}

func TestKeepAliveDuration(t *testing.T) {
	tests := []struct {
		hint    string
		wantOK  bool
		wantSec float64
	}{
		{"", false, 0},
		{"5m", true, 300},
		{"300", true, 300},
		{"-1", true, -1},
		{"nonsense", false, 0},
	}

	for _, tc := range tests {
		t.Run(tc.hint, func(t *testing.T) {
			got, ok := (Request{KeepAlive: tc.hint}).KeepAliveDuration()
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if ok && got.Seconds() != tc.wantSec {
				t.Errorf("duration = %v, want %v s", got.Seconds(), tc.wantSec)
			}
		})
	}
}
