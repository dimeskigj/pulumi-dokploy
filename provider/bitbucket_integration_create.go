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
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/pulumi/pulumi-go-provider/infer"
)

const bitbucketProviderType = "bitbucket"

type bitbucketCreateSnapshot struct {
	parentIDs   map[string]struct{}
	relationIDs map[string]struct{}
}

func readBitbucketProviderList(ctx context.Context, api *client.Client) ([]generated.GitProviderListEntry, error) {
	response, err := api.GitProviderGetAllWithResponse(ctx)
	if err != nil {
		return nil, sanitizeBitbucketIntegrationError(err)
	}
	if response == nil || response.StatusCode() != http.StatusOK || response.JSON200 == nil {
		if response != nil && response.StatusCode() != http.StatusOK {
			return nil, bitbucketStatus("gitProvider.getAll", response.StatusCode())
		}
		return nil, bitbucketMalformed()
	}
	return *response.JSON200, nil
}

func snapshotBitbucketProviders(entries []generated.GitProviderListEntry) (bitbucketCreateSnapshot, error) {
	snapshot := bitbucketCreateSnapshot{parentIDs: make(map[string]struct{}, len(entries)), relationIDs: make(map[string]struct{}, len(entries))}
	for _, entry := range entries {
		if entry.GitProviderId == "" {
			return bitbucketCreateSnapshot{}, bitbucketMalformed()
		}
		if _, ok := snapshot.parentIDs[entry.GitProviderId]; ok {
			return bitbucketCreateSnapshot{}, bitbucketMalformed()
		}
		snapshot.parentIDs[entry.GitProviderId] = struct{}{}
		providerType, hasProviderType := entry.AdditionalProperties["type"].(string)
		if hasProviderType && providerType == bitbucketProviderType && !entry.Bitbucket.IsSpecified() {
			return bitbucketCreateSnapshot{}, bitbucketMalformed()
		}
		if entry.Bitbucket.IsSpecified() {
			if entry.Bitbucket.IsNull() {
				return bitbucketCreateSnapshot{}, bitbucketMalformed()
			}
			if hasProviderType && providerType != bitbucketProviderType {
				return bitbucketCreateSnapshot{}, bitbucketMalformed()
			}
			summary, err := entry.Bitbucket.Get()
			if err != nil || summary.BitbucketId == "" {
				return bitbucketCreateSnapshot{}, bitbucketMalformed()
			}
			if _, ok := snapshot.relationIDs[summary.BitbucketId]; ok {
				return bitbucketCreateSnapshot{}, bitbucketMalformed()
			}
			snapshot.relationIDs[summary.BitbucketId] = struct{}{}
		}
	}
	return snapshot, nil
}

func bitbucketCreateParentFields(entry generated.GitProviderListEntry) (name, typ, organizationID, userID string, ok bool) {
	name, nOK := entry.AdditionalProperties["name"].(string)
	typ, tOK := entry.AdditionalProperties["type"].(string)
	organizationID, oOK := entry.AdditionalProperties["organizationId"].(string)
	userID, uOK := entry.AdditionalProperties["userId"].(string)
	return name, typ, organizationID, userID, nOK && tOK && oOK && uOK
}

func validateBitbucketCreateMarker(entries []generated.GitProviderListEntry, marker string) error {
	if marker == "" {
		return bitbucketMalformed()
	}
	for _, entry := range entries {
		if name, ok := entry.AdditionalProperties["name"].(string); ok && name == marker {
			return bitbucketMalformed()
		}
	}
	return nil
}

func discoverBitbucketIntegration(ctx context.Context, api *client.Client, args BitbucketIntegrationArgs, marker, organizationID, userID string, prior bitbucketCreateSnapshot) (bitbucketObservation, error) {
	if marker == "" || organizationID == "" || userID == "" {
		return bitbucketObservation{}, bitbucketMalformed()
	}
	for attempt := 0; attempt < 3; attempt++ {
		if err := ctx.Err(); err != nil {
			return bitbucketObservation{}, err
		}
		entries, err := readBitbucketProviderList(ctx, api)
		if err != nil {
			return bitbucketObservation{}, err
		}
		if _, err := snapshotBitbucketProviders(entries); err != nil {
			return bitbucketObservation{}, err
		}
		var candidate *generated.GitProviderListEntry
		for i := range entries {
			entry := &entries[i]
			name, typ, org, owner, complete := bitbucketCreateParentFields(*entry)
			if !complete {
				return bitbucketObservation{}, bitbucketMalformed()
			}
			if name != marker {
				continue
			}
			if entry.GitProviderId == "" || typ != bitbucketProviderType || org != organizationID || owner != userID || !entry.Bitbucket.IsSpecified() || entry.Bitbucket.IsNull() {
				return bitbucketObservation{}, bitbucketMalformed()
			}
			if _, exists := prior.parentIDs[entry.GitProviderId]; exists {
				return bitbucketObservation{}, bitbucketMalformed()
			}
			if candidate != nil {
				return bitbucketObservation{}, bitbucketMalformed()
			}
			candidate = entry
		}
		if candidate != nil {
			summary, err := candidate.Bitbucket.Get()
			if err != nil || summary.BitbucketId == "" {
				return bitbucketObservation{}, bitbucketMalformed()
			}
			if _, exists := prior.relationIDs[summary.BitbucketId]; exists {
				return bitbucketObservation{}, bitbucketMalformed()
			}
			if args.BitbucketUsername != nil && *args.BitbucketUsername != "" && (!summary.BitbucketUsername.IsSpecified() || summary.BitbucketUsername.IsNull()) {
				return bitbucketObservation{}, bitbucketMalformed()
			}
			if args.BitbucketUsername != nil && *args.BitbucketUsername != "" {
				username, e := summary.BitbucketUsername.Get()
				if e != nil || username != *args.BitbucketUsername {
					return bitbucketObservation{}, bitbucketMalformed()
				}
			}
			observed, err := readBitbucketIntegration(ctx, api, summary.BitbucketId, BitbucketIntegrationState{}, organizationID)
			if err != nil {
				return bitbucketObservation{}, err
			}
			if !summary.BitbucketUsername.IsSpecified() {
				return bitbucketObservation{}, bitbucketMalformed()
			}
			var summaryUsername *string
			if !summary.BitbucketUsername.IsNull() {
				value, getErr := summary.BitbucketUsername.Get()
				if getErr != nil {
					return bitbucketObservation{}, bitbucketMalformed()
				}
				if value != "" {
					summaryUsername = bitbucketStringPtr(value)
				}
			}
			temporaryDesired := args
			temporaryDesired.Name = marker
			if observed.State.BitbucketID != summary.BitbucketId || observed.State.GitProviderID != candidate.GitProviderId || observed.State.OrganizationID != organizationID || observed.OwnerUserID != userID ||
				!sameOptionalString(observed.State.BitbucketUsername, summaryUsername) || !bitbucketIntegrationSettingsMatch(observed, temporaryDesired, marker) || observed.State.Name != marker {
				return bitbucketObservation{}, bitbucketMalformed()
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
				return bitbucketObservation{}, ctx.Err()
			case <-timer.C:
			}
		}
	}
	return bitbucketObservation{}, errors.New("bitbucket integration creation found no verified candidate")
}

func bitbucketCreateError(err error) error {
	return fmt.Errorf("bitbucket integration creation failed: %w", sanitizeBitbucketIntegrationError(err))
}

func bitbucketCreatePartial(observed bitbucketObservation, err error) (infer.CreateResponse[BitbucketIntegrationState], error) {
	safe := bitbucketCreateError(err)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		safe = errors.Join(initFailed(safe), safe, err)
	} else {
		safe = initFailed(safe)
	}
	return infer.CreateResponse[BitbucketIntegrationState]{ID: observed.State.BitbucketID, Output: observed.State}, safe
}

func (r BitbucketIntegration) Create(ctx context.Context, req infer.CreateRequest[BitbucketIntegrationArgs]) (infer.CreateResponse[BitbucketIntegrationState], error) {
	if req.DryRun {
		return infer.CreateResponse[BitbucketIntegrationState]{Output: bitbucketIntegrationPreviewState(req.Inputs, "", BitbucketIntegrationState{})}, nil
	}
	if err := validateBitbucketIntegrationArgs(req.Inputs); err != nil {
		return infer.CreateResponse[BitbucketIntegrationState]{}, err
	}
	api := r.client(ctx)
	organizationID, err := bitbucketActiveOrganization(ctx, api)
	if err != nil {
		return infer.CreateResponse[BitbucketIntegrationState]{}, err
	}
	member, err := api.UserGetWithResponse(ctx)
	if err != nil {
		return infer.CreateResponse[BitbucketIntegrationState]{}, bitbucketCreateError(err)
	}
	if member == nil || member.StatusCode() != http.StatusOK || member.JSON200 == nil || member.JSON200.OrganizationId != organizationID || member.JSON200.UserId == "" || member.JSON200.User.Id != member.JSON200.UserId {
		return infer.CreateResponse[BitbucketIntegrationState]{}, bitbucketMalformed()
	}
	userID := member.JSON200.UserId
	entries, err := readBitbucketProviderList(ctx, api)
	if err != nil {
		return infer.CreateResponse[BitbucketIntegrationState]{}, err
	}
	prior, err := snapshotBitbucketProviders(entries)
	if err != nil {
		return infer.CreateResponse[BitbucketIntegrationState]{}, err
	}
	id, err := uuid.NewRandom()
	if err != nil {
		return infer.CreateResponse[BitbucketIntegrationState]{}, errors.New("bitbucket integration creation could not generate a unique marker")
	}
	marker := "pulumi-bitbucket-" + id.String()
	if err := validateBitbucketCreateMarker(entries, marker); err != nil {
		return infer.CreateResponse[BitbucketIntegrationState]{}, err
	}
	apiToken, appPassword := "", ""
	if req.Inputs.APIToken != nil {
		apiToken = *req.Inputs.APIToken
	}
	if req.Inputs.AppPassword != nil {
		appPassword = *req.Inputs.AppPassword
	}
	body := generated.BitbucketCreateJSONRequestBody{AuthId: userID, Name: marker, ApiToken: &apiToken, AppPassword: &appPassword}
	if req.Inputs.BitbucketEmail != nil {
		email := openapi_types.Email(*req.Inputs.BitbucketEmail)
		body.BitbucketEmail = &email
	}
	body.BitbucketUsername = req.Inputs.BitbucketUsername
	body.BitbucketWorkspaceName = req.Inputs.BitbucketWorkspaceName
	response, createErr := api.BitbucketCreate(ctx, body)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if createErr != nil {
		var apiErr *client.APIError
		if errors.As(createErr, &apiErr) && (apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden) {
			return infer.CreateResponse[BitbucketIntegrationState]{}, bitbucketCreateError(createErr)
		}
	}
	observed, err := discoverBitbucketIntegration(ctx, api, req.Inputs, marker, organizationID, userID, prior)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			if createErr != nil {
				err = errors.Join(createErr, err)
			}
			return bitbucketCreatePartial(bitbucketObservation{}, err)
		}
		if createErr != nil {
			return bitbucketCreatePartial(bitbucketObservation{}, createErr)
		}
		return bitbucketCreatePartial(bitbucketObservation{}, err)
	}
	if createErr != nil {
		return bitbucketCreatePartial(observed, createErr)
	}
	if err := ctx.Err(); err != nil {
		return bitbucketCreatePartial(observed, err)
	}
	apiTokenValue, appPasswordValue := "", ""
	if req.Inputs.APIToken != nil {
		apiTokenValue = *req.Inputs.APIToken
	}
	if req.Inputs.AppPassword != nil {
		appPasswordValue = *req.Inputs.AppPassword
	}
	update := generated.BitbucketUpdateJSONRequestBody{BitbucketId: observed.State.BitbucketID, GitProviderId: observed.State.GitProviderID, Name: req.Inputs.Name, ApiToken: &apiTokenValue, AppPassword: &appPasswordValue}
	if req.Inputs.BitbucketEmail != nil {
		email := openapi_types.Email(*req.Inputs.BitbucketEmail)
		update.BitbucketEmail = &email
	}
	update.BitbucketUsername = req.Inputs.BitbucketUsername
	update.BitbucketWorkspaceName = req.Inputs.BitbucketWorkspaceName
	response, updateErr := api.BitbucketUpdate(ctx, update)
	updateErr = bitbucketMutation(response, updateErr, "bitbucket.update")
	if ctx.Err() == nil {
		readback, readErr := readBitbucketIntegration(ctx, api, observed.State.BitbucketID, observed.State, organizationID)
		if readErr == nil && readback.State.GitProviderID == observed.State.GitProviderID && readback.State.BitbucketID == observed.State.BitbucketID && readback.State.OrganizationID == organizationID && readback.OwnerUserID == userID {
			observed = readback
		} else if readErr == nil {
			readErr = bitbucketMalformed()
		}
		if updateErr == nil && readErr != nil {
			return bitbucketCreatePartial(observed, readErr)
		}
	} else if updateErr == nil {
		updateErr = ctx.Err()
	}
	if ctx.Err() != nil {
		updateErr = errors.Join(updateErr, ctx.Err())
	}
	if updateErr != nil {
		return bitbucketCreatePartial(observed, updateErr)
	}
	if !bitbucketIntegrationSettingsMatch(observed, req.Inputs, req.Inputs.Name) {
		return bitbucketCreatePartial(observed, errors.New("bitbucket integration rename was not confirmed"))
	}
	return infer.CreateResponse[BitbucketIntegrationState]{ID: observed.State.BitbucketID, Output: observed.State}, nil
}
