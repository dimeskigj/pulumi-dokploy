package dokploy

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
