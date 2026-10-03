package decide

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestValidateQuestion(t *testing.T) {
	tests := []struct {
		name     string
		question Question
		wantErr  string
	}{
		{"valid choice", Choice("l", "Which?", Options{"a": "A", "b": "B"}), ""},
		{"valid noul", Noul("n", "Yes?"), ""},
		{"valid score", Score("s", "How?", Scale{"a", "b"}), ""},
		{"no name", Choice("", "Which?", Options{"a": "A", "b": "B"}), "question name"},
		{"no instructions", Choice("l", "", Options{"a": "A", "b": "B"}), "instructions"},
		{"too few options", Choice("l", "Which?", Options{"a": "A"}), "between 2 and 26"},
		{"too many options", Choice("l", "Which?", manyOptions(27)), "between 2 and 26"},
		{"empty option description", Choice("l", "Which?", Options{"a": "", "b": "B"}), "needs a description"},
		{"unknown type", Question{Name: "x", Type: "mystery", Instructions: "?"}, "unknown type"},
		{"too few levels", Score("s", "How?", Scale{"a"}), "between 2 and 26"},
		{"empty level", Score("s", "How?", Scale{"a", "  "}), "must not be empty"},
		{"noul with options", Question{Name: "n", Type: TypeNoul, Instructions: "?", Options: Options{"false": "No"}}, "NoulCriteria"},
		{"choice with scale", Question{Name: "c", Type: TypeChoice, Instructions: "?", Scale: Scale{"a", "b"}}, "not Scale"},
		{"score with options", Question{Name: "s", Type: TypeScore, Instructions: "?", Options: Options{"a": "A"}}, "not Options"},
		{"empty noul outcomes", Noul("n", "Yes?", NoulCriteria{}), "both empty"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			problems := &ValidationError{}
			tc.question.Validate(problems)
			err := problems.OrNil()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected an error mentioning %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want it to mention %q", err, tc.wantErr)
			}
			if !errors.Is(err, ErrInvalidRequest) {
				t.Error("error should match ErrInvalidRequest")
			}
		})
	}
}

// manyOptions builds n uniquely keyed options.
func manyOptions(n int) Options {
	options := make(Options, n)
	for i := range n {
		options[fmt.Sprintf("opt%02d", i)] = "description"
	}
	return options
}

func TestValidateRequest(t *testing.T) {
	valid := Request{
		Model:     "nimble",
		State:     Text("hello"),
		Questions: []Question{Noul("ok", "Fine?")},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tests := []struct {
		name string
		req  Request
	}{
		{"missing state", Request{Model: "m", Questions: []Question{Noul("ok", "Fine?")}}},
		{"blank state", Request{Model: "m", State: Text("   "), Questions: []Question{Noul("ok", "Fine?")}}},
		{"missing questions", Request{Model: "m", State: Text("hi")}},
		{"duplicate names", Request{Model: "m", State: Text("hi"), Questions: []Question{
			Noul("same", "A?"), Noul("same", "B?"),
		}}},
		{"bad image", Request{Model: "m", State: Text("hi"), Questions: []Question{Noul("ok", "Fine?")},
			Images: []Image{{Base64: "not base64!!"}}}},
		{"empty image", Request{Model: "m", State: Text("hi"), Questions: []Question{Noul("ok", "Fine?")},
			Images: []Image{{}}}},
		{"data url image", Request{Model: "m", State: Text("hi"), Questions: []Question{Noul("ok", "Fine?")},
			Images: []Image{{Base64: "data:image/png;base64,AAAA"}}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.req.Validate()
			if err == nil {
				t.Fatal("expected an error")
			}
			if !errors.Is(err, ErrInvalidRequest) {
				t.Errorf("error should match ErrInvalidRequest: %v", err)
			}
		})
	}
}

func TestValidationErrorCollectsEverything(t *testing.T) {
	err := Request{State: Text(" ")}.Validate()
	if err == nil {
		t.Fatal("expected an error")
	}

	var vErr *ValidationError
	if !errors.As(err, &vErr) {
		t.Fatalf("error is not a *ValidationError: %v", err)
	}
	if len(vErr.Problems) < 2 {
		t.Errorf("problems = %v, want at least 2", vErr.Problems)
	}
	if vErr.Error() == "" {
		t.Error("message is empty")
	}
}
