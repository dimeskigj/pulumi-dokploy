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

type MariaDBArgs struct {
	Name                 string  `pulumi:"name"`
	AppName              *string `pulumi:"appName,optional"`
	Description          *string `pulumi:"description,optional"`
	EnvironmentID        string  `pulumi:"environmentId" provider:"replaceOnChanges"`
	ServerID             *string `pulumi:"serverId,optional" provider:"replaceOnChanges"`
	DatabaseName         string  `pulumi:"databaseName"`
	DatabaseUser         string  `pulumi:"databaseUser"`
	DatabasePassword     string  `pulumi:"databasePassword" provider:"secret"`
	DatabaseRootPassword *string `pulumi:"databaseRootPassword,optional" provider:"secret"`
	DockerImage          string  `pulumi:"dockerImage,optional"`
	Environment          *string `pulumi:"environment,optional" provider:"secret"`
	ExternalPort         *int    `pulumi:"externalPort,optional"`
}

type MariaDBState struct {
	MariaDBArgs
	MariaDBID string `pulumi:"mariadbId"`
	Status    string `pulumi:"status"`
}

func (s *MariaDBState) Annotate(a infer.Annotator) {
	a.Describe(&s.MariaDBID, "The stable Dokploy MariaDB ID.")
	a.Describe(&s.Status, "The current MariaDB deployment status.")
}

type MariaDB struct{ client clientFactory }

func (r *MariaDB) Annotate(a infer.Annotator) {
	a.SetToken("index", "MariaDB")
	a.Describe(&r, "A Dokploy MariaDB database.")
}
func (a *MariaDBArgs) Annotate(annotator infer.Annotator) {
	annotator.Describe(&a.Name, "The database resource name.")
	annotator.Describe(&a.AppName, "The optional deployed database name.")
	annotator.Describe(&a.Description, "An optional database description.")
	annotator.Describe(&a.EnvironmentID, "The target environment ID.")
	annotator.Describe(&a.ServerID, "The optional server ID.")
	annotator.Describe(&a.DatabaseName, "The MariaDB database name.")
	annotator.Describe(&a.DatabaseUser, "The MariaDB database user.")
	annotator.Describe(&a.DatabasePassword, "The MariaDB database password.")
	annotator.Describe(&a.DatabaseRootPassword, "The optional MariaDB root password.")
	annotator.Describe(&a.DockerImage, "The MariaDB Docker image.")
	annotator.Describe(&a.Environment, "Environment variables for MariaDB.")
	annotator.Describe(&a.ExternalPort, "The optional externally exposed port.")
	annotator.SetDefault(&a.DockerImage, "mariadb:11")
}
func (r MariaDB) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[MariaDBArgs], error) {
	in, failures, err := infer.DefaultCheck[MariaDBArgs](ctx, req.NewInputs)
	if err != nil || len(failures) != 0 {
		return infer.CheckResponse[MariaDBArgs]{Inputs: in, Failures: failures}, err
	}
	if in.DockerImage == "" {
		in.DockerImage = "mariadb:11"
	}
	if in.Name == "" {
		failures = append(failures, p.CheckFailure{Property: "name", Reason: "name must not be empty"})
	}
	if in.EnvironmentID == "" && !req.NewInputs.Get("environmentId").HasComputed() {
		failures = append(failures, p.CheckFailure{Property: "environmentId", Reason: "environmentId must not be empty"})
	}
	return infer.CheckResponse[MariaDBArgs]{Inputs: in, Failures: failures}, nil
}

func (r MariaDB) Diff(_ context.Context, req infer.DiffRequest[MariaDBArgs, MariaDBState]) (infer.DiffResponse, error) {
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
	if !sameOptionalString(req.Inputs.DatabaseRootPassword, req.State.DatabaseRootPassword) {
		d["databaseRootPassword"] = p.PropertyDiff{Kind: p.Update}
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

func (r MariaDB) Create(ctx context.Context, req infer.CreateRequest[MariaDBArgs]) (infer.CreateResponse[MariaDBState], error) {
	state := MariaDBState{MariaDBArgs: req.Inputs}
	if req.DryRun {
		return infer.CreateResponse[MariaDBState]{Output: state}, nil
	}
	api := r.client(ctx)
	b := generated.MariadbCreateJSONRequestBody{Name: req.Inputs.Name, EnvironmentId: req.Inputs.EnvironmentID, DatabaseName: req.Inputs.DatabaseName, DatabaseUser: req.Inputs.DatabaseUser, DatabasePassword: req.Inputs.DatabasePassword, DockerImage: &req.Inputs.DockerImage, Description: nullable.NewNullNullable[string](), ServerId: nullable.NewNullNullable[string]()}
	if req.Inputs.AppName != nil {
		b.AppName = req.Inputs.AppName
	}
	if req.Inputs.Description != nil {
		b.Description = nullable.NewNullableWithValue(*req.Inputs.Description)
	}
	if req.Inputs.ServerID != nil {
		b.ServerId = nullable.NewNullableWithValue(*req.Inputs.ServerID)
	}
	if req.Inputs.DatabaseRootPassword != nil {
		b.DatabaseRootPassword = req.Inputs.DatabaseRootPassword
	}
	resp, err := api.MariadbCreateWithResponse(ctx, b)
	if err != nil {
		return infer.CreateResponse[MariaDBState]{}, sanitizeMariaDBError(err, req.Inputs)
	}
	if resp.JSON200 == nil || resp.JSON200.MariadbId == nil {
		return infer.CreateResponse[MariaDBState]{}, fmt.Errorf("mariadb.create returned incomplete mariadb")
	}
	state.MariaDBID = *resp.JSON200.MariadbId
	failSetup := func(e error) (infer.CreateResponse[MariaDBState], error) {
		if _, ce := api.MariadbRemoveWithResponse(ctx, generated.MariadbRemoveJSONRequestBody{MariadbId: state.MariaDBID}); ce != nil {
			p.GetLogger(ctx).Warningf("mariadb cleanup failed for %s: %s", state.MariaDBID, sanitizeMariaDBError(ce, req.Inputs))
		}
		return infer.CreateResponse[MariaDBState]{ID: state.MariaDBID, Output: state}, e
	}
	if req.Inputs.Environment != nil {
		if err := saveMariaDBEnvironment(ctx, api, state.MariaDBID, req.Inputs.Environment); err != nil {
			return failSetup(sanitizeMariaDBError(err, req.Inputs))
		}
	}
	if req.Inputs.ExternalPort != nil {
		if err := saveMariaDBPort(ctx, api, state.MariaDBID, req.Inputs.ExternalPort); err != nil {
			return failSetup(sanitizeMariaDBError(err, req.Inputs))
		}
	}
	if _, err := api.MariadbDeployWithResponse(ctx, generated.MariadbDeployJSONRequestBody{MariadbId: state.MariaDBID}); err != nil {
		return infer.CreateResponse[MariaDBState]{ID: state.MariaDBID, Output: state}, initFailed(sanitizeMariaDBError(err, req.Inputs))
	}
	if err := waitForDone(ctx, "mariadb", state.MariaDBID, func(c context.Context) (string, error) { return mariadbStatus(c, api, state.MariaDBID) }); err != nil {
		return infer.CreateResponse[MariaDBState]{ID: state.MariaDBID, Output: state}, initFailed(sanitizeMariaDBError(err, req.Inputs))
	}
	state.Status = statusDone
	return infer.CreateResponse[MariaDBState]{ID: state.MariaDBID, Output: state}, nil
}

func saveMariaDBEnvironment(ctx context.Context, api *client.Client, id string, env *string) error {
	e := nullable.NewNullNullable[string]()
	if env != nil {
		e = nullable.NewNullableWithValue(*env)
	}
	_, err := api.MariadbSaveEnvironmentWithResponse(ctx, generated.MariadbSaveEnvironmentJSONRequestBody{MariadbId: id, Env: e})
	return err
}
func saveMariaDBPort(ctx context.Context, api *client.Client, id string, port *int) error {
	p := nullable.NewNullNullable[float32]()
	if port != nil {
		p = nullable.NewNullableWithValue(float32(*port))
	}
	_, err := api.MariadbSaveExternalPortWithResponse(ctx, generated.MariadbSaveExternalPortJSONRequestBody{MariadbId: id, ExternalPort: p})
	return err
}
func sanitizeMariaDBError(err error, args MariaDBArgs, prior ...MariaDBArgs) error {
	secrets := []string{args.DatabasePassword}
	if args.DatabaseRootPassword != nil {
		secrets = append(secrets, *args.DatabaseRootPassword)
	}
	if args.Environment != nil {
		secrets = append(secrets, *args.Environment)
	}
	for _, old := range prior {
		secrets = append(secrets, old.DatabasePassword)
		if old.DatabaseRootPassword != nil {
			secrets = append(secrets, *old.DatabaseRootPassword)
		}
		if old.Environment != nil {
			secrets = append(secrets, *old.Environment)
		}
	}
	return sanitizeError(err, secrets...)
}

func mariadbStatus(ctx context.Context, api *client.Client, id string) (string, error) {
	r, err := api.MariadbOneWithResponse(ctx, &generated.MariadbOneParams{MariadbId: id})
	if err != nil {
		return "", err
	}
	if r.JSON200 == nil {
		return "", fmt.Errorf("mariadb.one returned incomplete mariadb")
	}
	return mariadbStatusValue(r.JSON200)
}
func mariadbStatusValue(v *generated.MariaDB) (string, error) {
	if v.AdditionalProperties == nil {
		return "", fmt.Errorf("mariadb.one returned mariadb without a status")
	}
	raw, ok := v.AdditionalProperties["applicationStatus"]
	if !ok {
		return "", fmt.Errorf("mariadb.one returned mariadb without a status")
	}
	s, ok := raw.(string)
	if !ok || s == "" {
		return "", fmt.Errorf("mariadb.one returned invalid status %v", raw)
	}
	return s, nil
}

func (r MariaDB) Read(ctx context.Context, req infer.ReadRequest[MariaDBArgs, MariaDBState]) (infer.ReadResponse[MariaDBArgs, MariaDBState], error) {
	resp, err := r.client(ctx).MariadbOneWithResponse(ctx, &generated.MariadbOneParams{MariadbId: req.ID})
	if err != nil {
		if client.IsNotFound(err) {
			return infer.ReadResponse[MariaDBArgs, MariaDBState]{ID: ""}, nil
		}
		return infer.ReadResponse[MariaDBArgs, MariaDBState]{}, err
	}
	if resp.JSON200 == nil || resp.JSON200.MariadbId == nil {
		return infer.ReadResponse[MariaDBArgs, MariaDBState]{}, fmt.Errorf("mariadb.one returned incomplete mariadb")
	}
	v := resp.JSON200
	for name, field := range map[string]*string{"name": v.Name, "environmentId": v.EnvironmentId, "databaseName": v.DatabaseName, "databaseUser": v.DatabaseUser} {
		if field == nil {
			return infer.ReadResponse[MariaDBArgs, MariaDBState]{}, fmt.Errorf("mariadb.one omitted required %s", name)
		}
	}
	a := req.State.MariaDBArgs
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
	if rootPassword := stringValue(v.AdditionalProperties, "databaseRootPassword"); rootPassword != "" {
		a.DatabaseRootPassword = &rootPassword
	}
	status, err := mariadbStatusValue(v)
	if err != nil {
		return infer.ReadResponse[MariaDBArgs, MariaDBState]{}, err
	}
	state := MariaDBState{MariaDBArgs: a, MariaDBID: *v.MariadbId, Status: status}
	return infer.ReadResponse[MariaDBArgs, MariaDBState]{ID: *v.MariadbId, Inputs: a, State: state}, nil
}

func (r MariaDB) Update(ctx context.Context, req infer.UpdateRequest[MariaDBArgs, MariaDBState]) (infer.UpdateResponse[MariaDBState], error) {
	state := MariaDBState{MariaDBArgs: req.Inputs, MariaDBID: req.ID, Status: req.State.Status}
	if req.DryRun {
		return infer.UpdateResponse[MariaDBState]{Output: state}, nil
	}
	api := r.client(ctx)
	metadata := req.Inputs.Name != req.State.Name || !sameOptionalString(req.Inputs.AppName, req.State.AppName) || !sameOptionalString(req.Inputs.Description, req.State.Description)
	runtime := req.Inputs.DatabaseName != req.State.DatabaseName || req.Inputs.DatabaseUser != req.State.DatabaseUser || req.Inputs.DatabasePassword != req.State.DatabasePassword || !sameOptionalString(req.Inputs.DatabaseRootPassword, req.State.DatabaseRootPassword) || req.Inputs.DockerImage != req.State.DockerImage
	if metadata || runtime {
		b := generated.MariadbUpdateJSONRequestBody{MariadbId: req.ID, Name: &req.Inputs.Name, AppName: req.Inputs.AppName, Description: nullable.NewNullNullable[string]()}
		if req.Inputs.Description != nil {
			b.Description = nullable.NewNullableWithValue(*req.Inputs.Description)
		}
		if runtime {
			b.DatabaseName = &req.Inputs.DatabaseName
			b.DatabaseUser = &req.Inputs.DatabaseUser
			b.DatabasePassword = &req.Inputs.DatabasePassword
			b.DatabaseRootPassword = req.Inputs.DatabaseRootPassword
			b.DockerImage = &req.Inputs.DockerImage
		}
		if _, err := api.MariadbUpdateWithResponse(ctx, b); err != nil {
			return infer.UpdateResponse[MariaDBState]{Output: state}, sanitizeMariaDBError(err, req.Inputs, req.State.MariaDBArgs)
		}
	}
	if !sameOptionalString(req.Inputs.Environment, req.State.Environment) {
		if err := saveMariaDBEnvironment(ctx, api, req.ID, req.Inputs.Environment); err != nil {
			return infer.UpdateResponse[MariaDBState]{Output: state}, sanitizeMariaDBError(err, req.Inputs, req.State.MariaDBArgs)
		}
		runtime = true
	}
	if !sameOptionalInt(req.Inputs.ExternalPort, req.State.ExternalPort) {
		if err := saveMariaDBPort(ctx, api, req.ID, req.Inputs.ExternalPort); err != nil {
			return infer.UpdateResponse[MariaDBState]{Output: state}, sanitizeMariaDBError(err, req.Inputs, req.State.MariaDBArgs)
		}
		runtime = true
	}
	if runtime {
		if _, err := api.MariadbDeployWithResponse(ctx, generated.MariadbDeployJSONRequestBody{MariadbId: req.ID}); err != nil {
			return infer.UpdateResponse[MariaDBState]{Output: state}, sanitizeMariaDBError(err, req.Inputs, req.State.MariaDBArgs)
		}
		if err := waitForDone(ctx, "mariadb", req.ID, func(c context.Context) (string, error) { return mariadbStatus(c, api, req.ID) }); err != nil {
			return infer.UpdateResponse[MariaDBState]{Output: state}, sanitizeMariaDBError(err, req.Inputs, req.State.MariaDBArgs)
		}
		state.Status = statusDone
	}
	return infer.UpdateResponse[MariaDBState]{Output: state}, nil
}
func (r MariaDB) Delete(ctx context.Context, req infer.DeleteRequest[MariaDBState]) (infer.DeleteResponse, error) {
	_, err := r.client(ctx).MariadbRemoveWithResponse(ctx, generated.MariadbRemoveJSONRequestBody{MariadbId: req.ID})
	if client.IsNotFound(err) {
		return infer.DeleteResponse{}, nil
	}
	return infer.DeleteResponse{}, err
}
func (r MariaDB) WireDependencies(f infer.FieldSelector, args *MariaDBArgs, state *MariaDBState) {
	deps := []infer.InputField{f.InputField(&args.Name), f.InputField(&args.AppName), f.InputField(&args.Description), f.InputField(&args.EnvironmentID), f.InputField(&args.ServerID), f.InputField(&args.DatabaseName), f.InputField(&args.DockerImage), f.InputField(&args.ExternalPort)}
	f.OutputField(&state.MariaDBID).DependsOn(deps...)
	f.OutputField(&state.Status).DependsOn(deps...)
	f.OutputField(&state.DatabasePassword).DependsOn(f.InputField(&args.DatabasePassword).Secret())
	f.OutputField(&state.DatabaseRootPassword).DependsOn(f.InputField(&args.DatabaseRootPassword).Secret())
	f.OutputField(&state.Environment).DependsOn(f.InputField(&args.Environment).Secret())
	f.OutputField(&state.DatabaseUser).DependsOn(f.InputField(&args.DatabaseUser))
}
