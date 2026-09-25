package dokploy

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

// TestComposeGitHubSourceRequestShape pins the compose.update body the provider
// sends for a GitHub Compose stack, including sourceType: "github".
func TestComposeGitHubSourceRequestShape(t *testing.T) {
	s := newScriptedServer(t, expectPOST("/api/compose.update", `{"composeId":"c1","branch":"release","composePath":"deploy/compose.yml","enableSubmodules":true,"githubId":"integration-sentinel","owner":"owner-sentinel","repository":"service-sentinel","sourceType":"github","triggerType":"tag","watchPaths":["deploy/**"]}`, `{}`))
	err := configureComposeSource(t.Context(), fixedClient(s.API())(t.Context()), "c1", ComposeSource{Type: ComposeSourceGitHub, GitHub: &GitHubComposeSource{
		IntegrationID: "integration-sentinel", Owner: "owner-sentinel", Repository: "service-sentinel", Branch: "release",
		ComposePath: "deploy/compose.yml", WatchPaths: []string{"deploy/**"}, TriggerType: ComposeTriggerTag, EnableSubmodules: true,
	}})
	require.NoError(t, err)
}

// TestComposeGitHubSourceDecodeToEncodeRoundTrip decodes a GitHub Compose stack as
// compose.one reports it and re-encodes it through the same compose.update call
// Create and Update use, so a read followed by a write cannot drop a field.
func TestComposeGitHubSourceDecodeToEncodeRoundTrip(t *testing.T) {
	const response = `{"composeId":"c1","sourceType":"github","githubId":"integration-sentinel","owner":"owner-sentinel","repository":"service-sentinel","branch":"release","composePath":"deploy/compose.yml","watchPaths":["deploy/**","README.md"],"triggerType":"tag","enableSubmodules":true}`
	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(response), &raw))

	decoded, err := decodeComposeSource(raw, ComposeSource{})
	require.NoError(t, err)
	require.Equal(t, ComposeSource{Type: ComposeSourceGitHub, GitHub: &GitHubComposeSource{
		IntegrationID: "integration-sentinel", Owner: "owner-sentinel", Repository: "service-sentinel", Branch: "release",
		ComposePath: "deploy/compose.yml", WatchPaths: []string{"deploy/**", "README.md"},
		TriggerType: ComposeTriggerTag, EnableSubmodules: true,
	}}, decoded)

	s := newScriptedServer(t, expectPOST("/api/compose.update", `{"composeId":"c1","branch":"release","composePath":"deploy/compose.yml","enableSubmodules":true,"githubId":"integration-sentinel","owner":"owner-sentinel","repository":"service-sentinel","sourceType":"github","triggerType":"tag","watchPaths":["deploy/**","README.md"]}`, `{}`))
	require.NoError(t, configureComposeSource(t.Context(), fixedClient(s.API())(t.Context()), "c1", decoded))
}

func TestComposeGitHubSourceDecodeDefaultsTriggerTypeToPush(t *testing.T) {
	decoded, err := decodeComposeSource(map[string]interface{}{
		"sourceType": "github", "githubId": "i1", "owner": "owner", "repository": "repo", "branch": "main",
	}, ComposeSource{})
	require.NoError(t, err)
	require.Equal(t, ComposeTriggerPush, decoded.GitHub.TriggerType)
	require.Equal(t, defaultComposePath, decoded.GitHub.ComposePath)
}

func TestComposeGitHubSourceCheckDefaultsComposePathAndTriggerType(t *testing.T) {
	source := property.New(map[string]property.Value{
		"type": property.New("github"),
		"github": property.New(map[string]property.Value{
			"integrationId": property.New("i1"), "owner": property.New("owner"),
			"repository": property.New("repo"), "branch": property.New("main"),
		}),
	})
	got, err := (Compose{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{
		"name": property.New("demo"), "environmentId": property.New("e1"), "source": source,
	})})
	require.NoError(t, err)
	require.Empty(t, got.Failures)
	require.Equal(t, defaultComposePath, got.Inputs.Source.GitHub.ComposePath)
	require.Equal(t, ComposeTriggerPush, got.Inputs.Source.GitHub.TriggerType)
}

// TestComposeGitHubImportedStateMatchesProgramInputs walks the import path the
// way it is really walked: a program through Check on one side, compose.one
// through Read on the other. Diffing a read against itself compares a value with
// itself and would pass even if the decoder returned nothing.
func TestComposeGitHubImportedStateMatchesProgramInputs(t *testing.T) {
	for _, tc := range []struct {
		name       string
		watchPaths string
		inputs     map[string]property.Value
	}{
		{
			name:       "declared",
			watchPaths: `["deploy/**"]`,
			inputs: map[string]property.Value{
				"integrationId": property.New("i1"), "owner": property.New("owner"),
				"repository": property.New("repo"), "branch": property.New("main"),
				"composePath": property.New("deploy/compose.yml"), "enableSubmodules": property.New(true),
				"watchPaths": property.New([]property.Value{property.New("deploy/**")}),
			},
		},
		{
			// A program that omits watchPaths yields nil; compose.one reports the
			// same stack with an empty list. Those mean the same thing, and must
			// not read as drift.
			name:       "omitted",
			watchPaths: `[]`,
			inputs: map[string]property.Value{
				"integrationId": property.New("i1"), "owner": property.New("owner"),
				"repository": property.New("repo"), "branch": property.New("main"),
				"composePath": property.New("deploy/compose.yml"), "enableSubmodules": property.New(true),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := `{"composeId":"c1","name":"demo","environmentId":"e1","composeStatus":"done","composeType":"docker-compose","type":"github","githubId":"i1","owner":"owner","repository":"repo","branch":"main","composePath":"deploy/compose.yml","watchPaths":` + tc.watchPaths + `,"triggerType":"push","enableSubmodules":true}`
			s := newScriptedServer(t, expectGET("/api/compose.one", map[string][]string{"composeId": {"c1"}}, http.StatusOK, response))
			read, err := (Compose{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[ComposeArgs, ComposeState]{ID: "c1"})
			require.NoError(t, err)
			require.Equal(t, ComposeSourceGitHub, read.Inputs.Source.Type)

			checked, err := (Compose{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{
				"name": property.New("demo"), "environmentId": property.New("e1"),
				"source": property.New(map[string]property.Value{
					"type": property.New("github"), "github": property.New(tc.inputs),
				}),
			})})
			require.NoError(t, err)
			require.Empty(t, checked.Failures)

			diff, err := (Compose{}).Diff(t.Context(), infer.DiffRequest[ComposeArgs, ComposeState]{Inputs: checked.Inputs, State: read.State})
			require.NoError(t, err)
			require.False(t, diff.HasChanges, "imported state must match the program that declared it")
			require.Empty(t, diff.DetailedDiff)
		})
	}
}

// TestComposeWatchPathsNilAndEmptyDoNotRedeploy pins the other half: a runtime
// change redeploys the stack, so nil versus empty must not count as one. The
// scripted server expects nothing, so any request fails the test.
func TestComposeWatchPathsNilAndEmptyDoNotRedeploy(t *testing.T) {
	s := newScriptedServer(t)
	program := ComposeArgs{Name: "demo", EnvironmentID: "e1", ComposeType: ComposeDocker, Source: ComposeSource{Type: ComposeSourceGitHub, GitHub: &GitHubComposeSource{
		IntegrationID: "i1", Owner: "owner", Repository: "repo", Branch: "main", ComposePath: defaultComposePath, TriggerType: ComposeTriggerPush,
	}}}
	imported := program
	imported.Source.GitHub = &GitHubComposeSource{
		IntegrationID: "i1", Owner: "owner", Repository: "repo", Branch: "main", ComposePath: defaultComposePath,
		TriggerType: ComposeTriggerPush, WatchPaths: []string{},
	}
	_, err := (Compose{client: fixedClient(s.API())}).Update(t.Context(), infer.UpdateRequest[ComposeArgs, ComposeState]{ID: "c1", Inputs: program, State: ComposeState{ComposeArgs: imported}})
	require.NoError(t, err)
}

func TestComposeGitHubSourceValidate(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source ComposeSource
		want   string
	}{
		{"missing", ComposeSource{Type: ComposeSourceGitHub}, "source.github is required when source.type is github"},
		{"extra raw", ComposeSource{Type: ComposeSourceGitHub, GitHub: validGitHubCompose(), Raw: &RawComposeSource{ComposeFile: "services: {}"}}, "source.raw must be omitted when source.type is github"},
		{"extra git", ComposeSource{Type: ComposeSourceGitHub, GitHub: validGitHubCompose(), Git: &GitComposeSource{URL: "u", Branch: "main"}}, "source.git must be omitted when source.type is github"},
		{"extra gitlab", ComposeSource{Type: ComposeSourceGitHub, GitHub: validGitHubCompose(), GitLab: &GitLabComposeSource{IntegrationID: "i"}}, "source.gitlab must be omitted when source.type is github"},
		{"empty integrationId", ComposeSource{Type: ComposeSourceGitHub, GitHub: withGitHubCompose(func(s *GitHubComposeSource) { s.IntegrationID = "" })}, "source.github.integrationId must not be empty"},
		{"empty owner", ComposeSource{Type: ComposeSourceGitHub, GitHub: withGitHubCompose(func(s *GitHubComposeSource) { s.Owner = "" })}, "source.github.owner must not be empty"},
		{"empty repository", ComposeSource{Type: ComposeSourceGitHub, GitHub: withGitHubCompose(func(s *GitHubComposeSource) { s.Repository = "" })}, "source.github.repository must not be empty"},
		{"empty branch", ComposeSource{Type: ComposeSourceGitHub, GitHub: withGitHubCompose(func(s *GitHubComposeSource) { s.Branch = "" })}, "source.github.branch must not be empty"},
		{"invalid triggerType", ComposeSource{Type: ComposeSourceGitHub, GitHub: withGitHubCompose(func(s *GitHubComposeSource) { s.TriggerType = "release" })}, "source.github.triggerType must be one of push or tag"},
		{"empty triggerType", ComposeSource{Type: ComposeSourceGitHub, GitHub: withGitHubCompose(func(s *GitHubComposeSource) { s.TriggerType = "" })}, "source.github.triggerType must be one of push or tag"},
		{"valid push", ComposeSource{Type: ComposeSourceGitHub, GitHub: validGitHubCompose()}, ""},
		{"valid tag", ComposeSource{Type: ComposeSourceGitHub, GitHub: withGitHubCompose(func(s *GitHubComposeSource) { s.TriggerType = ComposeTriggerTag })}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.source.validate()
			if tc.want == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tc.want)
			}
		})
	}
}

func TestComposeGitHubSourceSchemaExposesTriggerType(t *testing.T) {
	spec, err := p.GetSchema(t.Context(), Name, Version, Provider())
	require.NoError(t, err)
	compose := spec.Resources["dokploy:index:Compose"]
	source := spec.Types[strings.TrimPrefix(compose.InputProperties["source"].Ref, "#/types/")]
	require.NotEmpty(t, source.Properties["github"].Ref)
	github := spec.Types[strings.TrimPrefix(source.Properties["github"].Ref, "#/types/")]
	require.Equal(t, "string", github.Properties["triggerType"].Type)
	require.Equal(t, "string", github.Properties["integrationId"].Type)
}

func validGitHubCompose() *GitHubComposeSource {
	return &GitHubComposeSource{IntegrationID: "i", Owner: "o", Repository: "r", Branch: "main", TriggerType: ComposeTriggerPush}
}

func withGitHubCompose(mutate func(*GitHubComposeSource)) *GitHubComposeSource {
	s := validGitHubCompose()
	mutate(s)
	return s
}

func TestComposeSourceNormalReadReconstructsAllFields(t *testing.T) {
	for _, tc := range []struct {
		name     string
		response string
		want     ComposeSource
	}{
		{
			name:     "git",
			response: `{"composeId":"c1","name":"demo","environmentId":"e1","composeStatus":"done","type":"git","customGitUrl":"https://git.test/repo","customGitBranch":"release","composePath":"deploy/compose.yml","customGitSSHKeyId":"ssh-1","watchPaths":["deploy/**","README.md"],"enableSubmodules":true}`,
			want:     ComposeSource{Type: ComposeSourceGit, Git: &GitComposeSource{URL: "https://git.test/repo", Branch: "release", ComposePath: "deploy/compose.yml", SSHKeyID: stringPtr("ssh-1"), WatchPaths: []string{"deploy/**", "README.md"}, EnableSubmodules: true}},
		},
		{
			name:     "gitlab",
			response: `{"composeId":"c1","name":"demo","environmentId":"e1","composeStatus":"done","type":"gitlab","gitlabId":"integration-1","gitlabProjectId":42,"gitlabOwner":"owner","gitlabPathNamespace":"platform/api","gitlabRepository":"service","gitlabBranch":"main","composePath":"deploy/compose.yml","watchPaths":["cmd/**"],"enableSubmodules":true}`,
			want:     ComposeSource{Type: ComposeSourceGitLab, GitLab: &GitLabComposeSource{IntegrationID: "integration-1", ProjectID: 42, Owner: "owner", Namespace: "platform/api", Repository: "service", Branch: "main", ComposePath: "deploy/compose.yml", WatchPaths: []string{"cmd/**"}, EnableSubmodules: true}},
		},
		{
			name:     "github",
			response: `{"composeId":"c1","name":"demo","environmentId":"e1","composeStatus":"done","type":"github","githubId":"integration-1","owner":"owner","repository":"service","branch":"main","composePath":"deploy/compose.yml","watchPaths":["deploy/**"],"triggerType":"tag","enableSubmodules":true}`,
			want:     ComposeSource{Type: ComposeSourceGitHub, GitHub: &GitHubComposeSource{IntegrationID: "integration-1", Owner: "owner", Repository: "service", Branch: "main", ComposePath: "deploy/compose.yml", WatchPaths: []string{"deploy/**"}, TriggerType: ComposeTriggerTag, EnableSubmodules: true}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newScriptedServer(t, expectGET("/api/compose.one", map[string][]string{"composeId": {"c1"}}, http.StatusOK, tc.response))
			got, err := (Compose{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[ComposeArgs, ComposeState]{ID: "c1"})
			require.NoError(t, err)
			assertComposeSourceFields(t, tc.want, got.Inputs.Source)
		})
	}
}

func TestComposeSourceIDOnlyReadReconstructsAllFields(t *testing.T) {
	for _, tc := range []struct {
		name     string
		response string
		want     ComposeSource
	}{
		{
			name:     "git",
			response: `{"composeId":"c1","composeStatus":"done","sourceType":"git","customGitUrl":"https://git.test/repo","customGitBranch":"release","composePath":"deploy/compose.yml","customGitSSHKeyId":"ssh-1","watchPaths":["deploy/**","README.md"],"enableSubmodules":true}`,
			want:     ComposeSource{Type: ComposeSourceGit, Git: &GitComposeSource{URL: "https://git.test/repo", Branch: "release", ComposePath: "deploy/compose.yml", SSHKeyID: stringPtr("ssh-1"), WatchPaths: []string{"deploy/**", "README.md"}, EnableSubmodules: true}},
		},
		{
			name:     "gitlab",
			response: `{"composeId":"c1","composeStatus":"done","sourceType":"gitlab","gitlabId":"integration-1","gitlabProjectId":42,"gitlabOwner":"owner","gitlabPathNamespace":"platform/api","gitlabRepository":"service","gitlabBranch":"main","composePath":"deploy/compose.yml","watchPaths":["cmd/**"],"enableSubmodules":true}`,
			want:     ComposeSource{Type: ComposeSourceGitLab, GitLab: &GitLabComposeSource{IntegrationID: "integration-1", ProjectID: 42, Owner: "owner", Namespace: "platform/api", Repository: "service", Branch: "main", ComposePath: "deploy/compose.yml", WatchPaths: []string{"cmd/**"}, EnableSubmodules: true}},
		},
		{
			name:     "github",
			response: `{"composeId":"c1","composeStatus":"done","sourceType":"github","githubId":"integration-1","owner":"owner","repository":"service","branch":"main","composePath":"deploy/compose.yml","watchPaths":["deploy/**"],"triggerType":"tag","enableSubmodules":true}`,
			want:     ComposeSource{Type: ComposeSourceGitHub, GitHub: &GitHubComposeSource{IntegrationID: "integration-1", Owner: "owner", Repository: "service", Branch: "main", ComposePath: "deploy/compose.yml", WatchPaths: []string{"deploy/**"}, TriggerType: ComposeTriggerTag, EnableSubmodules: true}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newScriptedServer(t, expectGET("/api/compose.one", map[string][]string{"composeId": {"c1"}}, http.StatusOK, tc.response))
			got, err := (Compose{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[ComposeArgs, ComposeState]{ID: "c1"})
			require.NoError(t, err)
			assertComposeSourceFields(t, tc.want, got.Inputs.Source)
		})
	}
}

func TestComposeGitReadPreservesOptionalWatchPathsPresence(t *testing.T) {
	for _, tc := range []struct {
		name  string
		field string
		watch []string
	}{
		{"omitted", ``, nil},
		{"null", `,"watchPaths":null`, nil},
		{"empty", `,"watchPaths":[]`, []string{}},
		{"populated", `,"watchPaths":["deploy/**"]`, []string{"deploy/**"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := `{"composeId":"c1","composeStatus":"done","sourceType":"git","customGitUrl":"repo","customGitBranch":"main"` + tc.field + `}`
			s := newScriptedServer(t, expectGET("/api/compose.one", map[string][]string{"composeId": {"c1"}}, http.StatusOK, response))
			got, err := (Compose{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[ComposeArgs, ComposeState]{ID: "c1"})
			require.NoError(t, err)
			require.Equal(t, tc.watch, got.Inputs.Source.Git.WatchPaths)
		})
	}
}

func assertComposeSourceFields(t *testing.T, want, got ComposeSource) {
	t.Helper()
	require.Equal(t, want.Type, got.Type)
	if want.Git != nil {
		requireLiveEqual(t, "compose.source.git", true, got.Git != nil)
		if got.Git == nil {
			return
		}
		requireLiveEqual(t, "compose.source.git.url", want.Git.URL, got.Git.URL)
		requireLiveEqual(t, "compose.source.git.branch", want.Git.Branch, got.Git.Branch)
		requireLiveEqual(t, "compose.source.git.composePath", want.Git.ComposePath, got.Git.ComposePath)
		requireLiveEqual(t, "compose.source.git.sshKeyId", want.Git.SSHKeyID, got.Git.SSHKeyID)
		requireLiveEqual(t, "compose.source.git.watchPaths", want.Git.WatchPaths, got.Git.WatchPaths)
		requireLiveEqual(t, "compose.source.git.enableSubmodules", want.Git.EnableSubmodules, got.Git.EnableSubmodules)
	}
	if want.GitHub != nil {
		requireLiveEqual(t, "compose.source.github", true, got.GitHub != nil)
		if got.GitHub == nil {
			return
		}
		requireLiveEqual(t, "compose.source.github.integrationId", want.GitHub.IntegrationID, got.GitHub.IntegrationID)
		requireLiveEqual(t, "compose.source.github.owner", want.GitHub.Owner, got.GitHub.Owner)
		requireLiveEqual(t, "compose.source.github.repository", want.GitHub.Repository, got.GitHub.Repository)
		requireLiveEqual(t, "compose.source.github.branch", want.GitHub.Branch, got.GitHub.Branch)
		requireLiveEqual(t, "compose.source.github.composePath", want.GitHub.ComposePath, got.GitHub.ComposePath)
		requireLiveEqual(t, "compose.source.github.watchPaths", want.GitHub.WatchPaths, got.GitHub.WatchPaths)
		requireLiveEqual(t, "compose.source.github.triggerType", want.GitHub.TriggerType, got.GitHub.TriggerType)
		requireLiveEqual(t, "compose.source.github.enableSubmodules", want.GitHub.EnableSubmodules, got.GitHub.EnableSubmodules)
	}
	if want.GitLab != nil {
		requireLiveEqual(t, "compose.source.gitlab", true, got.GitLab != nil)
		if got.GitLab == nil {
			return
		}
		requireLiveEqual(t, "compose.source.gitlab.integrationId", want.GitLab.IntegrationID, got.GitLab.IntegrationID)
		requireLiveEqual(t, "compose.source.gitlab.projectId", want.GitLab.ProjectID, got.GitLab.ProjectID)
		requireLiveEqual(t, "compose.source.gitlab.owner", want.GitLab.Owner, got.GitLab.Owner)
		requireLiveEqual(t, "compose.source.gitlab.namespace", want.GitLab.Namespace, got.GitLab.Namespace)
		requireLiveEqual(t, "compose.source.gitlab.repository", want.GitLab.Repository, got.GitLab.Repository)
		requireLiveEqual(t, "compose.source.gitlab.branch", want.GitLab.Branch, got.GitLab.Branch)
		requireLiveEqual(t, "compose.source.gitlab.composePath", want.GitLab.ComposePath, got.GitLab.ComposePath)
		requireLiveEqual(t, "compose.source.gitlab.watchPaths", want.GitLab.WatchPaths, got.GitLab.WatchPaths)
		requireLiveEqual(t, "compose.source.gitlab.enableSubmodules", want.GitLab.EnableSubmodules, got.GitLab.EnableSubmodules)
	}
}

func TestComposeSourceValidateRequiresExactlyOneConfiguredVariant(t *testing.T) {
	validGit := &GitComposeSource{URL: "https://example.test/repo", Branch: "main"}
	tests := []struct {
		name   string
		source ComposeSource
		want   string
	}{
		{"missing variant", ComposeSource{Type: ComposeSourceRaw}, "source.raw is required when source.type is raw"},
		{"raw extra git", ComposeSource{Type: ComposeSourceRaw, Raw: &RawComposeSource{ComposeFile: "services: {}"}, Git: validGit}, "source.git must be omitted when source.type is raw"},
		{"git missing", ComposeSource{Type: ComposeSourceGit}, "source.git is required when source.type is git"},
		{"git empty url", ComposeSource{Type: ComposeSourceGit, Git: &GitComposeSource{Branch: "main"}}, "source.git.url must not be empty"},
		{"gitlab missing", ComposeSource{Type: ComposeSourceGitLab}, "source.gitlab is required when source.type is gitlab"},
		{"unknown", ComposeSource{Type: "other"}, "source.type must be one of raw, git, gitlab, or github"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.source.validate(); err == nil || err.Error() != tc.want {
				t.Fatalf("validate() = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestComposeSourceDefaults(t *testing.T) {
	source := property.New(map[string]property.Value{"type": property.New("raw"), "raw": property.New(map[string]property.Value{"composeFile": property.New("services: {}")})})
	got, err := (Compose{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{"name": property.New("demo"), "environmentId": property.New("e1"), "source": source})})
	if err != nil || len(got.Failures) != 0 {
		t.Fatalf("Check() = %#v, %v", got.Failures, err)
	}
	if got.Inputs.ComposeType != ComposeDocker {
		t.Fatalf("defaults = %#v", got.Inputs)
	}
}

func TestComposeGitSourceDefaultsComposePath(t *testing.T) {
	source := property.New(map[string]property.Value{
		"type": property.New("git"),
		"git":  property.New(map[string]property.Value{"url": property.New("https://example.test/repo"), "branch": property.New("main")}),
	})
	got, err := (Compose{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{"name": property.New("demo"), "environmentId": property.New("e1"), "source": source})})
	if err != nil || len(got.Failures) != 0 {
		t.Fatalf("Check() = %#v, %v", got.Failures, err)
	}
	if got.Inputs.Source.Git.ComposePath != "./docker-compose.yml" {
		t.Fatalf("compose path = %q", got.Inputs.Source.Git.ComposePath)
	}
}

func TestComposeGitLabSourceDefaultsComposePath(t *testing.T) {
	source := property.New(map[string]property.Value{
		"type": property.New("gitlab"),
		"gitlab": property.New(map[string]property.Value{
			"integrationId": property.New("i1"), "projectId": property.New(float64(42)),
			"owner": property.New("owner"), "namespace": property.New("namespace"),
			"repository": property.New("repo"), "branch": property.New("main"),
		}),
	})
	got, err := (Compose{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{"name": property.New("demo"), "environmentId": property.New("e1"), "source": source})})
	if err != nil || len(got.Failures) != 0 {
		t.Fatalf("Check() = %#v, %v", got.Failures, err)
	}
	if got.Inputs.Source.GitLab.ComposePath != "./docker-compose.yml" {
		t.Fatalf("compose path = %q", got.Inputs.Source.GitLab.ComposePath)
	}
}

func TestComposeCheckAllowsComputedEnvironmentIdDuringPreview(t *testing.T) {
	source := property.New(map[string]property.Value{"type": property.New("raw"), "raw": property.New(map[string]property.Value{"composeFile": property.New("services: {}")})})
	got, err := (Compose{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{"name": property.New("demo"), "environmentId": property.New(property.Computed), "source": source})})
	if err != nil || len(got.Failures) != 0 {
		t.Fatalf("Check() = %#v, %v", got.Failures, err)
	}
}

func TestComposeInferredSchemaHasVariantSpecificComposePaths(t *testing.T) {
	spec, err := p.GetSchema(t.Context(), Name, Version, Provider())
	if err != nil {
		t.Fatal(err)
	}
	compose := spec.Resources["dokploy:index:Compose"]
	if compose.InputProperties == nil {
		t.Fatal("Compose input properties are nil")
	}
	sourceProperty, ok := compose.InputProperties["source"]
	if !ok || sourceProperty.Ref == "" {
		t.Fatal("Compose source property is missing its type reference")
	}
	sourceRef := strings.TrimPrefix(sourceProperty.Ref, "#/types/")
	source, ok := spec.Types[sourceRef]
	if !ok || source.Properties == nil {
		t.Fatalf("source type %q is missing properties", sourceRef)
	}
	for _, variant := range []string{"raw", "git", "gitlab"} {
		if _, ok := source.Properties[variant]; !ok {
			t.Fatalf("source schema lacks %s variant", variant)
		}
		if source.Properties[variant].Ref == "" {
			t.Fatalf("source schema variant %s lacks type reference", variant)
		}
	}
	raw, ok := spec.Types[strings.TrimPrefix(source.Properties["raw"].Ref, "#/types/")]
	if !ok || raw.Properties == nil {
		t.Fatal("raw source type is missing properties")
	}
	composeFile, ok := raw.Properties["composeFile"]
	if !ok || composeFile.Type != "string" || !contains(raw.Required, "composeFile") {
		t.Fatal("raw schema lacks composeFile")
	}
	if _, ok := raw.Properties["composePath"]; ok {
		t.Fatal("raw schema unexpectedly exposes composePath")
	}
	for _, variant := range []string{"git", "gitlab"} {
		ref := strings.TrimPrefix(source.Properties[variant].Ref, "#/types/")
		typ, ok := spec.Types[ref]
		if !ok || typ.Properties == nil {
			t.Fatalf("%s source type is missing properties", variant)
		}
		path, ok := typ.Properties["composePath"]
		if !ok || path.Type != "string" || contains(typ.Required, "composePath") {
			t.Fatalf("repository schema %q lacks composePath", ref)
		}
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
