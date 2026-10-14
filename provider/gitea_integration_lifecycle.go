package dokploy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/oapi-codegen/nullable"
	"github.com/pulumi/pulumi-go-provider/infer"
)

func giteaActiveOrganization(ctx context.Context, api *client.Client) (string, error) {
	r, err := api.OrganizationActiveWithResponse(ctx)
	if err != nil {
		return "", sanitizeGiteaIntegrationError(err)
	}
	if r == nil || r.StatusCode() != http.StatusOK || r.JSON200 == nil || activeOrganizationID(*r.JSON200) == "" {
		if r != nil && r.StatusCode() != http.StatusOK {
			return "", giteaStatus("organization.active", r.StatusCode())
		}
		return "", giteaMalformed()
	}
	return activeOrganizationID(*r.JSON200), nil
}

func (r GiteaIntegration) Create(ctx context.Context, req infer.CreateRequest[GiteaIntegrationArgs]) (infer.CreateResponse[GiteaIntegrationState], error) {
	if req.DryRun {
		return infer.CreateResponse[GiteaIntegrationState]{Output: GiteaIntegrationState{GiteaIntegrationArgs: req.Inputs}}, nil
	}
	if err := validateGiteaIntegrationArgs(req.Inputs); err != nil {
		return infer.CreateResponse[GiteaIntegrationState]{}, err
	}
	api := r.client(ctx)
	org, err := giteaActiveOrganization(ctx, api)
	if err != nil {
		return infer.CreateResponse[GiteaIntegrationState]{}, err
	}
	internal := nullable.NewNullNullable[string]()
	if req.Inputs.GiteaInternalURL != nil {
		internal = nullable.NewNullableWithValue(*req.Inputs.GiteaInternalURL)
	}
	name, clientID, secret, redirect, group := req.Inputs.Name, req.Inputs.ClientID, req.Inputs.ClientSecret, req.Inputs.RedirectURI, req.Inputs.OrganizationName
	resp, err := api.GiteaCreate(ctx, generated.GiteaCreateJSONRequestBody{Name: name, GiteaUrl: req.Inputs.GiteaURL, ClientId: &clientID, ClientSecret: &secret, RedirectUri: &redirect, OrganizationName: &group, GiteaInternalUrl: internal})
	if err != nil {
		return infer.CreateResponse[GiteaIntegrationState]{}, sanitizeGiteaIntegrationError(err)
	}
	ack, ackErr := generated.ParseGiteaCreateResponse(resp)
	if ackErr != nil || ack == nil || ack.JSON200 == nil || !positiveGiteaID(ack.JSON200.GiteaId) {
		return infer.CreateResponse[GiteaIntegrationState]{}, giteaMalformed()
	}
	state := GiteaIntegrationState{GiteaID: ack.JSON200.GiteaId}
	observed, err := readGiteaIntegration(ctx, api, state.GiteaID, state, org)
	if err != nil {
		partial := GiteaIntegrationState{GiteaID: state.GiteaID, GiteaIntegrationArgs: GiteaIntegrationArgs{GiteaURL: ack.JSON200.GiteaUrl}}
		if ack.JSON200.ClientId.IsSpecified() {
			partial.ClientID = giteaNullable(ack.JSON200.ClientId)
		}
		cause := sanitizeGiteaIntegrationError(err)
		if errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
			return infer.CreateResponse[GiteaIntegrationState]{ID: state.GiteaID, Output: partial}, errors.Join(initFailed(cause), cause)
		}
		return infer.CreateResponse[GiteaIntegrationState]{ID: state.GiteaID, Output: partial}, initFailed(cause)
	}
	if !observed.SecretObserved || observed.OwnerUserID == "" || !giteaSettingsMatch(observed.State, req.Inputs) {
		return infer.CreateResponse[GiteaIntegrationState]{ID: state.GiteaID, Output: observed.State}, initFailed(errors.New("Gitea integration create could not be confirmed"))
	}
	return infer.CreateResponse[GiteaIntegrationState]{ID: state.GiteaID, Output: observed.State}, nil
}

func positiveGiteaID(id string) bool {
	n, err := strconv.ParseInt(id, 10, 64)
	return err == nil && n > 0
}

func (r GiteaIntegration) Read(ctx context.Context, req infer.ReadRequest[GiteaIntegrationArgs, GiteaIntegrationState]) (infer.ReadResponse[GiteaIntegrationArgs, GiteaIntegrationState], error) {
	api := r.client(ctx)
	org, err := giteaActiveOrganization(ctx, api)
	if err != nil {
		return infer.ReadResponse[GiteaIntegrationArgs, GiteaIntegrationState]{}, err
	}
	o, err := readGiteaIntegration(ctx, api, req.ID, req.State, org)
	if client.IsNotFound(err) {
		return infer.ReadResponse[GiteaIntegrationArgs, GiteaIntegrationState]{}, nil
	}
	if err != nil {
		return infer.ReadResponse[GiteaIntegrationArgs, GiteaIntegrationState]{}, sanitizeGiteaIntegrationError(err)
	}
	return infer.ReadResponse[GiteaIntegrationArgs, GiteaIntegrationState]{ID: req.ID, Inputs: o.State.GiteaIntegrationArgs, State: o.State}, nil
}

func giteaMutation(resp *http.Response, err error, op string) error {
	if err != nil {
		return sanitizeGiteaIntegrationError(err)
	}
	if resp == nil {
		return giteaMalformed()
	}
	status := resp.StatusCode
	if resp.Body != nil {
		_ = resp.Body.Close()
	}
	if status < 200 || status >= 300 {
		return giteaStatus(op, status)
	}
	return nil
}
func giteaSettingsMatch(a GiteaIntegrationState, d GiteaIntegrationArgs) bool {
	return a.Name == d.Name && a.GiteaURL == d.GiteaURL && a.ClientID == d.ClientID && a.ClientSecret == d.ClientSecret && a.RedirectURI == d.RedirectURI && sameOptionalString(a.GiteaInternalURL, d.GiteaInternalURL) && a.OrganizationName == d.OrganizationName
}
func giteaIntegrationPreviewState(inputs GiteaIntegrationArgs, relationID string, prior GiteaIntegrationState) GiteaIntegrationState {
	return GiteaIntegrationState{GiteaIntegrationArgs: inputs, GiteaID: relationID, GitProviderID: prior.GitProviderID, OrganizationID: prior.OrganizationID}
}

func (r GiteaIntegration) Update(ctx context.Context, req infer.UpdateRequest[GiteaIntegrationArgs, GiteaIntegrationState]) (infer.UpdateResponse[GiteaIntegrationState], error) {
	if req.DryRun {
		return infer.UpdateResponse[GiteaIntegrationState]{Output: giteaIntegrationPreviewState(req.Inputs, req.ID, req.State)}, nil
	}
	if err := validateGiteaIntegrationArgs(req.Inputs); err != nil {
		return infer.UpdateResponse[GiteaIntegrationState]{}, err
	}
	api := r.client(ctx)
	org, err := giteaActiveOrganization(ctx, api)
	if err != nil {
		return infer.UpdateResponse[GiteaIntegrationState]{}, err
	}
	before, err := readGiteaIntegration(ctx, api, req.ID, req.State, org)
	if err != nil {
		return infer.UpdateResponse[GiteaIntegrationState]{}, sanitizeGiteaIntegrationError(err)
	}
	if !before.ConfigurationComplete {
		return infer.UpdateResponse[GiteaIntegrationState]{Output: before.State}, initFailed(errors.New("Gitea integration configuration is incomplete; supply required settings"))
	}
	internal := nullable.NewNullNullable[string]()
	if req.Inputs.GiteaInternalURL != nil {
		internal = nullable.NewNullableWithValue(*req.Inputs.GiteaInternalURL)
	}
	name, clientID, secret, redirect, group := req.Inputs.Name, req.Inputs.ClientID, req.Inputs.ClientSecret, req.Inputs.RedirectURI, req.Inputs.OrganizationName
	resp, mutErr := api.GiteaUpdate(ctx, generated.GiteaUpdateJSONRequestBody{GiteaId: before.State.GiteaID, GitProviderId: before.State.GitProviderID, Name: name, GiteaUrl: req.Inputs.GiteaURL, ClientId: &clientID, ClientSecret: &secret, RedirectUri: &redirect, OrganizationName: &group, GiteaInternalUrl: internal})
	mutErr = giteaMutation(resp, mutErr, "gitea.update")
	state := before.State
	if ctx.Err() == nil {
		after, readErr := readGiteaIntegration(ctx, api, req.ID, state, org)
		if readErr == nil && after.OwnerUserID == before.OwnerUserID {
			state = after.State
			if mutErr == nil && (!after.SecretObserved || !giteaSettingsMatch(state, req.Inputs)) {
				mutErr = errors.New("Gitea integration update could not be confirmed")
			}
		} else if mutErr == nil {
			mutErr = errors.New("Gitea integration update could not be confirmed")
		}
	} else if mutErr == nil {
		mutErr = sanitizeGiteaIntegrationError(ctx.Err())
	}
	if ctx.Err() != nil {
		mutErr = sanitizeGiteaIntegrationError(ctx.Err())
	}
	if mutErr != nil {
		if errors.Is(mutErr, context.Canceled) || errors.Is(mutErr, context.DeadlineExceeded) {
			return infer.UpdateResponse[GiteaIntegrationState]{Output: state}, errors.Join(initFailed(mutErr), mutErr)
		}
		return infer.UpdateResponse[GiteaIntegrationState]{Output: state}, initFailed(mutErr)
	}
	return infer.UpdateResponse[GiteaIntegrationState]{Output: state}, nil
}

func (r GiteaIntegration) Delete(ctx context.Context, req infer.DeleteRequest[GiteaIntegrationState]) (infer.DeleteResponse, error) {
	api := r.client(ctx)
	org, err := giteaActiveOrganization(ctx, api)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	before, err := readGiteaIntegration(ctx, api, req.ID, req.State, org)
	if client.IsNotFound(err) {
		return infer.DeleteResponse{}, nil
	}
	if err != nil {
		return infer.DeleteResponse{}, sanitizeGiteaIntegrationError(err)
	}
	resp, mutErr := api.GitProviderRemove(ctx, generated.GitProviderRemoveJSONRequestBody{GitProviderId: before.State.GitProviderID})
	mutErr = giteaMutation(resp, mutErr, "gitProvider.remove")
	if ctx.Err() != nil {
		return infer.DeleteResponse{}, sanitizeGiteaIntegrationError(ctx.Err())
	}
	_, readErr := readGiteaIntegration(ctx, api, req.ID, before.State, org)
	if client.IsNotFound(readErr) {
		return infer.DeleteResponse{}, nil
	}
	if readErr != nil {
		return infer.DeleteResponse{}, fmt.Errorf("gitProvider.remove: absence unconfirmed: %w", sanitizeGiteaIntegrationError(readErr))
	}
	if mutErr != nil {
		return infer.DeleteResponse{}, mutErr
	}
	return infer.DeleteResponse{}, errors.New("gitProvider.remove: integration still present")
}
