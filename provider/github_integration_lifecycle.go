package dokploy

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/pulumi/pulumi-go-provider/infer"
)

func githubActiveOrganization(ctx context.Context, api *client.Client) (string, error) {
	r, err := api.OrganizationActiveWithResponse(ctx)
	if err != nil {
		return "", sanitizeGitHubIntegrationError(err)
	}
	if r == nil || r.StatusCode() != http.StatusOK || r.JSON200 == nil || activeOrganizationID(*r.JSON200) == "" {
		if r != nil && r.StatusCode() != http.StatusOK {
			return "", githubStatus("organization.active", r.StatusCode())
		}
		return "", githubMalformed()
	}
	return activeOrganizationID(*r.JSON200), nil
}
func (r GitHubIntegration) Read(ctx context.Context, req infer.ReadRequest[GitHubIntegrationArgs, GitHubIntegrationState]) (infer.ReadResponse[GitHubIntegrationArgs, GitHubIntegrationState], error) {
	api := r.client(ctx)
	org, err := githubActiveOrganization(ctx, api)
	if err != nil {
		return infer.ReadResponse[GitHubIntegrationArgs, GitHubIntegrationState]{}, err
	}
	o, err := readGitHubIntegration(ctx, api, req.ID, req.State, org)
	if client.IsNotFound(err) {
		return infer.ReadResponse[GitHubIntegrationArgs, GitHubIntegrationState]{}, nil
	}
	if err != nil {
		return infer.ReadResponse[GitHubIntegrationArgs, GitHubIntegrationState]{}, sanitizeGitHubIntegrationError(err)
	}
	return infer.ReadResponse[GitHubIntegrationArgs, GitHubIntegrationState]{ID: req.ID, Inputs: o.State.GitHubIntegrationArgs, State: o.State}, nil
}
func githubMutation(resp *http.Response, err error, op string) error {
	if err != nil {
		return sanitizeGitHubIntegrationError(err)
	}
	if resp == nil {
		return githubMalformed()
	}
	status := resp.StatusCode
	if resp.Body != nil {
		_ = resp.Body.Close()
	}
	if status < 200 || status >= 300 {
		return githubStatus(op, status)
	}
	return nil
}
func (r GitHubIntegration) Update(ctx context.Context, req infer.UpdateRequest[GitHubIntegrationArgs, GitHubIntegrationState]) (infer.UpdateResponse[GitHubIntegrationState], error) {
	if req.DryRun {
		return infer.UpdateResponse[GitHubIntegrationState]{Output: gitHubIntegrationPreviewState(req.Inputs, req.ID, req.State)}, nil
	}
	if err := validateGitHubIntegrationArgs(req.Inputs); err != nil {
		return infer.UpdateResponse[GitHubIntegrationState]{}, err
	}
	api := r.client(ctx)
	org, err := githubActiveOrganization(ctx, api)
	if err != nil {
		return infer.UpdateResponse[GitHubIntegrationState]{}, err
	}
	before, err := readGitHubIntegration(ctx, api, req.ID, req.State, org)
	if err != nil {
		return infer.UpdateResponse[GitHubIntegrationState]{}, sanitizeGitHubIntegrationError(err)
	}
	state := before.State
	resp, mutationErr := api.GithubUpdate(ctx, generated.GithubUpdateJSONRequestBody{GithubId: state.GitHubID, GitProviderId: state.GitProviderID, Name: req.Inputs.Name, GithubAppName: req.Inputs.GitHubAppName})
	mutationErr = githubMutation(resp, mutationErr, "github.update")
	if ctx.Err() == nil {
		after, readErr := readGitHubIntegration(ctx, api, req.ID, state, org)
		if readErr == nil {
			state = after.State
			if mutationErr == nil && (state.Name != req.Inputs.Name || state.GitHubAppName != req.Inputs.GitHubAppName) {
				mutationErr = errors.New("GitHub integration update could not be confirmed")
			}
		} else if mutationErr == nil {
			mutationErr = errors.New("GitHub integration update could not be confirmed")
		}
	} else if mutationErr == nil {
		mutationErr = sanitizeGitHubIntegrationError(ctx.Err())
	}
	if ctx.Err() != nil {
		mutationErr = sanitizeGitHubIntegrationError(ctx.Err())
	}
	if mutationErr != nil {
		if errors.Is(mutationErr, context.Canceled) || errors.Is(mutationErr, context.DeadlineExceeded) {
			return infer.UpdateResponse[GitHubIntegrationState]{Output: state}, errors.Join(initFailed(mutationErr), mutationErr)
		}
		return infer.UpdateResponse[GitHubIntegrationState]{Output: state}, initFailed(mutationErr)
	}
	return infer.UpdateResponse[GitHubIntegrationState]{Output: state}, nil
}
func (r GitHubIntegration) Delete(ctx context.Context, req infer.DeleteRequest[GitHubIntegrationState]) (infer.DeleteResponse, error) {
	api := r.client(ctx)
	org, err := githubActiveOrganization(ctx, api)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	before, err := readGitHubIntegration(ctx, api, req.ID, req.State, org)
	if client.IsNotFound(err) {
		return infer.DeleteResponse{}, nil
	}
	if err != nil {
		return infer.DeleteResponse{}, sanitizeGitHubIntegrationError(err)
	}
	resp, mutationErr := api.GitProviderRemove(ctx, generated.GitProviderRemoveJSONRequestBody{GitProviderId: before.State.GitProviderID})
	mutationErr = githubMutation(resp, mutationErr, "gitProvider.remove")
	if ctx.Err() != nil {
		return infer.DeleteResponse{}, sanitizeGitHubIntegrationError(ctx.Err())
	}
	_, readErr := readGitHubIntegration(ctx, api, req.ID, before.State, org)
	if client.IsNotFound(readErr) {
		return infer.DeleteResponse{}, nil
	}
	if readErr != nil {
		return infer.DeleteResponse{}, fmt.Errorf("gitProvider.remove: absence unconfirmed: %w", sanitizeGitHubIntegrationError(readErr))
	}
	if mutationErr != nil {
		return infer.DeleteResponse{}, mutationErr
	}
	return infer.DeleteResponse{}, errors.New("gitProvider.remove: integration still present")
}
