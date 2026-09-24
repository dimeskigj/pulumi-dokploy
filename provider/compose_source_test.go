package dokploy

import (
	"net/http"
	"strings"
	"testing"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

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
		{"unknown", ComposeSource{Type: "other"}, "source.type must be one of raw, git, or gitlab"},
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
