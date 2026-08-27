package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gjorgjidimeski/pulumi-dokploy/internal/client/generated"
	"github.com/stretchr/testify/require"
)

type headerServer struct {
	server *httptest.Server
	mu     sync.Mutex
	last   *http.Request
}

func newHeaderServer(t *testing.T) *headerServer {
	t.Helper()
	h := &headerServer{}
	h.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.mu.Lock()
		h.last = r.Clone(r.Context())
		h.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"p1"}`))
	}))
	t.Cleanup(h.server.Close)
	return h
}

func (h *headerServer) LastRequest() *http.Request {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.last
}

func TestNewNormalizesEndpointAndAuthenticates(t *testing.T) {
	server := newHeaderServer(t)
	c, err := New(server.server.URL+"/api/", "super-secret")
	require.NoError(t, err)
	_, err = c.ProjectOneWithResponse(t.Context(), &generated.ProjectOneParams{ProjectId: "p1"})
	require.NoError(t, err)
	require.Equal(t, "/api/project.one", server.LastRequest().URL.Path)
	require.Equal(t, "super-secret", server.LastRequest().Header.Get("x-api-key"))
	require.Equal(t, "application/json", server.LastRequest().Header.Get("Accept"))
}

func TestNewNormalizesEndpointForms(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"https://example.test", "https://example.test/api"},
		{"https://example.test/api", "https://example.test/api"},
		{"https://example.test/api/", "https://example.test/api"},
		{"https://example.test/base/", "https://example.test/base/api"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			c, err := New(tc.input, "key")
			require.NoError(t, err)
			require.Equal(t, tc.want, c.endpoint)
		})
	}
}

func TestNewRejectsInvalidEndpoints(t *testing.T) {
	for _, endpoint := range []string{"", "example.test", "ftp://example.test", "https://example.test?token=secret", "https://example.test/path#secret"} {
		t.Run(fmt.Sprintf("%q", endpoint), func(t *testing.T) {
			_, err := New(endpoint, "key")
			require.Error(t, err)
			require.NotContains(t, err.Error(), "secret")
		})
	}
}

func TestClientClassifiesErrorsWithoutSecrets(t *testing.T) {
	err := decodeError("project.one", 403, []byte(`{"code":"FORBIDDEN","message":"denied","issues":{"apiKey":"super-secret","password":"secret"}}`))
	require.Equal(t, "project.one: FORBIDDEN: denied", err.Error())
	require.NotContains(t, err.Error(), "super-secret")
	require.NotContains(t, err.Error(), "secret")
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, 403, apiErr.StatusCode)
}

func TestDecodeErrorHandlesMalformedBodies(t *testing.T) {
	err := decodeError("project.one", 500, []byte("not json"))
	require.Equal(t, "project.one: HTTP 500", err.Error())
}

func TestErrorClassification(t *testing.T) {
	for _, tc := range []struct {
		status              int
		code                string
		notFound, transient bool
	}{
		{404, "", true, false}, {200, "NOT_FOUND", true, false}, {429, "", false, true},
		{500, "", false, true}, {502, "", false, true}, {503, "", false, true}, {504, "", false, true},
	} {
		err := &APIError{StatusCode: tc.status, Code: tc.code}
		require.Equal(t, tc.notFound, IsNotFound(err))
		require.Equal(t, tc.transient, IsTransient(err))
	}
	require.False(t, IsTransient(context.Canceled))
	require.False(t, IsNotFound(context.Canceled))
	require.True(t, strings.Contains((&APIError{Operation: "x", Code: "C", Message: "m"}).Error(), "x: C: m"))
}
