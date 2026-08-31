package dokploy

import (
	"context"
	"fmt"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/oapi-codegen/nullable"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

type DestinationArgs struct {
	Name            string   `pulumi:"name"`
	Provider        *string  `pulumi:"provider,optional"`
	AccessKey       string   `pulumi:"accessKey"`
	SecretAccessKey string   `pulumi:"secretAccessKey" provider:"secret"`
	Bucket          string   `pulumi:"bucket"`
	Region          string   `pulumi:"region"`
	Endpoint        string   `pulumi:"endpoint"`
	AdditionalFlags []string `pulumi:"additionalFlags,optional"`
	ServerID        *string  `pulumi:"serverId,optional"`
}

type DestinationState struct {
	DestinationArgs
	DestinationID string `pulumi:"destinationId"`
}

func (s *DestinationState) Annotate(a infer.Annotator) {
	a.Describe(&s.DestinationID, "The stable Dokploy destination ID.")
}

type Destination struct{ client clientFactory }

func (r *Destination) Annotate(a infer.Annotator) {
	a.SetToken("index", "Destination")
	a.Describe(&r, "A Dokploy S3-compatible backup storage destination.")
}
func (a *DestinationArgs) Annotate(annotator infer.Annotator) {
	annotator.Describe(&a.Name, "The destination name.")
	annotator.Describe(&a.Provider, "The storage provider.")
	annotator.Describe(&a.AccessKey, "The storage access key ID.")
	annotator.Describe(&a.SecretAccessKey, "The storage secret access key.")
	annotator.Describe(&a.Bucket, "The storage bucket name.")
	annotator.Describe(&a.Region, "The storage region.")
	annotator.Describe(&a.Endpoint, "The storage endpoint URL.")
	annotator.Describe(&a.AdditionalFlags, "Additional rclone flags for backup and restore operations.")
	annotator.Describe(&a.ServerID, "The optional server ID this destination is scoped to.")
	annotator.SetDefault(&a.Provider, "s3")
}

func (r Destination) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[DestinationArgs], error) {
	in, failures, err := infer.DefaultCheck[DestinationArgs](ctx, req.NewInputs)
	if err != nil || len(failures) != 0 {
		return infer.CheckResponse[DestinationArgs]{Inputs: in, Failures: failures}, err
	}
	if in.Provider == nil || *in.Provider == "" {
		in.Provider = ptr("s3")
	}
	for _, field := range []struct {
		name, value string
	}{
		{"name", in.Name}, {"accessKey", in.AccessKey}, {"secretAccessKey", in.SecretAccessKey},
		{"bucket", in.Bucket}, {"region", in.Region}, {"endpoint", in.Endpoint},
	} {
		if field.value == "" && !req.NewInputs.Get(field.name).HasComputed() {
			failures = append(failures, p.CheckFailure{Property: field.name, Reason: fmt.Sprintf("%s must not be empty", field.name)})
		}
	}
	return infer.CheckResponse[DestinationArgs]{Inputs: in, Failures: failures}, nil
}

func (r Destination) Diff(_ context.Context, req infer.DiffRequest[DestinationArgs, DestinationState]) (infer.DiffResponse, error) {
	in, old := req.Inputs, req.State.DestinationArgs
	d := map[string]p.PropertyDiff{}
	for _, field := range []struct {
		name    string
		changed bool
	}{
		{"name", in.Name != old.Name}, {"provider", !sameOptionalString(in.Provider, old.Provider)},
		{"accessKey", in.AccessKey != old.AccessKey}, {"secretAccessKey", in.SecretAccessKey != old.SecretAccessKey},
		{"bucket", in.Bucket != old.Bucket}, {"region", in.Region != old.Region}, {"endpoint", in.Endpoint != old.Endpoint},
		{"additionalFlags", !sameStringSlice(in.AdditionalFlags, old.AdditionalFlags)},
		{"serverId", !sameOptionalString(in.ServerID, old.ServerID)},
	} {
		if field.changed {
			d[field.name] = p.PropertyDiff{Kind: p.Update}
		}
	}
	return infer.DiffResponse{HasChanges: len(d) > 0, DetailedDiff: d}, nil
}

func sameStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func destinationBody(a DestinationArgs) (provider nullable.Nullable[string], additionalFlags nullable.Nullable[[]string]) {
	provider = nullable.NewNullNullable[string]()
	if a.Provider != nil {
		provider = nullable.NewNullableWithValue(*a.Provider)
	}
	additionalFlags = nullable.NewNullableWithValue(a.AdditionalFlags)
	return provider, additionalFlags
}

func (r Destination) Create(ctx context.Context, req infer.CreateRequest[DestinationArgs]) (infer.CreateResponse[DestinationState], error) {
	state := DestinationState{DestinationArgs: req.Inputs}
	if req.DryRun {
		return infer.CreateResponse[DestinationState]{Output: state}, nil
	}
	api := r.client(ctx)
	providerValue, additionalFlags := destinationBody(req.Inputs)
	b := generated.DestinationCreateJSONRequestBody{
		Name: req.Inputs.Name, Provider: providerValue, AccessKey: req.Inputs.AccessKey, SecretAccessKey: req.Inputs.SecretAccessKey,
		Bucket: req.Inputs.Bucket, Region: req.Inputs.Region, Endpoint: req.Inputs.Endpoint, AdditionalFlags: additionalFlags,
	}
	if req.Inputs.ServerID != nil {
		b.ServerId = req.Inputs.ServerID
	}
	resp, err := api.DestinationCreateWithResponse(ctx, b)
	if err != nil {
		return infer.CreateResponse[DestinationState]{}, sanitizeDestinationError(err, req.Inputs)
	}
	if resp.JSON200 == nil || resp.JSON200.DestinationId == nil {
		return infer.CreateResponse[DestinationState]{}, fmt.Errorf("destination.create returned incomplete destination")
	}
	state.DestinationID = *resp.JSON200.DestinationId
	return infer.CreateResponse[DestinationState]{ID: state.DestinationID, Output: state}, nil
}

func sanitizeDestinationError(err error, args DestinationArgs, prior ...DestinationArgs) error {
	secrets := []string{args.SecretAccessKey}
	for _, old := range prior {
		secrets = append(secrets, old.SecretAccessKey)
	}
	return sanitizeError(err, secrets...)
}

func (r Destination) Read(ctx context.Context, req infer.ReadRequest[DestinationArgs, DestinationState]) (infer.ReadResponse[DestinationArgs, DestinationState], error) {
	resp, err := r.client(ctx).DestinationOneWithResponse(ctx, &generated.DestinationOneParams{DestinationId: req.ID})
	if err != nil {
		if client.IsNotFound(err) {
			return infer.ReadResponse[DestinationArgs, DestinationState]{ID: ""}, nil
		}
		return infer.ReadResponse[DestinationArgs, DestinationState]{}, err
	}
	if resp.JSON200 == nil || resp.JSON200.DestinationId == nil {
		return infer.ReadResponse[DestinationArgs, DestinationState]{}, fmt.Errorf("destination.one returned incomplete destination")
	}
	v := resp.JSON200
	a := req.State.DestinationArgs
	a.Name, a.AccessKey, a.Bucket, a.Region, a.Endpoint = value(v.Name), value(v.AccessKey), value(v.Bucket), value(v.Region), value(v.Endpoint)
	a.Provider, a.ServerID = v.Provider, v.ServerId
	if v.AdditionalFlags != nil {
		a.AdditionalFlags = *v.AdditionalFlags
	}
	if v.SecretAccessKey != nil && *v.SecretAccessKey != "" {
		a.SecretAccessKey = *v.SecretAccessKey
	}
	state := DestinationState{DestinationArgs: a, DestinationID: *v.DestinationId}
	return infer.ReadResponse[DestinationArgs, DestinationState]{ID: *v.DestinationId, Inputs: a, State: state}, nil
}

func (r Destination) Update(ctx context.Context, req infer.UpdateRequest[DestinationArgs, DestinationState]) (infer.UpdateResponse[DestinationState], error) {
	state := DestinationState{DestinationArgs: req.Inputs, DestinationID: req.ID}
	if req.DryRun {
		return infer.UpdateResponse[DestinationState]{Output: state}, nil
	}
	providerValue, additionalFlags := destinationBody(req.Inputs)
	b := generated.DestinationUpdateJSONRequestBody{
		DestinationId: req.ID, Name: req.Inputs.Name, Provider: providerValue, AccessKey: req.Inputs.AccessKey,
		SecretAccessKey: req.Inputs.SecretAccessKey, Bucket: req.Inputs.Bucket, Region: req.Inputs.Region,
		Endpoint: req.Inputs.Endpoint, AdditionalFlags: additionalFlags,
	}
	if req.Inputs.ServerID != nil {
		b.ServerId = req.Inputs.ServerID
	}
	if _, err := r.client(ctx).DestinationUpdateWithResponse(ctx, b); err != nil {
		return infer.UpdateResponse[DestinationState]{Output: state}, sanitizeDestinationError(err, req.Inputs, req.State.DestinationArgs)
	}
	return infer.UpdateResponse[DestinationState]{Output: state}, nil
}

func (r Destination) Delete(ctx context.Context, req infer.DeleteRequest[DestinationState]) (infer.DeleteResponse, error) {
	_, err := r.client(ctx).DestinationRemoveWithResponse(ctx, generated.DestinationRemoveJSONRequestBody{DestinationId: req.ID})
	if client.IsNotFound(err) {
		return infer.DeleteResponse{}, nil
	}
	return infer.DeleteResponse{}, err
}

func (r Destination) WireDependencies(f infer.FieldSelector, args *DestinationArgs, state *DestinationState) {
	deps := []infer.InputField{
		f.InputField(&args.Name), f.InputField(&args.Provider), f.InputField(&args.AccessKey), f.InputField(&args.Bucket),
		f.InputField(&args.Region), f.InputField(&args.Endpoint), f.InputField(&args.AdditionalFlags), f.InputField(&args.ServerID),
	}
	f.OutputField(&state.DestinationID).DependsOn(deps...)
	f.OutputField(&state.SecretAccessKey).DependsOn(f.InputField(&args.SecretAccessKey).Secret())
}
