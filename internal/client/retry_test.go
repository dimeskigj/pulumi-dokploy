package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gjorgjidimeski/pulumi-dokploy/internal/client/generated"
	"github.com/stretchr/testify/require"
)

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
	require.NoError(t, err)
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
