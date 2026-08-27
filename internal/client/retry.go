package client

import (
	"context"
	"errors"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// RetryPolicy controls bounded retries for idempotent GET requests.
type RetryPolicy struct {
	Attempts               int
	InitialDelay, MaxDelay time.Duration
	Factor                 float64
	Jitter                 float64
}

func defaultRetryPolicy() RetryPolicy {
	return RetryPolicy{Attempts: 5, InitialDelay: 250 * time.Millisecond, Factor: 2, MaxDelay: 4 * time.Second, Jitter: .2}
}

type retryTransport struct {
	base   http.RoundTripper
	policy RetryPolicy
}

func newRetryTransport(base http.RoundTripper, policy RetryPolicy) http.RoundTripper {
	if policy.Attempts < 1 {
		policy.Attempts = 1
	}
	if policy.InitialDelay < 0 {
		policy.InitialDelay = 0
	}
	if policy.Factor < 1 {
		policy.Factor = 1
	}
	if policy.MaxDelay <= 0 {
		policy.MaxDelay = 4 * time.Second
	}
	return &retryTransport{base: base, policy: policy}
}

func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	for attempt := 1; ; attempt++ {
		resp, err := t.base.RoundTrip(req)
		shouldRetry := req.Method == http.MethodGet && attempt < t.policy.Attempts &&
			!errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) &&
			(err != nil || (resp != nil && retryStatus(resp.StatusCode)))
		if !shouldRetry {
			return resp, err
		}
		retryAfter := ""
		if resp != nil {
			retryAfter = resp.Header.Get("Retry-After")
		}
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		delay := retryDelay(t.policy, attempt, retryAfter)
		timer := time.NewTimer(delay)
		select {
		case <-req.Context().Done():
			timer.Stop()
			return nil, req.Context().Err()
		case <-timer.C:
		}
	}
}

func retryStatus(status int) bool {
	return status == 429 || status == 500 || status == 502 || status == 503 || status == 504
}
func retryDelay(policy RetryPolicy, attempt int, retryAfter string) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && seconds >= 0 {
		d := time.Duration(seconds) * time.Second
		if d < policy.MaxDelay {
			return d
		}
		return policy.MaxDelay
	}
	if retryAt, err := http.ParseTime(strings.TrimSpace(retryAfter)); err == nil {
		d := time.Until(retryAt)
		if d < 0 {
			d = 0
		}
		if d < policy.MaxDelay {
			return d
		}
		return policy.MaxDelay
	}
	d := float64(policy.InitialDelay)
	for i := 1; i < attempt; i++ {
		d *= policy.Factor
		if d >= float64(policy.MaxDelay) {
			d = float64(policy.MaxDelay)
			break
		}
	}
	if policy.Jitter > 0 {
		d *= 1 + (rand.Float64()*2-1)*policy.Jitter
	}
	if d < 0 {
		d = 0
	}
	if d > float64(policy.MaxDelay) {
		d = float64(policy.MaxDelay)
	}
	return time.Duration(d)
}
