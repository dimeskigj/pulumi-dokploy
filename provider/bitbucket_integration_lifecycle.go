package dokploy

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/pulumi/pulumi-go-provider/infer"
)

func bitbucketActiveOrganization(ctx context.Context, api *client.Client) (string, error) {
	response, err := api.OrganizationActiveWithResponse(ctx)
	if err != nil {
		return "", sanitizeBitbucketIntegrationError(err)
	}
	if response == nil {
		return "", bitbucketMalformed()
	}
	if response.StatusCode() != http.StatusOK {
		return "", bitbucketStatus("organization.active", response.StatusCode())
	}
	if response.JSON200 == nil || activeOrganizationID(*response.JSON200) == "" {
		return "", bitbucketMalformed()
	}
	return activeOrganizationID(*response.JSON200), nil
}

func (r BitbucketIntegration) Read(ctx context.Context, req infer.ReadRequest[BitbucketIntegrationArgs, BitbucketIntegrationState]) (infer.ReadResponse[BitbucketIntegrationArgs, BitbucketIntegrationState], error) {
	api := r.client(ctx)
	organizationID, err := bitbucketActiveOrganization(ctx, api)
	if err != nil {
		return infer.ReadResponse[BitbucketIntegrationArgs, BitbucketIntegrationState]{}, err
	}
	observed, err := readBitbucketIntegration(ctx, api, req.ID, req.State, organizationID)
	if client.IsNotFound(err) {
		return infer.ReadResponse[BitbucketIntegrationArgs, BitbucketIntegrationState]{ID: ""}, nil
	}
	if err != nil {
		return infer.ReadResponse[BitbucketIntegrationArgs, BitbucketIntegrationState]{}, sanitizeBitbucketIntegrationError(err)
	}
	return infer.ReadResponse[BitbucketIntegrationArgs, BitbucketIntegrationState]{ID: req.ID, Inputs: observed.State.BitbucketIntegrationArgs, State: observed.State}, nil
}

// bitbucketMutation classifies the raw mutation response without decoding or
// exposing an upstream response body. Mutation endpoints are never retried.
func bitbucketMutation(response *http.Response, err error, operation string) error {
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err != nil {
		return sanitizeBitbucketIntegrationError(err)
	}
	if response == nil {
		return bitbucketMalformed()
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return bitbucketStatus(operation, response.StatusCode)
	}
	return nil
}

func bitbucketPartialUpdateError(err error) error {
	safe := sanitizeBitbucketIntegrationError(err)
	if errors.Is(safe, context.Canceled) || errors.Is(safe, context.DeadlineExceeded) {
		return errors.Join(initFailed(safe), safe)
	}
	return initFailed(safe)
}

func (r BitbucketIntegration) Update(ctx context.Context, req infer.UpdateRequest[BitbucketIntegrationArgs, BitbucketIntegrationState]) (infer.UpdateResponse[BitbucketIntegrationState], error) {
	if req.DryRun {
		return infer.UpdateResponse[BitbucketIntegrationState]{Output: bitbucketIntegrationPreviewState(req.Inputs, req.ID, req.State)}, nil
	}
	if err := validateBitbucketIntegrationArgs(req.Inputs); err != nil {
		return infer.UpdateResponse[BitbucketIntegrationState]{}, err
	}
	api := r.client(ctx)
	organizationID, err := bitbucketActiveOrganization(ctx, api)
	if err != nil {
		return infer.UpdateResponse[BitbucketIntegrationState]{}, err
	}
	before, err := readBitbucketIntegration(ctx, api, req.ID, req.State, organizationID)
	if err != nil {
		return infer.UpdateResponse[BitbucketIntegrationState]{}, sanitizeBitbucketIntegrationError(err)
	}
	state := before.State
	apiToken, appPassword := "", ""
	if req.Inputs.APIToken != nil && *req.Inputs.APIToken != "" {
		apiToken = *req.Inputs.APIToken
	}
	if req.Inputs.AppPassword != nil && *req.Inputs.AppPassword != "" {
		appPassword = *req.Inputs.AppPassword
	}
	body := generated.BitbucketUpdateJSONRequestBody{
		BitbucketId: req.ID, GitProviderId: before.State.GitProviderID,
		Name: req.Inputs.Name, ApiToken: &apiToken, AppPassword: &appPassword,
	}
	if req.Inputs.BitbucketEmail != nil {
		email := openapi_types.Email(*req.Inputs.BitbucketEmail)
		body.BitbucketEmail = &email
	}
	if req.Inputs.BitbucketUsername != nil {
		body.BitbucketUsername = req.Inputs.BitbucketUsername
	}
	if req.Inputs.BitbucketWorkspaceName != nil {
		body.BitbucketWorkspaceName = req.Inputs.BitbucketWorkspaceName
	}
	response, mutationErr := api.BitbucketUpdate(ctx, body)
	mutationErr = bitbucketMutation(response, mutationErr, "bitbucket.update")

	// Read back once when possible even after an uncertain mutation outcome. A
	// validated observation, never the desired inputs, is the partial state.
	if ctx.Err() == nil {
		after, readErr := readBitbucketIntegration(ctx, api, req.ID, before.State, organizationID)
		if readErr == nil && after.OwnerUserID == before.OwnerUserID {
			state = bitbucketIntegrationMutationState(after, req.Inputs)
			if mutationErr == nil && !bitbucketIntegrationSettingsMatch(after, req.Inputs, req.Inputs.Name) {
				mutationErr = errors.New("Bitbucket integration update could not be confirmed")
			}
		} else if mutationErr == nil {
			mutationErr = errors.New("Bitbucket integration update could not be confirmed")
		}
	} else {
		mutationErr = ctx.Err()
	}
	if ctx.Err() != nil {
		mutationErr = errors.Join(mutationErr, ctx.Err())
	}
	if mutationErr != nil {
		return infer.UpdateResponse[BitbucketIntegrationState]{Output: state}, bitbucketPartialUpdateError(mutationErr)
	}
	return infer.UpdateResponse[BitbucketIntegrationState]{Output: state}, nil
}

func (r BitbucketIntegration) Delete(ctx context.Context, req infer.DeleteRequest[BitbucketIntegrationState]) (infer.DeleteResponse, error) {
	api := r.client(ctx)
	organizationID, err := bitbucketActiveOrganization(ctx, api)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	before, err := readBitbucketIntegration(ctx, api, req.ID, req.State, organizationID)
	if client.IsNotFound(err) {
		return infer.DeleteResponse{}, nil
	}
	if err != nil {
		return infer.DeleteResponse{}, sanitizeBitbucketIntegrationError(err)
	}
	if err := ctx.Err(); err != nil {
		return infer.DeleteResponse{}, sanitizeBitbucketIntegrationError(err)
	}
	response, mutationErr := api.GitProviderRemove(ctx, generated.GitProviderRemoveJSONRequestBody{GitProviderId: before.State.GitProviderID})
	mutationErr = bitbucketMutation(response, mutationErr, "gitProvider.remove")
	if err := ctx.Err(); err != nil {
		return infer.DeleteResponse{}, sanitizeBitbucketIntegrationError(err)
	}
	_, readErr := readBitbucketIntegration(ctx, api, req.ID, before.State, organizationID)
	if client.IsNotFound(readErr) {
		return infer.DeleteResponse{}, nil
	}
	if readErr != nil {
		return infer.DeleteResponse{}, fmt.Errorf("gitProvider.remove: absence unconfirmed: %w", sanitizeBitbucketIntegrationError(readErr))
	}
	if mutationErr != nil {
		return infer.DeleteResponse{}, mutationErr
	}
	return infer.DeleteResponse{}, errors.New("gitProvider.remove: integration still present")
}
