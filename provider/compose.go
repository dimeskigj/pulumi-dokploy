package dokploy

import (
	"context"
	"fmt"
	"reflect"

	"github.com/gjorgjidimeski/pulumi-dokploy/internal/client"
	"github.com/gjorgjidimeski/pulumi-dokploy/internal/client/generated"
	"github.com/oapi-codegen/nullable"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

type ComposeArgs struct {
	Name                   string        `pulumi:"name"`
	AppName                *string       `pulumi:"appName,optional"`
	Description            *string       `pulumi:"description,optional"`
	EnvironmentID          string        `pulumi:"environmentId" provider:"replaceOnChanges"`
	ServerID               *string       `pulumi:"serverId,optional" provider:"replaceOnChanges"`
	ComposeType            ComposeType   `pulumi:"composeType,optional"`
	Source                 ComposeSource `pulumi:"source"`
	Environment            *string       `pulumi:"environment,optional" provider:"secret"`
	CreateEnvFile          bool          `pulumi:"createEnvFile,optional"`
	DeleteVolumesOnDestroy bool          `pulumi:"deleteVolumesOnDestroy,optional"`
}
type ComposeState struct {
	ComposeArgs
	ComposeID string `pulumi:"composeId"`
	Status    string `pulumi:"status"`
}

func (s *ComposeState) Annotate(a infer.Annotator) {
	a.Describe(&s.ComposeID, "The stable Dokploy Compose ID.")
	a.Describe(&s.Status, "The current Compose deployment status.")
}

type Compose struct{ client clientFactory }

func (r *Compose) Annotate(a infer.Annotator) {
	a.SetToken("index", "Compose")
	a.Describe(&r, "A Dokploy Compose stack.")
}
func (a *ComposeArgs) Annotate(annotator infer.Annotator) {
	annotator.Describe(&a.Name, "The Compose stack name.")
	annotator.Describe(&a.AppName, "The optional deployed stack name.")
	annotator.Describe(&a.Description, "An optional stack description.")
	annotator.Describe(&a.EnvironmentID, "The target environment ID.")
	annotator.Describe(&a.ServerID, "The optional server ID.")
	annotator.Describe(&a.ComposeType, "The Compose deployment type.")
	annotator.Describe(&a.Source, "The Compose source configuration.")
	annotator.Describe(&a.Environment, "Environment variables for the stack.")
	annotator.Describe(&a.CreateEnvFile, "Whether to create an environment file.")
	annotator.Describe(&a.DeleteVolumesOnDestroy, "Whether to delete volumes on destroy.")
	annotator.SetDefault(&a.ComposeType, string(ComposeDocker))
}
func (r Compose) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[ComposeArgs], error) {
	in, failures, err := infer.DefaultCheck[ComposeArgs](ctx, req.NewInputs)
	if err != nil || len(failures) != 0 {
		return infer.CheckResponse[ComposeArgs]{Inputs: in, Failures: failures}, err
	}
	if in.ComposeType == "" {
		in.ComposeType = ComposeDocker
	}
	if in.ComposeType != ComposeDocker && in.ComposeType != ComposeStack {
		failures = append(failures, p.CheckFailure{Property: "composeType", Reason: "composeType must be one of docker-compose or stack"})
	}
	if in.Source.Type == ComposeSourceGit && in.Source.Git != nil && in.Source.Git.ComposePath == "" {
		in.Source.Git.ComposePath = defaultComposePath
	}
	if in.Source.Type == ComposeSourceGitLab && in.Source.GitLab != nil && in.Source.GitLab.ComposePath == "" {
		in.Source.GitLab.ComposePath = defaultComposePath
	}
	if in.Name == "" {
		failures = append(failures, p.CheckFailure{Property: "name", Reason: "name must not be empty"})
	}
	if in.EnvironmentID == "" {
		failures = append(failures, p.CheckFailure{Property: "environmentId", Reason: "environmentId must not be empty"})
	}
	if err := in.Source.validate(); err != nil {
		failures = append(failures, p.CheckFailure{Property: "source", Reason: err.Error()})
	}
	return infer.CheckResponse[ComposeArgs]{Inputs: in, Failures: failures}, nil
}

func (r Compose) Diff(_ context.Context, req infer.DiffRequest[ComposeArgs, ComposeState]) (infer.DiffResponse, error) {
	d := map[string]p.PropertyDiff{}
	if req.Inputs.EnvironmentID != req.State.EnvironmentID {
		d["environmentId"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	if !sameOptionalString(req.Inputs.ServerID, req.State.ServerID) {
		d["serverId"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	if req.Inputs.Source.Type != req.State.Source.Type {
		d["source.type"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	if req.Inputs.ComposeType != req.State.ComposeType {
		d["composeType"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	if req.Inputs.Name != req.State.Name {
		d["name"] = p.PropertyDiff{Kind: p.Update}
	}
	if !sameOptionalString(req.Inputs.AppName, req.State.AppName) {
		d["appName"] = p.PropertyDiff{Kind: p.Update}
	}
	if !sameOptionalString(req.Inputs.Description, req.State.Description) {
		d["description"] = p.PropertyDiff{Kind: p.Update}
	}
	if !reflect.DeepEqual(req.Inputs.Source, req.State.Source) && req.Inputs.Source.Type == req.State.Source.Type {
		d["source"] = p.PropertyDiff{Kind: p.Update}
	}
	if !sameOptionalString(req.Inputs.Environment, req.State.Environment) {
		d["environment"] = p.PropertyDiff{Kind: p.Update}
	}
	if req.Inputs.CreateEnvFile != req.State.CreateEnvFile {
		d["createEnvFile"] = p.PropertyDiff{Kind: p.Update}
	}
	if req.Inputs.DeleteVolumesOnDestroy != req.State.DeleteVolumesOnDestroy {
		d["deleteVolumesOnDestroy"] = p.PropertyDiff{Kind: p.Update}
	}
	return infer.DiffResponse{HasChanges: len(d) > 0, DetailedDiff: d}, nil
}

func (r Compose) Create(ctx context.Context, req infer.CreateRequest[ComposeArgs]) (infer.CreateResponse[ComposeState], error) {
	state := ComposeState{ComposeArgs: req.Inputs}
	if req.DryRun {
		return infer.CreateResponse[ComposeState]{Output: state}, nil
	}
	b := generated.ComposeCreateJSONRequestBody{Name: req.Inputs.Name, EnvironmentId: req.Inputs.EnvironmentID, ComposeType: ptr(generated.ComposeCreateJSONBodyComposeType(req.Inputs.ComposeType))}
	if req.Inputs.ComposeType == "" {
		b.ComposeType = ptr(generated.ComposeCreateJSONBodyComposeType(ComposeDocker))
	}
	if req.Inputs.AppName != nil {
		b.AppName = req.Inputs.AppName
	}
	if req.Inputs.Description != nil {
		b.Description = nullable.NewNullableWithValue(*req.Inputs.Description)
	}
	if req.Inputs.ServerID != nil {
		b.ServerId = nullable.NewNullableWithValue(*req.Inputs.ServerID)
	}
	if req.Inputs.Source.Type == ComposeSourceRaw {
		b.ComposeFile = &req.Inputs.Source.Raw.ComposeFile
	}
	resp, err := r.client(ctx).ComposeCreateWithResponse(ctx, b)
	if err != nil {
		return infer.CreateResponse[ComposeState]{}, err
	}
	if resp.JSON200 == nil || resp.JSON200.ComposeId == nil {
		return infer.CreateResponse[ComposeState]{}, fmt.Errorf("compose.create returned incomplete compose")
	}
	state.ComposeID = *resp.JSON200.ComposeId
	fail := func(e error) (infer.CreateResponse[ComposeState], error) {
		_, ce := r.client(ctx).ComposeDeleteWithResponse(ctx, generated.ComposeDeleteJSONRequestBody{ComposeId: state.ComposeID, DeleteVolumes: req.Inputs.DeleteVolumesOnDestroy})
		if ce != nil {
			p.GetLogger(ctx).Warningf("compose cleanup failed for %s: %s", state.ComposeID, sanitizeComposeError(ce, req.Inputs))
		}
		return infer.CreateResponse[ComposeState]{ID: state.ComposeID, Output: state}, e
	}
	api := r.client(ctx)
	if req.Inputs.Source.Type != ComposeSourceRaw {
		if err := configureComposeSource(ctx, api, state.ComposeID, req.Inputs.Source); err != nil {
			return fail(sanitizeComposeError(err, req.Inputs))
		}
	}
	if err := fetchComposeSource(ctx, api, state.ComposeID, req.Inputs.Source.Type); err != nil {
		return fail(sanitizeComposeError(err, req.Inputs))
	}
	if err := configureComposeEnvironment(ctx, api, state.ComposeID, req.Inputs); err != nil {
		return fail(sanitizeComposeError(err, req.Inputs))
	}
	if _, err := api.ComposeDeployWithResponse(ctx, generated.ComposeDeployJSONRequestBody{ComposeId: state.ComposeID}); err != nil {
		return infer.CreateResponse[ComposeState]{ID: state.ComposeID, Output: state}, initFailed(sanitizeComposeError(err, req.Inputs))
	}
	if err := waitForDone(ctx, "compose", state.ComposeID, func(c context.Context) (string, error) { return composeStatus(c, api, state.ComposeID) }); err != nil {
		return infer.CreateResponse[ComposeState]{ID: state.ComposeID, Output: state}, initFailed(sanitizeComposeError(err, req.Inputs))
	}
	state.Status = "done"
	return infer.CreateResponse[ComposeState]{ID: state.ComposeID, Output: state}, nil
}

func configureComposeEnvironment(ctx context.Context, api *client.Client, id string, args ComposeArgs) error {
	env := nullable.NewNullNullable[string]()
	if args.Environment != nil {
		env = nullable.NewNullableWithValue(*args.Environment)
	}
	_, err := api.ComposeSaveEnvironmentWithResponse(ctx, generated.ComposeSaveEnvironmentJSONRequestBody{ComposeId: id, Env: env})
	return err
}
func sanitizeComposeError(err error, args ComposeArgs) error {
	if args.Environment == nil {
		return err
	}
	return sanitizeError(err, *args.Environment)
}
func composeStatus(ctx context.Context, api *client.Client, id string) (string, error) {
	r, e := api.ComposeOneWithResponse(ctx, &generated.ComposeOneParams{ComposeId: id})
	if e != nil {
		return "", e
	}
	if r.JSON200 == nil {
		return "", fmt.Errorf("compose.one returned incomplete compose")
	}
	return composeStatusValue(r.JSON200)
}

func composeStatusValue(c *generated.Compose) (string, error) {
	if c.AdditionalProperties == nil {
		return "", fmt.Errorf("compose.one returned compose without a status")
	}
	v, ok := c.AdditionalProperties["status"]
	if !ok {
		return "", fmt.Errorf("compose.one returned compose without a status")
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return "", fmt.Errorf("compose.one returned invalid status %v", v)
	}
	return s, nil
}

func (r Compose) Read(ctx context.Context, req infer.ReadRequest[ComposeArgs, ComposeState]) (infer.ReadResponse[ComposeArgs, ComposeState], error) {
	resp, e := r.client(ctx).ComposeOneWithResponse(ctx, &generated.ComposeOneParams{ComposeId: req.ID})
	if e != nil {
		if client.IsNotFound(e) {
			return infer.ReadResponse[ComposeArgs, ComposeState]{ID: ""}, nil
		}
		return infer.ReadResponse[ComposeArgs, ComposeState]{}, e
	}
	if resp.JSON200 == nil || resp.JSON200.ComposeId == nil {
		return infer.ReadResponse[ComposeArgs, ComposeState]{}, fmt.Errorf("compose.one returned incomplete compose")
	}
	c := resp.JSON200
	a := req.State.ComposeArgs
	a.Name = value(c.Name)
	a.AppName = c.AppName
	a.Description = c.Description
	a.EnvironmentID = value(c.EnvironmentId)
	a.ServerID = c.ServerId
	if c.ComposeType != nil {
		a.ComposeType = ComposeType(*c.ComposeType)
	}
	if c.CreateEnvFile != nil {
		a.CreateEnvFile = *c.CreateEnvFile
	}
	src, e := decodeComposeSource(c.Source, a.Source)
	if e != nil {
		return infer.ReadResponse[ComposeArgs, ComposeState]{}, e
	}
	a.Source = src
	status, e := composeStatusValue(c)
	if e != nil {
		return infer.ReadResponse[ComposeArgs, ComposeState]{}, e
	}
	st := ComposeState{ComposeArgs: a, ComposeID: *c.ComposeId, Status: status}
	return infer.ReadResponse[ComposeArgs, ComposeState]{ID: *c.ComposeId, Inputs: a, State: st}, nil
}

func decodeComposeSource(raw *map[string]interface{}, prior ComposeSource) (ComposeSource, error) {
	if raw == nil {
		return ComposeSource{}, fmt.Errorf("compose.one omitted source data required to reconstruct compose source")
	}
	m := *raw
	k := stringValue(m, "type", "sourceType")
	if k == "" {
		k = string(prior.Type)
	}
	s := ComposeSource{Type: ComposeSourceType(k)}
	switch s.Type {
	case ComposeSourceRaw:
		s.Raw = &RawComposeSource{ComposeFile: stringValue(m, "composeFile", "rawComposeFile")}
	case ComposeSourceGit:
		s.Git = &GitComposeSource{URL: stringValue(m, "url", "customGitUrl"), Branch: stringValue(m, "branch", "customGitBranch"), ComposePath: composePath(stringValue(m, "composePath")), SSHKeyID: stringPointer(m, "customGitSSHKeyId"), WatchPaths: stringSlice(m, "watchPaths"), EnableSubmodules: boolValue(m, "enableSubmodules")}
	case ComposeSourceGitLab:
		s.GitLab = &GitLabComposeSource{IntegrationID: stringValue(m, "integrationId", "gitlabId"), ProjectID: int(numberValue(m, "projectId", "gitlabProjectId")), Owner: stringValue(m, "owner", "gitlabOwner"), Namespace: stringValue(m, "namespace", "gitlabPathNamespace"), Repository: stringValue(m, "repository", "gitlabRepository"), Branch: stringValue(m, "branch", "gitlabBranch"), ComposePath: composePath(stringValue(m, "composePath")), WatchPaths: stringSlice(m, "watchPaths"), EnableSubmodules: boolValue(m, "enableSubmodules")}
	default:
		return ComposeSource{}, fmt.Errorf("compose source data has unsupported source.type %q", k)
	}
	return s, nil
}

func (r Compose) Update(ctx context.Context, req infer.UpdateRequest[ComposeArgs, ComposeState]) (infer.UpdateResponse[ComposeState], error) {
	st := ComposeState{ComposeArgs: req.Inputs, ComposeID: req.ID, Status: req.State.Status}
	if req.DryRun {
		return infer.UpdateResponse[ComposeState]{Output: st}, nil
	}
	api := r.client(ctx)
	meta := req.Inputs.Name != req.State.Name || !sameOptionalString(req.Inputs.AppName, req.State.AppName) || !sameOptionalString(req.Inputs.Description, req.State.Description)
	runtime := !reflect.DeepEqual(req.Inputs.Source, req.State.Source) || !sameOptionalString(req.Inputs.Environment, req.State.Environment) || req.Inputs.CreateEnvFile != req.State.CreateEnvFile
	if meta {
		b := generated.ComposeUpdateJSONRequestBody{ComposeId: req.ID, Name: &req.Inputs.Name, AppName: req.Inputs.AppName, Description: nullable.NewNullNullable[string]()}
		if req.Inputs.Description != nil {
			b.Description = nullable.NewNullableWithValue(*req.Inputs.Description)
		}
		if _, e := api.ComposeUpdateWithResponse(ctx, b); e != nil {
			return infer.UpdateResponse[ComposeState]{}, sanitizeComposeError(e, req.Inputs)
		}
	}
	if runtime {
		if e := configureComposeSource(ctx, api, req.ID, req.Inputs.Source); e != nil {
			return infer.UpdateResponse[ComposeState]{Output: st}, sanitizeComposeError(e, req.Inputs)
		}
		if e := fetchComposeSource(ctx, api, req.ID, req.Inputs.Source.Type); e != nil {
			return infer.UpdateResponse[ComposeState]{Output: st}, sanitizeComposeError(e, req.Inputs)
		}
		if e := configureComposeEnvironment(ctx, api, req.ID, req.Inputs); e != nil {
			return infer.UpdateResponse[ComposeState]{Output: st}, sanitizeComposeError(e, req.Inputs)
		}
		if _, e := api.ComposeRedeployWithResponse(ctx, generated.ComposeRedeployJSONRequestBody{ComposeId: req.ID}); e != nil {
			return infer.UpdateResponse[ComposeState]{Output: st}, sanitizeComposeError(e, req.Inputs)
		}
		if e := waitForDone(ctx, "compose", req.ID, func(c context.Context) (string, error) { return composeStatus(c, api, req.ID) }); e != nil {
			return infer.UpdateResponse[ComposeState]{Output: st}, sanitizeComposeError(e, req.Inputs)
		}
		st.Status = "done"
	}
	return infer.UpdateResponse[ComposeState]{Output: st}, nil
}
func (r Compose) Delete(ctx context.Context, req infer.DeleteRequest[ComposeState]) (infer.DeleteResponse, error) {
	_, e := r.client(ctx).ComposeDeleteWithResponse(ctx, generated.ComposeDeleteJSONRequestBody{ComposeId: req.ID, DeleteVolumes: req.State.DeleteVolumesOnDestroy})
	if client.IsNotFound(e) {
		return infer.DeleteResponse{}, nil
	}
	return infer.DeleteResponse{}, e
}
func (r Compose) WireDependencies(f infer.FieldSelector, args *ComposeArgs, state *ComposeState) {
	deps := []infer.InputField{f.InputField(&args.Name), f.InputField(&args.AppName), f.InputField(&args.Description), f.InputField(&args.EnvironmentID), f.InputField(&args.ServerID), f.InputField(&args.ComposeType), f.InputField(&args.Source), f.InputField(&args.CreateEnvFile), f.InputField(&args.DeleteVolumesOnDestroy)}
	f.OutputField(&state.ComposeID).DependsOn(deps...)
	f.OutputField(&state.Status).DependsOn(deps...)
	f.OutputField(&state.Environment).DependsOn(f.InputField(&args.Environment).Secret())
}
