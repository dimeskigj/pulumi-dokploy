package dokploy

import (
	"context"
	"fmt"

	"github.com/gjorgjidimeski/pulumi-dokploy/internal/client"
	"github.com/gjorgjidimeski/pulumi-dokploy/internal/client/generated"
	"github.com/oapi-codegen/nullable"
)

type ApplicationSourceType string
type BuildType string

const (
	SourceDocker    ApplicationSourceType = "docker"
	SourceGit       ApplicationSourceType = "git"
	SourceGitLab    ApplicationSourceType = "gitlab"
	BuildNixpacks   BuildType             = "nixpacks"
	BuildDockerfile BuildType             = "dockerfile"
)

type ApplicationSource struct {
	Type   ApplicationSourceType `pulumi:"type"`
	Docker *DockerSource         `pulumi:"docker,optional"`
	Git    *GitApplicationSource `pulumi:"git,optional"`
	GitLab *GitLabAppSource      `pulumi:"gitlab,optional"`
}

type DockerSource struct {
	Image       string  `pulumi:"image"`
	RegistryURL *string `pulumi:"registryUrl,optional"`
	Username    *string `pulumi:"username,optional"`
	Password    *string `pulumi:"password,optional" provider:"secret"`
}

type GitApplicationSource struct {
	URL              string           `pulumi:"url"`
	Branch           string           `pulumi:"branch"`
	BuildPath        *string          `pulumi:"buildPath,optional"`
	WatchPaths       []string         `pulumi:"watchPaths,optional"`
	EnableSubmodules bool             `pulumi:"enableSubmodules,optional"`
	Build            ApplicationBuild `pulumi:"build"`
}

type GitLabAppSource struct {
	IntegrationID    string           `pulumi:"integrationId"`
	ProjectID        int              `pulumi:"projectId"`
	Owner            string           `pulumi:"owner"`
	Namespace        string           `pulumi:"namespace"`
	Repository       string           `pulumi:"repository"`
	Branch           string           `pulumi:"branch"`
	BuildPath        *string          `pulumi:"buildPath,optional"`
	WatchPaths       []string         `pulumi:"watchPaths,optional"`
	EnableSubmodules bool             `pulumi:"enableSubmodules,optional"`
	Build            ApplicationBuild `pulumi:"build"`
}

type ApplicationBuild struct {
	Type              BuildType `pulumi:"type"`
	Dockerfile        *string   `pulumi:"dockerfile,optional"`
	DockerContextPath *string   `pulumi:"dockerContextPath,optional"`
	DockerBuildStage  *string   `pulumi:"dockerBuildStage,optional"`
}

func (s ApplicationSource) validate() error {
	count := 0
	if s.Docker != nil {
		count++
	}
	if s.Git != nil {
		count++
	}
	if s.GitLab != nil {
		count++
	}
	switch s.Type {
	case SourceDocker:
		if s.Docker == nil {
			return fmt.Errorf("source.docker is required when source.type is docker")
		}
		if s.Git != nil {
			return fmt.Errorf("source.git must be omitted when source.type is docker")
		}
		if s.GitLab != nil {
			return fmt.Errorf("source.gitlab must be omitted when source.type is docker")
		}
		if s.Docker.Image == "" {
			return fmt.Errorf("source.docker.image must not be empty")
		}
	case SourceGit:
		if s.Git == nil {
			return fmt.Errorf("source.git is required when source.type is git")
		}
		if s.Docker != nil {
			return fmt.Errorf("source.docker must be omitted when source.type is git")
		}
		if s.GitLab != nil {
			return fmt.Errorf("source.gitlab must be omitted when source.type is git")
		}
		if err := validateGit("source.git", s.Git.URL, s.Git.Branch, s.Git.Build); err != nil {
			return err
		}
	case SourceGitLab:
		if s.GitLab == nil {
			return fmt.Errorf("source.gitlab is required when source.type is gitlab")
		}
		if s.Docker != nil {
			return fmt.Errorf("source.docker must be omitted when source.type is gitlab")
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
		for name, value := range map[string]string{"owner": s.GitLab.Owner, "namespace": s.GitLab.Namespace, "repository": s.GitLab.Repository, "branch": s.GitLab.Branch} {
			if value == "" {
				return fmt.Errorf("source.gitlab.%s must not be empty", name)
			}
		}
		if err := validateBuild("source.gitlab", s.GitLab.Build); err != nil {
			return err
		}
	default:
		if count == 0 {
			return fmt.Errorf("source variant is required")
		}
		if count > 1 {
			return fmt.Errorf("exactly one source variant must be set")
		}
		return fmt.Errorf("source.type must be one of docker, git, or gitlab")
	}
	return nil
}

func validateGit(prefix, url, branch string, build ApplicationBuild) error {
	if url == "" {
		return fmt.Errorf("%s.url must not be empty", prefix)
	}
	if branch == "" {
		return fmt.Errorf("%s.branch must not be empty", prefix)
	}
	return validateBuild(prefix, build)
}

func validateBuild(prefix string, build ApplicationBuild) error {
	switch build.Type {
	case BuildNixpacks:
		if build.Dockerfile != nil {
			return fmt.Errorf("%s.build.dockerfile must be omitted for nixpacks builds", prefix)
		}
		if build.DockerContextPath != nil {
			return fmt.Errorf("%s.build.dockerContextPath must be omitted for nixpacks builds", prefix)
		}
		if build.DockerBuildStage != nil {
			return fmt.Errorf("%s.build.dockerBuildStage must be omitted for nixpacks builds", prefix)
		}
	case BuildDockerfile:
		if build.Dockerfile == nil || *build.Dockerfile == "" {
			return fmt.Errorf("%s.build.dockerfile is required for dockerfile builds", prefix)
		}
	default:
		return fmt.Errorf("%s.build.type must be one of nixpacks or dockerfile", prefix)
	}
	return nil
}

func configureApplicationSource(ctx context.Context, api *client.Client, id string, source ApplicationSource) error {
	var err error
	switch source.Type {
	case SourceDocker:
		s := source.Docker
		body := generated.ApplicationSaveDockerProviderJSONRequestBody{ApplicationId: id, DockerImage: nullable.NewNullableWithValue(s.Image)}
		if s.RegistryURL != nil {
			body.RegistryUrl = nullable.NewNullableWithValue(*s.RegistryURL)
		}
		if s.Username != nil {
			body.Username = nullable.NewNullableWithValue(*s.Username)
		}
		if s.Password != nil {
			body.Password = nullable.NewNullableWithValue(*s.Password)
		}
		_, err = api.ApplicationSaveDockerProviderWithResponse(ctx, body)
	case SourceGit:
		s := source.Git
		body := generated.ApplicationSaveGitProviderJSONRequestBody{ApplicationId: id, CustomGitBranch: s.Branch, EnableSubmodules: &s.EnableSubmodules, CustomGitUrl: nullable.NewNullableWithValue(s.URL), WatchPaths: nullable.NewNullableWithValue(s.WatchPaths)}
		if s.BuildPath != nil {
			body.CustomGitBuildPath = nullable.NewNullableWithValue(*s.BuildPath)
		}
		_, err = api.ApplicationSaveGitProviderWithResponse(ctx, body)
	case SourceGitLab:
		s := source.GitLab
		body := generated.ApplicationSaveGitlabProviderJSONRequestBody{ApplicationId: id, GitlabBranch: s.Branch, EnableSubmodules: &s.EnableSubmodules, GitlabId: nullable.NewNullableWithValue(s.IntegrationID), GitlabProjectId: nullable.NewNullableWithValue(float32(s.ProjectID)), GitlabOwner: nullable.NewNullableWithValue(s.Owner), GitlabPathNamespace: nullable.NewNullableWithValue(s.Namespace), GitlabRepository: nullable.NewNullableWithValue(s.Repository), WatchPaths: nullable.NewNullableWithValue(s.WatchPaths)}
		if s.BuildPath != nil {
			body.GitlabBuildPath = nullable.NewNullableWithValue(*s.BuildPath)
		}
		_, err = api.ApplicationSaveGitlabProviderWithResponse(ctx, body)
	}
	return err
}

func configureApplicationBuild(ctx context.Context, api *client.Client, id string, source ApplicationSource) error {
	if source.Type == SourceDocker {
		return nil
	}
	var b ApplicationBuild
	if source.Type == SourceGit {
		b = source.Git.Build
	} else {
		b = source.GitLab.Build
	}
	body := generated.ApplicationSaveBuildTypeJSONRequestBody{ApplicationId: id, BuildType: generated.ApplicationSaveBuildTypeJSONBodyBuildType(b.Type), Dockerfile: nullable.NewNullNullable[string](), DockerContextPath: nullable.NewNullNullable[string](), DockerBuildStage: nullable.NewNullNullable[string](), HerokuVersion: nullable.NewNullNullable[string](), RailpackVersion: nullable.NewNullNullable[string]()}
	if b.Dockerfile != nil {
		body.Dockerfile = nullable.NewNullableWithValue(*b.Dockerfile)
	}
	if b.DockerContextPath != nil {
		body.DockerContextPath = nullable.NewNullableWithValue(*b.DockerContextPath)
	}
	if b.DockerBuildStage != nil {
		body.DockerBuildStage = nullable.NewNullableWithValue(*b.DockerBuildStage)
	}
	_, err := api.ApplicationSaveBuildTypeWithResponse(ctx, body)
	return err
}
