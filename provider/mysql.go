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

type MySQLArgs struct {
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

type MySQLState struct {
	MySQLArgs
	MySQLID string `pulumi:"mysqlId"`
	Status  string `pulumi:"status"`
}

func (s *MySQLState) Annotate(a infer.Annotator) {
	a.Describe(&s.MySQLID, "The stable Dokploy MySQL ID.")
	a.Describe(&s.Status, "The current MySQL deployment status.")
}

type MySQL struct{ client clientFactory }

func (r *MySQL) Annotate(a infer.Annotator) {
	a.SetToken("index", "MySQL")
	a.Describe(&r, "A Dokploy MySQL database.")
}
func (a *MySQLArgs) Annotate(annotator infer.Annotator) {
	annotator.Describe(&a.Name, "The database resource name.")
	annotator.Describe(&a.AppName, "The optional deployed database name.")
	annotator.Describe(&a.Description, "An optional database description.")
	annotator.Describe(&a.EnvironmentID, "The target environment ID.")
	annotator.Describe(&a.ServerID, "The optional server ID.")
	annotator.Describe(&a.DatabaseName, "The MySQL database name.")
	annotator.Describe(&a.DatabaseUser, "The MySQL database user.")
	annotator.Describe(&a.DatabasePassword, "The MySQL database password.")
	annotator.Describe(&a.DatabaseRootPassword, "The optional MySQL root password.")
	annotator.Describe(&a.DockerImage, "The MySQL Docker image.")
	annotator.Describe(&a.Environment, "Environment variables for MySQL.")
	annotator.Describe(&a.ExternalPort, "The optional externally exposed port.")
	annotator.SetDefault(&a.DockerImage, "mysql:8")
}
func (r MySQL) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[MySQLArgs], error) {
	in, failures, err := infer.DefaultCheck[MySQLArgs](ctx, req.NewInputs)
	if err != nil || len(failures) != 0 {
		return infer.CheckResponse[MySQLArgs]{Inputs: in, Failures: failures}, err
	}
	if in.DockerImage == "" {
		in.DockerImage = "mysql:8"
	}
	if in.Name == "" {
		failures = append(failures, p.CheckFailure{Property: "name", Reason: "name must not be empty"})
	}
	if in.EnvironmentID == "" && !req.NewInputs.Get("environmentId").HasComputed() {
		failures = append(failures, p.CheckFailure{Property: "environmentId", Reason: "environmentId must not be empty"})
	}
	return infer.CheckResponse[MySQLArgs]{Inputs: in, Failures: failures}, nil
}

func (r MySQL) Diff(_ context.Context, req infer.DiffRequest[MySQLArgs, MySQLState]) (infer.DiffResponse, error) {
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

func (r MySQL) Create(ctx context.Context, req infer.CreateRequest[MySQLArgs]) (infer.CreateResponse[MySQLState], error) {
	state := MySQLState{MySQLArgs: req.Inputs}
	if req.DryRun {
		return infer.CreateResponse[MySQLState]{Output: state}, nil
	}
	api := r.client(ctx)
	b := generated.MysqlCreateJSONRequestBody{Name: req.Inputs.Name, EnvironmentId: req.Inputs.EnvironmentID, DatabaseName: req.Inputs.DatabaseName, DatabaseUser: req.Inputs.DatabaseUser, DatabasePassword: req.Inputs.DatabasePassword, DockerImage: &req.Inputs.DockerImage, Description: nullable.NewNullNullable[string](), ServerId: nullable.NewNullNullable[string]()}
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
	resp, err := api.MysqlCreateWithResponse(ctx, b)
	if err != nil {
		return infer.CreateResponse[MySQLState]{}, sanitizeMySQLError(err, req.Inputs)
	}
	if resp.JSON200 == nil || resp.JSON200.MysqlId == nil {
		return infer.CreateResponse[MySQLState]{}, fmt.Errorf("mysql.create returned incomplete mysql")
	}
	state.MySQLID = *resp.JSON200.MysqlId
	failSetup := func(e error) (infer.CreateResponse[MySQLState], error) {
		if _, ce := api.MysqlRemoveWithResponse(ctx, generated.MysqlRemoveJSONRequestBody{MysqlId: state.MySQLID}); ce != nil {
			p.GetLogger(ctx).Warningf("mysql cleanup failed for %s: %s", state.MySQLID, sanitizeMySQLError(ce, req.Inputs))
		}
		return infer.CreateResponse[MySQLState]{ID: state.MySQLID, Output: state}, e
	}
	if req.Inputs.Environment != nil {
		if err := saveMySQLEnvironment(ctx, api, state.MySQLID, req.Inputs.Environment); err != nil {
			return failSetup(sanitizeMySQLError(err, req.Inputs))
		}
	}
	if req.Inputs.ExternalPort != nil {
		if err := saveMySQLPort(ctx, api, state.MySQLID, req.Inputs.ExternalPort); err != nil {
			return failSetup(sanitizeMySQLError(err, req.Inputs))
		}
	}
	if _, err := api.MysqlDeployWithResponse(ctx, generated.MysqlDeployJSONRequestBody{MysqlId: state.MySQLID}); err != nil {
		return infer.CreateResponse[MySQLState]{ID: state.MySQLID, Output: state}, initFailed(sanitizeMySQLError(err, req.Inputs))
	}
	if err := waitForDone(ctx, "mysql", state.MySQLID, func(c context.Context) (string, error) { return mysqlStatus(c, api, state.MySQLID) }); err != nil {
		return infer.CreateResponse[MySQLState]{ID: state.MySQLID, Output: state}, initFailed(sanitizeMySQLError(err, req.Inputs))
	}
	state.Status = statusDone
	return infer.CreateResponse[MySQLState]{ID: state.MySQLID, Output: state}, nil
}

func saveMySQLEnvironment(ctx context.Context, api *client.Client, id string, env *string) error {
	e := nullable.NewNullNullable[string]()
	if env != nil {
		e = nullable.NewNullableWithValue(*env)
	}
	_, err := api.MysqlSaveEnvironmentWithResponse(ctx, generated.MysqlSaveEnvironmentJSONRequestBody{MysqlId: id, Env: e})
	return err
}
func saveMySQLPort(ctx context.Context, api *client.Client, id string, port *int) error {
	p := nullable.NewNullNullable[float32]()
	if port != nil {
		p = nullable.NewNullableWithValue(float32(*port))
	}
	_, err := api.MysqlSaveExternalPortWithResponse(ctx, generated.MysqlSaveExternalPortJSONRequestBody{MysqlId: id, ExternalPort: p})
	return err
}
func sanitizeMySQLError(err error, args MySQLArgs, prior ...MySQLArgs) error {
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

func mysqlStatus(ctx context.Context, api *client.Client, id string) (string, error) {
	r, err := api.MysqlOneWithResponse(ctx, &generated.MysqlOneParams{MysqlId: id})
	if err != nil {
		return "", err
	}
	if r.JSON200 == nil {
		return "", fmt.Errorf("mysql.one returned incomplete mysql")
	}
	return mysqlStatusValue(r.JSON200)
}
func mysqlStatusValue(v *generated.MySQL) (string, error) {
	if v.AdditionalProperties == nil {
		return "", fmt.Errorf("mysql.one returned mysql without a status")
	}
	raw, ok := v.AdditionalProperties["applicationStatus"]
	if !ok {
		return "", fmt.Errorf("mysql.one returned mysql without a status")
	}
	s, ok := raw.(string)
	if !ok || s == "" {
		return "", fmt.Errorf("mysql.one returned invalid status %v", raw)
	}
	return s, nil
}

func (r MySQL) Read(ctx context.Context, req infer.ReadRequest[MySQLArgs, MySQLState]) (infer.ReadResponse[MySQLArgs, MySQLState], error) {
	resp, err := r.client(ctx).MysqlOneWithResponse(ctx, &generated.MysqlOneParams{MysqlId: req.ID})
	if err != nil {
		if client.IsNotFound(err) {
			return infer.ReadResponse[MySQLArgs, MySQLState]{ID: ""}, nil
		}
		return infer.ReadResponse[MySQLArgs, MySQLState]{}, err
	}
	if resp.JSON200 == nil || resp.JSON200.MysqlId == nil {
		return infer.ReadResponse[MySQLArgs, MySQLState]{}, fmt.Errorf("mysql.one returned incomplete mysql")
	}
	v := resp.JSON200
	for name, field := range map[string]*string{"name": v.Name, "environmentId": v.EnvironmentId, "databaseName": v.DatabaseName, "databaseUser": v.DatabaseUser} {
		if field == nil {
			return infer.ReadResponse[MySQLArgs, MySQLState]{}, fmt.Errorf("mysql.one omitted required %s", name)
		}
	}
	a := req.State.MySQLArgs
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
	status, err := mysqlStatusValue(v)
	if err != nil {
		return infer.ReadResponse[MySQLArgs, MySQLState]{}, err
	}
	state := MySQLState{MySQLArgs: a, MySQLID: *v.MysqlId, Status: status}
	return infer.ReadResponse[MySQLArgs, MySQLState]{ID: *v.MysqlId, Inputs: a, State: state}, nil
}

func (r MySQL) Update(ctx context.Context, req infer.UpdateRequest[MySQLArgs, MySQLState]) (infer.UpdateResponse[MySQLState], error) {
	state := MySQLState{MySQLArgs: req.Inputs, MySQLID: req.ID, Status: req.State.Status}
	if req.DryRun {
		return infer.UpdateResponse[MySQLState]{Output: state}, nil
	}
	api := r.client(ctx)
	metadata := req.Inputs.Name != req.State.Name || !sameOptionalString(req.Inputs.AppName, req.State.AppName) || !sameOptionalString(req.Inputs.Description, req.State.Description)
	runtime := req.Inputs.DatabaseName != req.State.DatabaseName || req.Inputs.DatabaseUser != req.State.DatabaseUser || req.Inputs.DatabasePassword != req.State.DatabasePassword || !sameOptionalString(req.Inputs.DatabaseRootPassword, req.State.DatabaseRootPassword) || req.Inputs.DockerImage != req.State.DockerImage
	if metadata || runtime {
		b := generated.MysqlUpdateJSONRequestBody{MysqlId: req.ID, Name: &req.Inputs.Name, AppName: req.Inputs.AppName, Description: nullable.NewNullNullable[string]()}
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
		if _, err := api.MysqlUpdateWithResponse(ctx, b); err != nil {
			return infer.UpdateResponse[MySQLState]{Output: state}, sanitizeMySQLError(err, req.Inputs, req.State.MySQLArgs)
		}
	}
	if !sameOptionalString(req.Inputs.Environment, req.State.Environment) {
		if err := saveMySQLEnvironment(ctx, api, req.ID, req.Inputs.Environment); err != nil {
			return infer.UpdateResponse[MySQLState]{Output: state}, sanitizeMySQLError(err, req.Inputs, req.State.MySQLArgs)
		}
		runtime = true
	}
	if !sameOptionalInt(req.Inputs.ExternalPort, req.State.ExternalPort) {
		if err := saveMySQLPort(ctx, api, req.ID, req.Inputs.ExternalPort); err != nil {
			return infer.UpdateResponse[MySQLState]{Output: state}, sanitizeMySQLError(err, req.Inputs, req.State.MySQLArgs)
		}
		runtime = true
	}
	if runtime {
		if _, err := api.MysqlDeployWithResponse(ctx, generated.MysqlDeployJSONRequestBody{MysqlId: req.ID}); err != nil {
			return infer.UpdateResponse[MySQLState]{Output: state}, sanitizeMySQLError(err, req.Inputs, req.State.MySQLArgs)
		}
		if err := waitForDone(ctx, "mysql", req.ID, func(c context.Context) (string, error) { return mysqlStatus(c, api, req.ID) }); err != nil {
			return infer.UpdateResponse[MySQLState]{Output: state}, sanitizeMySQLError(err, req.Inputs, req.State.MySQLArgs)
		}
		state.Status = statusDone
	}
	return infer.UpdateResponse[MySQLState]{Output: state}, nil
}
func (r MySQL) Delete(ctx context.Context, req infer.DeleteRequest[MySQLState]) (infer.DeleteResponse, error) {
	_, err := r.client(ctx).MysqlRemoveWithResponse(ctx, generated.MysqlRemoveJSONRequestBody{MysqlId: req.ID})
	if client.IsNotFound(err) {
		return infer.DeleteResponse{}, nil
	}
	return infer.DeleteResponse{}, err
}
func (r MySQL) WireDependencies(f infer.FieldSelector, args *MySQLArgs, state *MySQLState) {
	deps := []infer.InputField{f.InputField(&args.Name), f.InputField(&args.AppName), f.InputField(&args.Description), f.InputField(&args.EnvironmentID), f.InputField(&args.ServerID), f.InputField(&args.DatabaseName), f.InputField(&args.DockerImage), f.InputField(&args.ExternalPort)}
	f.OutputField(&state.MySQLID).DependsOn(deps...)
	f.OutputField(&state.Status).DependsOn(deps...)
	f.OutputField(&state.DatabasePassword).DependsOn(f.InputField(&args.DatabasePassword).Secret())
	f.OutputField(&state.DatabaseRootPassword).DependsOn(f.InputField(&args.DatabaseRootPassword).Secret())
	f.OutputField(&state.Environment).DependsOn(f.InputField(&args.Environment).Secret())
	f.OutputField(&state.DatabaseUser).DependsOn(f.InputField(&args.DatabaseUser))
}
