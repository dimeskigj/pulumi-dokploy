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

type MongoDBArgs struct {
	Name             string  `pulumi:"name"`
	AppName          *string `pulumi:"appName,optional"`
	Description      *string `pulumi:"description,optional"`
	EnvironmentID    string  `pulumi:"environmentId" provider:"replaceOnChanges"`
	ServerID         *string `pulumi:"serverId,optional" provider:"replaceOnChanges"`
	DatabaseUser     string  `pulumi:"databaseUser"`
	DatabasePassword string  `pulumi:"databasePassword" provider:"secret"`
	DockerImage      string  `pulumi:"dockerImage,optional"`
	Environment      *string `pulumi:"environment,optional" provider:"secret"`
	ExternalPort     *int    `pulumi:"externalPort,optional"`
	ReplicaSets      *bool   `pulumi:"replicaSets,optional"`
}

type MongoDBState struct {
	MongoDBArgs
	MongoDBID string `pulumi:"mongoId"`
	Status    string `pulumi:"status"`
}

func (s *MongoDBState) Annotate(a infer.Annotator) {
	a.Describe(&s.MongoDBID, "The stable Dokploy MongoDB ID.")
	a.Describe(&s.Status, "The current MongoDB deployment status.")
}

type MongoDB struct{ client clientFactory }

func (r *MongoDB) Annotate(a infer.Annotator) {
	a.SetToken("index", "MongoDB")
	a.Describe(&r, "A Dokploy MongoDB database.")
}
func (a *MongoDBArgs) Annotate(annotator infer.Annotator) {
	annotator.Describe(&a.Name, "The database resource name.")
	annotator.Describe(&a.AppName, "The optional deployed database name.")
	annotator.Describe(&a.Description, "An optional database description.")
	annotator.Describe(&a.EnvironmentID, "The target environment ID.")
	annotator.Describe(&a.ServerID, "The optional server ID.")
	annotator.Describe(&a.DatabaseUser, "The MongoDB database user.")
	annotator.Describe(&a.DatabasePassword, "The MongoDB database password.")
	annotator.Describe(&a.DockerImage, "The MongoDB Docker image.")
	annotator.Describe(&a.Environment, "Environment variables for MongoDB.")
	annotator.Describe(&a.ExternalPort, "The optional externally exposed port.")
	annotator.Describe(&a.ReplicaSets, "Whether to enable a MongoDB replica set.")
	annotator.SetDefault(&a.DockerImage, "mongo:8")
}
func (r MongoDB) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[MongoDBArgs], error) {
	in, failures, err := infer.DefaultCheck[MongoDBArgs](ctx, req.NewInputs)
	if err != nil || len(failures) != 0 {
		return infer.CheckResponse[MongoDBArgs]{Inputs: in, Failures: failures}, err
	}
	if in.DockerImage == "" {
		in.DockerImage = "mongo:8"
	}
	if in.Name == "" {
		failures = append(failures, p.CheckFailure{Property: "name", Reason: "name must not be empty"})
	}
	if in.EnvironmentID == "" && !req.NewInputs.Get("environmentId").HasComputed() {
		failures = append(failures, p.CheckFailure{Property: "environmentId", Reason: "environmentId must not be empty"})
	}
	return infer.CheckResponse[MongoDBArgs]{Inputs: in, Failures: failures}, nil
}

func (r MongoDB) Diff(_ context.Context, req infer.DiffRequest[MongoDBArgs, MongoDBState]) (infer.DiffResponse, error) {
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
	if req.Inputs.DatabaseUser != req.State.DatabaseUser {
		d["databaseUser"] = p.PropertyDiff{Kind: p.Update}
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
	if !sameOptionalBool(req.Inputs.ReplicaSets, req.State.ReplicaSets) {
		d["replicaSets"] = p.PropertyDiff{Kind: p.Update}
	}
	return infer.DiffResponse{HasChanges: len(d) > 0, DetailedDiff: d}, nil
}

func (r MongoDB) Create(ctx context.Context, req infer.CreateRequest[MongoDBArgs]) (infer.CreateResponse[MongoDBState], error) {
	state := MongoDBState{MongoDBArgs: req.Inputs}
	if req.DryRun {
		return infer.CreateResponse[MongoDBState]{Output: state}, nil
	}
	api := r.client(ctx)
	b := generated.MongoCreateJSONRequestBody{Name: req.Inputs.Name, EnvironmentId: req.Inputs.EnvironmentID, DatabaseUser: req.Inputs.DatabaseUser, DatabasePassword: req.Inputs.DatabasePassword, DockerImage: &req.Inputs.DockerImage, Description: nullable.NewNullNullable[string](), ServerId: nullable.NewNullNullable[string]()}
	if req.Inputs.AppName != nil {
		b.AppName = req.Inputs.AppName
	}
	if req.Inputs.Description != nil {
		b.Description = nullable.NewNullableWithValue(*req.Inputs.Description)
	}
	if req.Inputs.ServerID != nil {
		b.ServerId = nullable.NewNullableWithValue(*req.Inputs.ServerID)
	}
	if req.Inputs.ReplicaSets != nil {
		b.ReplicaSets = nullable.NewNullableWithValue(*req.Inputs.ReplicaSets)
	}
	resp, err := api.MongoCreateWithResponse(ctx, b)
	if err != nil {
		return infer.CreateResponse[MongoDBState]{}, sanitizeMongoDBError(err, req.Inputs)
	}
	if resp.JSON200 == nil || resp.JSON200.MongoId == nil {
		return infer.CreateResponse[MongoDBState]{}, fmt.Errorf("mongo.create returned incomplete mongo")
	}
	state.MongoDBID = *resp.JSON200.MongoId
	failSetup := func(e error) (infer.CreateResponse[MongoDBState], error) {
		if _, ce := api.MongoRemoveWithResponse(ctx, generated.MongoRemoveJSONRequestBody{MongoId: state.MongoDBID}); ce != nil {
			p.GetLogger(ctx).Warningf("mongo cleanup failed for %s: %s", state.MongoDBID, sanitizeMongoDBError(ce, req.Inputs))
		}
		return infer.CreateResponse[MongoDBState]{ID: state.MongoDBID, Output: state}, e
	}
	if req.Inputs.Environment != nil {
		if err := saveMongoDBEnvironment(ctx, api, state.MongoDBID, req.Inputs.Environment); err != nil {
			return failSetup(sanitizeMongoDBError(err, req.Inputs))
		}
	}
	if req.Inputs.ExternalPort != nil {
		if err := saveMongoDBPort(ctx, api, state.MongoDBID, req.Inputs.ExternalPort); err != nil {
			return failSetup(sanitizeMongoDBError(err, req.Inputs))
		}
	}
	if _, err := api.MongoDeployWithResponse(ctx, generated.MongoDeployJSONRequestBody{MongoId: state.MongoDBID}); err != nil {
		return infer.CreateResponse[MongoDBState]{ID: state.MongoDBID, Output: state}, initFailed(sanitizeMongoDBError(err, req.Inputs))
	}
	if err := waitForDone(ctx, "mongo", state.MongoDBID, func(c context.Context) (string, error) { return mongodbStatus(c, api, state.MongoDBID) }); err != nil {
		return infer.CreateResponse[MongoDBState]{ID: state.MongoDBID, Output: state}, initFailed(sanitizeMongoDBError(err, req.Inputs))
	}
	state.Status = statusDone
	return infer.CreateResponse[MongoDBState]{ID: state.MongoDBID, Output: state}, nil
}

func saveMongoDBEnvironment(ctx context.Context, api *client.Client, id string, env *string) error {
	e := nullable.NewNullNullable[string]()
	if env != nil {
		e = nullable.NewNullableWithValue(*env)
	}
	_, err := api.MongoSaveEnvironmentWithResponse(ctx, generated.MongoSaveEnvironmentJSONRequestBody{MongoId: id, Env: e})
	return err
}
func saveMongoDBPort(ctx context.Context, api *client.Client, id string, port *int) error {
	p := nullable.NewNullNullable[float32]()
	if port != nil {
		p = nullable.NewNullableWithValue(float32(*port))
	}
	_, err := api.MongoSaveExternalPortWithResponse(ctx, generated.MongoSaveExternalPortJSONRequestBody{MongoId: id, ExternalPort: p})
	return err
}
func sanitizeMongoDBError(err error, args MongoDBArgs, prior ...MongoDBArgs) error {
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

func mongodbStatus(ctx context.Context, api *client.Client, id string) (string, error) {
	r, err := api.MongoOneWithResponse(ctx, &generated.MongoOneParams{MongoId: id})
	if err != nil {
		return "", err
	}
	if r.JSON200 == nil {
		return "", fmt.Errorf("mongo.one returned incomplete mongo")
	}
	return mongodbStatusValue(r.JSON200)
}
func mongodbStatusValue(v *generated.MongoDB) (string, error) {
	if v.AdditionalProperties == nil {
		return "", fmt.Errorf("mongo.one returned mongo without a status")
	}
	raw, ok := v.AdditionalProperties["applicationStatus"]
	if !ok {
		return "", fmt.Errorf("mongo.one returned mongo without a status")
	}
	s, ok := raw.(string)
	if !ok || s == "" {
		return "", fmt.Errorf("mongo.one returned invalid status %v", raw)
	}
	return s, nil
}

func (r MongoDB) Read(ctx context.Context, req infer.ReadRequest[MongoDBArgs, MongoDBState]) (infer.ReadResponse[MongoDBArgs, MongoDBState], error) {
	resp, err := r.client(ctx).MongoOneWithResponse(ctx, &generated.MongoOneParams{MongoId: req.ID})
	if err != nil {
		if client.IsNotFound(err) {
			return infer.ReadResponse[MongoDBArgs, MongoDBState]{ID: ""}, nil
		}
		return infer.ReadResponse[MongoDBArgs, MongoDBState]{}, err
	}
	if resp.JSON200 == nil || resp.JSON200.MongoId == nil {
		return infer.ReadResponse[MongoDBArgs, MongoDBState]{}, fmt.Errorf("mongo.one returned incomplete mongo")
	}
	v := resp.JSON200
	for name, field := range map[string]*string{"name": v.Name, "environmentId": v.EnvironmentId, "databaseUser": v.DatabaseUser} {
		if field == nil {
			return infer.ReadResponse[MongoDBArgs, MongoDBState]{}, fmt.Errorf("mongo.one omitted required %s", name)
		}
	}
	a := req.State.MongoDBArgs
	a.Name, a.EnvironmentID, a.DatabaseUser = value(v.Name), value(v.EnvironmentId), value(v.DatabaseUser)
	a.AppName, a.Description, a.ServerID, a.ExternalPort, a.ReplicaSets = v.AppName, v.Description, v.ServerId, v.ExternalPort, v.ReplicaSets
	if v.Image != nil {
		a.DockerImage = *v.Image
	}
	if v.Env != nil {
		a.Environment = v.Env
	}
	if password := stringValue(v.AdditionalProperties, "databasePassword"); password != "" {
		a.DatabasePassword = password
	}
	status, err := mongodbStatusValue(v)
	if err != nil {
		return infer.ReadResponse[MongoDBArgs, MongoDBState]{}, err
	}
	state := MongoDBState{MongoDBArgs: a, MongoDBID: *v.MongoId, Status: status}
	return infer.ReadResponse[MongoDBArgs, MongoDBState]{ID: *v.MongoId, Inputs: a, State: state}, nil
}

func (r MongoDB) Update(ctx context.Context, req infer.UpdateRequest[MongoDBArgs, MongoDBState]) (infer.UpdateResponse[MongoDBState], error) {
	state := MongoDBState{MongoDBArgs: req.Inputs, MongoDBID: req.ID, Status: req.State.Status}
	if req.DryRun {
		return infer.UpdateResponse[MongoDBState]{Output: state}, nil
	}
	api := r.client(ctx)
	metadata := req.Inputs.Name != req.State.Name || !sameOptionalString(req.Inputs.AppName, req.State.AppName) || !sameOptionalString(req.Inputs.Description, req.State.Description)
	runtime := req.Inputs.DatabaseUser != req.State.DatabaseUser || req.Inputs.DatabasePassword != req.State.DatabasePassword || req.Inputs.DockerImage != req.State.DockerImage || !sameOptionalBool(req.Inputs.ReplicaSets, req.State.ReplicaSets)
	if metadata || runtime {
		b := generated.MongoUpdateJSONRequestBody{MongoId: req.ID, Name: &req.Inputs.Name, AppName: req.Inputs.AppName, Description: nullable.NewNullNullable[string]()}
		if req.Inputs.Description != nil {
			b.Description = nullable.NewNullableWithValue(*req.Inputs.Description)
		}
		if runtime {
			b.DatabaseUser = &req.Inputs.DatabaseUser
			b.DatabasePassword = &req.Inputs.DatabasePassword
			b.DockerImage = &req.Inputs.DockerImage
			if req.Inputs.ReplicaSets != nil {
				b.ReplicaSets = nullable.NewNullableWithValue(*req.Inputs.ReplicaSets)
			}
		}
		if _, err := api.MongoUpdateWithResponse(ctx, b); err != nil {
			return infer.UpdateResponse[MongoDBState]{Output: state}, sanitizeMongoDBError(err, req.Inputs, req.State.MongoDBArgs)
		}
	}
	if !sameOptionalString(req.Inputs.Environment, req.State.Environment) {
		if err := saveMongoDBEnvironment(ctx, api, req.ID, req.Inputs.Environment); err != nil {
			return infer.UpdateResponse[MongoDBState]{Output: state}, sanitizeMongoDBError(err, req.Inputs, req.State.MongoDBArgs)
		}
		runtime = true
	}
	if !sameOptionalInt(req.Inputs.ExternalPort, req.State.ExternalPort) {
		if err := saveMongoDBPort(ctx, api, req.ID, req.Inputs.ExternalPort); err != nil {
			return infer.UpdateResponse[MongoDBState]{Output: state}, sanitizeMongoDBError(err, req.Inputs, req.State.MongoDBArgs)
		}
		runtime = true
	}
	if runtime {
		if _, err := api.MongoDeployWithResponse(ctx, generated.MongoDeployJSONRequestBody{MongoId: req.ID}); err != nil {
			return infer.UpdateResponse[MongoDBState]{Output: state}, sanitizeMongoDBError(err, req.Inputs, req.State.MongoDBArgs)
		}
		if err := waitForDone(ctx, "mongo", req.ID, func(c context.Context) (string, error) { return mongodbStatus(c, api, req.ID) }); err != nil {
			return infer.UpdateResponse[MongoDBState]{Output: state}, sanitizeMongoDBError(err, req.Inputs, req.State.MongoDBArgs)
		}
		state.Status = statusDone
	}
	return infer.UpdateResponse[MongoDBState]{Output: state}, nil
}
func (r MongoDB) Delete(ctx context.Context, req infer.DeleteRequest[MongoDBState]) (infer.DeleteResponse, error) {
	_, err := r.client(ctx).MongoRemoveWithResponse(ctx, generated.MongoRemoveJSONRequestBody{MongoId: req.ID})
	if client.IsNotFound(err) {
		return infer.DeleteResponse{}, nil
	}
	return infer.DeleteResponse{}, err
}
func (r MongoDB) WireDependencies(f infer.FieldSelector, args *MongoDBArgs, state *MongoDBState) {
	deps := []infer.InputField{f.InputField(&args.Name), f.InputField(&args.AppName), f.InputField(&args.Description), f.InputField(&args.EnvironmentID), f.InputField(&args.ServerID), f.InputField(&args.DockerImage), f.InputField(&args.ExternalPort), f.InputField(&args.ReplicaSets)}
	f.OutputField(&state.MongoDBID).DependsOn(deps...)
	f.OutputField(&state.Status).DependsOn(deps...)
	f.OutputField(&state.DatabasePassword).DependsOn(f.InputField(&args.DatabasePassword).Secret())
	f.OutputField(&state.Environment).DependsOn(f.InputField(&args.Environment).Secret())
	f.OutputField(&state.DatabaseUser).DependsOn(f.InputField(&args.DatabaseUser))
}
