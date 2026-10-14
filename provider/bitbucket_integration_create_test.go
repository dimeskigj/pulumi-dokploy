package dokploy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
)

type bitbucketCreateFixture struct {
	t            *testing.T
	mu           sync.Mutex
	server       *httptest.Server
	requests     []string
	marker       string
	before       string
	member       string
	list         string
	detail       string
	readback     string
	createStatus int
	updateStatus int
	renamed      bool
	postLists    int
	onRename     func()
	appMode      bool
}

func newBitbucketCreateFixture(t *testing.T) *bitbucketCreateFixture {
	f := &bitbucketCreateFixture{t: t}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	return f
}

func (f *bitbucketCreateFixture) api() *client.Client {
	api, err := client.New(f.server.URL, "fixture", client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
	require.NoError(f.t, err)
	return api
}

func (f *bitbucketCreateFixture) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r.Method+" "+r.URL.Path)
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/api/organization.active":
		_, _ = fmt.Fprint(w, `{"id":"org"}`)
	case "/api/user.get":
		member := f.member
		if member == "" {
			member = `{"userId":"member","organizationId":"org","user":{"id":"member"}}`
		}
		_, _ = fmt.Fprint(w, member)
	case "/api/gitProvider.getAll":
		if f.marker == "" {
			if f.before == "" {
				f.before = `[]`
			}
			_, _ = fmt.Fprint(w, f.before)
			return
		}
		f.postLists++
		if f.list != "" {
			_, _ = fmt.Fprint(w, strings.ReplaceAll(f.list, "MARKER", f.marker))
			return
		}
		_, _ = fmt.Fprint(w, bbCreateList(f.marker, "parent", "relation", "org", "member", "bitbucket", "user", true))
	case "/api/bitbucket.create":
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			f.t.Errorf("bad create request: %v", err)
			return
		}
		name, _ := body["name"].(string)
		if name == "" || name == "requested" || body["authId"] != "member" || body["bitbucketId"] != nil || body["gitProviderId"] != nil {
			f.t.Errorf("create payload does not use guarded marker/member without chosen IDs: %#v", body)
		}
		apiMode := body["apiToken"] == "api-secret" && body["appPassword"] == ""
		legacyMode := body["appPassword"] == "app-secret" && body["apiToken"] == ""
		if (!f.appMode && !apiMode) || (f.appMode && !legacyMode) {
			f.t.Errorf("create payload has incorrect credential selection: %#v", body)
		}
		if (!f.appMode && body["bitbucketEmail"] != "user@example.invalid") || (f.appMode && body["bitbucketEmail"] != nil) || body["bitbucketUsername"] != "user" || body["bitbucketWorkspaceName"] != "workspace" {
			f.t.Errorf("create payload has incorrect optional settings: %#v", body)
		}
		f.marker = name
		if f.createStatus != 0 {
			w.WriteHeader(f.createStatus)
		}
		_, _ = fmt.Fprint(w, "this acknowledgment body is intentionally not JSON")
	case "/api/bitbucket.one":
		if r.URL.Query().Get("bitbucketId") != "relation" {
			f.t.Errorf("detail requested with unverified ID: %q", r.URL.Query().Get("bitbucketId"))
		}
		name := f.marker
		if f.renamed {
			name = "requested"
		}
		body := bbCreateDetailMode("relation", "parent", name, "org", "member", f.appMode)
		if f.renamed && f.readback != "" {
			body = f.readback
		}
		if !f.renamed && f.detail != "" {
			body = strings.ReplaceAll(f.detail, "MARKER", f.marker)
		}
		_, _ = fmt.Fprint(w, body)
	case "/api/bitbucket.update":
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			f.t.Errorf("bad rename request: %v", err)
		}
		if body["bitbucketId"] != "relation" || body["gitProviderId"] != "parent" || body["name"] != "requested" {
			f.t.Errorf("rename did not use verified identities: %#v", body)
		}
		wantToken, wantPassword := any("api-secret"), any("")
		if f.appMode {
			wantToken, wantPassword = "", "app-secret"
		}
		if body["apiToken"] != wantToken || body["appPassword"] != wantPassword {
			f.t.Errorf("rename omitted complete credential configuration: %#v", body)
		}
		if f.updateStatus != 0 {
			w.WriteHeader(f.updateStatus)
		} else {
			f.renamed = true
		}
		_, _ = fmt.Fprint(w, "not JSON either")
		if f.onRename != nil {
			f.onRename()
		}
	default:
		f.t.Errorf("unexpected request: %s", r.URL.Path)
		http.NotFound(w, r)
	}
}

func bbCreateList(marker, parent, relation, org, owner, typ, username string, configured bool) string {
	return fmt.Sprintf(`[{"gitProviderId":%q,"name":%q,"type":%q,"organizationId":%q,"userId":%q,"bitbucket":{"bitbucketId":%q,"bitbucketUsername":%q,"isConfigured":%t,"isDeprecated":false}}]`, parent, marker, typ, org, owner, relation, username, configured)
}

func bbCreateDetail(relation, parent, name, org, owner string) string {
	return bbCreateDetailMode(relation, parent, name, org, owner, false)
}

func bbCreateDetailMode(relation, parent, name, org, owner string, appMode bool) string {
	token, password, email := "api-secret", "", `"user@example.invalid"`
	if appMode {
		token, password, email = "", "app-secret", "null"
	}
	return fmt.Sprintf(`{"bitbucketId":%q,"gitProviderId":%q,"apiToken":%q,"appPassword":%q,"bitbucketEmail":%s,"bitbucketUsername":"user","bitbucketWorkspaceName":"workspace","gitProvider":{"gitProviderId":%q,"name":%q,"providerType":"bitbucket","organizationId":%q,"userId":%q}}`, relation, parent, token, password, email, parent, name, org, owner)
}

func bbCreateArgs() BitbucketIntegrationArgs {
	return BitbucketIntegrationArgs{Name: "requested", APIToken: stringPtr("api-secret"), BitbucketEmail: stringPtr("user@example.invalid"), BitbucketUsername: stringPtr("user"), BitbucketWorkspaceName: stringPtr("workspace")}
}

func TestBitbucketCreateRejectsPriorOrInconsistentCandidate(t *testing.T) {
	for _, tc := range []struct {
		name, list, detail, before string
		attempts                   int
	}{
		{"wrong owner", `[{"gitProviderId":"parent","name":"MARKER","type":"bitbucket","organizationId":"org","userId":"else","bitbucket":{"bitbucketId":"relation","bitbucketUsername":"user","isConfigured":true,"isDeprecated":false}}]`, "", "", 1},
		{"wrong organization", `[{"gitProviderId":"parent","name":"MARKER","type":"bitbucket","organizationId":"else","userId":"member","bitbucket":{"bitbucketId":"relation","bitbucketUsername":"user","isConfigured":true,"isDeprecated":false}}]`, "", "", 1},
		{"wrong type", `[{"gitProviderId":"parent","name":"MARKER","type":"github","organizationId":"org","userId":"member","bitbucket":{"bitbucketId":"relation","bitbucketUsername":"user","isConfigured":true,"isDeprecated":false}}]`, "", "", 1},
		{"summary detail mismatch", `[{"gitProviderId":"parent","name":"MARKER","type":"bitbucket","organizationId":"org","userId":"member","bitbucket":{"bitbucketId":"relation","bitbucketUsername":"other","isConfigured":true,"isDeprecated":false}}]`, "", "", 1},
		{"detail summary username mismatch", "", strings.ReplaceAll(bbCreateDetail("relation", "parent", "MARKER", "org", "member"), `"bitbucketUsername":"user"`, `"bitbucketUsername":"other"`), "", 1},
		{"detail owner mismatch", "", strings.ReplaceAll(bbCreateDetail("relation", "parent", "MARKER", "org", "member"), `"userId":"member"`, `"userId":"else"`), "", 1},
		{"ambiguous markers", strings.TrimSuffix(bbCreateList("MARKER", "parent", "relation", "org", "member", "bitbucket", "user", true), "]") + `,{"gitProviderId":"parent2","name":"MARKER","type":"bitbucket","organizationId":"org","userId":"member","bitbucket":{"bitbucketId":"relation2","bitbucketUsername":"user","isConfigured":true,"isDeprecated":false}}]`, "", "", 1},
		{"marker collision inconsistent", strings.ReplaceAll(bbCreateList("MARKER", "parent", "relation", "org", "member", "bitbucket", "user", true), `"type":"bitbucket"`, `"type":"other"`), "", "", 1},
		{"empty discovery retries", `[]`, "", "", 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newBitbucketCreateFixture(t)
			f.list, f.detail = tc.list, tc.detail
			result, err := (BitbucketIntegration{client: fixedClient(f.api())}).Create(t.Context(), infer.CreateRequest[BitbucketIntegrationArgs]{Inputs: bbCreateArgs()})
			require.Error(t, err)
			require.Empty(t, result.ID)
			require.Empty(t, result.Output.BitbucketID)
			require.Equal(t, tc.attempts, f.postLists)
			require.NotContains(t, f.requests, "POST /api/bitbucket.update")
			require.NotContains(t, f.requests, "POST /api/gitProvider.remove")
		})
	}
	for _, tc := range []struct{ name, before string }{
		{"duplicate parent", `[{"gitProviderId":"prior"},{"gitProviderId":"prior"}]`},
		{"missing parent", `[{"gitProviderId":""}]`},
		{"empty relation in unconfigured summary", `[{"gitProviderId":"p","bitbucket":{"bitbucketId":"","isConfigured":false,"isDeprecated":false}}]`},
		{"duplicate relation", `[{"gitProviderId":"p1","bitbucket":{"bitbucketId":"r","isConfigured":false,"isDeprecated":false}},{"gitProviderId":"p2","bitbucket":{"bitbucketId":"r","isConfigured":false,"isDeprecated":false}}]`},
	} {
		t.Run("invalid full snapshot/"+tc.name, func(t *testing.T) {
			f := newBitbucketCreateFixture(t)
			f.before = tc.before
			result, err := (BitbucketIntegration{client: fixedClient(f.api())}).Create(t.Context(), infer.CreateRequest[BitbucketIntegrationArgs]{Inputs: bbCreateArgs()})
			require.Error(t, err)
			require.Empty(t, result.ID)
			require.NotContains(t, f.requests, "POST /api/bitbucket.create")
		})
	}
	for _, before := range []string{
		`[{"gitProviderId":"parent"}]`,
		`[{"gitProviderId":"old-parent","bitbucket":{"bitbucketId":"relation","isConfigured":false,"isDeprecated":false}}]`,
	} {
		t.Run("candidate reuses prior ID/"+before, func(t *testing.T) {
			f := newBitbucketCreateFixture(t)
			f.before = before
			result, err := (BitbucketIntegration{client: fixedClient(f.api())}).Create(t.Context(), infer.CreateRequest[BitbucketIntegrationArgs]{Inputs: bbCreateArgs()})
			require.Error(t, err)
			require.Empty(t, result.ID)
			require.NotContains(t, f.requests, "POST /api/bitbucket.update")
		})
	}
	t.Run("authenticated member organization mismatch prevents mutation", func(t *testing.T) {
		f := newBitbucketCreateFixture(t)
		f.member = `{"userId":"member","organizationId":"other","user":{"id":"member"}}`
		_, err := (BitbucketIntegration{client: fixedClient(f.api())}).Create(t.Context(), infer.CreateRequest[BitbucketIntegrationArgs]{Inputs: bbCreateArgs()})
		require.Error(t, err)
		require.NotContains(t, f.requests, "GET /api/gitProvider.getAll")
		require.NotContains(t, f.requests, "POST /api/bitbucket.create")
	})
}

func TestBitbucketCreateSeparateIdentityDomainsAndVerifiedPayload(t *testing.T) {
	f := newBitbucketCreateFixture(t)
	f.before = `[{"gitProviderId":"relation","bitbucket":{"bitbucketId":"old-relation","isConfigured":false,"isDeprecated":false}}]`
	result, err := (BitbucketIntegration{client: fixedClient(f.api())}).Create(t.Context(), infer.CreateRequest[BitbucketIntegrationArgs]{Inputs: bbCreateArgs()})
	require.NoError(t, err)
	require.Equal(t, "relation", result.ID)
	require.Equal(t, "parent", result.Output.GitProviderID)
	require.Equal(t, "org", result.Output.OrganizationID)
	require.Equal(t, "requested", result.Output.Name)
	require.Equal(t, []string{"GET /api/organization.active", "GET /api/user.get", "GET /api/gitProvider.getAll", "POST /api/bitbucket.create", "GET /api/gitProvider.getAll", "GET /api/bitbucket.one", "POST /api/bitbucket.update", "GET /api/bitbucket.one"}, f.requests)
}

func TestBitbucketCreateAppPasswordModeVerified(t *testing.T) {
	f := newBitbucketCreateFixture(t)
	f.appMode = true
	args := BitbucketIntegrationArgs{Name: "requested", AppPassword: stringPtr("app-secret"), BitbucketUsername: stringPtr("user"), BitbucketWorkspaceName: stringPtr("workspace")}
	result, err := (BitbucketIntegration{client: fixedClient(f.api())}).Create(t.Context(), infer.CreateRequest[BitbucketIntegrationArgs]{Inputs: args})
	require.NoError(t, err)
	require.Equal(t, "relation", result.ID)
	require.Nil(t, result.Output.BitbucketEmail)
	require.Equal(t, "user", *result.Output.BitbucketUsername)
}

func TestBitbucketCreateUncertainVerifiedIdentityReturnsPartialWithoutRename(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusInternalServerError} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			f := newBitbucketCreateFixture(t)
			f.createStatus = status
			result, err := (BitbucketIntegration{client: fixedClient(f.api())}).Create(t.Context(), infer.CreateRequest[BitbucketIntegrationArgs]{Inputs: bbCreateArgs()})
			var partial infer.ResourceInitFailedError
			require.ErrorAs(t, err, &partial)
			require.Equal(t, "relation", result.ID)
			require.Equal(t, "relation", result.Output.BitbucketID)
			require.NotEqual(t, "requested", result.Output.Name)
			require.NotContains(t, f.requests, "POST /api/bitbucket.update")
		})
	}
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		f := newBitbucketCreateFixture(t)
		f.createStatus = status
		result, err := (BitbucketIntegration{client: fixedClient(f.api())}).Create(t.Context(), infer.CreateRequest[BitbucketIntegrationArgs]{Inputs: bbCreateArgs()})
		require.Error(t, err)
		require.Empty(t, result.ID)
		require.Zero(t, f.postLists)
	}
	t.Run("uncertain response with unverified candidate returns no ID", func(t *testing.T) {
		f := newBitbucketCreateFixture(t)
		f.createStatus = http.StatusBadRequest
		f.detail = strings.ReplaceAll(bbCreateDetail("relation", "parent", "MARKER", "org", "member"), `"apiToken":"api-secret"`, `"apiToken":"wrong"`)
		result, err := (BitbucketIntegration{client: fixedClient(f.api())}).Create(t.Context(), infer.CreateRequest[BitbucketIntegrationArgs]{Inputs: bbCreateArgs()})
		require.Error(t, err)
		require.Empty(t, result.ID)
		require.Empty(t, result.Output.BitbucketID)
		require.NotContains(t, f.requests, "POST /api/bitbucket.update")
	})
}

func TestBitbucketCreateDiscoveryBounds(t *testing.T) {
	f := newBitbucketCreateFixture(t)
	f.list = `[]`
	_, err := (BitbucketIntegration{client: fixedClient(f.api())}).Create(t.Context(), infer.CreateRequest[BitbucketIntegrationArgs]{Inputs: bbCreateArgs()})
	require.Error(t, err)
	require.Equal(t, 3, f.postLists)
	require.Equal(t, 1, strings.Count(strings.Join(f.requests, ","), "POST /api/bitbucket.create"))
	f = newBitbucketCreateFixture(t)
	f.list = `[]`
	ctx := newLifecycleCancelContext()
	transport := bitbucketCreateRoundTripper(func(req *http.Request) (*http.Response, error) {
		resp, err := http.DefaultTransport.RoundTrip(req)
		if err == nil && req.URL.Path == "/api/gitProvider.getAll" && f.marker != "" {
			ctx.cancel(context.DeadlineExceeded)
		}
		return resp, err
	})
	api, err := client.New(f.server.URL, "fixture", client.WithHTTPClient(&http.Client{Transport: transport}), client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
	require.NoError(t, err)
	_, err = (BitbucketIntegration{client: fixedClient(api)}).Create(ctx, infer.CreateRequest[BitbucketIntegrationArgs]{Inputs: bbCreateArgs()})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	var partial infer.ResourceInitFailedError
	require.ErrorAs(t, err, &partial)
	require.Equal(t, 1, f.postLists, "one post-create discovery must finish before the deadline")
	require.Equal(t, 1, strings.Count(strings.Join(f.requests, ","), "POST /api/bitbucket.create"), "create is sent exactly once before cancellation")
	require.Equal(t, 2, strings.Count(strings.Join(f.requests, ","), "GET /api/gitProvider.getAll"), "no discovery may follow cancellation")
}

func TestBitbucketCreateRenamePartialState(t *testing.T) {
	t.Run("rename failure returns verified marker", func(t *testing.T) {
		f := newBitbucketCreateFixture(t)
		f.updateStatus = http.StatusBadRequest
		result, err := (BitbucketIntegration{client: fixedClient(f.api())}).Create(t.Context(), infer.CreateRequest[BitbucketIntegrationArgs]{Inputs: bbCreateArgs()})
		var partial infer.ResourceInitFailedError
		require.ErrorAs(t, err, &partial)
		require.Equal(t, "relation", result.ID)
		require.Equal(t, f.marker, result.Output.Name)
	})
	t.Run("failed readback preserves last verified marker and sanitized failure", func(t *testing.T) {
		f := newBitbucketCreateFixture(t)
		f.readback = `{`
		result, err := (BitbucketIntegration{client: fixedClient(f.api())}).Create(t.Context(), infer.CreateRequest[BitbucketIntegrationArgs]{Inputs: bbCreateArgs()})
		var partial infer.ResourceInitFailedError
		require.ErrorAs(t, err, &partial)
		require.Equal(t, f.marker, result.Output.Name)
		require.NotContains(t, err.Error(), "api-secret")
	})
	for _, ctxErr := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(ctxErr.Error(), func(t *testing.T) {
			f := newBitbucketCreateFixture(t)
			var ctx context.Context
			if ctxErr == context.DeadlineExceeded {
				var cancel context.CancelFunc
				ctx, cancel = context.WithDeadline(t.Context(), time.Now().Add(100*time.Millisecond))
				defer cancel()
				f.onRename = func() { <-ctx.Done() }
			} else {
				var cancel context.CancelCauseFunc
				ctx, cancel = context.WithCancelCause(t.Context())
				f.onRename = func() { cancel(ctxErr) }
			}
			result, err := (BitbucketIntegration{client: fixedClient(f.api())}).Create(ctx, infer.CreateRequest[BitbucketIntegrationArgs]{Inputs: bbCreateArgs()})
			var partial infer.ResourceInitFailedError
			require.ErrorAs(t, err, &partial)
			require.ErrorIs(t, err, ctxErr)
			require.Equal(t, "relation", result.ID)
		})
	}
}

func TestBitbucketCreatePreviewDoesNotCallAPI(t *testing.T) {
	f := newBitbucketCreateFixture(t)
	result, err := (BitbucketIntegration{client: fixedClient(f.api())}).Create(t.Context(), infer.CreateRequest[BitbucketIntegrationArgs]{DryRun: true, Inputs: bbCreateArgs()})
	require.NoError(t, err)
	require.Empty(t, result.ID)
	require.Empty(t, result.Output.BitbucketID)
	require.Empty(t, f.requests)
}

func TestBitbucketCreateMarkerCollision(t *testing.T) {
	entry := generated.GitProviderListEntry{AdditionalProperties: map[string]interface{}{"name": "pulumi-bitbucket-marker"}}
	require.Error(t, validateBitbucketCreateMarker([]generated.GitProviderListEntry{entry}, "pulumi-bitbucket-marker"))
	require.NoError(t, validateBitbucketCreateMarker([]generated.GitProviderListEntry{entry}, "pulumi-bitbucket-other"))
}

type bitbucketCreateRoundTripper func(*http.Request) (*http.Response, error)

func (fn bitbucketCreateRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type bitbucketCreateTrackedBody struct {
	io.ReadCloser
	closed bool
}

func (body *bitbucketCreateTrackedBody) Close() error {
	body.closed = true
	return body.ReadCloser.Close()
}

func TestBitbucketCreateClosesBodylessAcknowledgmentWithoutDecoding(t *testing.T) {
	f := newBitbucketCreateFixture(t)
	var body *bitbucketCreateTrackedBody
	httpClient := &http.Client{Transport: bitbucketCreateRoundTripper(func(req *http.Request) (*http.Response, error) {
		response, err := http.DefaultTransport.RoundTrip(req)
		if err == nil && req.URL.Path == "/api/bitbucket.create" {
			body = &bitbucketCreateTrackedBody{ReadCloser: response.Body}
			response.Body = body
		}
		return response, err
	})}
	api, err := client.New(f.server.URL, "fixture", client.WithHTTPClient(httpClient), client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
	require.NoError(t, err)
	result, err := (BitbucketIntegration{client: fixedClient(api)}).Create(t.Context(), infer.CreateRequest[BitbucketIntegrationArgs]{Inputs: bbCreateArgs()})
	require.NoError(t, err)
	require.Equal(t, "relation", result.ID)
	require.NotNil(t, body)
	require.True(t, body.closed)
}
