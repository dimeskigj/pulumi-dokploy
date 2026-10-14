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

type bitbucketCredentialEvidence struct {
	Specified bool
	Cleared   bool
	Value     string
}

type bitbucketObservation struct {
	State               BitbucketIntegrationState
	OwnerUserID         string
	APITokenEvidence    bitbucketCredentialEvidence
	AppPasswordEvidence bitbucketCredentialEvidence
}

func sanitizeBitbucketIntegrationError(err error) error {
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
		return fmt.Errorf("bitbucket integration HTTP %d", apiErr.StatusCode)
	}
	return errors.New("bitbucket integration request failed")
}

func bitbucketMalformed() error {
	return errors.New("bitbucket.one: incomplete or inconsistent response")
}

func bitbucketStringPtr(value string) *string { return &value }

func bitbucketStatus(operation string, status int) error {
	return fmt.Errorf("%s: HTTP %d", operation, status)
}

func bitbucketCredential(v nullable.Nullable[string], prior *string) (*string, bitbucketCredentialEvidence, error) {
	evidence := bitbucketCredentialEvidence{Specified: v.IsSpecified()}
	if v.IsNull() && v.IsSpecified() {
		evidence.Cleared = true
	}
	if !v.IsSpecified() || v.IsNull() {
		if prior == nil || *prior == "" {
			return nil, evidence, nil
		}
		return bitbucketStringPtr(*prior), evidence, nil
	}
	value, err := v.Get()
	if err != nil {
		return nil, evidence, err
	}
	evidence.Value = value
	if value == "" {
		evidence.Cleared = true
		return nil, evidence, nil
	}
	return bitbucketStringPtr(value), evidence, nil
}

func bitbucketOptional(v nullable.Nullable[string]) (*string, error) {
	if !v.IsSpecified() || v.IsNull() {
		return nil, nil
	}
	value, err := v.Get()
	if err != nil {
		return nil, err
	}
	if value == "" {
		return nil, nil
	}
	return bitbucketStringPtr(value), nil
}

func readBitbucketIntegration(ctx context.Context, api *client.Client, relationID string, prior BitbucketIntegrationState, organizationID string) (bitbucketObservation, error) {
	var out bitbucketObservation
	if relationID == "" || organizationID == "" {
		return out, bitbucketMalformed()
	}
	raw, err := api.BitbucketOne(ctx, &generated.BitbucketOneParams{BitbucketId: relationID})
	if err != nil {
		if client.IsNotFound(err) {
			return out, &client.APIError{StatusCode: http.StatusNotFound}
		}
		return out, sanitizeBitbucketIntegrationError(err)
	}
	if raw == nil {
		return out, bitbucketMalformed()
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
		return out, bitbucketStatus("bitbucket.one", raw.StatusCode)
	}
	response, err := generated.ParseBitbucketOneResponse(raw)
	if err != nil || response == nil || response.JSON200 == nil {
		return out, bitbucketMalformed()
	}
	v := response.JSON200
	parent := v.GitProvider
	if v.BitbucketId != relationID || v.GitProviderId == "" || parent.GitProviderId != v.GitProviderId || parent.Name == "" || parent.ProviderType != "bitbucket" || parent.OrganizationId == "" || parent.OrganizationId != organizationID || parent.UserId == "" ||
		(prior.BitbucketID != "" && prior.BitbucketID != relationID) || (prior.GitProviderID != "" && prior.GitProviderID != v.GitProviderId) || (prior.OrganizationID != "" && prior.OrganizationID != parent.OrganizationId) {
		return out, bitbucketMalformed()
	}
	apiToken, apiEvidence, err := bitbucketCredential(v.ApiToken, prior.APIToken)
	if err != nil {
		return out, bitbucketMalformed()
	}
	appPassword, appEvidence, err := bitbucketCredential(v.AppPassword, prior.AppPassword)
	if err != nil {
		return out, bitbucketMalformed()
	}
	email, err := bitbucketOptional(v.BitbucketEmail)
	if err != nil {
		return out, bitbucketMalformed()
	}
	username, err := bitbucketOptional(v.BitbucketUsername)
	if err != nil {
		return out, bitbucketMalformed()
	}
	workspace, err := bitbucketOptional(v.BitbucketWorkspaceName)
	if err != nil {
		return out, bitbucketMalformed()
	}
	state := BitbucketIntegrationState{
		BitbucketIntegrationArgs: BitbucketIntegrationArgs{Name: parent.Name, APIToken: apiToken, AppPassword: appPassword, BitbucketEmail: email, BitbucketUsername: username, BitbucketWorkspaceName: workspace},
		BitbucketID:              relationID, GitProviderID: v.GitProviderId, OrganizationID: parent.OrganizationId,
	}
	return bitbucketObservation{State: state, OwnerUserID: parent.UserId, APITokenEvidence: apiEvidence, AppPasswordEvidence: appEvidence}, nil
}

func bitbucketOptionalMatches(observed, desired *string) bool {
	if desired == nil {
		return true
	}
	actual := ""
	if observed != nil {
		actual = *observed
	}
	return actual == *desired
}

func bitbucketFreshCredentialMatches(evidence bitbucketCredentialEvidence, desired *string) bool {
	return desired != nil && *desired != "" && evidence.Specified && !evidence.Cleared && evidence.Value == *desired
}

func bitbucketIntegrationSettingsMatch(observed bitbucketObservation, desired BitbucketIntegrationArgs, expectedName string) bool {
	actual := observed.State
	if expectedName == "" || actual.Name != expectedName || desired.Name != expectedName || actual.BitbucketID == "" || actual.GitProviderID == "" || actual.OrganizationID == "" || observed.OwnerUserID == "" {
		return false
	}
	apiMode := desired.APIToken != nil && *desired.APIToken != ""
	appMode := desired.AppPassword != nil && *desired.AppPassword != ""
	if apiMode == appMode {
		return false
	}
	if apiMode {
		if !bitbucketFreshCredentialMatches(observed.APITokenEvidence, desired.APIToken) || !observed.AppPasswordEvidence.Specified || !observed.AppPasswordEvidence.Cleared || !bitbucketOptionalMatches(actual.BitbucketEmail, desired.BitbucketEmail) || actual.BitbucketEmail == nil || *actual.BitbucketEmail == "" {
			return false
		}
	} else if !bitbucketFreshCredentialMatches(observed.AppPasswordEvidence, desired.AppPassword) || !observed.APITokenEvidence.Specified || !observed.APITokenEvidence.Cleared || !bitbucketOptionalMatches(actual.BitbucketUsername, desired.BitbucketUsername) || actual.BitbucketUsername == nil || *actual.BitbucketUsername == "" {
		return false
	}
	return bitbucketOptionalMatches(actual.BitbucketEmail, desired.BitbucketEmail) && bitbucketOptionalMatches(actual.BitbucketUsername, desired.BitbucketUsername) && bitbucketOptionalMatches(actual.BitbucketWorkspaceName, desired.BitbucketWorkspaceName)
}

func bitbucketIntegrationMutationState(observed bitbucketObservation, desired BitbucketIntegrationArgs) BitbucketIntegrationState {
	state := observed.State
	if desired.APIToken != nil && *desired.APIToken != "" {
		if !bitbucketFreshCredentialMatches(observed.APITokenEvidence, desired.APIToken) {
			state.APIToken = nil
		}
	} else if observed.APITokenEvidence.Specified && observed.APITokenEvidence.Cleared {
		state.APIToken = nil
	}
	if desired.AppPassword != nil && *desired.AppPassword != "" {
		if !bitbucketFreshCredentialMatches(observed.AppPasswordEvidence, desired.AppPassword) {
			state.AppPassword = nil
		}
	} else if observed.AppPasswordEvidence.Specified && observed.AppPasswordEvidence.Cleared {
		state.AppPassword = nil
	}
	return state
}
