package dokploy

import (
	"context"
	"fmt"
	"strings"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/oapi-codegen/nullable"
	"github.com/pulumi/pulumi-go-provider/infer"
)

type ApplicationSourceType string
type BuildType string

const (
	SourceDocker    ApplicationSourceType = "docker"
	SourceGit       ApplicationSourceType = "git"
	SourceGitLab    ApplicationSourceType = "gitlab"
	BuildNixpacks   BuildType             = "nixpacks"
	BuildDockerfile BuildType             = "dockerfile"
	BuildRailpack   BuildType             = "railpack"
)

type ApplicationSource struct {
	Type   ApplicationSourceType `pulumi:"type"`
	Docker *DockerSource         `pulumi:"docker,optional"`
	Git    *GitApplicationSource `pulumi:"git,optional"`
	GitLab *GitLabAppSource      `pulumi:"gitlab,optional"`
}

func (s *ApplicationSource) Annotate(a infer.Annotator) {
	a.Describe(&s, "Application source configuration.")
	a.Describe(&s.Type, "The application source type.")
	a.Describe(&s.Docker, "Docker source configuration.")
	a.Describe(&s.Git, "Git source configuration.")
	a.Describe(&s.GitLab, "GitLab source configuration.")
}

type DockerSource struct {
	Image       string  `pulumi:"image"`
	RegistryURL *string `pulumi:"registryUrl,optional"`
	Username    *string `pulumi:"username,optional"`
	Password    *string `pulumi:"password,optional" provider:"secret"`
}

func (s *DockerSource) Annotate(a infer.Annotator) {
	a.Describe(&s, "Docker source configuration.")
	a.Describe(&s.Image, "The Docker image.")
	a.Describe(&s.RegistryURL, "The registry URL.")
	a.Describe(&s.Username, "The registry username.")
	a.Describe(&s.Password, "The registry password.")
}

type GitApplicationSource struct {
	URL              string           `pulumi:"url"`
	Branch           string           `pulumi:"branch"`
	BuildPath        *string          `pulumi:"buildPath,optional"`
	SSHKeyID         *string          `pulumi:"sshKeyId,optional"`
	WatchPaths       []string         `pulumi:"watchPaths,optional"`
	EnableSubmodules bool             `pulumi:"enableSubmodules,optional"`
	Build            ApplicationBuild `pulumi:"build"`
}

func (s *GitApplicationSource) Annotate(a infer.Annotator) {
	a.Describe(&s, "Git source configuration.")
	a.Describe(&s.URL, "The Git repository URL.")
	a.Describe(&s.Branch, "The Git branch.")
	a.Describe(&s.BuildPath, "The build path.")
	a.Describe(&s.SSHKeyID, "The SSH key ID.")
	a.Describe(&s.WatchPaths, "Paths to watch.")
	a.Describe(&s.EnableSubmodules, "Whether to enable submodules.")
	a.Describe(&s.Build, "The build configuration.")
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

func (s *GitLabAppSource) Annotate(a infer.Annotator) {
	a.Describe(&s, "GitLab source configuration.")
	a.Describe(&s.IntegrationID, "The GitLab integration ID.")
	a.Describe(&s.ProjectID, "The GitLab project ID.")
	a.Describe(&s.Owner, "The GitLab owner.")
	a.Describe(&s.Namespace, "The GitLab namespace.")
	a.Describe(&s.Repository, "The GitLab repository.")
	a.Describe(&s.Branch, "The GitLab branch.")
	a.Describe(&s.BuildPath, "The build path.")
	a.Describe(&s.WatchPaths, "Paths to watch.")
	a.Describe(&s.EnableSubmodules, "Whether to enable submodules.")
	a.Describe(&s.Build, "The build configuration.")
}

type ApplicationBuild struct {
	Type              BuildType `pulumi:"type"`
	Dockerfile        *string   `pulumi:"dockerfile,optional"`
	DockerContextPath *string   `pulumi:"dockerContextPath,optional"`
	DockerBuildStage  *string   `pulumi:"dockerBuildStage,optional"`
	RailpackVersion   *string   `pulumi:"railpackVersion,optional"`
	IsStaticSpa       bool      `pulumi:"isStaticSpa,optional"`
	PublishDirectory  *string   `pulumi:"publishDirectory,optional"`
}

func (b *ApplicationBuild) Annotate(a infer.Annotator) {
	a.Describe(&b, "Application build configuration.")
	a.Describe(&b.Type, "The build type.")
	a.Describe(&b.Dockerfile, "The Dockerfile path.")
	a.Describe(&b.DockerContextPath, "The Docker build context.")
	a.Describe(&b.DockerBuildStage, "The Docker build stage.")
	a.Describe(&b.RailpackVersion, "The Railpack version to build with.")
	a.Describe(&b.IsStaticSpa, "Whether the Railpack build produces a static single-page application.")
	a.Describe(&b.PublishDirectory, "The directory published by a Railpack static build.")
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
	dockerOnly := map[string]bool{"dockerfile": build.Dockerfile != nil, "dockerContextPath": build.DockerContextPath != nil, "dockerBuildStage": build.DockerBuildStage != nil}
	railpackOnly := map[string]bool{"railpackVersion": build.RailpackVersion != nil, "isStaticSpa": build.IsStaticSpa, "publishDirectory": build.PublishDirectory != nil}
	// Iterated in a fixed order so the reported field is deterministic.
	dockerFields := []string{"dockerfile", "dockerContextPath", "dockerBuildStage"}
	railpackFields := []string{"railpackVersion", "isStaticSpa", "publishDirectory"}
	switch build.Type {
	case BuildNixpacks:
		for _, field := range dockerFields {
			if dockerOnly[field] {
				return fmt.Errorf("%s.build.%s must be omitted for nixpacks builds", prefix, field)
			}
		}
		for _, field := range railpackFields {
			if railpackOnly[field] {
				return fmt.Errorf("%s.build.%s must be omitted for nixpacks builds", prefix, field)
			}
		}
	case BuildDockerfile:
		if build.Dockerfile == nil || *build.Dockerfile == "" {
			return fmt.Errorf("%s.build.dockerfile is required for dockerfile builds", prefix)
		}
		for _, field := range railpackFields {
			if railpackOnly[field] {
				return fmt.Errorf("%s.build.%s must be omitted for dockerfile builds", prefix, field)
			}
		}
	case BuildRailpack:
		for _, field := range dockerFields {
			if dockerOnly[field] {
				return fmt.Errorf("%s.build.%s must be omitted for railpack builds", prefix, field)
			}
		}
	default:
		return fmt.Errorf("%s.build.type must be one of nixpacks, dockerfile, or railpack", prefix)
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
		body := generated.ApplicationSaveGitProviderJSONRequestBody{ApplicationId: id, CustomGitBranch: s.Branch, EnableSubmodules: &s.EnableSubmodules, CustomGitUrl: nullable.NewNullableWithValue(s.URL), WatchPaths: nullable.NewNullableWithValue(s.WatchPaths), CustomGitSSHKeyId: nullable.NewNullNullable[string]()}
		if s.BuildPath != nil {
			body.CustomGitBuildPath = nullable.NewNullableWithValue(*s.BuildPath)
		}
		if s.SSHKeyID != nil {
			body.CustomGitSSHKeyId = nullable.NewNullableWithValue(*s.SSHKeyID)
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
	if source.Docker != nil && source.Docker.Password != nil {
		return sanitizeError(err, *source.Docker.Password)
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
	// Railpack-only fields are left unset for other build types so their request
	// bodies stay byte-identical to before Railpack support existed.
	if b.Type == BuildRailpack {
		body.IsStaticSpa = nullable.NewNullableWithValue(b.IsStaticSpa)
		// publishDirectory is sent on every railpack save, null when unset, the way
		// railpackVersion already is. Omitting the key would leave whatever Dokploy
		// holds in place, so removing it from a program could never clear it and
		// every later preview would show the same diff.
		body.PublishDirectory = nullable.NewNullNullable[string]()
		if b.RailpackVersion != nil {
			body.RailpackVersion = nullable.NewNullableWithValue(*b.RailpackVersion)
		}
		if b.PublishDirectory != nil {
			body.PublishDirectory = nullable.NewNullableWithValue(*b.PublishDirectory)
		}
	}
	_, err := api.ApplicationSaveBuildTypeWithResponse(ctx, body)
	return err
}

func sanitizeError(err error, secrets ...string) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	for _, secret := range secrets {
		if secret != "" {
			message = strings.ReplaceAll(message, secret, "[REDACTED]")
		}
	}
	return fmt.Errorf("%s", message)
}
