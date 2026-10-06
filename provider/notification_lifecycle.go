package dokploy

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/google/uuid"
	"github.com/pulumi/pulumi-go-provider/infer"
)

const (
	notificationBadRequest   = "BAD_REQUEST"
	notificationNotFoundCode = "NOT_FOUND"
)

// Never forward upstream prose, operation names, URLs, decoder excerpts, or IDs.
func sanitizeNotificationError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	var api *client.APIError
	if errors.As(err, &api) {
		code := ""
		switch api.Code {
		case notificationNotFoundCode, notificationBadRequest, "UNAUTHORIZED", "FORBIDDEN", "INTERNAL_SERVER_ERROR", "TOO_MANY_REQUESTS":
			code = api.Code
		}
		return &client.APIError{StatusCode: api.StatusCode, Code: code}
	}
	return errors.New("notification request failed; response details withheld")
}

func notificationFailure(operation string, err error) error {
	return fmt.Errorf("notification.%s failed: %w", operation, sanitizeNotificationError(err))
}

func notificationIdentityOf(s NotificationState, id string) notificationIdentity {
	return notificationIdentity{NotificationID: id, NotificationType: s.NotificationType, ChannelID: s.ChannelID, OrganizationID: s.OrganizationID}
}

func notificationList(ctx context.Context, api *client.Client) ([]generated.Notification, map[string]struct{}, error) {
	r, err := api.NotificationAllWithResponse(ctx)
	if err != nil {
		return nil, nil, err
	}
	if r == nil || r.JSON200 == nil {
		return nil, nil, errNotificationObservation
	}
	ids, err := notificationListIDs(*r.JSON200)
	return *r.JSON200, ids, err
}

func notificationSameSettings(observed NotificationState, desired NotificationArgs) bool {
	want := normalizeNotificationArgs(desired)
	kind, err := notificationKind(want)
	if err != nil || observed.NotificationType != kind || observed.Name != want.Name || *observed.Events != *want.Events {
		return false
	}
	return reflect.DeepEqual(notificationBlock(observed.NotificationArgs, kind), notificationBlock(want, kind))
}

func notificationValid(a NotificationArgs) bool { return len(validateNotificationArgs(a)) == 0 }

func notificationNotFound(err error) bool {
	var api *client.APIError
	return errors.As(err, &api) && (api.StatusCode == 404 || api.Code == notificationNotFoundCode)
}

func notificationDefiniteRejection(err error) bool {
	var api *client.APIError
	return errors.As(err, &api) && api.StatusCode >= 400 && api.StatusCode < 500 && api.StatusCode != 429
}

func (r Notification) Create(ctx context.Context, req infer.CreateRequest[NotificationArgs]) (infer.CreateResponse[NotificationState], error) {
	if req.DryRun {
		return infer.CreateResponse[NotificationState]{Output: NotificationState{NotificationArgs: req.Inputs}}, nil
	}
	var empty infer.CreateResponse[NotificationState]
	if !notificationValid(req.Inputs) {
		return empty, errors.New("notification.create invalid inputs")
	}
	api := r.client(ctx)
	org, err := notificationActiveOrganizationID(ctx, api)
	if err != nil {
		return empty, notificationFailure("create organization", err)
	}
	beforeList, before, err := notificationList(ctx, api)
	if err != nil {
		return empty, notificationFailure("create snapshot", err)
	}
	marker := "pulumi-notification-" + uuid.NewString()
	for _, v := range beforeList {
		if v.Name != nil && *v.Name == marker {
			return empty, errors.New("notification.create marker collision; no mutation attempted")
		}
	}
	createErr := createNotificationChannel(ctx, api, marker, normalizeNotificationArgs(req.Inputs))
	if notificationDefiniteRejection(createErr) {
		return empty, notificationFailure("create", createErr)
	}
	// A transport failure is uncertain; discovery is the only safe next step.
	var candidate NotificationState
	found := false
	for attempt := 0; attempt < 3; attempt++ {
		if err := ctx.Err(); err != nil {
			return empty, notificationFailure("create discovery; creation may have occurred; inspect disabled records", err)
		}
		list, _, listErr := notificationList(ctx, api)
		if listErr != nil {
			return empty, notificationFailure("create discovery; creation may have occurred; inspect disabled records", listErr)
		}
		for i := range list {
			state, match, candidateErr := notificationCandidate(&list[i], marker, org, req.Inputs, before)
			if candidateErr != nil {
				return empty, notificationFailure("create discovery; creation may have occurred; inspect disabled records", candidateErr)
			}
			if match {
				if found {
					return empty, errors.New("notification.create discovery ambiguous; creation may have occurred; inspect disabled records")
				}
				candidate, found = state, true
			}
		}
		if found {
			break
		}
	}
	if !found {
		return empty, errors.New("notification.create not discoverable; creation may have occurred; inspect disabled records")
	}
	partial := func(e error) (infer.CreateResponse[NotificationState], error) {
		return infer.CreateResponse[NotificationState]{ID: candidate.NotificationID, Output: candidate}, initFailed(e)
	}
	id := notificationIdentityOf(candidate, candidate.NotificationID)
	verified, err := readNotification(ctx, api, id, &candidate)
	if err != nil {
		return partial(notificationFailure("create verification; refresh before repair", err))
	}
	if verified.Name != marker || *verified.Events != (NotificationEvents{}) || !reflect.DeepEqual(notificationBlock(verified.NotificationArgs, id.NotificationType), notificationBlock(candidate.NotificationArgs, id.NotificationType)) {
		return partial(errors.New("notification.create verification mismatch; refresh before repair"))
	}
	candidate = verified
	if createErr != nil {
		return partial(notificationFailure("create uncertain; refresh before repair", createErr))
	}
	if err := updateNotificationChannel(ctx, api, id.NotificationID, id.ChannelID, normalizeNotificationArgs(req.Inputs)); err != nil {
		return partial(notificationFailure("create finalization; refresh before repair", err))
	}
	final, err := readNotification(ctx, api, id, &candidate)
	if err != nil {
		return partial(notificationFailure("create final read; refresh before repair", err))
	}
	if !notificationSameSettings(final, req.Inputs) {
		return partial(errors.New("notification.create final settings mismatch; refresh before repair"))
	}
	return infer.CreateResponse[NotificationState]{ID: id.NotificationID, Output: final}, nil
}

func (r Notification) Read(ctx context.Context, req infer.ReadRequest[NotificationArgs, NotificationState]) (infer.ReadResponse[NotificationArgs, NotificationState], error) {
	var empty infer.ReadResponse[NotificationArgs, NotificationState]
	if req.ID == "" || req.State.NotificationID != "" && (req.State.NotificationID != req.ID || req.State.OrganizationID == "" || req.State.NotificationType == "" || req.State.ChannelID == "") {
		return empty, errors.New("notification.read invalid identity")
	}
	api := r.client(ctx)
	org, err := notificationActiveOrganizationID(ctx, api)
	if err != nil {
		return empty, notificationFailure("read organization", err)
	}
	if req.State.OrganizationID != "" && req.State.OrganizationID != org {
		return empty, errors.New("notification.read organization changed")
	}
	id := notificationIdentityOf(req.State, req.ID)
	id.OrganizationID = org
	state, err := readNotification(ctx, api, id, &req.State)
	if notificationNotFound(err) {
		return empty, nil
	}
	if err != nil {
		return empty, notificationFailure("read", err)
	}
	return infer.ReadResponse[NotificationArgs, NotificationState]{ID: req.ID, Inputs: state.NotificationArgs, State: state}, nil
}

func (r Notification) Update(ctx context.Context, req infer.UpdateRequest[NotificationArgs, NotificationState]) (infer.UpdateResponse[NotificationState], error) {
	if req.DryRun {
		s := req.State
		s.NotificationArgs = req.Inputs
		return infer.UpdateResponse[NotificationState]{Output: s}, nil
	}
	previous := infer.UpdateResponse[NotificationState]{Output: req.State}
	if !notificationValid(req.Inputs) {
		return previous, errors.New("notification.update invalid inputs")
	}
	if req.ID == "" || req.State.NotificationID != req.ID || req.State.OrganizationID == "" || req.State.ChannelID == "" || req.State.NotificationType == "" {
		return previous, errors.New("notification.update invalid identity")
	}
	api := r.client(ctx)
	org, err := notificationActiveOrganizationID(ctx, api)
	if err != nil {
		return previous, notificationFailure("update organization", err)
	}
	if org != req.State.OrganizationID {
		return previous, errors.New("notification.update organization changed")
	}
	id := notificationIdentityOf(req.State, req.ID)
	verified, err := readNotification(ctx, api, id, &req.State)
	if err != nil {
		return previous, notificationFailure("update verification", err)
	}
	previous.Output = verified
	kind, _ := notificationKind(req.Inputs)
	if kind != verified.NotificationType {
		return previous, errors.New("notification.update channel replacement required")
	}
	if (kind == notificationGotify || kind == notificationNtfy) && verified.Events.ServerThreshold {
		return previous, errors.New("notification.update cannot clear unsupported serverThreshold")
	}
	if err := updateNotificationChannel(ctx, api, req.ID, verified.ChannelID, normalizeNotificationArgs(req.Inputs)); err != nil {
		return previous, notificationFailure("update", err)
	}
	final, err := readNotification(ctx, api, id, &verified)
	if err != nil {
		return previous, notificationFailure("update read", err)
	}
	if !notificationSameSettings(final, req.Inputs) {
		return previous, errors.New("notification.update observed settings mismatch")
	}
	return infer.UpdateResponse[NotificationState]{Output: final}, nil
}

func (r Notification) Delete(ctx context.Context, req infer.DeleteRequest[NotificationState]) (infer.DeleteResponse, error) {
	var result infer.DeleteResponse
	if req.ID == "" || req.State.NotificationID != req.ID || req.State.OrganizationID == "" || req.State.ChannelID == "" || req.State.NotificationType == "" {
		return result, errors.New("notification.delete invalid identity")
	}
	api := r.client(ctx)
	org, err := notificationActiveOrganizationID(ctx, api)
	if err != nil {
		return result, notificationFailure("delete organization", err)
	}
	if org != req.State.OrganizationID {
		return result, errors.New("notification.delete organization changed")
	}
	id := notificationIdentityOf(req.State, req.ID)
	_, err = readNotification(ctx, api, id, &req.State)
	if notificationNotFound(err) {
		return result, nil
	}
	if err != nil {
		return result, notificationFailure("delete verification", err)
	}
	resp, removeErr := api.NotificationRemove(ctx, generated.NotificationRemoveJSONRequestBody{NotificationId: req.ID})
	removeErr = notificationMutationResult(resp, removeErr)
	_, err = readNotification(ctx, api, id, &req.State)
	if notificationNotFound(err) {
		return result, nil
	}
	if removeErr != nil {
		return result, notificationFailure("delete", removeErr)
	}
	if err != nil {
		return result, notificationFailure("delete verification", err)
	}
	return result, errors.New("notification.delete still present after acknowledgment")
}
