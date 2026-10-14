package dokploy

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/oapi-codegen/nullable"
)

// gitLabObservation contains only whitelisted, verified fields. Never retain raw API records here.
type gitLabObservation struct {
	State                 GitLabIntegrationState
	OwnerUserID           string
	ConfigurationComplete bool
}

func sanitizeGitLabIntegrationError(err error) error {
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
		return fmt.Errorf("GitLab integration HTTP %d", apiErr.StatusCode)
	}
	return errors.New("GitLab integration request failed")
}

func gitLabStatus(operation string, status int) error {
	return fmt.Errorf("%s: HTTP %d", operation, status)
}
func gitLabMalformed(operation string) error {
	return fmt.Errorf("%s: incomplete or inconsistent response", operation)
}

func gitLabCommon(fields map[string]interface{}) (name, org, user string, ok bool) {
	name, nameOK := fields["name"].(string)
	org, orgOK := fields["organizationId"].(string)
	user, userOK := fields["userId"].(string)
	providerType, typeOK := fields["type"].(string)
	return name, org, user, nameOK && orgOK && userOK && typeOK && name != "" && org != "" && user != "" && providerType == string(SourceGitLab)
}

func gitLabListCommon(fields map[string]interface{}) (org string, gitlab bool, ok bool) {
	name, nameOK := fields["name"].(string)
	org, orgOK := fields["organizationId"].(string)
	user, userOK := fields["userId"].(string)
	providerType, typeOK := fields["type"].(string)
	return org, providerType == string(SourceGitLab), nameOK && orgOK && userOK && typeOK && name != "" && org != "" && user != "" && providerType != ""
}

func gitLabNullable(v nullable.Nullable[string]) string {
	if v.IsNull() || !v.IsSpecified() {
		return ""
	}
	value, err := v.Get()
	if err != nil {
		return ""
	}
	return value
}

func readGitLabIntegration(ctx context.Context, api *client.Client, id string, prior GitLabIntegrationState, organizationID string) (gitLabObservation, error) {
	var out gitLabObservation
	if id == "" || organizationID == "" {
		return out, gitLabMalformed("gitlab.one")
	}
	raw, err := api.GitlabOne(ctx, &generated.GitlabOneParams{GitlabId: id})
	if err != nil {
		if client.IsNotFound(err) {
			return out, &client.APIError{StatusCode: http.StatusNotFound}
		}
		return out, sanitizeGitLabIntegrationError(err)
	}
	if raw == nil {
		return out, gitLabMalformed("gitlab.one")
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
		return out, gitLabStatus("gitlab.one", raw.StatusCode)
	}
	response, err := generated.ParseGitlabOneResponse(raw)
	if err != nil {
		return out, gitLabMalformed("gitlab.one")
	}
	if response == nil {
		return out, gitLabMalformed("gitlab.one")
	}
	v := response.JSON200
	if v == nil || v.GitlabId != id || v.GitProviderId == "" || v.GitProvider.GitProviderId != v.GitProviderId || v.GitlabUrl == "" || !v.AccessToken.IsSpecified() || !v.RefreshToken.IsSpecified() {
		return out, gitLabMalformed("gitlab.one")
	}
	name, org, user, ok := gitLabCommon(v.GitProvider.AdditionalProperties)
	if !ok || org != organizationID || (prior.GitLabID != "" && prior.GitLabID != id) || (prior.GitProviderID != "" && prior.GitProviderID != v.GitProviderId) || (prior.OrganizationID != "" && prior.OrganizationID != org) {
		return out, gitLabMalformed("gitlab.one")
	}
	state := GitLabIntegrationState{
		GitLabIntegrationArgs: GitLabIntegrationArgs{Name: name, ApplicationID: gitLabNullable(v.ApplicationId), ApplicationSecret: prior.ApplicationSecret, RedirectURI: gitLabNullable(v.RedirectUri), GitlabURL: v.GitlabUrl, GroupName: gitLabNullable(v.GroupName), GitlabInternalURL: nullableValue(v.GitlabInternalUrl)},
		GitLabID:              id, GitProviderID: v.GitProviderId, OrganizationID: org,
		IsConfigured: gitLabNullable(v.AccessToken) != "" && gitLabNullable(v.RefreshToken) != "",
	}
	if observed := gitLabNullable(v.Secret); observed != "" {
		state.ApplicationSecret = observed
	}
	out = gitLabObservation{State: state, OwnerUserID: user,
		ConfigurationComplete: state.ApplicationID != "" && gitLabNullable(v.Secret) != "" && state.RedirectURI != "" && v.GroupName.IsSpecified() && v.GitlabInternalUrl.IsSpecified()}
	return out, nil
}

func readGitLabProviderList(ctx context.Context, api *client.Client, organizationID string) ([]generated.GitProviderListEntry, error) {
	response, err := api.GitProviderGetAllWithResponse(ctx)
	if err != nil {
		return nil, sanitizeGitLabIntegrationError(err)
	}
	if response == nil || response.StatusCode() != http.StatusOK {
		if response == nil {
			return nil, gitLabMalformed("gitProvider.getAll")
		}
		return nil, gitLabStatus("gitProvider.getAll", response.StatusCode())
	}
	if response.JSON200 == nil || organizationID == "" {
		return nil, gitLabMalformed("gitProvider.getAll")
	}
	parents := map[string]bool{}
	relations := map[string]bool{}
	for _, entry := range *response.JSON200 {
		if entry.GitProviderId == "" || parents[entry.GitProviderId] {
			return nil, gitLabMalformed("gitProvider.getAll")
		}
		parents[entry.GitProviderId] = true
		org, gitlab, ok := gitLabListCommon(entry.AdditionalProperties)
		if !ok || org != organizationID {
			return nil, gitLabMalformed("gitProvider.getAll")
		}
		if !gitlab {
			if entry.Gitlab.IsSpecified() && !entry.Gitlab.IsNull() {
				return nil, gitLabMalformed("gitProvider.getAll")
			}
			continue
		}
		if !entry.Gitlab.IsSpecified() || entry.Gitlab.IsNull() {
			return nil, gitLabMalformed("gitProvider.getAll")
		}
		summary, err := entry.Gitlab.Get()
		if err != nil || summary.GitlabId == "" || summary.GitlabUrl == "" || !summary.ApplicationId.IsSpecified() || relations[summary.GitlabId] {
			return nil, gitLabMalformed("gitProvider.getAll")
		}
		relations[summary.GitlabId] = true
	}
	return *response.JSON200, nil
}
