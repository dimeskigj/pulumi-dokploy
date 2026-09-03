package dokploy

import (
	"context"
	"testing"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/stretchr/testify/require"
)

func TestApplicationGitSourceSavesAndClearsSSHKey(t *testing.T) {
	key := "key-1"
	for _, tc := range []struct {
		name   string
		source GitApplicationSource
		body   string
	}{
		{"set", GitApplicationSource{URL: "https://git.test/repo", Branch: "main", SSHKeyID: &key, Build: ApplicationBuild{Type: BuildNixpacks}}, `{"applicationId":"a1","customGitBranch":"main","customGitBuildPath":"","customGitSSHKeyId":"key-1","customGitUrl":"https://git.test/repo","enableSubmodules":false,"watchPaths":null}`},
		{"cleared", GitApplicationSource{URL: "https://git.test/repo", Branch: "main", Build: ApplicationBuild{Type: BuildNixpacks}}, `{"applicationId":"a1","customGitBranch":"main","customGitBuildPath":"","customGitSSHKeyId":null,"customGitUrl":"https://git.test/repo","enableSubmodules":false,"watchPaths":null}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newScriptedServer(t, expectPOST("/api/application.saveGitProvider", tc.body, `true`))
			err := configureApplicationSource(context.Background(), fixedClient(s.API())(context.Background()), "a1", ApplicationSource{Type: SourceGit, Git: &tc.source})
			require.NoError(t, err)
		})
	}
}

func TestApplicationGitSourceSchemaIncludesSSHKeyID(t *testing.T) {
	spec, err := p.GetSchema(t.Context(), Name, Version, Provider())
	require.NoError(t, err)
	require.Contains(t, schemaProperty(spec, "dokploy:index:Application", "source.git.sshKeyId").Type, "string")
}

func TestApplicationSourceValidate(t *testing.T) {
	tests := []struct {
		name   string
		source ApplicationSource
		want   string
	}{
		{"docker missing", ApplicationSource{Type: SourceDocker}, "source.docker is required when source.type is docker"},
		{"docker extra", ApplicationSource{Type: SourceDocker, Docker: &DockerSource{Image: "x"}, Git: &GitApplicationSource{URL: "x", Branch: "main"}}, "source.git must be omitted when source.type is docker"},
		{"git mismatch", ApplicationSource{Type: SourceGit, Docker: &DockerSource{Image: "x"}}, "source.git is required when source.type is git"},
		{"git empty url", ApplicationSource{Type: SourceGit, Git: &GitApplicationSource{Branch: "main"}}, "source.git.url must not be empty"},
		{"git empty branch", ApplicationSource{Type: SourceGit, Git: &GitApplicationSource{URL: "https://example.test/repo"}}, "source.git.branch must not be empty"},
		{"git nixpacks docker fields", ApplicationSource{Type: SourceGit, Git: &GitApplicationSource{URL: "x", Branch: "main", Build: ApplicationBuild{Type: BuildNixpacks, Dockerfile: appStringPtr("Dockerfile")}}}, "source.git.build.dockerfile must be omitted for nixpacks builds"},
		{"git dockerfile missing", ApplicationSource{Type: SourceGit, Git: &GitApplicationSource{URL: "x", Branch: "main", Build: ApplicationBuild{Type: BuildDockerfile}}}, "source.git.build.dockerfile is required for dockerfile builds"},
		{"valid docker", ApplicationSource{Type: SourceDocker, Docker: &DockerSource{Image: "nginx"}}, ""},
		{"valid git", ApplicationSource{Type: SourceGit, Git: &GitApplicationSource{URL: "x", Branch: "main", Build: ApplicationBuild{Type: BuildNixpacks}}}, ""},
		{"valid gitlab", ApplicationSource{Type: SourceGitLab, GitLab: &GitLabAppSource{IntegrationID: "i", ProjectID: 1, Owner: "o", Namespace: "n", Repository: "r", Branch: "main", Build: ApplicationBuild{Type: BuildDockerfile, Dockerfile: appStringPtr("Dockerfile")}}}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.source.validate()
			if tt.want == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tt.want)
			}
		})
	}
}

func appStringPtr(s string) *string { return &s }
