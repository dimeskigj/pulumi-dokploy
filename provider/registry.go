package dokploy

import (
	"context"
	"errors"
	"fmt"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/oapi-codegen/nullable"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

type RegistryArgs struct {
	Name        string  `pulumi:"name"`
	Username    string  `pulumi:"username"`
	Password    string  `pulumi:"password" provider:"secret"`
	URL         string  `pulumi:"url"`
	ImagePrefix *string `pulumi:"imagePrefix,optional"`
	ServerID    *string `pulumi:"serverId,optional"`
}

type RegistryState struct {
	RegistryArgs
	RegistryID string `pulumi:"registryId"`
}

func (s *RegistryState) Annotate(a infer.Annotator) {
	a.Describe(&s.RegistryID, "The stable Dokploy registry ID.")
}

type Registry struct{ client clientFactory }

func (r *Registry) Annotate(a infer.Annotator) {
	a.SetToken("index", "Registry")
	a.Describe(&r, "A Dokploy container registry.")
}
func (a *RegistryArgs) Annotate(n infer.Annotator) {
	n.Describe(&a.Name, "The registry name.")
	n.Describe(&a.Username, "The registry username.")
	n.Describe(&a.Password, "The registry password.")
	n.Describe(&a.URL, "The registry URL.")
	n.Describe(&a.ImagePrefix, "The optional image prefix.")
	n.Describe(&a.ServerID, "The optional server ID.")
}

func (r Registry) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[RegistryArgs], error) {
	in, failures, err := infer.DefaultCheck[RegistryArgs](ctx, req.NewInputs)
	if err != nil || len(failures) != 0 {
		return infer.CheckResponse[RegistryArgs]{Inputs: in, Failures: failures}, err
	}
	for _, field := range []struct{ name, value string }{{"name", in.Name}, {"username", in.Username}, {"password", in.Password}, {"url", in.URL}} {
		if field.value == "" && !req.NewInputs.Get(field.name).HasComputed() {
			failures = append(failures, p.CheckFailure{Property: field.name, Reason: fmt.Sprintf("%s must not be empty", field.name)})
		}
	}
	return infer.CheckResponse[RegistryArgs]{Inputs: in, Failures: failures}, nil
}

func (r Registry) Diff(_ context.Context, req infer.DiffRequest[RegistryArgs, RegistryState]) (infer.DiffResponse, error) {
	in, old := req.Inputs, req.State.RegistryArgs
	d := map[string]p.PropertyDiff{}
	for _, f := range []struct {
		name    string
		changed bool
	}{{"name", in.Name != old.Name}, {"username", in.Username != old.Username}, {"password", in.Password != old.Password}, {"url", in.URL != old.URL}, {"imagePrefix", !sameOptionalString(in.ImagePrefix, old.ImagePrefix)}, {"serverId", !sameOptionalString(in.ServerID, old.ServerID)}} {
		if f.changed {
			d[f.name] = p.PropertyDiff{Kind: p.Update}
		}
	}
	return infer.DiffResponse{HasChanges: len(d) > 0, DetailedDiff: d}, nil
}

func registryNullable(v *string) nullable.Nullable[string] {
	if v == nil {
		return nullable.NewNullNullable[string]()
	}
	return nullable.NewNullableWithValue(*v)
}
func (r Registry) testRegistry(ctx context.Context, api *client.Client, a RegistryArgs) error {
	b := generated.RegistryTestRegistryJSONRequestBody{Password: a.Password, RegistryType: generated.RegistryTestRegistryJSONBodyRegistryTypeCloud, RegistryUrl: a.URL, Username: a.Username, ImagePrefix: registryNullable(a.ImagePrefix)}
	if a.ServerID != nil {
		b.ServerId = a.ServerID
	}
	_, err := api.RegistryTestRegistryWithResponse(ctx, b)
	return err
}

func (r Registry) Create(ctx context.Context, req infer.CreateRequest[RegistryArgs]) (infer.CreateResponse[RegistryState], error) {
	state := RegistryState{RegistryArgs: req.Inputs}
	if req.DryRun {
		return infer.CreateResponse[RegistryState]{Output: state}, nil
	}
	api := r.client(ctx)
	if err := r.testRegistry(ctx, api, req.Inputs); err != nil {
		return infer.CreateResponse[RegistryState]{}, sanitizeRegistryError(err, req.Inputs)
	}
	resp, err := api.RegistryCreateWithResponse(ctx, generated.RegistryCreateJSONRequestBody{ImagePrefix: registryNullable(req.Inputs.ImagePrefix), Password: req.Inputs.Password, RegistryName: req.Inputs.Name, RegistryType: generated.RegistryCreateJSONBodyRegistryTypeCloud, RegistryUrl: req.Inputs.URL, ServerId: req.Inputs.ServerID, Username: req.Inputs.Username})
	if err != nil {
		return infer.CreateResponse[RegistryState]{}, sanitizeRegistryError(err, req.Inputs)
	}
	if resp.JSON200 == nil || resp.JSON200.RegistryId == "" {
		return infer.CreateResponse[RegistryState]{}, errors.New("registry.create returned incomplete registry")
	}
	state.RegistryID = resp.JSON200.RegistryId
	read, err := r.Read(ctx, infer.ReadRequest[RegistryArgs, RegistryState]{ID: state.RegistryID, State: state})
	if err != nil {
		return infer.CreateResponse[RegistryState]{ID: state.RegistryID, Output: state}, initFailed(sanitizeRegistryError(err, req.Inputs))
	}
	if read.ID == "" {
		return infer.CreateResponse[RegistryState]{ID: state.RegistryID, Output: state}, initFailed(errors.New("registry.one returned not found after create"))
	}
	return infer.CreateResponse[RegistryState]{ID: state.RegistryID, Output: read.State}, nil
}

func sanitizeRegistryError(err error, args RegistryArgs, prior ...RegistryArgs) error {
	secrets := []string{args.Password}
	for _, old := range prior {
		secrets = append(secrets, old.Password)
	}
	return sanitizeError(err, secrets...)
}

func (r Registry) Read(ctx context.Context, req infer.ReadRequest[RegistryArgs, RegistryState]) (infer.ReadResponse[RegistryArgs, RegistryState], error) {
	resp, err := r.client(ctx).RegistryOneWithResponse(ctx, &generated.RegistryOneParams{RegistryId: req.ID})
	if err != nil {
		if client.IsNotFound(err) {
			return infer.ReadResponse[RegistryArgs, RegistryState]{ID: ""}, nil
		}
		return infer.ReadResponse[RegistryArgs, RegistryState]{}, sanitizeRegistryError(err, req.State.RegistryArgs)
	}
	if resp.JSON200 == nil || resp.JSON200.RegistryId == "" {
		return infer.ReadResponse[RegistryArgs, RegistryState]{}, errors.New("registry.one returned incomplete registry")
	}
	v := resp.JSON200
	a := req.State.RegistryArgs
	a.Name, a.URL = value(v.RegistryName), value(v.RegistryUrl)
	if v.Username != nil {
		a.Username = *v.Username
	}
	if v.Password != nil && *v.Password != "" {
		a.Password = *v.Password
	}
	a.ImagePrefix, a.ServerID = nullableValue(v.ImagePrefix), nullableValue(v.ServerId)
	state := RegistryState{RegistryArgs: a, RegistryID: v.RegistryId}
	return infer.ReadResponse[RegistryArgs, RegistryState]{ID: v.RegistryId, Inputs: a, State: state}, nil
}

func (r Registry) Update(ctx context.Context, req infer.UpdateRequest[RegistryArgs, RegistryState]) (infer.UpdateResponse[RegistryState], error) {
	state := RegistryState{RegistryArgs: req.Inputs, RegistryID: req.ID}
	if req.DryRun {
		return infer.UpdateResponse[RegistryState]{Output: state}, nil
	}
	old := req.State.RegistryArgs
	if req.Inputs.Username != old.Username || req.Inputs.Password != old.Password || req.Inputs.URL != old.URL || !sameOptionalString(req.Inputs.ImagePrefix, old.ImagePrefix) || !sameOptionalString(req.Inputs.ServerID, old.ServerID) {
		if err := r.testRegistry(ctx, r.client(ctx), req.Inputs); err != nil {
			return infer.UpdateResponse[RegistryState]{Output: state}, sanitizeRegistryError(err, req.Inputs, old)
		}
	}
	_, err := r.client(ctx).RegistryUpdateWithResponse(ctx, generated.RegistryUpdateJSONRequestBody{ImagePrefix: registryNullable(req.Inputs.ImagePrefix), Password: &req.Inputs.Password, RegistryId: req.ID, RegistryName: &req.Inputs.Name, RegistryType: ptr("cloud"), RegistryUrl: &req.Inputs.URL, ServerId: registryNullable(req.Inputs.ServerID), Username: &req.Inputs.Username})
	if err != nil {
		return infer.UpdateResponse[RegistryState]{Output: state}, sanitizeRegistryError(err, req.Inputs, old)
	}
	return infer.UpdateResponse[RegistryState]{Output: state}, nil
}

func (r Registry) Delete(ctx context.Context, req infer.DeleteRequest[RegistryState]) (infer.DeleteResponse, error) {
	_, err := r.client(ctx).RegistryRemoveWithResponse(ctx, generated.RegistryRemoveJSONRequestBody{RegistryId: req.ID})
	if client.IsNotFound(err) {
		return infer.DeleteResponse{}, nil
	}
	return infer.DeleteResponse{}, sanitizeRegistryError(err, req.State.RegistryArgs)
}
func (r Registry) WireDependencies(f infer.FieldSelector, args *RegistryArgs, state *RegistryState) {
	f.OutputField(&state.RegistryID).DependsOn(f.InputField(&args.Name), f.InputField(&args.Username), f.InputField(&args.Password).Secret(), f.InputField(&args.URL), f.InputField(&args.ImagePrefix), f.InputField(&args.ServerID))
	f.OutputField(&state.Password).DependsOn(f.InputField(&args.Password).Secret())
}
