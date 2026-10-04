package playground

import (
	"encoding/json"
	"errors"

	"github.com/gofiber/fiber/v3"
)

// jsonError is a malformed request body. It carries a 400 so a broken JSON
// payload is never confused with a provider failure.
type jsonError struct {
	err error
}

func (e *jsonError) Error() string {
	return "the request body is not valid JSON: " + e.err.Error()
}

// Status reports the HTTP status for a malformed body.
func (e *jsonError) Status() int { return fiber.StatusBadRequest }

// asJSONError extracts a jsonError from err.
func asJSONError(err error) (*jsonError, bool) {
	typed, ok := err.(*jsonError)
	return typed, ok
}

// readBody returns the raw request body. The result is reused by the history,
// so the history keeps the exact bytes the client sent.
func readBody(c fiber.Ctx) ([]byte, error) {
	body := c.Body()
	if len(body) == 0 {
		return nil, &jsonError{err: errors.New("the body is empty")}
	}
	return body, nil
}

// bindJSON decodes the request body into out.
func bindJSON(c fiber.Ctx, out any) error {
	if err := c.Bind().JSON(out); err != nil {
		return &jsonError{err: err}
	}
	return nil
}

// decodeJSON decodes raw bytes, wrapping failures as a client error.
func decodeJSON(raw []byte, out any) error {
	if len(raw) == 0 {
		return &jsonError{err: errors.New("the body is empty")}
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return &jsonError{err: err}
	}
	return nil
}

// openRouterDefaultBaseURL is repeated here rather than imported so the
// config endpoint does not need the provider package at all.
const openRouterDefaultBaseURL = "https://openrouter.ai"
