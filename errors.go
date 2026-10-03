package decide

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Sentinel errors for use with [errors.Is]. Every *Error matches the sentinel
// that corresponds to its [Kind].
var (
	ErrInvalidRequest  = errors.New("decide: invalid request")
	ErrAuth            = errors.New("decide: authentication failed")
	ErrPayment         = errors.New("decide: insufficient credits")
	ErrForbidden       = errors.New("decide: forbidden")
	ErrNotFound        = errors.New("decide: not found")
	ErrPayloadTooLarge = errors.New("decide: payload too large")
	ErrRateLimited     = errors.New("decide: rate limited")
	ErrServer          = errors.New("decide: server error")
	ErrUnavailable     = errors.New("decide: service unavailable")
	ErrTimeout         = errors.New("decide: request timed out")
	ErrDecode          = errors.New("decide: cannot decode provider response")
	ErrUnsupported     = errors.New("decide: unsupported capability")
)

// Kind is a provider-independent classification of a failure.
type Kind int

const (
	KindUnknown Kind = iota
	KindInvalidRequest
	KindAuth
	KindPayment
	KindForbidden
	KindNotFound
	KindPayloadTooLarge
	KindRateLimited
	KindServer
	KindUnavailable
	KindTimeout
	KindDecode
	KindUnsupported
)

var kindNames = map[Kind]string{
	KindUnknown:         "unknown",
	KindInvalidRequest:  "invalid_request",
	KindAuth:            "authentication",
	KindPayment:         "payment",
	KindForbidden:       "forbidden",
	KindNotFound:        "not_found",
	KindPayloadTooLarge: "payload_too_large",
	KindRateLimited:     "rate_limited",
	KindServer:          "server_error",
	KindUnavailable:     "unavailable",
	KindTimeout:         "timeout",
	KindDecode:          "decode_error",
	KindUnsupported:     "unsupported",
}

// String implements [fmt.Stringer].
func (k Kind) String() string {
	if name, ok := kindNames[k]; ok {
		return name
	}
	return "unknown"
}

func (k Kind) sentinel() error {
	switch k {
	case KindInvalidRequest:
		return ErrInvalidRequest
	case KindAuth:
		return ErrAuth
	case KindPayment:
		return ErrPayment
	case KindForbidden:
		return ErrForbidden
	case KindNotFound:
		return ErrNotFound
	case KindPayloadTooLarge:
		return ErrPayloadTooLarge
	case KindRateLimited:
		return ErrRateLimited
	case KindServer:
		return ErrServer
	case KindUnavailable:
		return ErrUnavailable
	case KindTimeout:
		return ErrTimeout
	case KindDecode:
		return ErrDecode
	case KindUnsupported:
		return ErrUnsupported
	default:
		return nil
	}
}

// ClassifyStatus maps an HTTP status code onto a [Kind].
func ClassifyStatus(status int) Kind {
	switch status {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return KindInvalidRequest
	case http.StatusUnauthorized:
		return KindAuth
	case http.StatusPaymentRequired:
		return KindPayment
	case http.StatusForbidden:
		return KindForbidden
	case http.StatusNotFound:
		return KindNotFound
	case http.StatusRequestEntityTooLarge:
		return KindPayloadTooLarge
	case http.StatusTooManyRequests:
		return KindRateLimited
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return KindTimeout
	case http.StatusServiceUnavailable, http.StatusBadGateway:
		return KindUnavailable
	case 524: // Cloudflare/OpenRouter upstream timeout.
		return KindTimeout
	case 529: // OpenRouter reports upstream provider errors with this code.
		return KindUnavailable
	}

	if status >= 500 {
		return KindServer
	}
	return KindUnknown
}

// Error is the error type returned by every provider adapter. It keeps enough
// context to log, retry or branch on a failure.
type Error struct {
	// Provider is the adapter name, e.g. "ollama".
	Provider string
	// Kind is the provider-independent classification.
	Kind Kind
	// StatusCode is the HTTP status, or 0 for transport-level failures.
	StatusCode int
	// Message is the provider supplied error message.
	Message string
	// Body is the raw response body, truncated for readability.
	Body string
	// RetryAfter is the delay requested by the provider, if any.
	RetryAfter time.Duration
	// Err is the underlying cause, if any.
	Err error
}

// Error implements the error interface.
func (e *Error) Error() string {
	var b strings.Builder
	b.WriteString("decide")
	if e.Provider != "" {
		b.WriteString(" ")
		b.WriteString(e.Provider)
	}
	b.WriteString(": ")
	if e.Kind != KindUnknown {
		b.WriteString(e.Kind.String())
	} else {
		b.WriteString("error")
	}
	if e.StatusCode != 0 {
		b.WriteString(fmt.Sprintf(" (HTTP %d)", e.StatusCode))
	}
	switch {
	case e.Message != "":
		b.WriteString(": ")
		b.WriteString(e.Message)
	case e.Err != nil:
		b.WriteString(": ")
		b.WriteString(e.Err.Error())
	}
	return b.String()
}

// Unwrap exposes the underlying cause to [errors.Is] and [errors.As].
func (e *Error) Unwrap() error { return e.Err }

// Is reports whether the error matches a package sentinel. The sentinel is
// compared by identity, so a wrapper around a sentinel does not match; use
// [Error.Kind] to branch on a classification instead.
func (e *Error) Is(target error) bool {
	sentinel := e.Kind.sentinel()
	return sentinel != nil && sentinel == target
}

// Retryable reports whether retrying the same request could succeed.
func (e *Error) Retryable() bool {
	switch e.Kind {
	case KindRateLimited, KindServer, KindUnavailable, KindTimeout:
		return true
	default:
		return false
	}
}

// Temporary is an alias of [Error.Retryable].
func (e *Error) Temporary() bool { return e.Retryable() }

// RetryDelay returns the delay the provider asked for, or a zero duration.
func (e *Error) RetryDelay() time.Duration { return e.RetryAfter }

// NewError builds an *Error for a transport level failure.
func NewError(provider string, kind Kind, err error) *Error {
	if kind == KindUnknown && isTimeout(err) {
		kind = KindTimeout
	}
	return &Error{Provider: provider, Kind: kind, Err: err, Message: errMessage(err)}
}

// NewHTTPError builds an *Error from an HTTP status, a provider message and
// the raw body. RetryAfter is parsed from the "Retry-After" header when
// present.
func NewHTTPError(provider string, status int, message, body string, header http.Header) *Error {
	return &Error{
		Provider:   provider,
		Kind:       ClassifyStatus(status),
		StatusCode: status,
		Message:    message,
		Body:       truncate(body, 2048),
		RetryAfter: parseRetryAfter(header),
	}
}

func isTimeout(err error) bool {
	var netErr interface{ Timeout() bool }
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}
	return false
}

func errMessage(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func parseRetryAfter(header http.Header) time.Duration {
	if header == nil {
		return 0
	}
	raw := header.Get("Retry-After")
	if raw == "" {
		return 0
	}
	if secs, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil {
		if secs <= 0 {
			return 0
		}
		return time.Duration(secs) * time.Second
	}
	if when, err := http.ParseTime(raw); err == nil {
		if d := time.Until(when); d > 0 {
			return d
		}
	}
	return 0
}

func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "..."
}

// ValidationError collects every problem found while validating a request so
// callers see all of them at once.
type ValidationError struct {
	// Problems is a human readable list of issues.
	Problems []string
}

// Error implements the error interface.
func (v *ValidationError) Error() string {
	if v == nil || len(v.Problems) == 0 {
		return "decide: invalid request"
	}
	return "decide: invalid request: " + strings.Join(v.Problems, "; ")
}

// Is lets callers test validation failures with [errors.Is].
func (v *ValidationError) Is(target error) bool { return target == ErrInvalidRequest }

// As lets callers reach a validation failure with [errors.As] using the same
// *[Error] shape every provider returns:
//
//	var dErr *decide.Error
//	if errors.As(err, &dErr) && dErr.Kind == decide.KindInvalidRequest {
//	        for _, problem := range err.(*decide.ValidationError).Problems {
//	                log.Print(problem)
//	        }
//	}
func (v *ValidationError) As(target any) bool {
	dErr, ok := target.(**Error)
	if !ok || dErr == nil {
		return false
	}
	message := ""
	if len(v.Problems) > 0 {
		message = strings.Join(v.Problems, "; ")
	}
	*dErr = &Error{Kind: KindInvalidRequest, Message: message, Err: v}
	return true
}

// Add appends a problem, formatting it with [fmt.Sprintf].
func (v *ValidationError) Add(format string, args ...any) {
	v.Problems = append(v.Problems, fmt.Sprintf(format, args...))
}

// OrNil returns nil when no problems were recorded, otherwise v.
func (v *ValidationError) OrNil() error {
	if v == nil || len(v.Problems) == 0 {
		return nil
	}
	return v
}
