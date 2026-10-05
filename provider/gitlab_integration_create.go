package dokploy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/google/uuid"
	"github.com/oapi-codegen/nullable"
	"github.com/pulumi/pulumi-go-provider/infer"
)

func gitLabCreateFailure(err error) error {
	return fmt.Errorf("GitLab integration creation could not verify ownership: %w", sanitizeGitLabIntegrationError(err))
}

func gitLabCreatePartial(observed gitLabObservation, err error) (infer.CreateResponse[GitLabIntegrationState], error) {
	return infer.CreateResponse[GitLabIntegrationState]{ID: observed.State.GitLabID, Output: observed.State}, initFailed(gitLabCreateFailure(err))
}

func gitLabCreateSettingsMatch(observed gitLabObservation, args GitLabIntegrationArgs, name string) bool {
	state := observed.State
	return observed.ConfigurationComplete && state.Name == name && state.ApplicationID == args.ApplicationID &&
		state.ApplicationSecret == args.ApplicationSecret && state.RedirectURI == args.RedirectURI &&
		state.GitlabURL == args.GitlabURL && state.GroupName == args.GroupName &&
		sameOptionalString(state.GitlabInternalURL, args.GitlabInternalURL)
}

func gitLabCreateParentMatches(entry generated.GitProviderListEntry, name, organizationID, userID string) bool {
	fields := entry.AdditionalProperties
	return fields["name"] == name && fields["type"] == "gitlab" &&
		fields["organizationId"] == organizationID && fields["userId"] == userID
}

// discoverGitLabIntegration only accepts a new marker-matched parent with a
// matching relation and fully observed OAuth settings. It never adopts by name
// alone, list order, or a partially observable configuration.
func discoverGitLabIntegration(ctx context.Context, api *client.Client, args GitLabIntegrationArgs, marker, organizationID, userID string, priorIDs map[string]struct{}) (gitLabObservation, error) {
	if marker == "" || organizationID == "" || userID == "" {
		return gitLabObservation{}, errors.New("GitLab creation discovery has incomplete identity")
	}
	for attempt := 0; attempt < 3; attempt++ {
		if err := ctx.Err(); err != nil {
			return gitLabObservation{}, err
		}
		entries, err := readGitLabProviderList(ctx, api, organizationID)
		if err != nil {
			return gitLabObservation{}, gitLabCreateFailure(err)
		}
		var candidate *generated.GitProviderListEntry
		for i := range entries {
			entry := &entries[i]
			if entry.AdditionalProperties["name"] != marker {
				continue
			}
			if !gitLabCreateParentMatches(*entry, marker, organizationID, userID) || entry.GitProviderId == "" || !entry.Gitlab.IsSpecified() || entry.Gitlab.IsNull() {
				return gitLabObservation{}, errors.New("GitLab creation discovery found an inconsistent marker")
			}
			if _, exists := priorIDs[entry.GitProviderId]; exists || candidate != nil {
				return gitLabObservation{}, errors.New("GitLab creation discovery found an ambiguous marker")
			}
			candidate = entry
		}
		if candidate != nil {
			summary := candidate.Gitlab.MustGet()
			if summary.GitlabId == "" || summary.GitlabUrl != args.GitlabURL ||
				!summary.ApplicationId.IsSpecified() || summary.ApplicationId.IsNull() || summary.ApplicationId.MustGet() != args.ApplicationID {
				return gitLabObservation{}, errors.New("GitLab creation discovery found incomplete configuration")
			}
			observed, err := readGitLabIntegration(ctx, api, summary.GitlabId, GitLabIntegrationState{}, organizationID)
			if err != nil {
				return gitLabObservation{}, gitLabCreateFailure(err)
			}
			if observed.State.GitLabID != summary.GitlabId || observed.State.GitProviderID != candidate.GitProviderId ||
				observed.State.OrganizationID != organizationID || observed.OwnerUserID != userID ||
				!gitLabCreateSettingsMatch(observed, args, marker) || observed.State.IsConfigured != summary.IsConfigured {
				return gitLabObservation{}, errors.New("GitLab creation discovery found inconsistent detail")
			}
			return observed, nil
		}
		if attempt < 2 {
			timer := time.NewTimer(100 * time.Millisecond)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return gitLabObservation{}, ctx.Err()
			case <-timer.C:
			}
		}
	}
	return gitLabObservation{}, errors.New("GitLab creation discovery found no verified candidate")
}

func (r GitLabIntegration) Create(ctx context.Context, req infer.CreateRequest[GitLabIntegrationArgs]) (infer.CreateResponse[GitLabIntegrationState], error) {
	if req.DryRun {
		return infer.CreateResponse[GitLabIntegrationState]{Output: GitLabIntegrationState{GitLabIntegrationArgs: req.Inputs}}, nil
	}
	if err := validateGitLabIntegrationArgs(req.Inputs); err != nil {
		return infer.CreateResponse[GitLabIntegrationState]{}, err
	}
	api := r.client(ctx)
	org, err := api.OrganizationActiveWithResponse(ctx)
	if err != nil {
		return infer.CreateResponse[GitLabIntegrationState]{}, gitLabCreateFailure(err)
	}
	if org == nil || org.JSON200 == nil || activeOrganizationID(*org.JSON200) == "" {
		return infer.CreateResponse[GitLabIntegrationState]{}, errors.New("GitLab creation requires an active organization")
	}
	organizationID := activeOrganizationID(*org.JSON200)
	member, err := api.UserGetWithResponse(ctx)
	if err != nil {
		return infer.CreateResponse[GitLabIntegrationState]{}, gitLabCreateFailure(err)
	}
	if member == nil || member.JSON200 == nil || member.JSON200.OrganizationId != organizationID ||
		member.JSON200.UserId == "" || member.JSON200.User.Id != member.JSON200.UserId {
		return infer.CreateResponse[GitLabIntegrationState]{}, errors.New("GitLab creation requires a consistent authenticated member")
	}
	userID := member.JSON200.UserId
	before, err := readGitLabProviderList(ctx, api, organizationID)
	if err != nil {
		return infer.CreateResponse[GitLabIntegrationState]{}, gitLabCreateFailure(err)
	}
	priorIDs := make(map[string]struct{}, len(before))
	for _, entry := range before {
		if entry.GitProviderId == "" {
			return infer.CreateResponse[GitLabIntegrationState]{}, errors.New("GitLab creation snapshot has incomplete identity")
		}
		if _, exists := priorIDs[entry.GitProviderId]; exists {
			return infer.CreateResponse[GitLabIntegrationState]{}, errors.New("GitLab creation snapshot has duplicate identity")
		}
		priorIDs[entry.GitProviderId] = struct{}{}
	}
	id, err := uuid.NewRandom()
	if err != nil {
		return infer.CreateResponse[GitLabIntegrationState]{}, errors.New("GitLab creation could not generate a unique marker")
	}
	marker := "pulumi-gitlab-" + id.String()
	for _, entry := range before {
		if entry.AdditionalProperties["name"] == marker {
			return infer.CreateResponse[GitLabIntegrationState]{}, errors.New("GitLab creation marker already exists")
		}
	}
	group, secret, appID, redirect := req.Inputs.GroupName, req.Inputs.ApplicationSecret, req.Inputs.ApplicationID, req.Inputs.RedirectURI
	internal := nullable.NewNullNullable[string]()
	if req.Inputs.GitlabInternalURL != nil {
		internal = nullable.NewNullableWithValue(*req.Inputs.GitlabInternalURL)
	}
	response, createErr := api.GitlabCreate(ctx, generated.GitlabCreateJSONRequestBody{
		AuthId: userID, Name: marker, GitlabUrl: req.Inputs.GitlabURL,
		ApplicationId: &appID, Secret: &secret, RedirectUri: &redirect, GroupName: &group, GitlabInternalUrl: internal,
	})
	if response != nil && response.Body != nil {
		_ = response.Body.Close() // acknowledgment does not depend on success-body decoding or close errors
	}
	if createErr != nil {
		var apiErr *client.APIError
		if errors.As(createErr, &apiErr) && (apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden) {
			return infer.CreateResponse[GitLabIntegrationState]{}, gitLabCreateFailure(createErr)
		}
	}
	observed, err := discoverGitLabIntegration(ctx, api, req.Inputs, marker, organizationID, userID, priorIDs)
	if err != nil {
		if createErr != nil {
			return infer.CreateResponse[GitLabIntegrationState]{}, gitLabCreateFailure(createErr)
		}
		return infer.CreateResponse[GitLabIntegrationState]{}, gitLabCreateFailure(err)
	}
	if createErr != nil {
		return gitLabCreatePartial(observed, createErr)
	}
	updateResponse, updateErr := api.GitlabUpdate(ctx, generated.GitlabUpdateJSONRequestBody{
		GitlabId: observed.State.GitLabID, GitProviderId: observed.State.GitProviderID,
		Name: req.Inputs.Name, GitlabUrl: req.Inputs.GitlabURL,
		ApplicationId: &appID, Secret: &secret, RedirectUri: &redirect, GroupName: &group, GitlabInternalUrl: internal,
	})
	if updateResponse != nil && updateResponse.Body != nil {
		_ = updateResponse.Body.Close()
	}
	// Even when rename fails, read back if possible: the parent and relation
	// updates may have committed independently. Retain only verified actual state.
	if ctx.Err() == nil {
		readback, readErr := readGitLabIntegration(ctx, api, observed.State.GitLabID, observed.State, organizationID)
		if readErr == nil && readback.State.GitProviderID == observed.State.GitProviderID && readback.State.GitLabID == observed.State.GitLabID &&
			readback.State.OrganizationID == organizationID && readback.OwnerUserID == userID {
			observed = readback
		} else if readErr == nil {
			readErr = errors.New("GitLab creation readback changed identity")
		}
		if updateErr == nil && readErr != nil {
			return gitLabCreatePartial(observed, readErr)
		}
	} else if updateErr == nil {
		return gitLabCreatePartial(observed, ctx.Err())
	}
	if updateErr != nil {
		return gitLabCreatePartial(observed, updateErr)
	}
	if !gitLabCreateSettingsMatch(observed, req.Inputs, req.Inputs.Name) {
		return gitLabCreatePartial(observed, errors.New("GitLab creation rename was not confirmed"))
	}
	return infer.CreateResponse[GitLabIntegrationState]{ID: observed.State.GitLabID, Output: observed.State}, nil
}
