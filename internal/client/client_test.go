package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

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
	for _, endpoint := range []string{"", "example.test", "ftp://example.test", "https://user:password@example.test", "https://example.test?token=secret", "https://example.test/path#secret"} {
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

func TestAPIErrorRedactsSensitiveCodeAndMessageValues(t *testing.T) {
	for _, tc := range []struct {
		code, message, secret string
	}{
		{"apiKey=super-secret", "denied", "super-secret"},
		{"FORBIDDEN", "password=hunter2", ""},
		{`{"token":"token-value"}`, "denied", ""},
	} {
		err := decodeErrorWithSecret("project.one", 403, []byte(fmt.Sprintf(`{"code":%q,"message":%q}`, tc.code, tc.message)), tc.secret)
		require.NotContains(t, err.Error(), "super-secret")
		require.NotContains(t, err.Error(), "hunter2")
		require.NotContains(t, err.Error(), "token-value")
	}
}

func TestAPIErrorPreservesSafeSensitiveKeywordsInProse(t *testing.T) {
	err := decodeError("project.one", 500, []byte(`{"code":"LOOKUP","message":"registry lookup failed in environment"}`))
	require.Equal(t, "project.one: LOOKUP: registry lookup failed in environment", err.Error())
}

func TestAPIErrorRedactsSupportedSecretKeyValueSyntax(t *testing.T) {
	for _, message := range []string{
		`password=hunter2`, `apiKey: super-secret`, `{"token":"token-value"}`,
		`'secret' = 'quoted-value'`,
	} {
		err := decodeErrorWithSecret("project.one", 403, []byte(fmt.Sprintf(`{"code":"FORBIDDEN","message":%q}`, message)), "super-secret")
		require.NotContains(t, err.Error(), "hunter2")
		require.NotContains(t, err.Error(), "super-secret")
		require.NotContains(t, err.Error(), "token-value")
		require.NotContains(t, err.Error(), "quoted-value")
	}
}

func TestAPIErrorRedactsEscapedQuotedSecretValues(t *testing.T) {
	for _, tc := range []struct {
		message string
		want    string
	}{
		{`{"token":"abc\"secret"}`, `project.one: FORBIDDEN: {"token":"[REDACTED]"}`},
		{`{"token":"abc\\secret"}`, `project.one: FORBIDDEN: {"token":"[REDACTED]"}`},
		{`{"token":"abc\u0053ecret"}`, `project.one: FORBIDDEN: {"token":"[REDACTED]"}`},
		{`{"token":"unterminated`, `project.one: FORBIDDEN: {"token":"[REDACTED]`},
	} {
		err := decodeErrorWithSecret("project.one", 403, []byte(fmt.Sprintf(`{"code":"FORBIDDEN","message":%q}`, tc.message)), "known-key")
		require.Equal(t, tc.want, err.Error())
	}
}

func TestAPIErrorRedactsBuildArgAliases(t *testing.T) {
	for _, tc := range []struct {
		name, message, value string
	}{
		{"camelCase buildArgs unquoted", `buildArgs=build-arg-value`, "build-arg-value"},
		{"camelCase buildSecrets unquoted", `buildSecrets: build-secret-value`, "build-secret-value"},
		{"case-insensitive buildArgs unquoted", `BUILDARGS=upper-build-arg-value`, "upper-build-arg-value"},
		{"case-insensitive buildSecrets unquoted", `bUiLdSeCrEtS: mixed-build-secret-value`, "mixed-build-secret-value"},
		{"camelCase buildArgs quoted", `"buildArgs":"quoted-build-arg-value"`, "quoted-build-arg-value"},
		{"camelCase buildSecrets quoted", `"buildSecrets":"quoted-build-secret-value"`, "quoted-build-secret-value"},
		{"case-insensitive buildArgs quoted", `"BUILDARGS":"upper-quoted-build-arg-value"`, "upper-quoted-build-arg-value"},
		{"case-insensitive buildSecrets quoted", `"bUiLdSeCrEtS":"mixed-quoted-build-secret-value"`, "mixed-quoted-build-secret-value"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := decodeErrorWithSecret("project.one", 403, []byte(fmt.Sprintf(`{"code":"FORBIDDEN","message":%q}`, tc.message)), "known-key")
			require.NotContains(t, err.Error(), tc.value)
			require.Contains(t, err.Error(), "[REDACTED]")
		})
	}
}

func TestGeneratedOperationReturnsTypedAPIErrorForNonSuccess(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusTooManyRequests, http.StatusInternalServerError} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				code := "TEMPORARY"
				if status == http.StatusNotFound {
					code = "NOT_FOUND"
				}
				_, _ = w.Write([]byte(fmt.Sprintf(`{"code":%q,"message":"missing"}`, code)))
			}))
			defer server.Close()
			c, err := New(server.URL, "super-secret", WithRetryPolicy(RetryPolicy{Attempts: 1, InitialDelay: time.Millisecond}))
			require.NoError(t, err)
			_, err = c.ProjectOneWithResponse(t.Context(), &generated.ProjectOneParams{ProjectId: "p1"})
			require.Error(t, err)
			var apiErr *APIError
			require.ErrorAs(t, err, &apiErr)
			require.Equal(t, status, apiErr.StatusCode)
			require.Equal(t, status == 404, IsNotFound(err))
			require.Equal(t, status != 404, IsTransient(err))
			require.NotContains(t, err.Error(), "super-secret")
		})
	}
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
