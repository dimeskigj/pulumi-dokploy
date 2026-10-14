package dokploy

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
)

type giteaObservation struct {
	State                 GiteaIntegrationState
	OwnerUserID           string
	ConfigurationComplete bool
	SecretObserved        bool
}

func sanitizeGiteaIntegrationError(err error) error {
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
		return fmt.Errorf("Gitea integration HTTP %d", apiErr.StatusCode)
	}
	return errors.New("Gitea integration request failed")
}

func giteaMalformed() error                   { return errors.New("gitea.one: incomplete or inconsistent response") }
func giteaStatus(op string, status int) error { return fmt.Errorf("%s: HTTP %d", op, status) }

func giteaNullable(v interface {
	IsSpecified() bool
	IsNull() bool
	Get() (string, error)
}) string {
	if !v.IsSpecified() || v.IsNull() {
		return ""
	}
	s, err := v.Get()
	if err != nil {
		return ""
	}
	return s
}

func readGiteaIntegration(ctx context.Context, api *client.Client, id string, prior GiteaIntegrationState, organizationID string) (giteaObservation, error) {
	var out giteaObservation
	if id == "" || organizationID == "" {
		return out, giteaMalformed()
	}
	raw, err := api.GiteaOne(ctx, &generated.GiteaOneParams{GiteaId: id})
	if err != nil {
		if client.IsNotFound(err) {
			return out, &client.APIError{StatusCode: http.StatusNotFound}
		}
		return out, sanitizeGiteaIntegrationError(err)
	}
	if raw == nil {
		return out, giteaMalformed()
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
		return out, giteaStatus("gitea.one", raw.StatusCode)
	}
	response, err := generated.ParseGiteaOneResponse(raw)
	if err != nil || response == nil || response.JSON200 == nil {
		return out, giteaMalformed()
	}
	v := response.JSON200
	p := v.GitProvider
	if v.GiteaId != id || v.GitProviderId == "" || p.GitProviderId != v.GitProviderId || p.Name == "" || p.ProviderType != "gitea" || p.OrganizationId != organizationID || p.UserId == "" || v.GiteaUrl == "" ||
		(prior.GiteaID != "" && prior.GiteaID != id) || (prior.GitProviderID != "" && prior.GitProviderID != v.GitProviderId) || (prior.OrganizationID != "" && prior.OrganizationID != p.OrganizationId) {
		return out, giteaMalformed()
	}
	secret := prior.ClientSecret
	if v.ClientSecret.IsSpecified() {
		secret = giteaNullable(v.ClientSecret)
	}
	internal := giteaNullable(v.GiteaInternalUrl)
	var internalPtr *string
	if internal != "" {
		internalPtr = &internal
	}
	state := GiteaIntegrationState{GiteaIntegrationArgs: GiteaIntegrationArgs{Name: p.Name, GiteaURL: v.GiteaUrl, ClientID: giteaNullable(v.ClientId), ClientSecret: secret, RedirectURI: giteaNullable(v.RedirectUri), GiteaInternalURL: internalPtr, OrganizationName: giteaNullable(v.OrganizationName)}, GiteaID: id, GitProviderID: v.GitProviderId, OrganizationID: p.OrganizationId}
	secretObserved := v.ClientSecret.IsSpecified() && !v.ClientSecret.IsNull() && giteaNullable(v.ClientSecret) != ""
	out = giteaObservation{State: state, OwnerUserID: p.UserId, ConfigurationComplete: state.ClientID != "" && secretObserved && state.RedirectURI != "", SecretObserved: secretObserved}
	return out, nil
}
