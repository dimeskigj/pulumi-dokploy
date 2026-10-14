package dokploy

import (
	"context"
	"net/http"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
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
