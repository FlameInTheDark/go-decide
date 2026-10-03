package decide

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetryRetriesRetryableErrors(t *testing.T) {
	attempts := 0
	flaky := ProviderFunc{
		ID: "flaky",
		Fn: func(context.Context, Request) (*Result, error) {
			attempts++
			if attempts < 3 {
				return nil, &Error{Kind: KindServer, Message: "boom"}
			}
			return &Result{Provider: "flaky"}, nil
		},
	}

	policy := RetryPolicy{MaxAttempts: 3}
	result, err := policy.Do(context.Background(), func() (*Result, error) {
		return flaky.Decide(context.Background(), okRequest())
	})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
}

func TestRetryStopsOnNonRetryable(t *testing.T) {
	attempts := 0
	policy := RetryPolicy{MaxAttempts: 5}

	_, err := policy.Do(context.Background(), func() (*Result, error) {
		attempts++
		return nil, &Error{Kind: KindInvalidRequest, Message: "bad"}
	})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("error = %v", err)
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1", attempts)
	}
}

func TestRetryHonoursContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	policy := RetryPolicy{MaxAttempts: 3}
	_, err := policy.Do(ctx, func() (*Result, error) {
		t.Error("the operation must not run with a cancelled context")
		return nil, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
}

func TestRetryCustomPredicate(t *testing.T) {
	attempts := 0
	policy := RetryPolicy{
		MaxAttempts: 2,
		RetryOn:     func(error) bool { return true },
	}

	if _, err := policy.Do(context.Background(), func() (*Result, error) {
		attempts++
		return nil, errors.New("always fails")
	}); err == nil {
		t.Fatal("expected an error")
	}
	if attempts != 2 {
		t.Errorf("attempts = %d, want 2", attempts)
	}
}

func TestNoRetryPerformsOneAttempt(t *testing.T) {
	attempts := 0
	policy := NoRetry()

	if _, err := policy.Do(context.Background(), func() (*Result, error) {
		attempts++
		return nil, &Error{Kind: KindServer}
	}); err == nil {
		t.Fatal("expected an error")
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1", attempts)
	}
}

func TestRetryDelayRespectsRetryAfter(t *testing.T) {
	policy := RetryPolicy{MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: time.Hour}

	delay := policy.delay(0, &Error{Kind: KindRateLimited, RetryAfter: 2 * time.Minute})
	if delay != 2*time.Minute {
		t.Errorf("delay = %v, want 2m", delay)
	}
}

func TestRetryDelayCapsAtMaxDelay(t *testing.T) {
	policy := RetryPolicy{MaxAttempts: 4, BaseDelay: 100 * time.Millisecond, MaxDelay: 250 * time.Millisecond}

	if got := policy.delay(3, nil); got > 250*time.Millisecond {
		t.Errorf("delay = %v, want at most 250ms", got)
	}
}

func TestRetryDelayGrowsExponentially(t *testing.T) {
	policy := RetryPolicy{MaxAttempts: 5, BaseDelay: 10 * time.Millisecond, MaxDelay: time.Second}

	first := policy.delay(0, nil)
	third := policy.delay(2, nil)
	if third <= first {
		t.Errorf("backoff did not grow: %v then %v", first, third)
	}
}

func TestClientAppliesRetryPolicy(t *testing.T) {
	attempts := 0
	flaky := ProviderFunc{
		ID: "flaky",
		Fn: func(context.Context, Request) (*Result, error) {
			attempts++
			if attempts == 1 {
				return nil, &Error{Kind: KindRateLimited, Message: "slow down"}
			}
			return &Result{Provider: "flaky"}, nil
		},
	}

	client := New(
		WithProvider(flaky),
		WithDefault("flaky"),
		WithRetry(RetryPolicy{MaxAttempts: 2}),
	)

	if _, err := client.Decide(context.Background(), okRequest()); err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if attempts != 2 {
		t.Errorf("attempts = %d, want 2", attempts)
	}
}

func TestClassifyStatus(t *testing.T) {
	tests := []struct {
		status int
		want   Kind
	}{
		{400, KindInvalidRequest},
		{401, KindAuth},
		{402, KindPayment},
		{403, KindForbidden},
		{404, KindNotFound},
		{413, KindPayloadTooLarge},
		{429, KindRateLimited},
		{500, KindServer},
		{502, KindUnavailable},
		{503, KindUnavailable},
		{504, KindTimeout},
		{524, KindTimeout},
		{529, KindUnavailable},
		{418, KindUnknown},
	}

	for _, tc := range tests {
		if got := ClassifyStatus(tc.status); got != tc.want {
			t.Errorf("ClassifyStatus(%d) = %v, want %v", tc.status, got, tc.want)
		}
	}
}

func TestErrorFormatting(t *testing.T) {
	err := &Error{Provider: "ollama", Kind: KindRateLimited, StatusCode: 429, Message: "slow down"}
	if got := err.Error(); got != "decide ollama: rate_limited (HTTP 429): slow down" {
		t.Errorf("message = %q", got)
	}

	transport := NewError("ollama", KindUnknown, errors.New("dial tcp: refused"))
	if !errors.Is(transport, context.Canceled) && transport.Kind != KindUnknown {
		t.Errorf("kind = %v", transport.Kind)
	}
	if transport.Error() == "" {
		t.Error("message is empty")
	}
}

func TestErrorSentinelMatching(t *testing.T) {
	err := &Error{Kind: KindNotFound, Message: "nope"}

	for _, sentinel := range []error{ErrNotFound, ErrInvalidRequest, ErrServer, ErrRateLimited} {
		if got := errors.Is(err, sentinel); got != (sentinel == ErrNotFound) {
			t.Errorf("errors.Is(%v) = %v", sentinel, got)
		}
	}
}

func TestParseRetryAfterHeader(t *testing.T) {
	header := map[string][]string{"Retry-After": {"7"}}
	if got := parseRetryAfter(header); got != 7*time.Second {
		t.Errorf("delay = %v, want 7s", got)
	}

	if got := parseRetryAfter(nil); got != 0 {
		t.Errorf("nil header = %v, want 0", got)
	}
	if got := parseRetryAfter(map[string][]string{}); got != 0 {
		t.Errorf("missing header = %v, want 0", got)
	}
	if got := parseRetryAfter(map[string][]string{"Retry-After": {"garbage"}}); got != 0 {
		t.Errorf("garbage header = %v, want 0", got)
	}
}
