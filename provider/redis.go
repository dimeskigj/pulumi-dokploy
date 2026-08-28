package dokploy

import (
	"context"
	"fmt"

	"github.com/gjorgjidimeski/pulumi-dokploy/internal/client"
	"github.com/gjorgjidimeski/pulumi-dokploy/internal/client/generated"
	"github.com/oapi-codegen/nullable"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

type RedisArgs struct {
	Name             string  `pulumi:"name"`
	AppName          *string `pulumi:"appName,optional"`
	Description      *string `pulumi:"description,optional"`
	EnvironmentID    string  `pulumi:"environmentId" provider:"replaceOnChanges"`
	ServerID         *string `pulumi:"serverId,optional" provider:"replaceOnChanges"`
	DatabasePassword string  `pulumi:"databasePassword" provider:"secret"`
	DockerImage      string  `pulumi:"dockerImage,optional"`
	Environment      *string `pulumi:"environment,optional" provider:"secret"`
	ExternalPort     *int    `pulumi:"externalPort,optional"`
}

type RedisState struct {
	RedisArgs
	RedisID string `pulumi:"redisId"`
	Status  string `pulumi:"status"`
}

type Redis struct{ client clientFactory }

func (r *Redis) Annotate(a infer.Annotator) {
	a.SetToken("index", "Redis")
	a.Describe(&r, "A Dokploy Redis database.")
}
func (a *RedisArgs) Annotate(annotator infer.Annotator) {
	annotator.Describe(&a.Name, "The database resource name.")
	annotator.Describe(&a.AppName, "The optional deployed database name.")
	annotator.Describe(&a.Description, "An optional database description.")
	annotator.Describe(&a.EnvironmentID, "The target environment ID.")
	annotator.Describe(&a.ServerID, "The optional server ID.")
	annotator.Describe(&a.DatabasePassword, "The Redis database password.")
	annotator.Describe(&a.DockerImage, "The Redis Docker image.")
	annotator.Describe(&a.Environment, "Environment variables for Redis.")
	annotator.Describe(&a.ExternalPort, "The optional externally exposed port.")
	annotator.SetDefault(&a.DockerImage, "redis:8")
}

func (r Redis) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[RedisArgs], error) {
	in, failures, err := infer.DefaultCheck[RedisArgs](ctx, req.NewInputs)
	if err != nil || len(failures) != 0 {
		return infer.CheckResponse[RedisArgs]{Inputs: in, Failures: failures}, err
	}
	if in.DockerImage == "" {
		in.DockerImage = "redis:8"
	}
	if in.Name == "" {
		failures = append(failures, p.CheckFailure{Property: "name", Reason: "name must not be empty"})
	}
	if in.EnvironmentID == "" {
		failures = append(failures, p.CheckFailure{Property: "environmentId", Reason: "environmentId must not be empty"})
	}
	return infer.CheckResponse[RedisArgs]{Inputs: in, Failures: failures}, nil
}

func (r Redis) Diff(_ context.Context, req infer.DiffRequest[RedisArgs, RedisState]) (infer.DiffResponse, error) {
	d := map[string]p.PropertyDiff{}
	if req.Inputs.EnvironmentID != req.State.EnvironmentID {
		d["environmentId"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	if !sameOptionalString(req.Inputs.ServerID, req.State.ServerID) {
		d["serverId"] = p.PropertyDiff{Kind: p.UpdateReplace}
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
	if req.Inputs.DatabasePassword != req.State.DatabasePassword {
		d["databasePassword"] = p.PropertyDiff{Kind: p.Update}
	}
	if req.Inputs.DockerImage != req.State.DockerImage {
		d["dockerImage"] = p.PropertyDiff{Kind: p.Update}
	}
	if !sameOptionalString(req.Inputs.Environment, req.State.Environment) {
		d["environment"] = p.PropertyDiff{Kind: p.Update}
	}
	if !sameOptionalInt(req.Inputs.ExternalPort, req.State.ExternalPort) {
		d["externalPort"] = p.PropertyDiff{Kind: p.Update}
	}
	return infer.DiffResponse{HasChanges: len(d) > 0, DetailedDiff: d}, nil
}

func (r Redis) Create(ctx context.Context, req infer.CreateRequest[RedisArgs]) (infer.CreateResponse[RedisState], error) {
	state := RedisState{RedisArgs: req.Inputs}
	if req.DryRun {
		return infer.CreateResponse[RedisState]{Output: state}, nil
	}
	api := r.client(ctx)
	b := generated.RedisCreateJSONRequestBody{Name: req.Inputs.Name, EnvironmentId: req.Inputs.EnvironmentID, DatabasePassword: req.Inputs.DatabasePassword, DockerImage: &req.Inputs.DockerImage, Description: nullable.NewNullNullable[string](), ServerId: nullable.NewNullNullable[string]()}
	if req.Inputs.AppName != nil {
		b.AppName = req.Inputs.AppName
	}
	if req.Inputs.Description != nil {
		b.Description = nullable.NewNullableWithValue(*req.Inputs.Description)
	}
	if req.Inputs.ServerID != nil {
		b.ServerId = nullable.NewNullableWithValue(*req.Inputs.ServerID)
	}
	resp, err := api.RedisCreateWithResponse(ctx, b)
	if err != nil {
		return infer.CreateResponse[RedisState]{}, sanitizeRedisError(err, req.Inputs)
	}
	if resp.JSON200 == nil || resp.JSON200.RedisId == nil {
		return infer.CreateResponse[RedisState]{}, fmt.Errorf("redis.create returned incomplete redis")
	}
	state.RedisID = *resp.JSON200.RedisId
	failSetup := func(e error) (infer.CreateResponse[RedisState], error) {
		if _, ce := api.RedisRemoveWithResponse(ctx, generated.RedisRemoveJSONRequestBody{RedisId: state.RedisID}); ce != nil {
			p.GetLogger(ctx).Warningf("redis cleanup failed for %s: %s", state.RedisID, sanitizeRedisError(ce, req.Inputs))
		}
		return infer.CreateResponse[RedisState]{ID: state.RedisID, Output: state}, e
	}
	if req.Inputs.Environment != nil {
		if err := saveRedisEnvironment(ctx, api, state.RedisID, req.Inputs.Environment); err != nil {
			return failSetup(sanitizeRedisError(err, req.Inputs))
		}
	}
	if req.Inputs.ExternalPort != nil {
		if err := saveRedisPort(ctx, api, state.RedisID, req.Inputs.ExternalPort); err != nil {
			return failSetup(sanitizeRedisError(err, req.Inputs))
		}
	}
	if _, err := api.RedisDeployWithResponse(ctx, generated.RedisDeployJSONRequestBody{RedisId: state.RedisID}); err != nil {
		return infer.CreateResponse[RedisState]{ID: state.RedisID, Output: state}, initFailed(sanitizeRedisError(err, req.Inputs))
	}
	if err := waitForDone(ctx, "redis", state.RedisID, func(c context.Context) (string, error) { return redisStatus(c, api, state.RedisID) }); err != nil {
		return infer.CreateResponse[RedisState]{ID: state.RedisID, Output: state}, initFailed(sanitizeRedisError(err, req.Inputs))
	}
	state.Status = "done"
	return infer.CreateResponse[RedisState]{ID: state.RedisID, Output: state}, nil
}

func saveRedisEnvironment(ctx context.Context, api *client.Client, id string, env *string) error {
	e := nullable.NewNullNullable[string]()
	if env != nil {
		e = nullable.NewNullableWithValue(*env)
	}
	_, err := api.RedisSaveEnvironmentWithResponse(ctx, generated.RedisSaveEnvironmentJSONRequestBody{RedisId: id, Env: e})
	return err
}
func saveRedisPort(ctx context.Context, api *client.Client, id string, port *int) error {
	p := nullable.NewNullNullable[float32]()
	if port != nil {
		p = nullable.NewNullableWithValue(float32(*port))
	}
	_, err := api.RedisSaveExternalPortWithResponse(ctx, generated.RedisSaveExternalPortJSONRequestBody{RedisId: id, ExternalPort: p})
	return err
}
func sanitizeRedisError(err error, args RedisArgs, prior ...RedisArgs) error {
	secrets := []string{args.DatabasePassword}
	if args.Environment != nil {
		secrets = append(secrets, *args.Environment)
	}
	for _, old := range prior {
		secrets = append(secrets, old.DatabasePassword)
		if old.Environment != nil {
			secrets = append(secrets, *old.Environment)
		}
	}
	return sanitizeError(err, secrets...)
}
func redisStatus(ctx context.Context, api *client.Client, id string) (string, error) {
	r, err := api.RedisOneWithResponse(ctx, &generated.RedisOneParams{RedisId: id})
	if err != nil {
		return "", err
	}
	if r.JSON200 == nil {
		return "", fmt.Errorf("redis.one returned incomplete redis")
	}
	return redisStatusValue(r.JSON200)
}
func redisStatusValue(v *generated.Redis) (string, error) {
	if v.AdditionalProperties == nil {
		return "", fmt.Errorf("redis.one returned redis without a status")
	}
	raw, ok := v.AdditionalProperties["status"]
	if !ok {
		return "", fmt.Errorf("redis.one returned redis without a status")
	}
	s, ok := raw.(string)
	if !ok || s == "" {
		return "", fmt.Errorf("redis.one returned invalid status %v", raw)
	}
	return s, nil
}

func (r Redis) Read(ctx context.Context, req infer.ReadRequest[RedisArgs, RedisState]) (infer.ReadResponse[RedisArgs, RedisState], error) {
	resp, err := r.client(ctx).RedisOneWithResponse(ctx, &generated.RedisOneParams{RedisId: req.ID})
	if err != nil {
		if client.IsNotFound(err) {
			return infer.ReadResponse[RedisArgs, RedisState]{ID: ""}, nil
		}
		return infer.ReadResponse[RedisArgs, RedisState]{}, err
	}
	if resp.JSON200 == nil || resp.JSON200.RedisId == nil {
		return infer.ReadResponse[RedisArgs, RedisState]{}, fmt.Errorf("redis.one returned incomplete redis")
	}
	v := resp.JSON200
	a := req.State.RedisArgs
	a.Name, a.EnvironmentID = value(v.Name), value(v.EnvironmentId)
	a.AppName, a.Description, a.ServerID, a.ExternalPort = v.AppName, v.Description, v.ServerId, v.ExternalPort
	if v.Image != nil {
		a.DockerImage = *v.Image
	}
	if v.Env != nil {
		a.Environment = v.Env
	}
	if password := stringValue(v.AdditionalProperties, "databasePassword"); password != "" {
		a.DatabasePassword = password
	}
	if a.DatabasePassword == "" {
		return infer.ReadResponse[RedisArgs, RedisState]{}, fmt.Errorf("redis.one omitted required databasePassword; import requires an observable password or prior state")
	}
	status, err := redisStatusValue(v)
	if err != nil {
		return infer.ReadResponse[RedisArgs, RedisState]{}, err
	}
	state := RedisState{RedisArgs: a, RedisID: *v.RedisId, Status: status}
	return infer.ReadResponse[RedisArgs, RedisState]{ID: *v.RedisId, Inputs: a, State: state}, nil
}

func (r Redis) Update(ctx context.Context, req infer.UpdateRequest[RedisArgs, RedisState]) (infer.UpdateResponse[RedisState], error) {
	state := RedisState{RedisArgs: req.Inputs, RedisID: req.ID, Status: req.State.Status}
	if req.DryRun {
		return infer.UpdateResponse[RedisState]{Output: state}, nil
	}
	api := r.client(ctx)
	metadata := req.Inputs.Name != req.State.Name || !sameOptionalString(req.Inputs.AppName, req.State.AppName) || !sameOptionalString(req.Inputs.Description, req.State.Description)
	runtime := req.Inputs.DatabasePassword != req.State.DatabasePassword || req.Inputs.DockerImage != req.State.DockerImage
	if metadata || runtime {
		b := generated.RedisUpdateJSONRequestBody{RedisId: req.ID, Name: &req.Inputs.Name, AppName: req.Inputs.AppName, Description: nullable.NewNullNullable[string]()}
		if req.Inputs.Description != nil {
			b.Description = nullable.NewNullableWithValue(*req.Inputs.Description)
		}
		if runtime {
			b.DatabasePassword = &req.Inputs.DatabasePassword
			b.DockerImage = &req.Inputs.DockerImage
		}
		if _, err := api.RedisUpdateWithResponse(ctx, b); err != nil {
			return infer.UpdateResponse[RedisState]{Output: state}, sanitizeRedisError(err, req.Inputs, req.State.RedisArgs)
		}
	}
	if !sameOptionalString(req.Inputs.Environment, req.State.Environment) {
		if err := saveRedisEnvironment(ctx, api, req.ID, req.Inputs.Environment); err != nil {
			return infer.UpdateResponse[RedisState]{Output: state}, sanitizeRedisError(err, req.Inputs, req.State.RedisArgs)
		}
		runtime = true
	}
	if !sameOptionalInt(req.Inputs.ExternalPort, req.State.ExternalPort) {
		if err := saveRedisPort(ctx, api, req.ID, req.Inputs.ExternalPort); err != nil {
			return infer.UpdateResponse[RedisState]{Output: state}, sanitizeRedisError(err, req.Inputs, req.State.RedisArgs)
		}
		runtime = true
	}
	if runtime {
		if _, err := api.RedisDeployWithResponse(ctx, generated.RedisDeployJSONRequestBody{RedisId: req.ID}); err != nil {
			return infer.UpdateResponse[RedisState]{Output: state}, sanitizeRedisError(err, req.Inputs, req.State.RedisArgs)
		}
		if err := waitForDone(ctx, "redis", req.ID, func(c context.Context) (string, error) { return redisStatus(c, api, req.ID) }); err != nil {
			return infer.UpdateResponse[RedisState]{Output: state}, sanitizeRedisError(err, req.Inputs, req.State.RedisArgs)
		}
		state.Status = "done"
	}
	return infer.UpdateResponse[RedisState]{Output: state}, nil
}
func (r Redis) Delete(ctx context.Context, req infer.DeleteRequest[RedisState]) (infer.DeleteResponse, error) {
	_, err := r.client(ctx).RedisRemoveWithResponse(ctx, generated.RedisRemoveJSONRequestBody{RedisId: req.ID})
	if client.IsNotFound(err) {
		return infer.DeleteResponse{}, nil
	}
	return infer.DeleteResponse{}, err
}
func (r Redis) WireDependencies(f infer.FieldSelector, args *RedisArgs, state *RedisState) {
	deps := []infer.InputField{f.InputField(&args.Name), f.InputField(&args.AppName), f.InputField(&args.Description), f.InputField(&args.EnvironmentID), f.InputField(&args.ServerID), f.InputField(&args.DockerImage), f.InputField(&args.ExternalPort)}
	f.OutputField(&state.RedisID).DependsOn(deps...)
	f.OutputField(&state.Status).DependsOn(deps...)
	f.OutputField(&state.DatabasePassword).DependsOn(f.InputField(&args.DatabasePassword).Secret())
	f.OutputField(&state.Environment).DependsOn(f.InputField(&args.Environment).Secret())
}
