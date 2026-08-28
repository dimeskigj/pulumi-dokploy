package dokploy

import (
	"context"
	"fmt"

	"github.com/gjorgjidimeski/pulumi-dokploy/internal/client"
	"github.com/gjorgjidimeski/pulumi-dokploy/internal/client/generated"
	"github.com/oapi-codegen/nullable"
)

type ComposeSourceType string
type ComposeType string

const (
	ComposeSourceRaw    ComposeSourceType = "raw"
	ComposeSourceGit    ComposeSourceType = "git"
	ComposeSourceGitLab ComposeSourceType = "gitlab"
	ComposeDocker       ComposeType       = "docker-compose"
	ComposeStack        ComposeType       = "stack"
)

type ComposeSource struct {
	Type   ComposeSourceType    `pulumi:"type"`
	Raw    *RawComposeSource    `pulumi:"raw,optional"`
	Git    *GitComposeSource    `pulumi:"git,optional"`
	GitLab *GitLabComposeSource `pulumi:"gitlab,optional"`
}

type RawComposeSource struct {
	ComposeFile string `pulumi:"composeFile"`
	ComposePath string `pulumi:"composePath,optional"`
}

type GitComposeSource struct {
	URL              string   `pulumi:"url"`
	Branch           string   `pulumi:"branch"`
	ComposePath      string   `pulumi:"composePath,optional"`
	SSHKeyID         *string  `pulumi:"sshKeyId,optional"`
	WatchPaths       []string `pulumi:"watchPaths,optional"`
	EnableSubmodules bool     `pulumi:"enableSubmodules,optional"`
}

type GitLabComposeSource struct {
	IntegrationID    string   `pulumi:"integrationId"`
	ProjectID        int      `pulumi:"projectId"`
	Owner            string   `pulumi:"owner"`
	Namespace        string   `pulumi:"namespace"`
	Repository       string   `pulumi:"repository"`
	Branch           string   `pulumi:"branch"`
	ComposePath      string   `pulumi:"composePath,optional"`
	WatchPaths       []string `pulumi:"watchPaths,optional"`
	EnableSubmodules bool     `pulumi:"enableSubmodules,optional"`
}

func (s ComposeSource) validate() error {
	switch s.Type {
	case ComposeSourceRaw:
		if s.Raw == nil {
			return fmt.Errorf("source.raw is required when source.type is raw")
		}
		if s.Git != nil {
			return fmt.Errorf("source.git must be omitted when source.type is raw")
		}
		if s.GitLab != nil {
			return fmt.Errorf("source.gitlab must be omitted when source.type is raw")
		}
		if s.Raw.ComposeFile == "" {
			return fmt.Errorf("source.raw.composeFile must not be empty")
		}
	case ComposeSourceGit:
		if s.Git == nil {
			return fmt.Errorf("source.git is required when source.type is git")
		}
		if s.Raw != nil {
			return fmt.Errorf("source.raw must be omitted when source.type is git")
		}
		if s.GitLab != nil {
			return fmt.Errorf("source.gitlab must be omitted when source.type is git")
		}
		if s.Git.URL == "" {
			return fmt.Errorf("source.git.url must not be empty")
		}
		if s.Git.Branch == "" {
			return fmt.Errorf("source.git.branch must not be empty")
		}
	case ComposeSourceGitLab:
		if s.GitLab == nil {
			return fmt.Errorf("source.gitlab is required when source.type is gitlab")
		}
		if s.Raw != nil {
			return fmt.Errorf("source.raw must be omitted when source.type is gitlab")
		}
		if s.Git != nil {
			return fmt.Errorf("source.git must be omitted when source.type is gitlab")
		}
		if s.GitLab.IntegrationID == "" {
			return fmt.Errorf("source.gitlab.integrationId must not be empty")
		}
		if s.GitLab.ProjectID == 0 {
			return fmt.Errorf("source.gitlab.projectId must not be zero")
		}
		for n, v := range map[string]string{"owner": s.GitLab.Owner, "namespace": s.GitLab.Namespace, "repository": s.GitLab.Repository, "branch": s.GitLab.Branch} {
			if v == "" {
				return fmt.Errorf("source.gitlab.%s must not be empty", n)
			}
		}
	default:
		return fmt.Errorf("source.type must be one of raw, git, or gitlab")
	}
	return nil
}

func composePath(path string) string {
	if path == "" {
		return "./docker-compose.yml"
	}
	return path
}

func configureComposeSource(ctx context.Context, api *client.Client, id string, source ComposeSource) error {
	if source.Type == ComposeSourceRaw {
		_, err := api.ComposeUpdateWithResponse(ctx, generated.ComposeUpdateJSONRequestBody{ComposeId: id, ComposeFile: ptr(source.Raw.ComposeFile), ComposePath: ptr(composeSourcePath(source))})
		return err
	}
	b := generated.ComposeUpdateJSONRequestBody{ComposeId: id, SourceType: ptr(generated.ComposeUpdateJSONBodySourceType(source.Type)), ComposePath: ptr(composeSourcePath(source))}
	switch source.Type {
	case ComposeSourceGit:
		s := source.Git
		b.CustomGitUrl = nullable.NewNullableWithValue(s.URL)
		b.CustomGitBranch = nullable.NewNullableWithValue(s.Branch)
		b.WatchPaths = nullable.NewNullableWithValue(s.WatchPaths)
		b.EnableSubmodules = &s.EnableSubmodules
		if s.SSHKeyID != nil {
			b.CustomGitSSHKeyId = nullable.NewNullableWithValue(*s.SSHKeyID)
		}
	case ComposeSourceGitLab:
		s := source.GitLab
		b.GitlabId = nullable.NewNullableWithValue(s.IntegrationID)
		b.GitlabProjectId = nullable.NewNullableWithValue(float32(s.ProjectID))
		b.GitlabOwner = nullable.NewNullableWithValue(s.Owner)
		b.GitlabPathNamespace = nullable.NewNullableWithValue(s.Namespace)
		b.GitlabRepository = nullable.NewNullableWithValue(s.Repository)
		b.GitlabBranch = nullable.NewNullableWithValue(s.Branch)
		b.WatchPaths = nullable.NewNullableWithValue(s.WatchPaths)
		b.EnableSubmodules = &s.EnableSubmodules
	}
	_, err := api.ComposeUpdateWithResponse(ctx, b)
	return err
}

func fetchComposeSource(ctx context.Context, api *client.Client, id string, source ComposeSourceType) error {
	if source == ComposeSourceRaw {
		return nil
	}
	_, err := api.ComposeFetchSourceTypeWithResponse(ctx, generated.ComposeFetchSourceTypeJSONRequestBody{ComposeId: id})
	return err
}

func composeSourcePath(s ComposeSource) string {
	if s.Type == ComposeSourceGit {
		return composePath(s.Git.ComposePath)
	}
	if s.Type == ComposeSourceGitLab {
		return composePath(s.GitLab.ComposePath)
	}
	return composePath(s.Raw.ComposePath)
}

func ptr[T any](v T) *T { return &v }
