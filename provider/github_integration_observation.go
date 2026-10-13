package dokploy

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
)

type githubObservation struct {
	State                 GitHubIntegrationState
	ConfigurationComplete bool
}

func sanitizeGitHubIntegrationError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		return fmt.Errorf("GitHub integration HTTP %d", apiErr.StatusCode)
	}
	return errors.New("GitHub integration request failed")
}
func githubMalformed() error                   { return errors.New("github.one: incomplete or inconsistent response") }
func githubStatus(op string, status int) error { return fmt.Errorf("%s: HTTP %d", op, status) }

func readGitHubIntegration(ctx context.Context, api *client.Client, relationID string, prior GitHubIntegrationState, organizationID string) (githubObservation, error) {
	var out githubObservation
	if relationID == "" || organizationID == "" {
		return out, githubMalformed()
	}
	raw, err := api.GithubOne(ctx, &generated.GithubOneParams{GithubId: relationID})
	if err != nil {
		if client.IsNotFound(err) {
			return out, &client.APIError{StatusCode: http.StatusNotFound}
		}
		return out, sanitizeGitHubIntegrationError(err)
	}
	if raw == nil {
		return out, githubMalformed()
	}
	if raw.StatusCode == http.StatusNotFound {
		if raw.Body != nil {
			_ = raw.Body.Close()
		}
		return out, &client.APIError{StatusCode: http.StatusNotFound}
	}
	if raw.StatusCode != http.StatusOK {
		if raw.Body != nil {
			_ = raw.Body.Close()
		}
		return out, githubStatus("github.one", raw.StatusCode)
	}
	response, err := generated.ParseGithubOneResponse(raw)
	if err != nil || response == nil || response.JSON200 == nil {
		return out, githubMalformed()
	}
	v := response.JSON200
	p := v.GitProvider
	if v.GithubId != relationID || v.GitProviderId == "" || p.GitProviderId != v.GitProviderId || p.Name == "" || p.ProviderType != "github" || p.OrganizationId == "" || p.OrganizationId != organizationID || !v.GithubAppName.IsSpecified() ||
		(prior.GitHubID != "" && prior.GitHubID != relationID) || (prior.GitProviderID != "" && prior.GitProviderID != v.GitProviderId) || (prior.OrganizationID != "" && prior.OrganizationID != p.OrganizationId) {
		return out, githubMalformed()
	}
	app := ""
	if !v.GithubAppName.IsNull() {
		app, err = v.GithubAppName.Get()
		if err != nil {
			return out, githubMalformed()
		}
	}
	state := GitHubIntegrationState{GitHubIntegrationArgs: GitHubIntegrationArgs{Name: p.Name, GitHubAppName: app}, GitHubID: relationID, GitProviderID: v.GitProviderId, OrganizationID: p.OrganizationId}
	return githubObservation{State: state, ConfigurationComplete: app != ""}, nil
}
