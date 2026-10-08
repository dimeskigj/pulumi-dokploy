package dokploy

import (
	"context"
	"fmt"
	"reflect"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/oapi-codegen/nullable"
	"github.com/pulumi/pulumi-go-provider/infer"
)

type ComposeSourceType string
type ComposeType string
type ComposeTriggerType string

const (
	ComposeSourceRaw    ComposeSourceType  = "raw"
	ComposeSourceGit    ComposeSourceType  = "git"
	ComposeSourceGitLab ComposeSourceType  = "gitlab"
	ComposeSourceGitHub ComposeSourceType  = "github"
	ComposeDocker       ComposeType        = "docker-compose"
	ComposeStack        ComposeType        = "stack"
	ComposeTriggerPush  ComposeTriggerType = "push"
	ComposeTriggerTag   ComposeTriggerType = "tag"
)

type ComposeSource struct {
	Type   ComposeSourceType    `pulumi:"type"`
	Raw    *RawComposeSource    `pulumi:"raw,optional"`
	Git    *GitComposeSource    `pulumi:"git,optional"`
	GitLab *GitLabComposeSource `pulumi:"gitlab,optional"`
	GitHub *GitHubComposeSource `pulumi:"github,optional"`
}

func (s *ComposeSource) Annotate(a infer.Annotator) {
	a.Describe(&s, "Compose source configuration.")
	a.Describe(&s.Type, "The Compose source type.")
	a.Describe(&s.Raw, "Raw Compose source.")
	a.Describe(&s.Git, "Git Compose source.")
	a.Describe(&s.GitLab, "GitLab Compose source.")
	a.Describe(&s.GitHub, "GitHub Compose source.")
}

type RawComposeSource struct {
	ComposeFile string `pulumi:"composeFile"`
}

func (s *RawComposeSource) Annotate(a infer.Annotator) {
	a.Describe(&s, "Raw Compose source configuration.")
	a.Describe(&s.ComposeFile, "The raw Compose file.")
}

type GitComposeSource struct {
	URL              string   `pulumi:"url"`
	Branch           string   `pulumi:"branch"`
	ComposePath      string   `pulumi:"composePath,optional"`
	SSHKeyID         *string  `pulumi:"sshKeyId,optional"`
	WatchPaths       []string `pulumi:"watchPaths,optional"`
	EnableSubmodules bool     `pulumi:"enableSubmodules,optional"`
}

func (s *GitComposeSource) Annotate(a infer.Annotator) {
	a.Describe(&s, "Git Compose source configuration.")
	a.Describe(&s.URL, "The Git repository URL.")
	a.Describe(&s.Branch, "The Git branch.")
	a.Describe(&s.ComposePath, "The Compose file path.")
	a.Describe(&s.SSHKeyID, "The SSH key ID.")
	a.Describe(&s.WatchPaths, "Paths to watch.")
	a.Describe(&s.EnableSubmodules, "Whether to enable submodules.")
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

func (s *GitLabComposeSource) Annotate(a infer.Annotator) {
	a.Describe(&s, "GitLab Compose source configuration.")
	a.Describe(&s.IntegrationID, "The GitLab integration ID.")
	a.Describe(&s.ProjectID, "The GitLab project ID.")
	a.Describe(&s.Owner, "The GitLab owner.")
	a.Describe(&s.Namespace, "The GitLab namespace.")
	a.Describe(&s.Repository, "The GitLab repository.")
	a.Describe(&s.Branch, "The GitLab branch.")
	a.Describe(&s.ComposePath, "The Compose file path.")
	a.Describe(&s.WatchPaths, "Paths to watch.")
	a.Describe(&s.EnableSubmodules, "Whether to enable submodules.")
}

type GitHubComposeSource struct {
	IntegrationID    string             `pulumi:"integrationId"`
	Owner            string             `pulumi:"owner"`
	Repository       string             `pulumi:"repository"`
	Branch           string             `pulumi:"branch"`
	ComposePath      string             `pulumi:"composePath,optional"`
	WatchPaths       []string           `pulumi:"watchPaths,optional"`
	TriggerType      ComposeTriggerType `pulumi:"triggerType,optional"`
	EnableSubmodules bool               `pulumi:"enableSubmodules,optional"`
}

func (s *GitHubComposeSource) Annotate(a infer.Annotator) {
	a.Describe(&s, "GitHub Compose source configuration.")
	a.Describe(&s.IntegrationID, "The GitHub integration ID.")
	a.Describe(&s.Owner, "The GitHub owner.")
	a.Describe(&s.Repository, "The GitHub repository.")
	a.Describe(&s.Branch, "The GitHub branch.")
	a.Describe(&s.ComposePath, "The Compose file path.")
	a.Describe(&s.WatchPaths, "Paths to watch.")
	a.Describe(&s.TriggerType, "The deployment trigger type, either push or tag.")
	a.Describe(&s.EnableSubmodules, "Whether to enable submodules.")
	a.SetDefault(&s.TriggerType, string(ComposeTriggerPush))
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
		if s.GitHub != nil {
			return fmt.Errorf("source.github must be omitted when source.type is raw")
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
		if s.GitHub != nil {
			return fmt.Errorf("source.github must be omitted when source.type is git")
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
		if s.GitHub != nil {
			return fmt.Errorf("source.github must be omitted when source.type is gitlab")
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
	case ComposeSourceGitHub:
		if s.GitHub == nil {
			return fmt.Errorf("source.github is required when source.type is github")
		}
		if s.Raw != nil {
			return fmt.Errorf("source.raw must be omitted when source.type is github")
		}
		if s.Git != nil {
			return fmt.Errorf("source.git must be omitted when source.type is github")
		}
		if s.GitLab != nil {
			return fmt.Errorf("source.gitlab must be omitted when source.type is github")
		}
		if s.GitHub.IntegrationID == "" {
			return fmt.Errorf("source.github.integrationId must not be empty")
		}
		for _, field := range []struct{ name, value string }{{"owner", s.GitHub.Owner}, {"repository", s.GitHub.Repository}, {"branch", s.GitHub.Branch}} {
			if field.value == "" {
				return fmt.Errorf("source.github.%s must not be empty", field.name)
			}
		}
		switch s.GitHub.TriggerType {
		case ComposeTriggerPush, ComposeTriggerTag:
		default:
			return fmt.Errorf("source.github.triggerType must be one of push or tag")
		}
	default:
		return fmt.Errorf("source.type must be one of raw, git, gitlab, or github")
	}
	return nil
}

func composePath(path string) string {
	if path == "" {
		return defaultComposePath
	}
	return path
}

func configureComposeSource(ctx context.Context, api *client.Client, id string, source ComposeSource) error {
	if source.Type == ComposeSourceRaw {
		_, err := api.ComposeUpdateWithResponse(ctx, generated.ComposeUpdateJSONRequestBody{ComposeId: id, SourceType: ptr(generated.ComposeUpdateJSONBodySourceType(ComposeSourceRaw)), ComposeFile: ptr(source.Raw.ComposeFile)})
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
		} else {
			b.CustomGitSSHKeyId = nullable.NewNullNullable[string]()
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
	case ComposeSourceGitHub:
		s := source.GitHub
		b.GithubId = nullable.NewNullableWithValue(s.IntegrationID)
		b.Owner = nullable.NewNullableWithValue(s.Owner)
		b.Repository = nullable.NewNullableWithValue(s.Repository)
		b.Branch = nullable.NewNullableWithValue(s.Branch)
		b.TriggerType = nullable.NewNullableWithValue(generated.ComposeUpdateJSONBodyTriggerType(s.TriggerType))
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
	if s.Type == ComposeSourceGitHub {
		return composePath(s.GitHub.ComposePath)
	}
	return ""
}

func ptr[T any](v T) *T { return &v }

// sameComposeSource compares a compose source the way a user's program and a
// Read-derived state must be compared. A program that omits watchPaths yields
// nil while compose.one reports the same stack with an empty list, and
// reflect.DeepEqual would call those different — a diff, and in Update a
// redeploy, for a stack nobody changed.
func sameComposeSource(a, b ComposeSource) bool {
	return reflect.DeepEqual(normalizeComposeSource(a), normalizeComposeSource(b))
}

func normalizeComposeSource(source ComposeSource) ComposeSource {
	switch source.Type {
	case ComposeSourceGit:
		if source.Git != nil {
			git := *source.Git
			git.WatchPaths = normalizeWatchPaths(git.WatchPaths)
			source.Git = &git
		}
	case ComposeSourceGitLab:
		if source.GitLab != nil {
			gitlab := *source.GitLab
			gitlab.WatchPaths = normalizeWatchPaths(gitlab.WatchPaths)
			source.GitLab = &gitlab
		}
	case ComposeSourceGitHub:
		if source.GitHub != nil {
			github := *source.GitHub
			github.WatchPaths = normalizeWatchPaths(github.WatchPaths)
			source.GitHub = &github
		}
	}
	return source
}
