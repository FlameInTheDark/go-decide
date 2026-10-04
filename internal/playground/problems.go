package playground

import (
	"strings"

	decide "github.com/FlameInTheDark/go-decide"
)

// Problem prefixes used when a wire payload does not line up with the library's
// own expectation. They identify which part of the UI to blame, so a form can
// highlight the right field instead of showing a sentence with no anchor.
const (
	prefixState     = "state"
	prefixQuestions = "questions"
)

// problem is one validation failure, split into an anchor and a message.
type problem struct {
	// Anchor names the part of the payload at fault: "state",
	// "questions[2]", or "" for a problem about the request as a whole.
	Anchor string
	// Message is the library's own text, never reworded.
	Message string
}

// String renders the problem as "<anchor>: <message>", or just the message when
// there is no anchor.
func (p problem) String() string {
	if p.Anchor == "" {
		return p.Message
	}
	return p.Anchor + ": " + p.Message
}

// problemsError is the validation failure produced while converting a wire
// payload into a library request. It carries the library's own messages, each
// anchored to the field it came from.
type problemsError struct {
	Problems []problem
}

// Error implements the error interface.
func (e *problemsError) Error() string {
	flat := make([]string, 0, len(e.Problems))
	for _, p := range e.Problems {
		flat = append(flat, p.String())
	}
	return decide.ErrInvalidRequest.Error() + ": " + strings.Join(flat, "; ")
}

// Fields returns the payload anchors that had problems, so the UI can
// highlight them.
func (e *problemsError) Fields() []string {
	fields := make([]string, 0, len(e.Problems))
	for _, p := range e.Problems {
		if p.Anchor != "" {
			fields = append(fields, p.Anchor)
		}
	}
	return fields
}

// splitProblems separates an error's problem list into anchored problems,
// attributing each one to the part of the payload it came from.
func splitProblems(err error, questions []string) []problem {
	var validation *decide.ValidationError
	if !asValidation(err, &validation) {
		return []problem{{Message: err.Error()}}
	}

	// Each library message names its question, so the anchor is recovered by
	// matching that name against the payload's question list.
	out := make([]problem, 0, len(validation.Problems))
	for _, message := range validation.Problems {
		out = append(out, problem{Anchor: anchorFor(message, questions), Message: message})
	}
	return out
}

// anchorFor recovers the payload anchor from a library problem message. The
// library formats a question problem as: question "name": <what is wrong>.
func anchorFor(message string, questions []string) string {
	const marker = `question "`
	if start := strings.Index(message, marker); start >= 0 {
		rest := message[start+len(marker):]
		if end := strings.Index(rest, `"`); end > 0 {
			name := rest[:end]
			for i, candidate := range questions {
				if candidate == name {
					return "questions[" + itoa(i) + "]"
				}
			}
			return prefixQuestions
		}
	}
	return ""
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
