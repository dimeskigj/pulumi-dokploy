package dokploy

import (
	"context"
	"fmt"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/oapi-codegen/nullable"
	"github.com/pulumi/pulumi-go-provider/infer"
)

type GitHubTriggerType string

const (
	GitHubTriggerPush GitHubTriggerType = "push"
	GitHubTriggerTag  GitHubTriggerType = "tag"
)

type GitHubAppSource struct {
	IntegrationID    string             `pulumi:"integrationId"`
	Owner            string             `pulumi:"owner"`
	Repository       string             `pulumi:"repository"`
	Branch           string             `pulumi:"branch"`
	BuildPath        *string            `pulumi:"buildPath,optional"`
	TriggerType      *GitHubTriggerType `pulumi:"triggerType,optional"`
	WatchPaths       []string           `pulumi:"watchPaths,optional"`
	EnableSubmodules bool               `pulumi:"enableSubmodules,optional"`
	Build            ApplicationBuild   `pulumi:"build"`
}

func (s *GitHubAppSource) Annotate(a infer.Annotator) {
	a.Describe(&s, "Beta. GitHub source configuration.")
	a.Describe(&s.IntegrationID, "The GitHub integration ID.")
	a.Describe(&s.Owner, "The GitHub owner.")
	a.Describe(&s.Repository, "The GitHub repository.")
	a.Describe(&s.Branch, "The GitHub branch.")
	a.Describe(&s.BuildPath, "The build path.")
	a.Describe(&s.TriggerType, "The GitHub trigger type; defaults to push.")
	a.Describe(&s.WatchPaths, "Paths to watch.")
	a.Describe(&s.EnableSubmodules, "Whether to enable submodules.")
	a.Describe(&s.Build, "The build configuration.")
}

type GiteaAppSource struct {
	IntegrationID    string           `pulumi:"integrationId"`
	Owner            string           `pulumi:"owner"`
	Repository       string           `pulumi:"repository"`
	Branch           string           `pulumi:"branch"`
	BuildPath        *string          `pulumi:"buildPath,optional"`
	WatchPaths       []string         `pulumi:"watchPaths,optional"`
	EnableSubmodules bool             `pulumi:"enableSubmodules,optional"`
	Build            ApplicationBuild `pulumi:"build"`
}

func (s *GiteaAppSource) Annotate(a infer.Annotator) {
	a.Describe(&s, "Beta. Gitea source configuration.")
	a.Describe(&s.IntegrationID, "The Gitea integration ID.")
	a.Describe(&s.Owner, "The Gitea owner.")
	a.Describe(&s.Repository, "The Gitea repository.")
	a.Describe(&s.Branch, "The Gitea branch.")
	a.Describe(&s.BuildPath, "The build path.")
	a.Describe(&s.WatchPaths, "Paths to watch.")
	a.Describe(&s.EnableSubmodules, "Whether to enable submodules.")
	a.Describe(&s.Build, "The build configuration.")
}

type BitbucketAppSource struct {
	IntegrationID    string           `pulumi:"integrationId"`
	Owner            string           `pulumi:"owner"`
	Repository       string           `pulumi:"repository"`
	RepositorySlug   string           `pulumi:"repositorySlug"`
	Branch           string           `pulumi:"branch"`
	BuildPath        *string          `pulumi:"buildPath,optional"`
	WatchPaths       []string         `pulumi:"watchPaths,optional"`
	EnableSubmodules bool             `pulumi:"enableSubmodules,optional"`
	Build            ApplicationBuild `pulumi:"build"`
}

func (s *BitbucketAppSource) Annotate(a infer.Annotator) {
	a.Describe(&s, "Beta. Bitbucket source configuration.")
	a.Describe(&s.IntegrationID, "The Bitbucket integration ID.")
	a.Describe(&s.Owner, "The Bitbucket owner.")
	a.Describe(&s.Repository, "The Bitbucket repository.")
	a.Describe(&s.RepositorySlug, "The Bitbucket repository slug.")
	a.Describe(&s.Branch, "The Bitbucket branch.")
	a.Describe(&s.BuildPath, "The build path.")
	a.Describe(&s.WatchPaths, "Paths to watch.")
	a.Describe(&s.EnableSubmodules, "Whether to enable submodules.")
	a.Describe(&s.Build, "The build configuration.")
}

func validateIntegratedSource(prefix, integration, owner, repository, branch string, build ApplicationBuild) error {
	for _, field := range []struct{ name, value string }{{"integrationId", integration}, {"owner", owner}, {"repository", repository}, {"branch", branch}} {
		if field.value == "" {
			return fmt.Errorf("%s.%s must not be empty", prefix, field.name)
		}
	}
	return validateBuild(prefix, build)
}

func configureGitHubApplicationSource(ctx context.Context, api *client.Client, id string, source *GitHubAppSource) error {
	path := ""
	if source.BuildPath != nil {
		path = *source.BuildPath
	}
	trigger := GitHubTriggerPush
	if source.TriggerType != nil {
		trigger = *source.TriggerType
	}
	body := generated.ApplicationSaveGithubProviderJSONRequestBody{
		ApplicationId: id, Branch: source.Branch, BuildPath: nullable.NewNullableWithValue(path),
		EnableSubmodules: &source.EnableSubmodules, GithubId: nullable.NewNullableWithValue(source.IntegrationID),
		Owner: nullable.NewNullableWithValue(source.Owner), Repository: nullable.NewNullableWithValue(source.Repository),
		TriggerType: generated.ApplicationSaveGithubProviderJSONBodyTriggerType(trigger),
		WatchPaths:  nullable.NewNullableWithValue(source.WatchPaths),
	}
	_, err := api.ApplicationSaveGithubProviderWithResponse(ctx, body)
	return err
}

func configureGiteaApplicationSource(ctx context.Context, api *client.Client, id string, source *GiteaAppSource) error {
	path := ""
	if source.BuildPath != nil {
		path = *source.BuildPath
	}
	body := generated.ApplicationSaveGiteaProviderJSONRequestBody{
		ApplicationId: id, GiteaBranch: source.Branch, GiteaBuildPath: nullable.NewNullableWithValue(path),
		GiteaId: nullable.NewNullableWithValue(source.IntegrationID), GiteaOwner: nullable.NewNullableWithValue(source.Owner),
		GiteaRepository: nullable.NewNullableWithValue(source.Repository), EnableSubmodules: &source.EnableSubmodules,
		WatchPaths: nullable.NewNullableWithValue(source.WatchPaths),
	}
	_, err := api.ApplicationSaveGiteaProviderWithResponse(ctx, body)
	return err
}

func configureBitbucketApplicationSource(ctx context.Context, api *client.Client, id string, source *BitbucketAppSource) error {
	path := ""
	if source.BuildPath != nil {
		path = *source.BuildPath
	}
	body := generated.ApplicationSaveBitbucketProviderJSONRequestBody{
		ApplicationId: id, BitbucketBranch: source.Branch, BitbucketBuildPath: nullable.NewNullableWithValue(path),
		BitbucketId: nullable.NewNullableWithValue(source.IntegrationID), BitbucketOwner: nullable.NewNullableWithValue(source.Owner),
		BitbucketRepository: nullable.NewNullableWithValue(source.Repository), BitbucketRepositorySlug: nullable.NewNullableWithValue(source.RepositorySlug),
		EnableSubmodules: &source.EnableSubmodules, WatchPaths: nullable.NewNullableWithValue(source.WatchPaths),
	}
	_, err := api.ApplicationSaveBitbucketProviderWithResponse(ctx, body)
	return err
}
