package dokploy

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/oapi-codegen/nullable"
	"github.com/pulumi/pulumi-go-provider/infer"
)

func gitLabActiveOrganization(ctx context.Context, api *client.Client) (string, error) {
	resp, err := api.OrganizationActiveWithResponse(ctx)
	if err != nil {
		return "", sanitizeGitLabIntegrationError(err)
	}
	if resp == nil {
		return "", gitLabMalformed("organization.active")
	}
	if resp.StatusCode() != http.StatusOK {
		return "", gitLabStatus("organization.active", resp.StatusCode())
	}
	if resp.JSON200 == nil || activeOrganizationID(*resp.JSON200) == "" {
		return "", gitLabMalformed("organization.active")
	}
	return activeOrganizationID(*resp.JSON200), nil
}

func (r GitLabIntegration) Read(ctx context.Context, req infer.ReadRequest[GitLabIntegrationArgs, GitLabIntegrationState]) (infer.ReadResponse[GitLabIntegrationArgs, GitLabIntegrationState], error) {
	api := r.client(ctx)
	org, err := gitLabActiveOrganization(ctx, api)
	if err != nil {
		return infer.ReadResponse[GitLabIntegrationArgs, GitLabIntegrationState]{}, err
	}
	observed, err := readGitLabIntegration(ctx, api, req.ID, req.State, org)
	if client.IsNotFound(err) {
		return infer.ReadResponse[GitLabIntegrationArgs, GitLabIntegrationState]{ID: ""}, nil
	}
	if err != nil {
		return infer.ReadResponse[GitLabIntegrationArgs, GitLabIntegrationState]{}, sanitizeGitLabIntegrationError(err)
	}
	return infer.ReadResponse[GitLabIntegrationArgs, GitLabIntegrationState]{ID: req.ID, Inputs: observed.State.GitLabIntegrationArgs, State: observed.State}, nil
}

// A successful status acknowledges the mutation even when its body is malformed.
func gitLabMutation(response *http.Response, err error, operation string) error {
	if err != nil {
		return sanitizeGitLabIntegrationError(err)
	}
	if response == nil {
		return gitLabMalformed(operation)
	}
	status := response.StatusCode
	if response.Body != nil {
		_ = response.Body.Close()
	}
	if status < 200 || status >= 300 {
		return gitLabStatus(operation, status)
	}
	return nil
}

func gitLabSettingsMatch(actual GitLabIntegrationState, desired GitLabIntegrationArgs) bool {
	return actual.Name == desired.Name && actual.ApplicationID == desired.ApplicationID && actual.RedirectURI == desired.RedirectURI && actual.GitlabURL == desired.GitlabURL && actual.GroupName == desired.GroupName && sameOptionalString(actual.GitlabInternalURL, desired.GitlabInternalURL)
}

func (r GitLabIntegration) Update(ctx context.Context, req infer.UpdateRequest[GitLabIntegrationArgs, GitLabIntegrationState]) (infer.UpdateResponse[GitLabIntegrationState], error) {
	if req.DryRun {
		return infer.UpdateResponse[GitLabIntegrationState]{Output: GitLabIntegrationState{GitLabIntegrationArgs: req.Inputs, GitLabID: req.ID, GitProviderID: req.State.GitProviderID, OrganizationID: req.State.OrganizationID}}, nil
	}
	if err := validateGitLabIntegrationArgs(req.Inputs); err != nil {
		return infer.UpdateResponse[GitLabIntegrationState]{}, err
	}
	api := r.client(ctx)
	org, err := gitLabActiveOrganization(ctx, api)
	if err != nil {
		return infer.UpdateResponse[GitLabIntegrationState]{}, err
	}
	before, err := readGitLabIntegration(ctx, api, req.ID, req.State, org)
	if err != nil {
		return infer.UpdateResponse[GitLabIntegrationState]{}, sanitizeGitLabIntegrationError(err)
	}
	state := before.State
	internal := nullable.NewNullNullable[string]()
	if req.Inputs.GitlabInternalURL != nil {
		internal = nullable.NewNullableWithValue(*req.Inputs.GitlabInternalURL)
	}
	response, mutationErr := api.GitlabUpdate(ctx, generated.GitlabUpdateJSONRequestBody{
		GitlabId: state.GitLabID, GitProviderId: state.GitProviderID,
		Name: req.Inputs.Name, ApplicationId: &req.Inputs.ApplicationID, Secret: &req.Inputs.ApplicationSecret,
		RedirectUri: &req.Inputs.RedirectURI, GitlabUrl: req.Inputs.GitlabURL,
		GroupName: &req.Inputs.GroupName, GitlabInternalUrl: internal,
	})
	mutationErr = gitLabMutation(response, mutationErr, "gitlab.update")
	// On any failure, only a validated readback may replace the last verified state.
	if ctx.Err() == nil {
		after, readErr := readGitLabIntegration(ctx, api, req.ID, state, org)
		if readErr == nil && after.OwnerUserID == before.OwnerUserID {
			state = after.State
			if mutationErr == nil {
				if !gitLabSettingsMatch(state, req.Inputs) || state.ApplicationSecret != req.Inputs.ApplicationSecret {
					mutationErr = errors.New("GitLab integration update could not be confirmed")
				}
			}
		} else if mutationErr == nil {
			mutationErr = errors.New("GitLab integration update could not be confirmed")
		}
	} else if mutationErr == nil {
		mutationErr = sanitizeGitLabIntegrationError(ctx.Err())
	}
	if mutationErr != nil {
		return infer.UpdateResponse[GitLabIntegrationState]{Output: state}, initFailed(mutationErr)
	}
	return infer.UpdateResponse[GitLabIntegrationState]{Output: state}, nil
}

func (r GitLabIntegration) Delete(ctx context.Context, req infer.DeleteRequest[GitLabIntegrationState]) (infer.DeleteResponse, error) {
	api := r.client(ctx)
	org, err := gitLabActiveOrganization(ctx, api)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	before, err := readGitLabIntegration(ctx, api, req.ID, req.State, org)
	if client.IsNotFound(err) {
		return infer.DeleteResponse{}, nil
	}
	if err != nil {
		return infer.DeleteResponse{}, sanitizeGitLabIntegrationError(err)
	}
	response, mutationErr := api.GitProviderRemove(ctx, generated.GitProviderRemoveJSONRequestBody{GitProviderId: before.State.GitProviderID})
	mutationErr = gitLabMutation(response, mutationErr, "gitProvider.remove")
	if ctx.Err() != nil {
		return infer.DeleteResponse{}, sanitizeGitLabIntegrationError(ctx.Err())
	}
	_, readErr := readGitLabIntegration(ctx, api, req.ID, before.State, org)
	if client.IsNotFound(readErr) {
		return infer.DeleteResponse{}, nil
	}
	if readErr != nil {
		return infer.DeleteResponse{}, fmt.Errorf("gitProvider.remove: absence unconfirmed: %w", sanitizeGitLabIntegrationError(readErr))
	}
	if mutationErr != nil {
		return infer.DeleteResponse{}, mutationErr
	}
	return infer.DeleteResponse{}, errors.New("gitProvider.remove: integration still present")
}
