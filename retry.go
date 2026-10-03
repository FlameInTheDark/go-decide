package decide

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"time"
)

// RetryPolicy controls how transient failures are retried. The zero value
// performs a single attempt; use [DefaultRetry] to opt into retries.
type RetryPolicy struct {
	// MaxAttempts is the total number of attempts, including the first
	// one. Values below 1 are treated as 1.
	MaxAttempts int
	// BaseDelay is the delay before the second attempt.
	BaseDelay time.Duration
	// MaxDelay caps the exponential backoff.
	MaxDelay time.Duration
	// Jitter randomises the delay within ±Jitter (0 disables it).
	Jitter float64
	// Rand sources the jitter; nil uses the shared source.
	Rand *rand.Rand
	// RetryOn decides whether an error is worth retrying. Nil retries
	// errors that report themselves as retryable.
	RetryOn func(error) bool
}

// NoRetry returns a policy that performs exactly one attempt.
func NoRetry() RetryPolicy { return RetryPolicy{MaxAttempts: 1} }

// DefaultRetry returns a sensible policy: three attempts with exponential
// backoff from 250ms up to 5s.
func DefaultRetry() RetryPolicy {
	return RetryPolicy{
		MaxAttempts: 3,
		BaseDelay:   250 * time.Millisecond,
		MaxDelay:    5 * time.Second,
		Jitter:      0.2,
	}
}

// Do runs fn until it succeeds, the policy gives up, or ctx is done.
func (p RetryPolicy) Do(ctx context.Context, fn func() (*Result, error)) (*Result, error) {
	attempts := p.MaxAttempts
	if attempts < 1 {
		attempts = 1
	}

	retryOn := p.RetryOn
	if retryOn == nil {
		retryOn = isRetryable
	}

	var lastErr error
	for attempt := range attempts {
		if err := ctx.Err(); err != nil {
			if lastErr != nil {
				return nil, lastErr
			}
			return nil, err
		}

		result, err := fn()
		if err == nil {
			return result, nil
		}
		lastErr = err

		if attempt == attempts-1 || !retryOn(err) {
			return result, err
		}

		if err := sleepCtx(ctx, p.delay(attempt, err)); err != nil {
			return nil, lastErr
		}
	}

	return nil, lastErr
}

// delay computes the backoff before the given retry, honouring a
// provider supplied Retry-After when it is longer.
func (p RetryPolicy) delay(attempt int, err error) time.Duration {
	base := p.BaseDelay
	if base <= 0 {
		return 0
	}

	wait := time.Duration(float64(base) * math.Pow(2, float64(attempt)))
	if p.MaxDelay > 0 && wait > p.MaxDelay {
		wait = p.MaxDelay
	}

	if requested := retryAfter(err); requested > wait {
		wait = requested
	}
	if wait > p.MaxDelay && p.MaxDelay > 0 && retryAfter(err) <= 0 {
		wait = p.MaxDelay
	}

	if p.Jitter > 0 {
		spread := float64(wait) * p.Jitter
		wait += time.Duration((rand.Float64()*2 - 1) * spread)
		if wait < 0 {
			wait = 0
		}
	}
	return wait
}

func isRetryable(err error) bool {
	var decideErr *Error
	if errors.As(err, &decideErr) {
		return decideErr.Retryable()
	}
	return false
}

func retryAfter(err error) time.Duration {
	var decideErr *Error
	if errors.As(err, &decideErr) {
		return decideErr.RetryDelay()
	}
	return 0
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
