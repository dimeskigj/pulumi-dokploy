package dokploy

import (
	"encoding/json"
	"net/http"
	"testing"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
)

func TestComposeStatusPreservesAPIStatusAndRejectsMissingStatus(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
	}{
		{"running", `{"composeId":"c1","status":"running"}`, "running"},
		{"done", `{"composeId":"c1","status":"done"}`, "done"},
		{"error", `{"composeId":"c1","status":"error"}`, "error"},
		{"unknown", `{"composeId":"c1","status":"paused"}`, "paused"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newScriptedServer(t, expectGET("/api/compose.one", map[string][]string{"composeId": {"c1"}}, http.StatusOK, tc.body))
			got, err := composeStatus(t.Context(), s.API(), "c1")
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
	s := newScriptedServer(t, expectGET("/api/compose.one", map[string][]string{"composeId": {"c1"}}, http.StatusOK, `{"composeId":"c1"}`))
	_, err := composeStatus(t.Context(), s.API(), "c1")
	require.EqualError(t, err, "compose.one returned compose without a status")
}

func TestComposeStatusRejectsUnparseableStatus(t *testing.T) {
	s := newScriptedServer(t, expectGET("/api/compose.one", map[string][]string{"composeId": {"c1"}}, http.StatusOK, `{"composeId":"c1","status":42}`))
	_, err := composeStatus(t.Context(), s.API(), "c1")
	require.EqualError(t, err, "compose.one returned invalid status 42")
}

func TestComposeGitUpdateClearingSSHKeySendsNull(t *testing.T) {
	s := newScriptedServer(t, expectPOST("/api/compose.update", `{"composeId":"c1","composePath":"./docker-compose.yml","customGitBranch":"main","customGitSSHKeyId":null,"customGitUrl":"https://example.test/repo","enableSubmodules":false,"sourceType":"git","watchPaths":null}`, `{}`))
	args := ComposeSource{Type: ComposeSourceGit, Git: &GitComposeSource{URL: "https://example.test/repo", Branch: "main"}}
	require.NoError(t, configureComposeSource(t.Context(), s.API(), "c1", args))
}

func TestComposeReadPreservesAPIStatus(t *testing.T) {
	s := newScriptedServer(t, expectGET("/api/compose.one", map[string][]string{"composeId": {"c1"}}, http.StatusOK, `{"composeId":"c1","name":"demo","environmentId":"e1","status":"running","source":{"type":"raw","composeFile":"services: {}"}}`))
	got, err := (Compose{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[ComposeArgs, ComposeState]{ID: "c1"})
	require.NoError(t, err)
	require.Equal(t, "running", got.State.Status)
}

func TestComposeReadMissingStatusFails(t *testing.T) {
	s := newScriptedServer(t, expectGET("/api/compose.one", map[string][]string{"composeId": {"c1"}}, http.StatusOK, `{"composeId":"c1","name":"demo","environmentId":"e1","source":{"type":"raw","composeFile":"services: {}"}}`))
	_, err := (Compose{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[ComposeArgs, ComposeState]{ID: "c1"})
	require.EqualError(t, err, "compose.one returned compose without a status")
}

func TestComposeCreateErrorStatusReturnsPartialInitializationFailure(t *testing.T) {
	old := waitPollInterval
	waitPollInterval = 0
	t.Cleanup(func() { waitPollInterval = old })
	s := newScriptedServer(t,
		expectPOST("/api/compose.create", `{"composeFile":"services: {}","composeType":"docker-compose","environmentId":"e1","name":"demo"}`, `{"composeId":"c1"}`),
		expectPOST("/api/compose.saveEnvironment", `{"composeId":"c1","env":null}`, `true`),
		expectPOST("/api/compose.deploy", `{"composeId":"c1"}`, `"running"`),
		expectGET("/api/compose.one", map[string][]string{"composeId": {"c1"}}, http.StatusOK, `{"composeId":"c1","status":"error"}`),
	)
	got, err := (Compose{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[ComposeArgs]{Inputs: ComposeArgs{Name: "demo", EnvironmentID: "e1", Source: ComposeSource{Type: ComposeSourceRaw, Raw: &RawComposeSource{ComposeFile: "services: {}"}}}})
	require.Error(t, err)
	require.Equal(t, "c1", got.ID)
}

func TestComposeSourceSchemaDoesNotExposeRawComposePath(t *testing.T) {
	b, err := json.Marshal(RawComposeSource{ComposeFile: "services: {}"})
	require.NoError(t, err)
	require.NotContains(t, string(b), "composePath")
}

func TestComposeImportReconstructsGitAndGitLabSources(t *testing.T) {
	git, err := decodeComposeSource(&map[string]interface{}{"type": "git", "customGitUrl": "https://git.test/repo", "customGitBranch": "main", "composePath": "app.yml", "customGitSSHKeyId": "key", "watchPaths": []interface{}{"app/**"}, "enableSubmodules": true}, ComposeSource{})
	require.NoError(t, err)
	require.Equal(t, "https://git.test/repo", git.Git.URL)
	require.Equal(t, "app.yml", git.Git.ComposePath)
	require.Equal(t, "key", *git.Git.SSHKeyID)
	gitlab, err := decodeComposeSource(&map[string]interface{}{"type": "gitlab", "gitlabId": "i1", "gitlabProjectId": float64(42), "gitlabOwner": "owner", "gitlabPathNamespace": "namespace", "gitlabRepository": "repo", "gitlabBranch": "main"}, ComposeSource{})
	require.NoError(t, err)
	require.Equal(t, 42, gitlab.GitLab.ProjectID)
	require.Equal(t, "./docker-compose.yml", gitlab.GitLab.ComposePath)
}

func TestComposeReadAndDeleteNotFoundAreIdempotent(t *testing.T) {
	s := newScriptedServer(t,
		scriptedRequest{Method: http.MethodGet, Path: "/api/compose.one", Query: map[string][]string{"composeId": {"missing"}}, Status: http.StatusNotFound, Response: []byte(`{"code":"NOT_FOUND"}`)},
		scriptedRequest{Method: http.MethodPost, Path: "/api/compose.delete", Body: json.RawMessage(`{"composeId":"missing","deleteVolumes":false}`), Status: http.StatusNotFound, Response: []byte(`{"code":"NOT_FOUND"}`)},
	)
	r := Compose{client: fixedClient(s.API())}
	read, err := r.Read(t.Context(), infer.ReadRequest[ComposeArgs, ComposeState]{ID: "missing"})
	require.NoError(t, err)
	require.Empty(t, read.ID)
	_, err = r.Delete(t.Context(), infer.DeleteRequest[ComposeState]{ID: "missing"})
	require.NoError(t, err)
}

func TestComposeGitSetupFailureCleansUpAndRedactsEnvironment(t *testing.T) {
	env := "TOP-SECRET"
	s := newScriptedServer(t,
		expectPOST("/api/compose.create", `{"composeType":"docker-compose","environmentId":"e1","name":"demo"}`, `{"composeId":"c1"}`),
		scriptedRequest{Method: http.MethodPost, Path: "/api/compose.update", Body: json.RawMessage(`{"composeId":"c1","composePath":"./docker-compose.yml","customGitBranch":"main","customGitSSHKeyId":null,"customGitUrl":"https://example.test/repo","enableSubmodules":false,"sourceType":"git","watchPaths":null}`), Status: http.StatusBadRequest, Response: []byte(`{"code":"SETUP_FAILED","message":"failed TOP-SECRET"}`)},
		expectPOST("/api/compose.delete", `{"composeId":"c1","deleteVolumes":false}`, `true`),
	)
	got, err := (Compose{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[ComposeArgs]{Inputs: ComposeArgs{Name: "demo", EnvironmentID: "e1", Environment: &env, Source: ComposeSource{Type: ComposeSourceGit, Git: &GitComposeSource{URL: "https://example.test/repo", Branch: "main"}}}})
	require.Error(t, err)
	require.Equal(t, "c1", got.ID)
	require.NotContains(t, err.Error(), env)
}

func TestComposeRawCreateOrdersEnvironmentBeforeDeploy(t *testing.T) {
	old := waitPollInterval
	waitPollInterval = 0
	t.Cleanup(func() { waitPollInterval = old })
	s := newScriptedServer(t,
		expectPOST("/api/compose.create", `{"composeFile":"services: {}","composeType":"docker-compose","environmentId":"e1","name":"demo"}`, `{"composeId":"c1"}`),
		expectPOST("/api/compose.saveEnvironment", `{"composeId":"c1","env":null}`, `true`),
		expectPOST("/api/compose.deploy", `{"composeId":"c1"}`, `"running"`),
		expectGET("/api/compose.one", map[string][]string{"composeId": {"c1"}}, http.StatusOK, `{"composeId":"c1","status":"done"}`),
	)
	got, err := (Compose{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[ComposeArgs]{Inputs: ComposeArgs{Name: "demo", EnvironmentID: "e1", Source: ComposeSource{Type: ComposeSourceRaw, Raw: &RawComposeSource{ComposeFile: "services: {}"}}}})
	require.NoError(t, err)
	require.Equal(t, "c1", got.ID)
	require.Equal(t, "done", got.Output.Status)
}

func TestComposeDeleteUsesConfiguredVolumeFlag(t *testing.T) {
	s := newScriptedServer(t, expectPOST("/api/compose.delete", `{"composeId":"c1","deleteVolumes":true}`, `true`))
	_, err := (Compose{client: fixedClient(s.API())}).Delete(t.Context(), infer.DeleteRequest[ComposeState]{ID: "c1", State: ComposeState{ComposeArgs: ComposeArgs{DeleteVolumesOnDestroy: true}}})
	require.NoError(t, err)
}

func TestComposeGitCreateConfiguresFetchEnvironmentDeployAndPoll(t *testing.T) {
	old := waitPollInterval
	waitPollInterval = 0
	t.Cleanup(func() { waitPollInterval = old })
	s := newScriptedServer(t,
		expectPOST("/api/compose.create", `{"composeType":"docker-compose","environmentId":"e1","name":"demo"}`, `{"composeId":"c1"}`),
		expectPOST("/api/compose.update", `{"composeId":"c1","composePath":"app/compose.yml","customGitBranch":"main","customGitSSHKeyId":null,"customGitUrl":"https://example.test/repo","enableSubmodules":true,"sourceType":"git","watchPaths":["app/**"]}`, `{}`),
		expectPOST("/api/compose.fetchSourceType", `{"composeId":"c1"}`, `true`),
		expectPOST("/api/compose.saveEnvironment", `{"composeId":"c1","env":"ENV-SECRET"}`, `true`),
		expectPOST("/api/compose.deploy", `{"composeId":"c1"}`, `"running"`),
		expectGET("/api/compose.one", map[string][]string{"composeId": {"c1"}}, http.StatusOK, `{"composeId":"c1","status":"done"}`),
	)
	env := "ENV-SECRET"
	args := ComposeArgs{Name: "demo", EnvironmentID: "e1", Environment: &env, Source: ComposeSource{Type: ComposeSourceGit, Git: &GitComposeSource{URL: "https://example.test/repo", Branch: "main", ComposePath: "app/compose.yml", WatchPaths: []string{"app/**"}, EnableSubmodules: true}}}
	got, err := (Compose{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[ComposeArgs]{Inputs: args})
	require.NoError(t, err)
	require.Equal(t, "c1", got.ID)
}

func TestComposeGitLabRuntimeUpdateConfiguresFetchEnvironmentRedeployAndPoll(t *testing.T) {
	old := waitPollInterval
	waitPollInterval = 0
	t.Cleanup(func() { waitPollInterval = old })
	s := newScriptedServer(t,
		expectPOST("/api/compose.update", `{"composeId":"c1","composePath":"./docker-compose.yml","enableSubmodules":false,"gitlabBranch":"main","gitlabId":"i1","gitlabOwner":"owner","gitlabPathNamespace":"namespace","gitlabProjectId":42,"gitlabRepository":"repo","sourceType":"gitlab","watchPaths":null}`, `{}`),
		expectPOST("/api/compose.fetchSourceType", `{"composeId":"c1"}`, `true`),
		expectPOST("/api/compose.saveEnvironment", `{"composeId":"c1","env":null}`, `true`),
		expectPOST("/api/compose.redeploy", `{"composeId":"c1"}`, `"running"`),
		expectGET("/api/compose.one", map[string][]string{"composeId": {"c1"}}, http.StatusOK, `{"composeId":"c1","status":"done"}`),
	)
	newArgs := ComposeArgs{Name: "demo", EnvironmentID: "e1", Source: ComposeSource{Type: ComposeSourceGitLab, GitLab: &GitLabComposeSource{IntegrationID: "i1", ProjectID: 42, Owner: "owner", Namespace: "namespace", Repository: "repo", Branch: "main"}}}
	oldArgs := ComposeArgs{Name: "demo", EnvironmentID: "e1", Source: ComposeSource{Type: ComposeSourceGitLab, GitLab: &GitLabComposeSource{IntegrationID: "i1", ProjectID: 42, Owner: "owner", Namespace: "namespace", Repository: "old", Branch: "main"}}}
	_, err := (Compose{client: fixedClient(s.API())}).Update(t.Context(), infer.UpdateRequest[ComposeArgs, ComposeState]{ID: "c1", Inputs: newArgs, State: ComposeState{ComposeArgs: oldArgs}})
	require.NoError(t, err)
}

func TestComposeUpdateErrorStatusReturnsError(t *testing.T) {
	old := waitPollInterval
	waitPollInterval = 0
	t.Cleanup(func() { waitPollInterval = old })
	s := newScriptedServer(t,
		expectPOST("/api/compose.update", `{"composeFile":"new","composeId":"c1"}`, `{}`),
		expectPOST("/api/compose.saveEnvironment", `{"composeId":"c1","env":null}`, `true`),
		expectPOST("/api/compose.redeploy", `{"composeId":"c1"}`, `"running"`),
		expectGET("/api/compose.one", map[string][]string{"composeId": {"c1"}}, http.StatusOK, `{"composeId":"c1","status":"error"}`),
	)
	newArgs := ComposeArgs{Name: "demo", EnvironmentID: "e1", Source: ComposeSource{Type: ComposeSourceRaw, Raw: &RawComposeSource{ComposeFile: "new"}}}
	oldArgs := ComposeArgs{Name: "demo", EnvironmentID: "e1", Source: ComposeSource{Type: ComposeSourceRaw, Raw: &RawComposeSource{ComposeFile: "old"}}}
	_, err := (Compose{client: fixedClient(s.API())}).Update(t.Context(), infer.UpdateRequest[ComposeArgs, ComposeState]{ID: "c1", Inputs: newArgs, State: ComposeState{ComposeArgs: oldArgs}})
	require.Error(t, err)
}

func TestComposeGitAndGitLabEnvironmentErrorsAreRedactedAcrossLifecycle(t *testing.T) {
	secret := "ENVIRONMENT-SECRET"
	for _, tc := range []struct {
		name                       string
		source                     ComposeSource
		providerPath, providerBody string
	}{
		{"git", ComposeSource{Type: ComposeSourceGit, Git: &GitComposeSource{URL: "https://git.test/repo", Branch: "main"}}, "/api/compose.update", `{"composeId":"c1","composePath":"./docker-compose.yml","customGitBranch":"main","customGitSSHKeyId":null,"customGitUrl":"https://git.test/repo","enableSubmodules":false,"sourceType":"git","watchPaths":null}`},
		{"gitlab", ComposeSource{Type: ComposeSourceGitLab, GitLab: &GitLabComposeSource{IntegrationID: "i1", ProjectID: 42, Owner: "owner", Namespace: "namespace", Repository: "repo", Branch: "main"}}, "/api/compose.update", `{"composeId":"c1","composePath":"./docker-compose.yml","enableSubmodules":false,"gitlabBranch":"main","gitlabId":"i1","gitlabOwner":"owner","gitlabPathNamespace":"namespace","gitlabProjectId":42,"gitlabRepository":"repo","sourceType":"gitlab","watchPaths":null}`},
	} {
		for _, stage := range []string{"source", "saveEnvironment", "deploy", "poll"} {
			t.Run(tc.name+"-"+stage, func(t *testing.T) {
				old := waitPollInterval
				waitPollInterval = 0
				t.Cleanup(func() { waitPollInterval = old })
				expectations := []scriptedRequest{expectPOST("/api/compose.create", `{"composeType":"docker-compose","environmentId":"e1","name":"demo"}`, `{"composeId":"c1"}`)}
				if stage == "source" {
					expectations = append(expectations, scriptedRequest{Method: http.MethodPost, Path: tc.providerPath, Body: json.RawMessage(tc.providerBody), Status: http.StatusBadRequest, Response: []byte(`{"code":"SOURCE_FAILED","message":"failed ENVIRONMENT-SECRET"}`)})
				} else {
					expectations = append(expectations, expectPOST(tc.providerPath, tc.providerBody, `{}`), expectPOST("/api/compose.fetchSourceType", `{"composeId":"c1"}`, `true`))
					if stage == "saveEnvironment" {
						expectations = append(expectations, scriptedRequest{Method: http.MethodPost, Path: "/api/compose.saveEnvironment", Body: json.RawMessage(`{"composeId":"c1","env":"ENVIRONMENT-SECRET"}`), Status: http.StatusBadRequest, Response: []byte(`{"code":"ENV_FAILED","message":"failed ENVIRONMENT-SECRET"}`)})
					} else {
						expectations = append(expectations, expectPOST("/api/compose.saveEnvironment", `{"composeId":"c1","env":"ENVIRONMENT-SECRET"}`, `true`))
						if stage == "deploy" {
							expectations = append(expectations, scriptedRequest{Method: http.MethodPost, Path: "/api/compose.deploy", Body: json.RawMessage(`{"composeId":"c1"}`), Status: http.StatusBadRequest, Response: []byte(`{"code":"DEPLOY_FAILED","message":"failed ENVIRONMENT-SECRET"}`)})
						} else {
							expectations = append(expectations, expectPOST("/api/compose.deploy", `{"composeId":"c1"}`, `"running"`), scriptedRequest{Method: http.MethodGet, Path: "/api/compose.one", Query: map[string][]string{"composeId": {"c1"}}, Status: http.StatusBadRequest, Response: []byte(`{"code":"POLL_FAILED","message":"failed ENVIRONMENT-SECRET"}`)})
						}
					}
				}
				if stage == "source" || stage == "saveEnvironment" {
					cleanupStatus := http.StatusOK
					cleanupResponse := []byte(`true`)
					if stage == "source" {
						cleanupStatus = http.StatusInternalServerError
						cleanupResponse = []byte(`{"code":"CLEANUP_FAILED","message":"cleanup ENVIRONMENT-SECRET"}`)
					}
					expectations = append(expectations, scriptedRequest{Method: http.MethodPost, Path: "/api/compose.delete", Body: json.RawMessage(`{"composeId":"c1","deleteVolumes":false}`), Status: cleanupStatus, Response: cleanupResponse})
				}
				s := newScriptedServer(t, expectations...)
				got, err := (Compose{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[ComposeArgs]{Inputs: ComposeArgs{Name: "demo", EnvironmentID: "e1", Environment: &secret, Source: tc.source}})
				require.Error(t, err)
				require.Equal(t, "c1", got.ID)
				require.NotContains(t, err.Error(), secret)
				require.NotContains(t, err.Error(), "CLEANUP_FAILED")
			})
		}
	}
}

func TestComposeDiffMarksSourceEnvironmentAndServerReplacements(t *testing.T) {
	oldServer, newServer := "s1", "s2"
	old := ComposeArgs{EnvironmentID: "e1", ServerID: &oldServer, Source: ComposeSource{Type: ComposeSourceGit, Git: &GitComposeSource{URL: "u", Branch: "main"}}}
	in := ComposeArgs{EnvironmentID: "e2", ServerID: &newServer, Source: ComposeSource{Type: ComposeSourceGitLab, GitLab: &GitLabComposeSource{IntegrationID: "i", ProjectID: 1, Owner: "o", Namespace: "n", Repository: "r", Branch: "main"}}}
	diff, err := (Compose{}).Diff(t.Context(), infer.DiffRequest[ComposeArgs, ComposeState]{Inputs: in, State: ComposeState{ComposeArgs: old}})
	require.NoError(t, err)
	require.Equal(t, p.UpdateReplace, diff.DetailedDiff["environmentId"].Kind)
	require.Equal(t, p.UpdateReplace, diff.DetailedDiff["serverId"].Kind)
	require.Equal(t, p.UpdateReplace, diff.DetailedDiff["source.type"].Kind)
}

func TestComposeReadReconstructsGitAndPreservesEnvironmentSecret(t *testing.T) {
	secret := "ENV-SECRET"
	s := newScriptedServer(t, expectGET("/api/compose.one", map[string][]string{"composeId": {"c1"}}, http.StatusOK, `{"composeId":"c1","name":"demo","environmentId":"e1","status":"done","source":{"type":"git","customGitUrl":"https://git.test/repo","customGitBranch":"main","composePath":"app.yml","watchPaths":["app/**"],"enableSubmodules":true}}`))
	got, err := (Compose{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[ComposeArgs, ComposeState]{ID: "c1", State: ComposeState{ComposeArgs: ComposeArgs{Environment: &secret}}})
	require.NoError(t, err)
	require.Equal(t, secret, *got.Inputs.Environment)
	require.Equal(t, "https://git.test/repo", got.Inputs.Source.Git.URL)
	require.Equal(t, "app.yml", got.Inputs.Source.Git.ComposePath)
}
