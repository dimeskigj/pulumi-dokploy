package dokploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
)

// gitLabCreateFixture keeps the temporary name local to each server. The server
// supplies it only after seeing the single create request, never in advance.
type gitLabCreateFixture struct {
	t             *testing.T
	mu            sync.Mutex
	marker        string
	requests      []string
	server        *httptest.Server
	createStatus  int
	createBody    string
	beforeBody    string
	memberBody    string
	listBody      string
	detailBody    string
	readbackBody  string
	postListCount int
	updateStatus  int
	renamed       bool
}

func gitLabCreateServer(t *testing.T) *gitLabCreateFixture {
	t.Helper()
	f := &gitLabCreateFixture{t: t}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	return f
}

func (f *gitLabCreateFixture) api() *client.Client {
	f.t.Helper()
	api, err := client.New(f.server.URL, "fixture-key")
	require.NoError(f.t, err)
	return api
}

func (f *gitLabCreateFixture) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r.Method+" "+r.URL.Path)
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/api/organization.active":
		_, _ = w.Write([]byte(`{"id":"org-fixture"}`))
	case "/api/user.get":
		body := f.memberBody
		if body == "" {
			body = `{"userId":"member-fixture","organizationId":"org-fixture","user":{"id":"member-fixture"}}`
		}
		_, _ = w.Write([]byte(body))
	case "/api/gitProvider.getAll":
		if f.marker == "" {
			body := f.beforeBody
			if body == "" {
				body = `[]`
			}
			_, _ = w.Write([]byte(body))
		} else {
			f.postListCount++
			if f.listBody != "" {
				_, _ = w.Write([]byte(f.listBody))
			} else {
				_, _ = w.Write([]byte(gitLabCreateList(f.marker)))
			}
		}
	case "/api/gitlab.create":
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			f.t.Errorf("invalid create JSON: %v", err)
			return
		}
		name, _ := body["name"].(string)
		if f.marker != "" || name == "" || name == "requested-name" {
			f.t.Error("create did not use one distinct temporary marker")
		}
		if body["authId"] != "member-fixture" || body["secret"] != "secret-fixture" || body["applicationId"] != "app-fixture" {
			f.t.Error("create sent incorrect ownership or OAuth settings")
		}
		f.marker = name
		if f.createStatus != 0 {
			w.WriteHeader(f.createStatus)
		}
		bodyText := f.createBody
		if bodyText == "" {
			bodyText = `{}`
		}
		_, _ = w.Write([]byte(bodyText))
	case "/api/gitlab.one":
		if r.URL.Query().Get("gitlabId") != "gitlab-fixture" {
			f.t.Error("read used an unverified integration ID")
		}
		name := f.marker
		if f.renamed {
			name = "requested-name"
		}
		if f.renamed && f.readbackBody != "" {
			_, _ = w.Write([]byte(f.readbackBody))
		} else if f.detailBody != "" {
			_, _ = w.Write([]byte(f.detailBody))
		} else {
			_, _ = w.Write([]byte(gitLabCreateDetail(name)))
		}
	case "/api/gitlab.update":
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			f.t.Errorf("invalid update JSON: %v", err)
		}
		if body["gitlabId"] != "gitlab-fixture" || body["gitProviderId"] != "parent-fixture" || body["name"] != "requested-name" {
			f.t.Error("rename used incorrect verified IDs or requested name")
		}
		if f.updateStatus != 0 {
			w.WriteHeader(f.updateStatus)
		} else {
			f.renamed = true
		}
		_, _ = w.Write([]byte(`{}`))
	default:
		f.t.Errorf("unexpected create request path: %s", r.URL.Path)
		http.NotFound(w, r)
	}
}

func gitLabCreateJSON(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func gitLabCreateList(marker string) string {
	return `[{"gitProviderId":"parent-fixture","name":` + gitLabCreateJSON(marker) + `,"type":"gitlab","organizationId":"org-fixture","userId":"member-fixture","gitlab":{"gitlabId":"gitlab-fixture","applicationId":"app-fixture","gitlabUrl":"https://gitlab.com","isConfigured":false}}]`
}

func gitLabCreateDetail(name string) string {
	return `{"gitlabId":"gitlab-fixture","gitProviderId":"parent-fixture","gitlabUrl":"https://gitlab.com","applicationId":"app-fixture","secret":"secret-fixture","redirectUri":"https://example.test/callback","groupName":"","gitlabInternalUrl":null,"accessToken":null,"refreshToken":null,"gitProvider":{"gitProviderId":"parent-fixture","name":` + gitLabCreateJSON(name) + `,"type":"gitlab","organizationId":"org-fixture","userId":"member-fixture"}}`
}

func gitLabCreateArgs() GitLabIntegrationArgs {
	return GitLabIntegrationArgs{Name: "requested-name", ApplicationID: "app-fixture", ApplicationSecret: "secret-fixture", RedirectURI: "https://example.test/callback", GitlabURL: "https://gitlab.com"}
}

func TestGitLabIntegrationCreateVerified(t *testing.T) {
	f := gitLabCreateServer(t)
	created, err := (GitLabIntegration{client: fixedClient(f.api())}).Create(t.Context(), infer.CreateRequest[GitLabIntegrationArgs]{Inputs: gitLabCreateArgs()})
	require.NoError(t, err)
	require.Equal(t, "gitlab-fixture", created.ID)
	require.Equal(t, "gitlab-fixture", created.Output.GitLabID)
	require.Equal(t, "parent-fixture", created.Output.GitProviderID)
	require.Equal(t, "requested-name", created.Output.Name)
	require.False(t, created.Output.IsConfigured)
	f.mu.Lock()
	defer f.mu.Unlock()
	require.NotEmpty(t, f.marker)
	require.NotEqual(t, "requested-name", f.marker)
	require.Equal(t, []string{
		"GET /api/organization.active", "GET /api/user.get", "GET /api/gitProvider.getAll",
		"POST /api/gitlab.create", "GET /api/gitProvider.getAll", "GET /api/gitlab.one",
		"POST /api/gitlab.update", "GET /api/gitlab.one",
	}, f.requests)
}

func TestGitLabIntegrationCreateDiscoveryFailures(t *testing.T) {
	tests := []struct {
		name     string
		list     func(string) string
		detail   func(string) string
		prior    map[string]struct{}
		attempts int
	}{
		{"duplicate new parent ID", func(m string) string {
			return gitLabCreateList(m)[:len(gitLabCreateList(m))-1] + `,` + gitLabCreateList(m)[1:]
		}, nil, nil, 1},
		{"wrong organization", func(m string) string { return gitLabCreateReplace(gitLabCreateList(m), `"org-fixture"`, `"other-org"`) }, nil, nil, 1},
		{"wrong owner", func(m string) string {
			return gitLabCreateReplace(gitLabCreateList(m), `"member-fixture"`, `"other-member"`)
		}, nil, nil, 1},
		{"wrong type", func(m string) string { return gitLabCreateReplace(gitLabCreateList(m), `"gitlab"`, `"github"`) }, nil, nil, 1},
		{"empty parent ID", func(m string) string { return gitLabCreateReplace(gitLabCreateList(m), `"parent-fixture"`, `""`) }, nil, nil, 1},
		{"empty relation ID", func(m string) string { return gitLabCreateReplace(gitLabCreateList(m), `"gitlab-fixture"`, `""`) }, nil, nil, 1},
		{"previously present ID", nil, nil, map[string]struct{}{"parent-fixture": {}}, 1},
		{"different marker", func(m string) string {
			return gitLabCreateReplace(gitLabCreateList(m), gitLabCreateJSON(m), `"other-marker"`)
		}, nil, nil, 3},
		{"two markers", func(m string) string {
			return gitLabCreateList(m)[:len(gitLabCreateList(m))-1] + `,` + gitLabCreateReplace(gitLabCreateList(m)[1:], `"parent-fixture"`, `"other-parent"`)
		}, nil, nil, 1},
		{"detail parent mismatch", nil, func(m string) string {
			return gitLabCreateReplace(gitLabCreateDetail(m), `"parent-fixture"`, `"other-parent"`)
		}, nil, 1},
		{"summary detail configuration mismatch", nil, func(m string) string {
			return gitLabCreateReplace(gitLabCreateDetail(m), `"app-fixture"`, `"other-app"`)
		}, nil, 1},
		{"detail owner mismatch", nil, func(m string) string {
			return gitLabCreateReplace(gitLabCreateDetail(m), `"member-fixture"`, `"other-member"`)
		}, nil, 1},
		{"detail relation mismatch", nil, func(m string) string {
			return gitLabCreateReplace(gitLabCreateDetail(m), `"gitlab-fixture"`, `"other-relation"`)
		}, nil, 1},
		{"missing secret", nil, func(m string) string {
			return gitLabCreateReplace(gitLabCreateDetail(m), `"secret":"secret-fixture"`, `"secret":null`)
		}, nil, 1},
		{"missing settings", nil, func(m string) string {
			return gitLabCreateReplace(gitLabCreateDetail(m), `"redirectUri":"https://example.test/callback"`, `"redirectUri":null`)
		}, nil, 1},
		{"malformed list", func(string) string { return `{` }, nil, nil, 1},
		{"list summary missing client ID", func(m string) string {
			return gitLabCreateReplace(gitLabCreateList(m), `"applicationId":"app-fixture"`, `"applicationId":null`)
		}, nil, nil, 1},
		{"ID disappears", func(string) string { return `[]` }, nil, nil, 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := gitLabCreateServer(t)
			// Keep the helper under test independent of the create POST; the marker
			// is known to this test only as a simulated POST-observed marker.
			f.marker = "pulumi-gitlab-fixture-marker"
			if tc.list != nil {
				f.listBody = tc.list(f.marker)
			}
			if tc.detail != nil {
				f.detailBody = tc.detail(f.marker)
			}
			observed, err := discoverGitLabIntegration(t.Context(), f.api(), gitLabCreateArgs(), f.marker, "org-fixture", "member-fixture", tc.prior)
			require.Error(t, err)
			require.Empty(t, observed.State.GitLabID)
			require.Equal(t, tc.attempts, f.postListCount)
			for _, request := range f.requests {
				require.NotEqual(t, "POST /api/gitlab.update", request)
				require.NotEqual(t, "POST /api/gitProvider.remove", request)
			}
		})
	}
	t.Run("canceled while waiting for candidate", func(t *testing.T) {
		f := gitLabCreateServer(t)
		f.marker, f.listBody = "pulumi-gitlab-fixture-marker", `[]`
		ctx, cancel := context.WithTimeout(t.Context(), 40*time.Millisecond)
		defer cancel()
		observed, err := discoverGitLabIntegration(ctx, f.api(), gitLabCreateArgs(), f.marker, "org-fixture", "member-fixture", nil)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Empty(t, observed.State.GitLabID)
		require.Equal(t, 1, f.postListCount)
	})
}

func gitLabCreateReplace(text, before, after string) string {
	return strings.Replace(text, before, after, 1)
}

func TestGitLabIntegrationCreatePartialFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"generic bad request after commit", http.StatusBadRequest, `{"code":"BAD_REQUEST","message":"private-server-message"}`},
		{"server error after commit", http.StatusInternalServerError, `{"code":"SERVER_ERROR"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := gitLabCreateServer(t)
			f.createStatus, f.createBody = tc.status, tc.body
			created, err := (GitLabIntegration{client: fixedClient(f.api())}).Create(t.Context(), infer.CreateRequest[GitLabIntegrationArgs]{Inputs: gitLabCreateArgs()})
			var partial infer.ResourceInitFailedError
			require.ErrorAs(t, err, &partial)
			require.NotContains(t, err.Error(), "private-server-message")
			require.Equal(t, "gitlab-fixture", created.ID)
			require.Equal(t, "parent-fixture", created.Output.GitProviderID)
			require.Equal(t, f.marker, created.Output.Name)
			require.NotContains(t, f.requests, "POST /api/gitlab.update")
		})
	}
	t.Run("rejected credentials do not trigger discovery", func(t *testing.T) {
		f := gitLabCreateServer(t)
		f.createStatus, f.createBody = http.StatusForbidden, `{"code":"FORBIDDEN"}`
		created, err := (GitLabIntegration{client: fixedClient(f.api())}).Create(t.Context(), infer.CreateRequest[GitLabIntegrationArgs]{Inputs: gitLabCreateArgs()})
		require.Error(t, err)
		require.Empty(t, created.ID)
		require.Zero(t, f.postListCount)
	})
	t.Run("rename failure retains marker", func(t *testing.T) {
		f := gitLabCreateServer(t)
		f.createBody = `not-json`
		f.updateStatus = http.StatusBadRequest
		created, err := (GitLabIntegration{client: fixedClient(f.api())}).Create(t.Context(), infer.CreateRequest[GitLabIntegrationArgs]{Inputs: gitLabCreateArgs()})
		var partial infer.ResourceInitFailedError
		require.ErrorAs(t, err, &partial)
		require.Equal(t, "gitlab-fixture", created.ID)
		require.Equal(t, f.marker, created.Output.Name)
		require.Equal(t, "parent-fixture", created.Output.GitProviderID)
	})
	t.Run("final detail disagrees with rename", func(t *testing.T) {
		f := gitLabCreateServer(t)
		f.readbackBody = gitLabCreateDetail("unexpected-name")
		created, err := (GitLabIntegration{client: fixedClient(f.api())}).Create(t.Context(), infer.CreateRequest[GitLabIntegrationArgs]{Inputs: gitLabCreateArgs()})
		var partial infer.ResourceInitFailedError
		require.ErrorAs(t, err, &partial)
		require.Equal(t, "gitlab-fixture", created.ID)
		require.Equal(t, "unexpected-name", created.Output.Name)
	})
	t.Run("malformed final detail preserves last verified state", func(t *testing.T) {
		f := gitLabCreateServer(t)
		f.readbackBody = `{`
		created, err := (GitLabIntegration{client: fixedClient(f.api())}).Create(t.Context(), infer.CreateRequest[GitLabIntegrationArgs]{Inputs: gitLabCreateArgs()})
		var partial infer.ResourceInitFailedError
		require.ErrorAs(t, err, &partial)
		require.Equal(t, "gitlab-fixture", created.ID)
		require.Equal(t, f.marker, created.Output.Name)
	})
	for _, body := range []string{"not-json", "[]"} {
		t.Run("creation acknowledgment body ignored/"+body, func(t *testing.T) {
			f := gitLabCreateServer(t)
			f.createBody = body
			created, err := (GitLabIntegration{client: fixedClient(f.api())}).Create(t.Context(), infer.CreateRequest[GitLabIntegrationArgs]{Inputs: gitLabCreateArgs()})
			require.NoError(t, err)
			require.Equal(t, "gitlab-fixture", created.ID)
		})
	}
	t.Run("member organization disagreement prevents create", func(t *testing.T) {
		f := gitLabCreateServer(t)
		f.memberBody = `{"userId":"member-fixture","organizationId":"other-org","user":{"id":"member-fixture"}}`
		created, err := (GitLabIntegration{client: fixedClient(f.api())}).Create(t.Context(), infer.CreateRequest[GitLabIntegrationArgs]{Inputs: gitLabCreateArgs()})
		require.Error(t, err)
		require.Empty(t, created.ID)
		require.NotContains(t, f.requests, "POST /api/gitlab.create")
	})
	for _, tc := range []struct{ name, before string }{
		{"malformed snapshot", `{`},
		{"duplicate snapshot IDs", `[{"gitProviderId":"prior"},{"gitProviderId":"prior"}]`},
		{"empty snapshot ID", `[{"gitProviderId":""}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := gitLabCreateServer(t)
			f.beforeBody = tc.before
			created, err := (GitLabIntegration{client: fixedClient(f.api())}).Create(t.Context(), infer.CreateRequest[GitLabIntegrationArgs]{Inputs: gitLabCreateArgs()})
			require.Error(t, err)
			require.Empty(t, created.ID)
			require.NotContains(t, f.requests, "POST /api/gitlab.create")
		})
	}
}

type gitLabCreateRoundTrip func(*http.Request) (*http.Response, error)

func (fn gitLabCreateRoundTrip) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }

type gitLabCreateCloseErrorBody struct{ io.ReadCloser }

func (b gitLabCreateCloseErrorBody) Close() error {
	_ = b.ReadCloser.Close()
	return errors.New("private-close-message")
}

func TestGitLabIntegrationCreateUncertainTransportAndClose(t *testing.T) {
	for _, tc := range []struct {
		name      string
		transport bool
	}{
		{"response close error is acknowledged", false},
		{"transport failed after server committed", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := gitLabCreateServer(t)
			api, err := client.New(f.server.URL, "fixture-key", client.WithHTTPClient(&http.Client{Transport: gitLabCreateRoundTrip(func(req *http.Request) (*http.Response, error) {
				response, err := http.DefaultTransport.RoundTrip(req)
				if err != nil || req.URL.Path != "/api/gitlab.create" {
					return response, err
				}
				if tc.transport {
					_ = response.Body.Close()
					return nil, errors.New("private-transport-message")
				}
				response.Body = gitLabCreateCloseErrorBody{response.Body}
				return response, nil
			})}), client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
			require.NoError(t, err)
			result, err := (GitLabIntegration{client: fixedClient(api)}).Create(t.Context(), infer.CreateRequest[GitLabIntegrationArgs]{Inputs: gitLabCreateArgs()})
			if tc.transport {
				var partial infer.ResourceInitFailedError
				require.ErrorAs(t, err, &partial)
				require.NotContains(t, err.Error(), "private-transport-message")
				require.Equal(t, f.marker, result.Output.Name)
				require.NotContains(t, f.requests, "POST /api/gitlab.update")
			} else {
				require.NoError(t, err)
				require.Equal(t, "requested-name", result.Output.Name)
			}
			require.Equal(t, "gitlab-fixture", result.ID)
		})
	}
}

func TestGitLabIntegrationCreateSimultaneousSameDisplayName(t *testing.T) {
	type record struct {
		marker  string
		renamed bool
	}
	var mu sync.Mutex
	var records []record
	var snapshots int
	ready := make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/api/organization.active":
			_, _ = w.Write([]byte(`{"id":"org-fixture"}`))
		case "/api/user.get":
			_, _ = w.Write([]byte(`{"userId":"member-fixture","organizationId":"org-fixture","user":{"id":"member-fixture"}}`))
		case "/api/gitProvider.getAll":
			if snapshots < 2 {
				snapshots++
				if snapshots == 2 {
					close(ready)
				}
				mu.Unlock()
				<-ready
				mu.Lock()
				_, _ = w.Write([]byte(`[]`))
				return
			}
			var entries []string
			for i, v := range records {
				name := v.marker
				if v.renamed {
					name = "requested-name"
				}
				entries = append(entries, fmt.Sprintf(`{"gitProviderId":"parent-%d","name":%s,"type":"gitlab","organizationId":"org-fixture","userId":"member-fixture","gitlab":{"gitlabId":"relation-%d","applicationId":"app-fixture","gitlabUrl":"https://gitlab.com","isConfigured":false}}`, i, gitLabCreateJSON(name), i))
			}
			_, _ = w.Write([]byte("[" + strings.Join(entries, ",") + "]"))
		case "/api/gitlab.create":
			var body struct {
				Name string `json:"name"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body.Name == "" || body.Name == "requested-name" {
				t.Error("unsafe temporary marker")
			}
			records = append(records, record{marker: body.Name})
			_, _ = w.Write([]byte(`{}`))
		case "/api/gitlab.one":
			var idx int
			if _, err := fmt.Sscanf(r.URL.Query().Get("gitlabId"), "relation-%d", &idx); err != nil || idx >= len(records) {
				t.Error("unknown relation")
				return
			}
			name := records[idx].marker
			if records[idx].renamed {
				name = "requested-name"
			}
			detail := gitLabCreateDetail(name)
			detail = strings.ReplaceAll(detail, `"parent-fixture"`, fmt.Sprintf(`"parent-%d"`, idx))
			detail = strings.ReplaceAll(detail, `"gitlab-fixture"`, fmt.Sprintf(`"relation-%d"`, idx))
			_, _ = w.Write([]byte(detail))
		case "/api/gitlab.update":
			var body struct{ GitlabID, GitProviderID, Name string }
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			var idx int
			if _, err := fmt.Sscanf(body.GitlabID, "relation-%d", &idx); err != nil || idx >= len(records) || body.GitProviderID != fmt.Sprintf("parent-%d", idx) || body.Name != "requested-name" {
				t.Error("cross-adopted parent")
				return
			}
			records[idx].renamed = true
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected operation %s", r.URL.Path)
		}
	}))
	defer s.Close()
	api, err := client.New(s.URL, "fixture-key", client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
	require.NoError(t, err)
	results := make(chan infer.CreateResponse[GitLabIntegrationState], 2)
	errs := make(chan error, 2)
	for range 2 {
		go func() {
			res, err := (GitLabIntegration{client: fixedClient(api)}).Create(t.Context(), infer.CreateRequest[GitLabIntegrationArgs]{Inputs: gitLabCreateArgs()})
			results <- res
			errs <- err
		}()
	}
	for range 2 {
		require.NoError(t, <-errs)
	}
	first, second := <-results, <-results
	require.NotEqual(t, first.ID, second.ID)
	require.Equal(t, "requested-name", first.Output.Name)
	require.Equal(t, "requested-name", second.Output.Name)
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, records, 2)
	require.NotEqual(t, records[0].marker, records[1].marker)
}
