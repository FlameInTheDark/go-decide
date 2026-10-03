package decide

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// MaxImageBytes is the largest single image accepted by the Ollama endpoint,
// which allows 32 MiB for image requests including base64 and JSON.
const MaxImageBytes = 32 << 20

// Image is a base64 encoded image shared by every question of a request.
// Build one with [ImageFromBytes] or [ImageFromFile].
type Image struct {
	// Base64 holds the encoded image, without a data URL prefix.
	Base64 string
	// MIMEType is the detected content type, e.g. "image/png". It is
	// informational: the wire format carries only base64 data.
	MIMEType string
}

// ImageFromBytes encodes raw image bytes for transport.
func ImageFromBytes(data []byte) Image {
	return Image{Base64: base64.StdEncoding.EncodeToString(data), MIMEType: http.DetectContentType(data)}
}

// ImageFromFile reads a file from disk and encodes it for transport.
func ImageFromFile(path string) (Image, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Image{}, fmt.Errorf("decide: read image: %w", err)
	}
	return ImageFromBytes(data), nil
}

// ApproxSize returns the encoded size of the image in bytes.
func (i Image) ApproxSize() int { return len(i.Base64) }

// Validate reports whether the image can be sent.
func (i Image) Validate(v *ValidationError, index int) {
	if i.Base64 == "" {
		v.Add("image %d is empty", index)
		return
	}
	if strings.HasPrefix(i.Base64, "data:") {
		v.Add("image %d must be raw base64, data URLs are not supported", index)
	}
	if _, err := base64.StdEncoding.DecodeString(i.Base64); err != nil {
		v.Add("image %d is not valid base64: %v", index, err)
	}
	if size := i.ApproxSize(); size > MaxImageBytes {
		v.Add("image %d is %d bytes, the maximum is %d", index, size, MaxImageBytes)
	}
}

// MarshalJSON implements [json.Marshaler]: images travel as base64 strings.
func (i Image) MarshalJSON() ([]byte, error) { return json.Marshal(i.Base64) }

// UnmarshalJSON implements [json.Unmarshaler].
func (i *Image) UnmarshalJSON(data []byte) error { return json.Unmarshal(data, &i.Base64) }

// Request is a decision request. Adapters translate it into their own wire
// format; providers that speak the System One dialect can use
// [Request.SystemOnePayload] directly.
type Request struct {
	// Model is the decision model to use, e.g. "nimble" or
	// "typesafe/jev-1.13". Adapters may provide a default.
	Model string
	// State is the content to evaluate.
	State State
	// Questions are the typed questions to answer.
	Questions []Question
	// Images are optional base64 images shared by all questions, in order.
	// They require a vision capable model such as Ollama's clef.
	Images []Image
	// KeepAlive is an Ollama hint controlling how long the model stays
	// loaded, e.g. "5m" or "300" for seconds. Zero unloads the model and a
	// negative value keeps it loaded. Other providers ignore it.
	KeepAlive string
	// SessionID groups related requests for provider observability.
	SessionID string
	// User identifies the end user on providers that accept it.
	User string
	// Trace carries observability metadata such as "trace_id" or
	// "trace_name".
	Trace map[string]string
	// Extra carries provider specific fields that have no counterpart in
	// this package. Keys are merged into the request body as-is.
	Extra map[string]json.RawMessage
}

// SystemOnePayload is the request body shared by the System One endpoints
// (Ollama's /v1/systemone and OpenRouter's /api/alpha/decisions). Adapters for
// that dialect embed it and add their own fields; see the provider packages.
type SystemOnePayload struct {
	// Model is the requested model.
	Model string `json:"model"`
	// State is the content to evaluate.
	State State `json:"state"`
	// Questions maps question names to typed questions.
	Questions map[string]Question `json:"questions"`
	// Images are base64 images shared by all questions.
	Images []Image `json:"images,omitempty"`
	// KeepAlive is the Ollama model residency hint. A zero duration omits
	// the field so the server default applies.
	KeepAlive string `json:"keep_alive,omitempty"`
}

// SystemOnePayload converts the request into the shared wire payload. Optional
// fields are normalised so that adapters can override them.
func (r Request) SystemOnePayload() SystemOnePayload {
	payload := SystemOnePayload{
		Model:     r.Model,
		State:     r.State,
		Questions: make(map[string]Question, len(r.Questions)),
		Images:    r.Images,
		KeepAlive: r.KeepAlive,
	}
	for _, question := range r.Questions {
		payload.Questions[question.Name] = question
	}
	return payload
}

// Validate checks the request for problems every provider would reject.
func (r Request) Validate() error {
	v := &ValidationError{}

	if r.State.IsZero() {
		v.Add("state must be a non-empty string, object or array")
	}
	if len(r.Questions) == 0 {
		v.Add("at least one question is required")
	}

	seen := make(map[string]struct{}, len(r.Questions))
	for _, question := range r.Questions {
		question.Validate(v)
		if name := strings.TrimSpace(question.Name); name != "" {
			if _, dup := seen[name]; dup {
				v.Add("duplicate question name %q", name)
			}
			seen[name] = struct{}{}
		}
	}

	for i, image := range r.Images {
		image.Validate(v, i)
	}

	return v.OrNil()
}

// KeepAliveDuration converts the KeepAlive hint into a duration. It accepts
// either a Go duration string such as "5m" or a plain number of seconds.
// Returns ok=false when the hint is empty or malformed.
func (r Request) KeepAliveDuration() (time.Duration, bool) {
	hint := strings.TrimSpace(r.KeepAlive)
	if hint == "" {
		return 0, false
	}
	if secs, err := strconv.ParseFloat(hint, 64); err == nil {
		return time.Duration(secs * float64(time.Second)), true
	}
	if d, err := time.ParseDuration(hint); err == nil {
		return d, true
	}
	return 0, false
}
