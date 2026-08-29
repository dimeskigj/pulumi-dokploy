package client

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/stretchr/testify/require"
)

type recordingRoundTripper struct {
	bodies [][]byte
	status int
}

func (r *recordingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(req.Body)
	r.bodies = append(r.bodies, body)
	return &http.Response{StatusCode: r.status, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader([]byte(`{"code":"TEMP"}`))), Request: req}, nil
}

func TestRetryReplaysGETBodyEqually(t *testing.T) {
	base := &recordingRoundTripper{status: http.StatusServiceUnavailable}
	policy := RetryPolicy{Attempts: 3, InitialDelay: time.Millisecond, MaxDelay: time.Millisecond, Jitter: 0}
	req := httptest.NewRequest(http.MethodGet, "http://example.test", bytes.NewReader([]byte("body")))
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader([]byte("body"))), nil }
	resp, err := newRetryTransport(base, policy).RoundTrip(req)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	require.Nil(t, resp)
	require.Equal(t, http.StatusServiceUnavailable, apiErr.StatusCode)
	require.Len(t, base.bodies, 3)
	require.Equal(t, base.bodies[0], base.bodies[1])
	require.Equal(t, base.bodies[1], base.bodies[2])
}

func TestRetryDoesNotReplayGETBodyWithoutGetBody(t *testing.T) {
	base := &recordingRoundTripper{status: http.StatusServiceUnavailable}
	req := httptest.NewRequest(http.MethodGet, "http://example.test", bytes.NewReader([]byte("body")))
	resp, err := newRetryTransport(base, RetryPolicy{Attempts: 5, InitialDelay: time.Millisecond}).RoundTrip(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	require.Len(t, base.bodies, 1)
}

func TestRetryRetriesTransientGETOnly(t *testing.T) {
	var gets, posts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			gets.Add(1)
		} else {
			posts.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && gets.Load() < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"code":"TEMP","message":"retry"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"p1"}`))
	}))
	defer server.Close()
	c, err := New(server.URL, "key", WithRetryPolicy(RetryPolicy{Attempts: 3, InitialDelay: time.Millisecond, MaxDelay: time.Millisecond}))
	require.NoError(t, err)
	_, err = c.ProjectOneWithResponse(t.Context(), &generated.ProjectOneParams{ProjectId: "p1"})
	require.NoError(t, err)
	require.Equal(t, int32(3), gets.Load())
	require.Equal(t, int32(0), posts.Load())
}

func TestRetryDoesNotRetryPOST(t *testing.T) {
	var posts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	c, err := New(server.URL, "key", WithRetryPolicy(RetryPolicy{Attempts: 5, InitialDelay: time.Millisecond}))
	require.NoError(t, err)
	_, err = c.ProjectCreateWithResponse(t.Context(), generated.ProjectCreateJSONRequestBody{})
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, http.StatusServiceUnavailable, apiErr.StatusCode)
	require.Equal(t, int32(1), posts.Load())
}

func TestRetryStopsOnCancellation(t *testing.T) {
	var gets atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gets.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	c, err := New(server.URL, "key", WithRetryPolicy(RetryPolicy{Attempts: 5, InitialDelay: time.Second, Jitter: 0}))
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = c.ProjectOneWithResponse(ctx, &generated.ProjectOneParams{ProjectId: "p1"})
	require.ErrorIs(t, err, context.Canceled)
	require.LessOrEqual(t, gets.Load(), int32(1))
}
