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

type PostgresArgs struct {
	Name             string  `pulumi:"name"`
	AppName          *string `pulumi:"appName,optional"`
	Description      *string `pulumi:"description,optional"`
	EnvironmentID    string  `pulumi:"environmentId" provider:"replaceOnChanges"`
	ServerID         *string `pulumi:"serverId,optional" provider:"replaceOnChanges"`
	DatabaseName     string  `pulumi:"databaseName"`
	DatabaseUser     string  `pulumi:"databaseUser"`
	DatabasePassword string  `pulumi:"databasePassword" provider:"secret"`
	DockerImage      string  `pulumi:"dockerImage,optional"`
	Environment      *string `pulumi:"environment,optional" provider:"secret"`
	ExternalPort     *int    `pulumi:"externalPort,optional"`
}

type PostgresState struct {
	PostgresArgs
	PostgresID string `pulumi:"postgresId"`
	Status     string `pulumi:"status"`
}

func (s *PostgresState) Annotate(a infer.Annotator) {
	a.Describe(&s.PostgresID, "The stable Dokploy PostgreSQL ID.")
	a.Describe(&s.Status, "The current PostgreSQL deployment status.")
}

type Postgres struct{ client clientFactory }

func (r *Postgres) Annotate(a infer.Annotator) {
	a.SetToken("index", "Postgres")
	a.Describe(&r, "A Dokploy PostgreSQL database.")
}
func (a *PostgresArgs) Annotate(annotator infer.Annotator) {
	annotator.Describe(&a.Name, "The database resource name.")
	annotator.Describe(&a.AppName, "The optional deployed database name.")
	annotator.Describe(&a.Description, "An optional database description.")
	annotator.Describe(&a.EnvironmentID, "The target environment ID.")
	annotator.Describe(&a.ServerID, "The optional server ID.")
	annotator.Describe(&a.DatabaseName, "The PostgreSQL database name.")
	annotator.Describe(&a.DatabaseUser, "The PostgreSQL database user.")
	annotator.Describe(&a.DatabasePassword, "The PostgreSQL database password.")
	annotator.Describe(&a.DockerImage, "The PostgreSQL Docker image.")
	annotator.Describe(&a.Environment, "Environment variables for PostgreSQL.")
	annotator.Describe(&a.ExternalPort, "The optional externally exposed port.")
	annotator.SetDefault(&a.DockerImage, "postgres:18")
}
func (r Postgres) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[PostgresArgs], error) {
	in, failures, err := infer.DefaultCheck[PostgresArgs](ctx, req.NewInputs)
	if err != nil || len(failures) != 0 {
		return infer.CheckResponse[PostgresArgs]{Inputs: in, Failures: failures}, err
	}
	if in.DockerImage == "" {
		in.DockerImage = "postgres:18"
	}
	if in.Name == "" {
		failures = append(failures, p.CheckFailure{Property: "name", Reason: "name must not be empty"})
	}
	if in.EnvironmentID == "" {
		failures = append(failures, p.CheckFailure{Property: "environmentId", Reason: "environmentId must not be empty"})
	}
	return infer.CheckResponse[PostgresArgs]{Inputs: in, Failures: failures}, nil
}

func (r Postgres) Diff(_ context.Context, req infer.DiffRequest[PostgresArgs, PostgresState]) (infer.DiffResponse, error) {
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
	if req.Inputs.DatabaseName != req.State.DatabaseName {
		d["databaseName"] = p.PropertyDiff{Kind: p.Update}
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
	return infer.DiffResponse{HasChanges: len(d) > 0, DetailedDiff: d}, nil
}

func sameOptionalInt(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func (r Postgres) Create(ctx context.Context, req infer.CreateRequest[PostgresArgs]) (infer.CreateResponse[PostgresState], error) {
	state := PostgresState{PostgresArgs: req.Inputs}
	if req.DryRun {
		return infer.CreateResponse[PostgresState]{Output: state}, nil
	}
	api := r.client(ctx)
	b := generated.PostgresCreateJSONRequestBody{Name: req.Inputs.Name, EnvironmentId: req.Inputs.EnvironmentID, DatabaseName: req.Inputs.DatabaseName, DatabaseUser: req.Inputs.DatabaseUser, DatabasePassword: req.Inputs.DatabasePassword, DockerImage: &req.Inputs.DockerImage, Description: nullable.NewNullNullable[string](), ServerId: nullable.NewNullNullable[string]()}
	if req.Inputs.AppName != nil {
		b.AppName = req.Inputs.AppName
	}
	if req.Inputs.Description != nil {
		b.Description = nullable.NewNullableWithValue(*req.Inputs.Description)
	}
	if req.Inputs.ServerID != nil {
		b.ServerId = nullable.NewNullableWithValue(*req.Inputs.ServerID)
	}
	resp, err := api.PostgresCreateWithResponse(ctx, b)
	if err != nil {
		return infer.CreateResponse[PostgresState]{}, sanitizePostgresError(err, req.Inputs)
	}
	if resp.JSON200 == nil || resp.JSON200.PostgresId == nil {
		return infer.CreateResponse[PostgresState]{}, fmt.Errorf("postgres.create returned incomplete postgres")
	}
	state.PostgresID = *resp.JSON200.PostgresId
	failSetup := func(e error) (infer.CreateResponse[PostgresState], error) {
		if _, ce := api.PostgresRemoveWithResponse(ctx, generated.PostgresRemoveJSONRequestBody{PostgresId: state.PostgresID}); ce != nil {
			p.GetLogger(ctx).Warningf("postgres cleanup failed for %s: %s", state.PostgresID, sanitizePostgresError(ce, req.Inputs))
		}
		return infer.CreateResponse[PostgresState]{ID: state.PostgresID, Output: state}, e
	}
	if req.Inputs.Environment != nil {
		if err := savePostgresEnvironment(ctx, api, state.PostgresID, req.Inputs.Environment); err != nil {
			return failSetup(sanitizePostgresError(err, req.Inputs))
		}
	}
	if req.Inputs.ExternalPort != nil {
		if err := savePostgresPort(ctx, api, state.PostgresID, req.Inputs.ExternalPort); err != nil {
			return failSetup(sanitizePostgresError(err, req.Inputs))
		}
	}
	if _, err := api.PostgresDeployWithResponse(ctx, generated.PostgresDeployJSONRequestBody{PostgresId: state.PostgresID}); err != nil {
		return infer.CreateResponse[PostgresState]{ID: state.PostgresID, Output: state}, initFailed(sanitizePostgresError(err, req.Inputs))
	}
	if err := waitForDone(ctx, "postgres", state.PostgresID, func(c context.Context) (string, error) { return postgresStatus(c, api, state.PostgresID) }); err != nil {
		return infer.CreateResponse[PostgresState]{ID: state.PostgresID, Output: state}, initFailed(sanitizePostgresError(err, req.Inputs))
	}
	state.Status = statusDone
	return infer.CreateResponse[PostgresState]{ID: state.PostgresID, Output: state}, nil
}

func savePostgresEnvironment(ctx context.Context, api *client.Client, id string, env *string) error {
	e := nullable.NewNullNullable[string]()
	if env != nil {
		e = nullable.NewNullableWithValue(*env)
	}
	_, err := api.PostgresSaveEnvironmentWithResponse(ctx, generated.PostgresSaveEnvironmentJSONRequestBody{PostgresId: id, Env: e})
	return err
}
func savePostgresPort(ctx context.Context, api *client.Client, id string, port *int) error {
	p := nullable.NewNullNullable[float32]()
	if port != nil {
		p = nullable.NewNullableWithValue(float32(*port))
	}
	_, err := api.PostgresSaveExternalPortWithResponse(ctx, generated.PostgresSaveExternalPortJSONRequestBody{PostgresId: id, ExternalPort: p})
	return err
}
func sanitizePostgresError(err error, args PostgresArgs, prior ...PostgresArgs) error {
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

func postgresStatus(ctx context.Context, api *client.Client, id string) (string, error) {
	r, err := api.PostgresOneWithResponse(ctx, &generated.PostgresOneParams{PostgresId: id})
	if err != nil {
		return "", err
	}
	if r.JSON200 == nil {
		return "", fmt.Errorf("postgres.one returned incomplete postgres")
	}
	return postgresStatusValue(r.JSON200)
}
func postgresStatusValue(v *generated.Postgres) (string, error) {
	if v.AdditionalProperties == nil {
		return "", fmt.Errorf("postgres.one returned postgres without a status")
	}
	raw, ok := v.AdditionalProperties["status"]
	if !ok {
		return "", fmt.Errorf("postgres.one returned postgres without a status")
	}
	s, ok := raw.(string)
	if !ok || s == "" {
		return "", fmt.Errorf("postgres.one returned invalid status %v", raw)
	}
	return s, nil
}

func (r Postgres) Read(ctx context.Context, req infer.ReadRequest[PostgresArgs, PostgresState]) (infer.ReadResponse[PostgresArgs, PostgresState], error) {
	resp, err := r.client(ctx).PostgresOneWithResponse(ctx, &generated.PostgresOneParams{PostgresId: req.ID})
	if err != nil {
		if client.IsNotFound(err) {
			return infer.ReadResponse[PostgresArgs, PostgresState]{ID: ""}, nil
		}
		return infer.ReadResponse[PostgresArgs, PostgresState]{}, err
	}
	if resp.JSON200 == nil || resp.JSON200.PostgresId == nil {
		return infer.ReadResponse[PostgresArgs, PostgresState]{}, fmt.Errorf("postgres.one returned incomplete postgres")
	}
	v := resp.JSON200
	for name, field := range map[string]*string{"name": v.Name, "environmentId": v.EnvironmentId, "databaseName": v.DatabaseName, "databaseUser": v.DatabaseUser} {
		if field == nil {
			return infer.ReadResponse[PostgresArgs, PostgresState]{}, fmt.Errorf("postgres.one omitted required %s", name)
		}
	}
	a := req.State.PostgresArgs
	a.Name, a.EnvironmentID, a.DatabaseName, a.DatabaseUser = value(v.Name), value(v.EnvironmentId), value(v.DatabaseName), value(v.DatabaseUser)
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
	status, err := postgresStatusValue(v)
	if err != nil {
		return infer.ReadResponse[PostgresArgs, PostgresState]{}, err
	}
	state := PostgresState{PostgresArgs: a, PostgresID: *v.PostgresId, Status: status}
	return infer.ReadResponse[PostgresArgs, PostgresState]{ID: *v.PostgresId, Inputs: a, State: state}, nil
}

func (r Postgres) Update(ctx context.Context, req infer.UpdateRequest[PostgresArgs, PostgresState]) (infer.UpdateResponse[PostgresState], error) {
	state := PostgresState{PostgresArgs: req.Inputs, PostgresID: req.ID, Status: req.State.Status}
	if req.DryRun {
		return infer.UpdateResponse[PostgresState]{Output: state}, nil
	}
	api := r.client(ctx)
	metadata := req.Inputs.Name != req.State.Name || !sameOptionalString(req.Inputs.AppName, req.State.AppName) || !sameOptionalString(req.Inputs.Description, req.State.Description)
	runtime := req.Inputs.DatabaseName != req.State.DatabaseName || req.Inputs.DatabaseUser != req.State.DatabaseUser || req.Inputs.DatabasePassword != req.State.DatabasePassword || req.Inputs.DockerImage != req.State.DockerImage
	if metadata || runtime {
		b := generated.PostgresUpdateJSONRequestBody{PostgresId: req.ID, Name: &req.Inputs.Name, AppName: req.Inputs.AppName, Description: nullable.NewNullNullable[string]()}
		if req.Inputs.Description != nil {
			b.Description = nullable.NewNullableWithValue(*req.Inputs.Description)
		}
		if runtime {
			b.DatabaseName = &req.Inputs.DatabaseName
			b.DatabaseUser = &req.Inputs.DatabaseUser
			b.DatabasePassword = &req.Inputs.DatabasePassword
			b.DockerImage = &req.Inputs.DockerImage
		}
		if _, err := api.PostgresUpdateWithResponse(ctx, b); err != nil {
			return infer.UpdateResponse[PostgresState]{Output: state}, sanitizePostgresError(err, req.Inputs, req.State.PostgresArgs)
		}
	}
	if !sameOptionalString(req.Inputs.Environment, req.State.Environment) {
		if err := savePostgresEnvironment(ctx, api, req.ID, req.Inputs.Environment); err != nil {
			return infer.UpdateResponse[PostgresState]{Output: state}, sanitizePostgresError(err, req.Inputs, req.State.PostgresArgs)
		}
		runtime = true
	}
	if !sameOptionalInt(req.Inputs.ExternalPort, req.State.ExternalPort) {
		if err := savePostgresPort(ctx, api, req.ID, req.Inputs.ExternalPort); err != nil {
			return infer.UpdateResponse[PostgresState]{Output: state}, sanitizePostgresError(err, req.Inputs, req.State.PostgresArgs)
		}
		runtime = true
	}
	if runtime {
		if _, err := api.PostgresDeployWithResponse(ctx, generated.PostgresDeployJSONRequestBody{PostgresId: req.ID}); err != nil {
			return infer.UpdateResponse[PostgresState]{Output: state}, sanitizePostgresError(err, req.Inputs, req.State.PostgresArgs)
		}
		if err := waitForDone(ctx, "postgres", req.ID, func(c context.Context) (string, error) { return postgresStatus(c, api, req.ID) }); err != nil {
			return infer.UpdateResponse[PostgresState]{Output: state}, sanitizePostgresError(err, req.Inputs, req.State.PostgresArgs)
		}
		state.Status = statusDone
	}
	return infer.UpdateResponse[PostgresState]{Output: state}, nil
}
func (r Postgres) Delete(ctx context.Context, req infer.DeleteRequest[PostgresState]) (infer.DeleteResponse, error) {
	_, err := r.client(ctx).PostgresRemoveWithResponse(ctx, generated.PostgresRemoveJSONRequestBody{PostgresId: req.ID})
	if client.IsNotFound(err) {
		return infer.DeleteResponse{}, nil
	}
	return infer.DeleteResponse{}, err
}
func (r Postgres) WireDependencies(f infer.FieldSelector, args *PostgresArgs, state *PostgresState) {
	deps := []infer.InputField{f.InputField(&args.Name), f.InputField(&args.AppName), f.InputField(&args.Description), f.InputField(&args.EnvironmentID), f.InputField(&args.ServerID), f.InputField(&args.DatabaseName), f.InputField(&args.DockerImage), f.InputField(&args.ExternalPort)}
	f.OutputField(&state.PostgresID).DependsOn(deps...)
	f.OutputField(&state.Status).DependsOn(deps...)
	f.OutputField(&state.DatabasePassword).DependsOn(f.InputField(&args.DatabasePassword).Secret())
	f.OutputField(&state.Environment).DependsOn(f.InputField(&args.Environment).Secret())
	f.OutputField(&state.DatabaseUser).DependsOn(f.InputField(&args.DatabaseUser))
}
