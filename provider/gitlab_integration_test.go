package dokploy

import (
	"strings"
	"testing"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

func TestGitLabIntegrationCheckDefaults(t *testing.T) {
	inputs := property.NewMap(map[string]property.Value{
		"name": property.New("gitlab"), "applicationId": property.New("app-id"),
		"applicationSecret": property.New("app-secret"), "redirectUri": property.New("https://example.com/oauth/callback"),
	})
	checked, err := (GitLabIntegration{}).Check(t.Context(), infer.CheckRequest{NewInputs: inputs})
	require.NoError(t, err)
	require.Empty(t, checked.Failures)
	require.Equal(t, "https://gitlab.com", checked.Inputs.GitlabURL)
	require.Equal(t, "", checked.Inputs.GroupName)
	require.Nil(t, checked.Inputs.GitlabInternalURL)
}

func TestGitLabIntegrationCheckKnownAndComputed(t *testing.T) {
	checked, err := (GitLabIntegration{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{
		"name": property.New(""), "applicationId": property.New("app-id"),
		"applicationSecret": property.New(property.Computed), "redirectUri": property.New("https://example.com/callback"),
		"gitlabUrl": property.New("not-a-url"),
	})})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(checked.Failures), 2)
	for _, failure := range checked.Failures {
		require.NotContains(t, failure.Reason, "not-a-url")
	}
}

func TestGitLabIntegrationCheckRejectsExplicitNullPublicURL(t *testing.T) {
	checked, err := (GitLabIntegration{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{
		"name": property.New("gitlab"), "applicationId": property.New("app-id"),
		"applicationSecret": property.New(property.Computed), "redirectUri": property.New("https://example.com/callback"),
		"gitlabUrl": property.New(property.Null),
	})})
	require.NoError(t, err)
	require.NotEmpty(t, checked.Failures)
	require.NotContains(t, checked.Failures[0].Reason, "example.com")
}

func TestGitLabIntegrationCheckRejectsExplicitEmptyURLs(t *testing.T) {
	for _, field := range []string{"gitlabUrl", "gitlabInternalUrl"} {
		t.Run(field, func(t *testing.T) {
			inputs := map[string]property.Value{
				"name": property.New("gitlab"), "applicationId": property.New("app-id"),
				"applicationSecret": property.New(property.Computed), "redirectUri": property.New("https://example.com/callback"),
				"gitlabUrl": property.New("https://gitlab.com"),
			}
			inputs[field] = property.New("")
			checked, err := (GitLabIntegration{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(inputs)})
			require.NoError(t, err)
			require.NotEmpty(t, checked.Failures)
			for _, failure := range checked.Failures {
				require.NotContains(t, failure.Reason, "example.com")
				require.NotContains(t, failure.Reason, "gitlab.com")
			}
		})
	}
}

func TestGitLabIntegrationURLValidation(t *testing.T) {
	valid := GitLabIntegrationArgs{Name: "gitlab", ApplicationID: "app-id", ApplicationSecret: "secret", RedirectURI: "https://example.com/oauth/callback", GitlabURL: "https://gitlab.example.com/gitlab"}
	t.Run("valid HTTP(S) host and path", func(t *testing.T) {
		require.NoError(t, validateGitLabIntegrationArgs(valid))
	})
	tests := []struct {
		name string
		edit func(*GitLabIntegrationArgs)
	}{
		{"empty name", func(a *GitLabIntegrationArgs) { a.Name = "" }},
		{"empty public URL", func(a *GitLabIntegrationArgs) { a.GitlabURL = "" }},
		{"public URL without host", func(a *GitLabIntegrationArgs) { a.GitlabURL = "https:///path" }},
		{"public URL with userinfo", func(a *GitLabIntegrationArgs) { a.GitlabURL = "https://user:pass@gitlab.example.com" }},
		{"callback with userinfo", func(a *GitLabIntegrationArgs) { a.RedirectURI = "https://user:pass@example.com/callback" }},
		{"unsupported public scheme", func(a *GitLabIntegrationArgs) { a.GitlabURL = "ftp://gitlab.example.com" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := valid
			tt.edit(&args)
			err := validateGitLabIntegrationArgs(args)
			require.Error(t, err)
			require.NotContains(t, strings.ToLower(err.Error()), "secret")
			require.NotContains(t, err.Error(), "gitlab.example.com")
		})
	}
	t.Run("internal URL allows basic auth", func(t *testing.T) {
		args := valid
		args.GitlabInternalURL = stringPtr("https://user:pass@gitlab.internal.example.com/api")
		require.NoError(t, validateGitLabIntegrationArgs(args))
	})
	t.Run("explicit empty internal URL is invalid", func(t *testing.T) {
		args := valid
		args.GitlabInternalURL = stringPtr("")
		err := validateGitLabIntegrationArgs(args)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "gitlab.internal.example.com")
	})
}

func TestGitLabIntegrationDiff(t *testing.T) {
	old := GitLabIntegrationArgs{Name: "old", ApplicationID: "app-id", ApplicationSecret: "secret", RedirectURI: "https://example.com/callback", GitlabURL: "https://gitlab.com"}
	in := old
	in.Name = "new"
	in.ApplicationID = "app-id-updated"
	in.ApplicationSecret = "secret-updated"
	in.RedirectURI = "https://example.com/updated-callback"
	in.GitlabURL = "https://gitlab.example.com"
	in.GroupName = "team"
	in.GitlabInternalURL = stringPtr("https://gitlab.internal.example.com")
	changes, err := (GitLabIntegration{}).Diff(t.Context(), infer.DiffRequest[GitLabIntegrationArgs, GitLabIntegrationState]{Inputs: in, State: GitLabIntegrationState{GitLabIntegrationArgs: old}})
	require.NoError(t, err)
	require.True(t, changes.HasChanges)
	for _, field := range []string{"name", "applicationId", "applicationSecret", "redirectUri", "gitlabUrl", "groupName", "gitlabInternalUrl"} {
		require.Contains(t, changes.DetailedDiff, field)
	}
	for _, diff := range changes.DetailedDiff {
		require.Equal(t, p.Update, diff.Kind)
	}
	unchanged, err := (GitLabIntegration{}).Diff(t.Context(), infer.DiffRequest[GitLabIntegrationArgs, GitLabIntegrationState]{Inputs: old, State: GitLabIntegrationState{GitLabIntegrationArgs: old}})
	require.NoError(t, err)
	require.False(t, unchanged.HasChanges)
	require.Empty(t, unchanged.DetailedDiff)
}
