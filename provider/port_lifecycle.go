package dokploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/pulumi/pulumi-go-provider/infer"
)

const portCreateBodyLimit = 1 << 20

func sanitizePortError(err error) error {
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
		return &client.APIError{StatusCode: apiErr.StatusCode}
	}
	return errors.New("port request or response failed; details withheld")
}

func validPortID(id string) bool { return strings.TrimSpace(id) != "" }

func readPort(ctx context.Context, api *client.Client, id string) (PortState, error) {
	if !validPortID(id) {
		return PortState{}, errors.New("invalid port identity")
	}
	resp, err := api.PortOneWithResponse(ctx, &generated.PortOneParams{PortId: id})
	if err != nil {
		return PortState{}, err
	}
	if resp.JSON200 == nil {
		return PortState{}, errors.New("port.one returned an incomplete port")
	}
	v := resp.JSON200
	if v.PortId != id || v.ApplicationId == nil || !validPortID(*v.ApplicationId) || v.PublishedPort == nil || v.TargetPort == nil || v.Protocol == nil || v.PublishMode == nil {
		return PortState{}, errors.New("port.one returned an invalid port")
	}
	// The generated optional fields treat JSON null and omission identically.
	// Both are invalid for canonical port settings.
	a := PortArgs{ApplicationID: *v.ApplicationId, PublishedPort: *v.PublishedPort, TargetPort: *v.TargetPort, Protocol: ptr(PortProtocol(*v.Protocol)), PublishMode: ptr(PortPublishMode(*v.PublishMode))}
	if err := validatePortArgs(a); err != nil {
		return PortState{}, errors.New("port.one returned invalid settings")
	}
	return PortState{PortArgs: a, PortID: id}, nil
}

func createPort(ctx context.Context, api *client.Client, args PortArgs) (string, error) {
	resp, err := api.PortCreate(ctx, generated.PortCreateJSONRequestBody{ApplicationId: args.ApplicationID, PublishedPort: float32(args.PublishedPort), TargetPort: float32(args.TargetPort), Protocol: generated.PortCreateJSONBodyProtocol(portProtocol(args.Protocol)), PublishMode: generated.PortCreateJSONBodyPublishMode(portMode(args.PublishMode))})
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, portCreateBodyLimit+1))
	if err != nil || len(data) > portCreateBodyLimit {
		return "", errors.New("port.create returned an invalid acknowledgment")
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil {
		return "", errors.New("port.create returned an invalid acknowledgment")
	}
	var id string
	if json.Unmarshal(fields["portId"], &id) != nil || !validPortID(id) {
		return "", errors.New("port.create returned no valid identity")
	}
	return id, nil
}

func (r Port) Create(ctx context.Context, req infer.CreateRequest[PortArgs]) (infer.CreateResponse[PortState], error) {
	state := PortState{PortArgs: portDefaults(req.Inputs)}
	if req.DryRun {
		return infer.CreateResponse[PortState]{Output: state}, nil
	}
	if err := validatePortArgs(req.Inputs); err != nil {
		return infer.CreateResponse[PortState]{}, fmt.Errorf("port.create invalid inputs: %w", sanitizePortError(err))
	}
	api := r.client(ctx)
	id, err := createPort(ctx, api, state.PortArgs)
	if err != nil {
		return infer.CreateResponse[PortState]{}, fmt.Errorf("port.create could not confirm identity; inspect Dokploy before retrying: %w", sanitizePortError(err))
	}
	state.PortID = id
	observed, err := readPort(ctx, api, id)
	if err == nil && observed.ApplicationID != req.Inputs.ApplicationID {
		err = errors.New("port.one returned a different owner")
	}
	if err != nil {
		return infer.CreateResponse[PortState]{ID: id, Output: state}, initFailed(fmt.Errorf("port.create acknowledged but read-back failed: %w", sanitizePortError(err)))
	}
	return infer.CreateResponse[PortState]{ID: id, Output: observed}, nil
}

func (r Port) Read(ctx context.Context, req infer.ReadRequest[PortArgs, PortState]) (infer.ReadResponse[PortArgs, PortState], error) {
	if !validPortID(req.ID) {
		return infer.ReadResponse[PortArgs, PortState]{}, errors.New("port.one invalid identity")
	}
	state, err := readPort(ctx, r.client(ctx), req.ID)
	if client.IsNotFound(err) {
		return infer.ReadResponse[PortArgs, PortState]{}, nil
	}
	if err != nil {
		return infer.ReadResponse[PortArgs, PortState]{}, fmt.Errorf("port.one failed: %w", sanitizePortError(err))
	}
	return infer.ReadResponse[PortArgs, PortState]{ID: req.ID, Inputs: state.PortArgs, State: state}, nil
}

func (r Port) Update(ctx context.Context, req infer.UpdateRequest[PortArgs, PortState]) (infer.UpdateResponse[PortState], error) {
	prior := infer.UpdateResponse[PortState]{Output: req.State}
	if !validPortID(req.ID) || req.Inputs.ApplicationID != req.State.ApplicationID {
		return prior, errors.New("port.update cannot change identity or owner")
	}
	if req.DryRun {
		return infer.UpdateResponse[PortState]{Output: PortState{PortArgs: portDefaults(req.Inputs), PortID: req.ID}}, nil
	}
	if err := validatePortArgs(req.Inputs); err != nil {
		return prior, fmt.Errorf("port.update invalid inputs: %w", sanitizePortError(err))
	}
	api := r.client(ctx)
	resp, err := api.PortUpdate(ctx, generated.PortUpdateJSONRequestBody{PortId: req.ID, PublishedPort: float32(req.Inputs.PublishedPort), TargetPort: float32(req.Inputs.TargetPort), Protocol: generated.PortUpdateJSONBodyProtocol(portProtocol(req.Inputs.Protocol)), PublishMode: generated.PortUpdateJSONBodyPublishMode(portMode(req.Inputs.PublishMode))})
	if err != nil {
		return prior, fmt.Errorf("port.update failed: %w", sanitizePortError(err))
	}
	_ = resp.Body.Close()
	attempted := infer.UpdateResponse[PortState]{Output: PortState{PortArgs: portDefaults(req.Inputs), PortID: req.ID}}
	observed, err := readPort(ctx, api, req.ID)
	if err == nil && observed.ApplicationID != req.Inputs.ApplicationID {
		err = errors.New("port.one returned a different owner")
	}
	if err != nil {
		return attempted, fmt.Errorf("port.update acknowledged but read-back failed: %w", sanitizePortError(err))
	}
	return infer.UpdateResponse[PortState]{Output: observed}, nil
}

func (r Port) Delete(ctx context.Context, req infer.DeleteRequest[PortState]) (infer.DeleteResponse, error) {
	if !validPortID(req.ID) {
		return infer.DeleteResponse{}, errors.New("port.delete invalid identity")
	}
	resp, err := r.client(ctx).PortDelete(ctx, generated.PortDeleteJSONRequestBody{PortId: req.ID})
	if client.IsNotFound(err) {
		return infer.DeleteResponse{}, nil
	}
	if err != nil {
		return infer.DeleteResponse{}, fmt.Errorf("port.delete failed: %w", sanitizePortError(err))
	}
	_ = resp.Body.Close()
	return infer.DeleteResponse{}, nil
}
