package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	decide "github.com/FlameInTheDark/go-decide"
)

// questionFile is the on-disk shape of a questions file, for example
// examples/triage.json:
//
//	{
//	  "questions": [
//	    {
//	      "name": "category",
//	      "type": "choice",
//	      "instructions": "Which team should own this?",
//	      "options": { "bug": "Broken software" }
//	    },
//	    {
//	      "name": "refund",
//	      "type": "noul",
//	      "instructions": "Is money back requested?",
//	      "outcomes": { "false": "No refund requested", "true": "A refund is requested" }
//	    },
//	    {
//	      "name": "urgency",
//	      "type": "score",
//	      "instructions": "How urgent is this?",
//	      "scale": ["Annoying", "Blocking"]
//	    }
//	  ]
//	}
type questionFile struct {
	Questions []fileQuestion `json:"questions"`
}

// fileQuestion is one question in a questions file. Only the field matching
// Type carries criteria.
type fileQuestion struct {
	Name         string            `json:"name"`
	Type         decide.Type       `json:"type"`
	Instructions string            `json:"instructions"`
	Options      map[string]string `json:"options,omitempty"`
	Outcomes     *struct {
		False string `json:"false"`
		True  string `json:"true"`
	} `json:"outcomes,omitempty"`
	Scale []string `json:"scale,omitempty"`
}

// loadQuestionsFile reads questions from a JSON file. The resulting questions
// are validated with the same rules the library applies, so a malformed file
// reports the same errors as the equivalent command line.
func loadQuestionsFile(path string) ([]decide.Question, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, usageError("read questions file: %v", err)
	}

	var file questionFile
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&file); err != nil {
		return nil, usageError("parse %s: %v", path, err)
	}
	if len(file.Questions) == 0 {
		return nil, usageError("%s contains no questions", path)
	}

	questions := make([]decide.Question, 0, len(file.Questions))
	for i, entry := range file.Questions {
		question, err := entry.question(path, i)
		if err != nil {
			return nil, err
		}
		questions = append(questions, question)
	}

	// Reuse the library validation so a file and the equivalent command line
	// behave identically. Only the questions are checked here: the state is
	// validated separately and may still be unknown at this point.
	if err := validateQuestionSet(questions); err != nil {
		return nil, usageError("%s: %v", path, err)
	}

	return questions, nil
}

// validateQuestionSet applies the library rules to a whole question set,
// including the duplicate-name rule that only exists for a full request.
func validateQuestionSet(questions []decide.Question) error {
	v := &decide.ValidationError{}
	seen := make(map[string]struct{}, len(questions))

	for _, question := range questions {
		question.Validate(v)
		if name := question.Name; name != "" {
			if _, dup := seen[name]; dup {
				v.Add("duplicate question name %q", name)
				continue
			}
			seen[name] = struct{}{}
		}
	}

	return v.OrNil()
}

// question converts one file entry into a decision question.
func (f fileQuestion) question(path string, index int) (decide.Question, error) {
	invalid := func(err error) (decide.Question, error) {
		return decide.Question{}, usageError("%s: questions[%d]: %v", path, index, err)
	}

	switch f.Type {
	case decide.TypeChoice:
		if len(f.Options) == 0 {
			return invalid(fmt.Errorf("a choice question needs an %q object", "options"))
		}
		return decide.Choice(f.Name, f.Instructions, f.Options), nil

	case decide.TypeNoul:
		question := decide.Noul(f.Name, f.Instructions)
		if f.Outcomes != nil {
			question = question.WithOutcomes(f.Outcomes.False, f.Outcomes.True)
		}
		return question, nil

	case decide.TypeScore:
		if len(f.Scale) == 0 {
			return invalid(fmt.Errorf("a score question needs a %q array", "scale"))
		}
		return decide.Score(f.Name, f.Instructions, decide.Scale(f.Scale)), nil

	default:
		return invalid(fmt.Errorf("unknown type %q, want %q, %q or %q",
			f.Type, decide.TypeChoice, decide.TypeNoul, decide.TypeScore))
	}
}

// loadStateFile reads the state to evaluate. A .json file is decoded into a
// structured state so that objects and arrays reach the model intact; anything
// else is sent as plain text.
func loadStateFile(path string) (decide.State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return decide.State{}, usageError("read state file: %v", err)
	}

	if strings.EqualFold(filepath.Ext(path), ".json") {
		var raw any
		if err := json.Unmarshal(data, &raw); err != nil {
			return decide.State{}, usageError("parse %s: %v", path, err)
		}
		state, err := decide.JSON(raw)
		if err != nil {
			return decide.State{}, usageError("parse %s: %v", path, err)
		}
		return state, nil
	}

	if strings.TrimSpace(string(data)) == "" {
		return decide.State{}, usageError("state file %q is empty", path)
	}
	return decide.Text(string(data)), nil
}
