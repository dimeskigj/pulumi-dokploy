package dokploy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/stretchr/testify/require"
)

// This fixture scripts the API boundary while exercising the real Application
// provider and live configuration lifecycle. Any deploy, redeploy, or fetch
// route is rejected by the default case below.
type liveApplicationConfigurationHTTPFixture struct {
	mu                  sync.Mutex
	t                   *testing.T
	api                 *client.Client
	phase               string
	rejectCreateStatus  int
	routes              []string
	counts              map[string]int
	buildTypes          []string
	buildBodies         []map[string]any
	mutationIDs         []string
	app                 map[string]any
	name                string
	typedReadBeforeGone bool
}

func newLiveApplicationConfigurationHTTPFixture(t *testing.T, phase string) *liveApplicationConfigurationHTTPFixture {
	t.Helper()
	f := &liveApplicationConfigurationHTTPFixture{t: t, phase: phase, counts: map[string]int{}}
	s := httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(s.Close)
	api, err := client.New(s.URL, "offline-key", client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
	require.NoError(t, err)
	f.api = api
	return f
}

func (f *liveApplicationConfigurationHTTPFixture) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	route := strings.TrimPrefix(r.URL.Path, "/api/")
	f.routes = append(f.routes, route)
	f.counts[route]++
	w.Header().Set("Content-Type", "application/json")
	body := map[string]any{}
	if r.Method == http.MethodPost {
		require.NoError(f.t, json.NewDecoder(r.Body).Decode(&body))
	} else {
		require.Equal(f.t, http.MethodGet, r.Method)
	}
	write := func(value any) { require.NoError(f.t, json.NewEncoder(w).Encode(value)) }
	fail := func(status int) {
		w.WriteHeader(status)
		write(map[string]any{"code": "OFFLINE_FAILURE", "message": "private-upstream-detail"})
	}
	if route == "application.create" && f.rejectCreateStatus != 0 {
		fail(f.rejectCreateStatus)
		return
	}
	switch route {
	case "application.create":
		require.Equal(f.t, 2, len(body), "create must be a bare, non-deploying application record")
		require.Equal(f.t, "offline-environment", body["environmentId"])
		f.name, _ = body["name"].(string)
		require.True(f.t, strings.HasPrefix(f.name, "pulumi-acceptance-application-configuration-"), "fixture name must use the reserved unique prefix")
		if f.phase == "transport-create" {
			connection, _, err := w.(http.Hijacker).Hijack()
			require.NoError(f.t, err)
			require.NoError(f.t, connection.Close())
			return
		}
		if f.phase == "no-id-create" {
			write(map[string]any{})
			return
		}
		f.app = map[string]any{"applicationId": "offline-application", "name": f.name, "environmentId": "offline-environment", "applicationStatus": "done"}
		write(map[string]any{"applicationId": "offline-application"})
	case "application.update":
		require.NotNil(f.t, f.app)
		applicationID, _ := body["applicationId"].(string)
		f.mutationIDs = append(f.mutationIDs, applicationID)
		require.Equal(f.t, "offline-application", body["applicationId"])
		require.Equal(f.t, false, body["autoDeploy"], "configuration fixtures must never enable autoDeploy")
		if f.phase == "auto-deploy-error" {
			fail(http.StatusForbidden)
			return
		}
		f.app["autoDeploy"] = false
		write(true)
	case "application.saveGitProvider":
		require.NotNil(f.t, f.app)
		require.Equal(f.t, false, f.app["autoDeploy"], "autoDeploy must be false before source attachment")
		f.app["type"] = "git"
		f.app["customGitUrl"] = body["customGitUrl"]
		f.app["customGitBranch"] = body["customGitBranch"]
		f.app["customGitBuildPath"] = body["customGitBuildPath"]
		f.app["watchPaths"] = body["watchPaths"]
		f.app["enableSubmodules"] = body["enableSubmodules"]
		if f.phase == "source-error" {
			// The acknowledged application now has deliberately incomplete source
			// data. Cleanup must use raw ID-only API checks, not Application.Read.
			f.app["type"] = "git"
			delete(f.app, "customGitUrl")
			delete(f.app, "customGitBranch")
			fail(http.StatusForbidden)
			return
		}
		write(true)
	case "application.saveGithubProvider", "application.saveGiteaProvider", "application.saveBitbucketProvider", "application.saveGitlabProvider":
		require.NotNil(f.t, f.app)
		require.Equal(f.t, false, f.app["autoDeploy"], "autoDeploy must be false before source attachment")
		write(true)
	case "application.saveBuildType":
		require.NotNil(f.t, f.app)
		require.Equal(f.t, false, f.app["autoDeploy"], "autoDeploy must be false before build configuration")
		buildType, _ := body["buildType"].(string)
		f.buildTypes = append(f.buildTypes, buildType)
		f.buildBodies = append(f.buildBodies, body)
		for key, value := range body {
			if key != "applicationId" {
				f.app[key] = value
			}
		}
		write(true)
	case "application.one":
		require.Equal(f.t, "offline-application", r.URL.Query().Get("applicationId"))
		if f.app == nil {
			w.WriteHeader(http.StatusNotFound)
			write(map[string]any{"code": "NOT_FOUND"})
			return
		}
		if !f.appHasValidSource() {
			f.typedReadBeforeGone = true
		}
		copy := map[string]any{}
		for key, value := range f.app {
			copy[key] = value
		}
		if f.counts[route] == 1 {
			switch f.phase {
			case "missing-auto-deploy-id":
				delete(copy, "applicationId")
			case "blank-auto-deploy-id":
				copy["applicationId"] = "  "
			case "mismatched-auto-deploy-id", "mismatched-integrated-source-id":
				copy["applicationId"] = "offline-unrelated-application"
			}
		}
		write(copy)
	case "application.delete":
		require.NotNil(f.t, f.app)
		applicationID, _ := body["applicationId"].(string)
		f.mutationIDs = append(f.mutationIDs, applicationID)
		require.Equal(f.t, map[string]any{"applicationId": "offline-application"}, body)
		if f.phase == "delete-error" {
			fail(http.StatusForbidden)
			return
		}
		f.app = nil
		write(true)
	default:
		f.t.Errorf("unexpected or unsafe application configuration route: %s", route)
		fail(http.StatusForbidden)
	}
}

func (f *liveApplicationConfigurationHTTPFixture) appHasValidSource() bool {
	if f.app == nil {
		return false
	}
	kind, _ := f.app["type"].(string)
	switch kind {
	case "git":
		return f.app["customGitUrl"] != nil && f.app["customGitBranch"] != nil
	default:
		return true
	}
}

func offlineConfigurationGitSource() ApplicationSource {
	return ApplicationSource{Type: SourceGit, Git: &GitApplicationSource{URL: "https://github.com/pulumi/pulumi-dokploy", Branch: "main", BuildPath: stringPtr("services/api"), WatchPaths: []string{"services/api/**"}, EnableSubmodules: true, Build: ApplicationBuild{Type: BuildNixpacks}}}
}

func offlineConfigurationGitHubSource() ApplicationSource {
	return ApplicationSource{Type: SourceGitHub, GitHub: &GitHubAppSource{IntegrationID: "offline-github-integration", Owner: "offline-owner", Repository: "offline-repository", Branch: "main", Build: ApplicationBuild{Type: BuildNixpacks}}}
}

func TestLiveApplicationConfigurationAutoDeployReadRequiresAcknowledgedIdentity(t *testing.T) {
	tests := []struct {
		name   string
		phase  string
		source ApplicationSource
	}{
		{name: "missing ID", phase: "missing-auto-deploy-id", source: offlineConfigurationGitSource()},
		{name: "blank ID", phase: "blank-auto-deploy-id", source: offlineConfigurationGitSource()},
		{name: "mismatched ID", phase: "mismatched-auto-deploy-id", source: offlineConfigurationGitSource()},
		{name: "mismatched ID before integrated source attachment", phase: "mismatched-integrated-source-id", source: offlineConfigurationGitHubSource()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			marker := isolateLiveRecordHarness(t)
			f := newLiveApplicationConfigurationHTTPFixture(t, tc.phase)
			guard := newLiveApplicationConfigurationGuard()
			lease := beginLiveHeavyOperation(t, "offline-configuration-create")
			fixture, setupErr := createLiveApplicationConfigurationFixture(t, t.Context(), f.api, "offline-environment", tc.source, guard)
			lease.releaseIfNeeded(t)
			require.NotNil(t, fixture, "the acknowledged fixture must remain owned after identity verification fails")
			require.NoError(t, fixture.cleanup(), "cleanup must use only the acknowledged create ID")

			f.mu.Lock()
			sourceRoute := "application.saveGitProvider"
			if tc.source.Type == SourceGitHub {
				sourceRoute = "application.saveGithubProvider"
			}
			require.Equal(t, 1, f.counts["application.create"])
			require.Equal(t, 1, f.counts["application.update"], "only autoDeploy:false may mutate the acknowledged fixture before identity verification")
			require.Zero(t, f.counts[sourceRoute], "source must not attach when application.one identity is untrusted")
			require.Zero(t, f.counts["application.saveBuildType"], "build must not attach when application.one identity is untrusted")
			require.Equal(t, 1, f.counts["application.delete"], "the acknowledged fixture must be cleaned up")
			require.Equal(t, []string{"application.create", "application.update", "application.one", "application.delete", "application.one"}, f.routes)
			require.Equal(t, []string{"offline-application", "offline-application"}, f.mutationIDs, "update and delete must never target the unrelated ID returned by application.one")
			require.Nil(t, f.app)
			f.mu.Unlock()

			require.Error(t, setupErr, "missing, blank, and mismatched read identities must fail closed")
			for _, private := range []string{"offline-application", "offline-unrelated-application", "private-upstream-detail"} {
				require.NotContains(t, setupErr.Error(), private, "identity diagnostics must not expose IDs or upstream details")
			}
			retained := false
			guard.retainProjectIfNeeded(func() { retained = true })
			require.False(t, retained, "successful acknowledged fixture cleanup releases parent retention")
			require.False(t, heavyLiveTierStopped())
			_, err := os.Stat(marker)
			require.True(t, os.IsNotExist(err))
		})
	}
}

func TestLiveApplicationConfigurationNeverDeploys(t *testing.T) {
	marker := isolateLiveRecordHarness(t)
	f := newLiveApplicationConfigurationHTTPFixture(t, "success")
	guard := newLiveApplicationConfigurationGuard()
	err := runLiveApplicationConfigurationLifecycle(t, t.Context(), f.api, "offline-environment", offlineConfigurationGitSource(), guard)
	require.NoError(t, err)
	retained := false
	guard.retainProjectIfNeeded(func() { retained = true })
	require.False(t, retained)

	f.mu.Lock()
	defer f.mu.Unlock()
	require.Nil(t, f.app)
	require.False(t, f.typedReadBeforeGone)
	require.Equal(t, 1, f.counts["application.create"])
	require.GreaterOrEqual(t, f.counts["application.update"], 2, "disable autoDeploy before source save and exercise its configuration-only provider update")
	require.Equal(t, 1, f.counts["application.saveGitProvider"])
	require.GreaterOrEqual(t, f.counts["application.saveBuildType"], 8, "all builders, same-builder options, transitions, and cleared Nixpacks fields are saved")
	require.GreaterOrEqual(t, f.counts["application.one"], 2, "read/import and raw ID-only absence verification are exercised")
	require.Equal(t, 1, f.counts["application.delete"])
	require.Equal(t, []string{"application.create", "application.update", "application.one", "application.saveGitProvider", "application.saveBuildType"}, f.routes[:5], "verify autoDeploy:false on the bare record before attaching its source")
	require.NotContains(t, f.routes, "application.deploy")
	require.NotContains(t, f.routes, "application.redeploy")
	require.NotContains(t, f.routes, "application.fetch")
	require.NotContains(t, f.routes, "compose.fetch")
	require.False(t, heavyLiveTierStopped())
	_, err = os.Stat(marker)
	require.True(t, os.IsNotExist(err))
	for _, builder := range []string{string(BuildDockerfile), string(BuildNixpacks), string(BuildHerokuBuildpacks), string(BuildPaketoBuildpacks), string(BuildStatic), string(BuildRailpack)} {
		require.Contains(t, f.buildTypes, builder)
	}
	require.Equal(t, "dist", f.buildBodies[1]["publishDirectory"])
	require.Equal(t, "public", f.buildBodies[2]["publishDirectory"], "same-builder option mutation must be saved")
	lastBuild := f.buildBodies[len(f.buildBodies)-1]
	require.Equal(t, string(BuildNixpacks), lastBuild["buildType"])
	for _, option := range []string{"dockerfile", "dockerContextPath", "dockerBuildStage", "herokuVersion", "railpackVersion", "publishDirectory", "isStaticSpa"} {
		require.Nil(t, lastBuild[option], "returning to Nixpacks clears %s", option)
	}
}

func TestLiveApplicationConfigurationOwnership(t *testing.T) {
	t.Run("accepted identity and independent cleanup context", func(t *testing.T) {
		isolateLiveRecordHarness(t)
		f := newLiveApplicationConfigurationHTTPFixture(t, "success")
		guard := newLiveApplicationConfigurationGuard()
		ctx, cancel := context.WithCancel(t.Context())
		lease := beginLiveHeavyOperation(t, "offline-configuration-create")
		fixture, err := createLiveApplicationConfigurationFixture(t, ctx, f.api, "offline-environment", offlineConfigurationGitSource(), guard)
		lease.releaseIfNeeded(t)
		require.NoError(t, err)
		require.Equal(t, "offline-application", fixture.id)
		cancel()
		require.NoError(t, fixture.cleanup(), "cleanup must use a fresh bounded context, not the canceled setup context")
		retained := false
		guard.retainProjectIfNeeded(func() { retained = true })
		require.False(t, retained)
		f.mu.Lock()
		defer f.mu.Unlock()
		require.Nil(t, f.app)
		require.Equal(t, 1, f.counts["application.delete"])
		require.Equal(t, 2, f.counts["application.one"], "one raw autoDeploy check and one raw absence check must not decode source data")
		require.False(t, f.typedReadBeforeGone)
	})

	t.Run("acknowledged partial setup error cleans incomplete source", func(t *testing.T) {
		marker := isolateLiveRecordHarness(t)
		f := newLiveApplicationConfigurationHTTPFixture(t, "source-error")
		guard := newLiveApplicationConfigurationGuard()
		var fixture *liveApplicationConfigurationFixture
		t.Run("fixture", func(t *testing.T) {
			lease := beginLiveHeavyOperation(t, "offline-configuration-create")
			var err error
			fixture, err = createLiveApplicationConfigurationFixture(t, t.Context(), f.api, "offline-environment", offlineConfigurationGitSource(), guard)
			lease.releaseIfNeeded(t)
			require.Error(t, err)
			require.NotNil(t, fixture, "the acknowledged application remains owned after source setup fails")
		})
		f.mu.Lock()
		require.Nil(t, f.app)
		require.Equal(t, 1, f.counts["application.delete"])
		require.Equal(t, 2, f.counts["application.one"], "autoDeploy and absence checks must not invoke Application.Read on incomplete source data")
		require.False(t, f.typedReadBeforeGone)
		f.mu.Unlock()
		retained := false
		guard.retainProjectIfNeeded(func() { retained = true })
		require.False(t, retained)
		require.False(t, heavyLiveTierStopped())
		_, err := os.Stat(marker)
		require.True(t, os.IsNotExist(err))
	})

	t.Run("definite create rejection does not retain or stop", func(t *testing.T) {
		marker := isolateLiveRecordHarness(t)
		f := newLiveApplicationConfigurationHTTPFixture(t, "success")
		f.mu.Lock()
		f.rejectCreateStatus = http.StatusForbidden
		f.mu.Unlock()
		guard := newLiveApplicationConfigurationGuard()
		lease := beginLiveHeavyOperation(t, "offline-configuration-create")
		fixture, err := createLiveApplicationConfigurationFixture(t, t.Context(), f.api, "offline-environment", offlineConfigurationGitSource(), guard)
		lease.releaseIfNeeded(t)
		require.Error(t, err)
		require.Nil(t, fixture)
		retained := false
		guard.retainProjectIfNeeded(func() { retained = true })
		require.False(t, retained)
		f.mu.Lock()
		defer f.mu.Unlock()
		require.Equal(t, []string{"application.create"}, f.routes, "a definite rejection has no acknowledged identity to clean up")
		require.False(t, heavyLiveTierStopped())
		_, err = os.Stat(marker)
		require.True(t, os.IsNotExist(err))
	})

	for _, phase := range []string{"no-id-create", "transport-create"} {
		t.Run(phase+" retains parent and stops without retry or search", func(t *testing.T) {
			marker := isolateLiveRecordHarness(t)
			f := newLiveApplicationConfigurationHTTPFixture(t, phase)
			guard := newLiveApplicationConfigurationGuard()
			lease := beginLiveHeavyOperation(t, "offline-configuration-create")
			fixture, err := createLiveApplicationConfigurationFixture(t, t.Context(), f.api, "offline-environment", offlineConfigurationGitSource(), guard)
			lease.releaseIfNeeded(t)
			require.Error(t, err)
			require.Nil(t, fixture)
			retained := false
			guard.retainProjectIfNeeded(func() { retained = true })
			require.True(t, retained)
			require.True(t, heavyLiveTierStopped())
			contents, err := os.ReadFile(marker)
			require.NoError(t, err)
			require.Equal(t, "stop\n", string(contents))
			f.mu.Lock()
			require.Equal(t, []string{"application.create"}, f.routes, "unknown create ownership must never retry, search by name, or delete an unacknowledged ID")
			require.Nil(t, f.app)
			f.mu.Unlock()
		})
	}
}

func TestLiveApplicationConfigurationCleanupFailure(t *testing.T) {
	phase := os.Getenv("PULUMI_DOKPLOY_OFFLINE_APPLICATION_CONFIGURATION_CLEANUP_CHILD")
	if phase == "" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestLiveApplicationConfigurationCleanupFailure$", "-test.v") //nolint:gosec
		cmd.Env = append(os.Environ(), "DOKPLOY_ACCEPTANCE=0", "PULUMI_DOKPLOY_OFFLINE_APPLICATION_CONFIGURATION_CLEANUP_CHILD=delete-error")
		out, err := cmd.CombinedOutput()
		require.Error(t, err, "unresolved acknowledged fixture cleanup must fail the child test")
		require.Contains(t, string(out), "offline application configuration cleanup guard verified")
		require.Contains(t, string(out), "operation=cleanup")
		for _, private := range []string{"offline-application", "offline-environment", "private-upstream-detail"} {
			require.NotContains(t, string(out), private)
		}
		return
	}

	marker := isolateLiveRecordHarness(t)
	f := newLiveApplicationConfigurationHTTPFixture(t, phase)
	guard := newLiveApplicationConfigurationGuard()
	t.Run("fixture", func(t *testing.T) {
		lease := beginLiveHeavyOperation(t, "offline-configuration-create")
		fixture, err := createLiveApplicationConfigurationFixture(t, t.Context(), f.api, "offline-environment", offlineConfigurationGitSource(), guard)
		lease.releaseIfNeeded(t)
		require.NoError(t, err)
		require.NotNil(t, fixture)
	})
	retained := false
	guard.retainProjectIfNeeded(func() { retained = true })
	require.True(t, retained)
	require.True(t, heavyLiveTierStopped())
	contents, err := os.ReadFile(marker)
	require.NoError(t, err)
	require.Equal(t, "stop\n", string(contents))
	f.mu.Lock()
	require.NotNil(t, f.app)
	require.Equal(t, 1, f.counts["application.delete"])
	f.mu.Unlock()
	t.Log("offline application configuration cleanup guard verified")
}

func TestLiveApplicationConfigurationPrerequisites(t *testing.T) {
	tests := []struct {
		name   string
		kind   ApplicationSourceType
		env    map[string]string
		want   ApplicationSourceType
		absent bool
		bad    bool
	}{
		{name: "github absent", kind: SourceGitHub, absent: true},
		{name: "github complete", kind: SourceGitHub, want: SourceGitHub, env: map[string]string{"DOKPLOY_GITHUB_INTEGRATION_ID": "github-integration-sentinel", "DOKPLOY_GITHUB_OWNER": "github-owner-sentinel", "DOKPLOY_GITHUB_REPOSITORY": "github-repository-sentinel", "DOKPLOY_GITHUB_BRANCH": "main"}},
		{name: "github invalid populated", kind: SourceGitHub, bad: true, env: map[string]string{"DOKPLOY_GITHUB_INTEGRATION_ID": "github-integration-sentinel", "DOKPLOY_GITHUB_OWNER": "github-owner-sentinel", "DOKPLOY_GITHUB_REPOSITORY": "github-repository-sentinel", "DOKPLOY_GITHUB_BRANCH": " "}},
		{name: "gitea absent", kind: SourceGitea, absent: true},
		{name: "gitea complete", kind: SourceGitea, want: SourceGitea, env: map[string]string{"DOKPLOY_GITEA_INTEGRATION_ID": "gitea-integration-sentinel", "DOKPLOY_GITEA_OWNER": "gitea-owner-sentinel", "DOKPLOY_GITEA_REPOSITORY": "gitea-repository-sentinel", "DOKPLOY_GITEA_BRANCH": "main"}},
		{name: "gitea invalid populated", kind: SourceGitea, bad: true, env: map[string]string{"DOKPLOY_GITEA_INTEGRATION_ID": "gitea-integration-sentinel", "DOKPLOY_GITEA_OWNER": "gitea-owner-sentinel", "DOKPLOY_GITEA_REPOSITORY": "gitea-repository-sentinel"}},
		{name: "bitbucket absent", kind: SourceBitbucket, absent: true},
		{name: "bitbucket complete", kind: SourceBitbucket, want: SourceBitbucket, env: map[string]string{"DOKPLOY_BITBUCKET_INTEGRATION_ID": "bitbucket-integration-sentinel", "DOKPLOY_BITBUCKET_OWNER": "bitbucket-owner-sentinel", "DOKPLOY_BITBUCKET_REPOSITORY": "bitbucket-repository-sentinel", "DOKPLOY_BITBUCKET_REPOSITORY_SLUG": "workspace/repository-sentinel", "DOKPLOY_BITBUCKET_BRANCH": "main"}},
		{name: "bitbucket invalid populated", kind: SourceBitbucket, bad: true, env: map[string]string{"DOKPLOY_BITBUCKET_INTEGRATION_ID": "bitbucket-integration-sentinel", "DOKPLOY_BITBUCKET_OWNER": "bitbucket-owner-sentinel", "DOKPLOY_BITBUCKET_REPOSITORY": "bitbucket-repository-sentinel", "DOKPLOY_BITBUCKET_REPOSITORY_SLUG": "workspace/repository-sentinel"}},
		{name: "gitlab absent", kind: SourceGitLab, absent: true},
		{name: "gitlab complete", kind: SourceGitLab, want: SourceGitLab, env: map[string]string{"DOKPLOY_GITLAB_INTEGRATION_ID": "gitlab-integration-sentinel", "DOKPLOY_GITLAB_PROJECT_ID": "42", "DOKPLOY_GITLAB_OWNER": "gitlab-owner-sentinel", "DOKPLOY_GITLAB_NAMESPACE": "gitlab-namespace-sentinel", "DOKPLOY_GITLAB_REPOSITORY": "gitlab-repository-sentinel", "DOKPLOY_GITLAB_BRANCH": "main", "DOKPLOY_GITLAB_USERNAME": "gitlab-username-sentinel", "DOKPLOY_GITLAB_TOKEN": "gitlab-token-sentinel"}},
		{name: "gitlab invalid populated", kind: SourceGitLab, bad: true, env: map[string]string{"DOKPLOY_GITLAB_INTEGRATION_ID": "gitlab-integration-sentinel", "DOKPLOY_GITLAB_PROJECT_ID": "not-a-number-sentinel", "DOKPLOY_GITLAB_OWNER": "gitlab-owner-sentinel", "DOKPLOY_GITLAB_NAMESPACE": "gitlab-namespace-sentinel", "DOKPLOY_GITLAB_REPOSITORY": "gitlab-repository-sentinel", "DOKPLOY_GITLAB_BRANCH": "main", "DOKPLOY_GITLAB_USERNAME": "gitlab-username-sentinel", "DOKPLOY_GITLAB_TOKEN": "gitlab-token-sentinel"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			source, configured, err := parseLiveApplicationSourcePrerequisites(tc.kind, tc.env)
			if tc.absent {
				require.NoError(t, err)
				require.False(t, configured)
				require.Empty(t, source.Type)
				return
			}
			if tc.bad {
				require.Error(t, err)
				require.True(t, configured, "invalid populated values must fail rather than become optional skips")
				require.Empty(t, source.Type)
				for _, secret := range tc.env {
					if strings.TrimSpace(secret) != "" {
						require.NotContains(t, err.Error(), secret)
					}
				}
				return
			}
			require.NoError(t, err)
			require.True(t, configured)
			require.Equal(t, tc.want, source.Type)
			require.NoError(t, source.validate())
		})
	}
	_, _, err := parseLiveApplicationSourcePrerequisites(ApplicationSourceType("unknown"), map[string]string{})
	require.Error(t, err)
}

func TestLiveApplicationConfigurationPrerequisiteErrorIsStructural(t *testing.T) {
	_, _, err := parseLiveApplicationSourcePrerequisites(SourceGitHub, map[string]string{
		"DOKPLOY_GITHUB_INTEGRATION_ID": "credential-sentinel", "DOKPLOY_GITHUB_OWNER": "owner-sentinel",
		"DOKPLOY_GITHUB_REPOSITORY": "repository-sentinel", "DOKPLOY_GITHUB_BRANCH": "invalid branch sentinel",
	})
	require.Error(t, err)
	for _, private := range []string{"credential-sentinel", "owner-sentinel", "repository-sentinel", "invalid branch sentinel"} {
		require.NotContains(t, err.Error(), private)
	}
}

func TestLiveApplicationConfigurationMetadataRedactsAllSourceFields(t *testing.T) {
	resetLiveSecretRegistry()
	t.Cleanup(resetLiveSecretRegistry)
	sources := []struct {
		source ApplicationSource
		values []string
	}{
		{
			source: ApplicationSource{Type: SourceGit, Git: &GitApplicationSource{URL: "git-url-sentinel", Branch: "git-branch-sentinel", BuildPath: stringPtr("git-build-path-sentinel"), SSHKeyID: stringPtr("git-ssh-id-sentinel"), WatchPaths: []string{"git-watch-sentinel"}, EnableSubmodules: true, Build: ApplicationBuild{Type: BuildNixpacks, PublishDirectory: stringPtr("git-publish-sentinel"), IsStaticSpa: appBoolPtr(true)}}},
			values: []string{"git-url-sentinel", "git-branch-sentinel", "git-build-path-sentinel", "git-ssh-id-sentinel", "git-watch-sentinel", "git-publish-sentinel"},
		},
		{
			source: ApplicationSource{Type: SourceGitHub, GitHub: &GitHubAppSource{IntegrationID: "github-id-sentinel", Owner: "github-owner-sentinel", Repository: "github-repo-sentinel", Branch: "github-branch-sentinel", BuildPath: stringPtr("github-path-sentinel"), TriggerType: githubTriggerPointer(GitHubTriggerTag), WatchPaths: []string{"github-watch-sentinel"}, EnableSubmodules: true, Build: ApplicationBuild{Type: BuildDockerfile, Dockerfile: stringPtr("github-dockerfile-sentinel"), DockerContextPath: stringPtr("github-context-sentinel"), DockerBuildStage: stringPtr("github-stage-sentinel")}}},
			values: []string{"github-id-sentinel", "github-owner-sentinel", "github-repo-sentinel", "github-branch-sentinel", "github-path-sentinel", "github-watch-sentinel", "tag", "github-dockerfile-sentinel", "github-context-sentinel", "github-stage-sentinel"},
		},
		{
			source: ApplicationSource{Type: SourceGitea, Gitea: &GiteaAppSource{IntegrationID: "gitea-id-sentinel", Owner: "gitea-owner-sentinel", Repository: "gitea-repo-sentinel", Branch: "gitea-branch-sentinel", BuildPath: stringPtr("gitea-path-sentinel"), WatchPaths: []string{"gitea-watch-sentinel"}, EnableSubmodules: true, Build: ApplicationBuild{Type: BuildHerokuBuildpacks, HerokuVersion: stringPtr("gitea-heroku-version-sentinel")}}},
			values: []string{"gitea-id-sentinel", "gitea-owner-sentinel", "gitea-repo-sentinel", "gitea-branch-sentinel", "gitea-path-sentinel", "gitea-watch-sentinel", "gitea-heroku-version-sentinel"},
		},
		{
			source: ApplicationSource{Type: SourceBitbucket, Bitbucket: &BitbucketAppSource{IntegrationID: "bitbucket-id-sentinel", Owner: "bitbucket-owner-sentinel", Repository: "bitbucket-repo-sentinel", RepositorySlug: "bitbucket-slug-sentinel", Branch: "bitbucket-branch-sentinel", BuildPath: stringPtr("bitbucket-path-sentinel"), WatchPaths: []string{"bitbucket-watch-sentinel"}, EnableSubmodules: true, Build: ApplicationBuild{Type: BuildRailpack, RailpackVersion: stringPtr("bitbucket-railpack-version-sentinel")}}},
			values: []string{"bitbucket-id-sentinel", "bitbucket-owner-sentinel", "bitbucket-repo-sentinel", "bitbucket-slug-sentinel", "bitbucket-branch-sentinel", "bitbucket-path-sentinel", "bitbucket-watch-sentinel", "bitbucket-railpack-version-sentinel"},
		},
	}
	for _, tc := range sources {
		unregister := registerLiveApplicationSourceMetadata(tc.source)
		diagnostic := sanitizeLiveDiagnostic(strings.Join(tc.values, " ") + " true")
		unregister()
		for _, value := range tc.values {
			require.NotContains(t, diagnostic, value)
		}
		require.NotContains(t, diagnostic, "true", "boolean source/build values are included in metadata redaction")
	}
}
