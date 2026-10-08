package playground

import (
	"bytes"
	"encoding/json"

	decide "github.com/FlameInTheDark/go-decide"
)

// renderPrompt renders the state of a decision request as readable text.
//
// The System One endpoints compose the real prompt inside the model server,
// which never sends it back, so nothing here is the model's prompt verbatim.
// What is worth reading back is the state: it is the content every question is
// answered from, and the thing a stored run cannot be interpreted without.
func renderPrompt(state decide.State) string {
	compact := state.String()
	if compact == "" {
		return "(empty)"
	}

	// A text state marshals to a JSON string. Decoding it here keeps the
	// quotes and escapes out of the text, which is meant to be read.
	if compact[0] == '"' {
		var text string
		if err := json.Unmarshal([]byte(compact), &text); err == nil {
			return text
		}
	}

	// An object or array state is already the JSON a reader expects, so it
	// only needs indenting.
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(compact), "", "  "); err != nil {
		return compact
	}
	return buf.String()
}
