package dokploy

import (
	"context"
	"fmt"
	"reflect"
	"strconv"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/oapi-codegen/nullable"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

type ApplicationArgs struct {
	Name          string            `pulumi:"name"`
	AppName       *string           `pulumi:"appName,optional"`
	Description   *string           `pulumi:"description,optional"`
	EnvironmentID string            `pulumi:"environmentId" provider:"replaceOnChanges"`
	ServerID      *string           `pulumi:"serverId,optional" provider:"replaceOnChanges"`
	Source        ApplicationSource `pulumi:"source"`
	Environment   *string           `pulumi:"environment,optional" provider:"secret"`
	BuildArgs     *string           `pulumi:"buildArgs,optional" provider:"secret"`
	BuildSecrets  *string           `pulumi:"buildSecrets,optional" provider:"secret"`
	CreateEnvFile bool              `pulumi:"createEnvFile,optional"`
}

type ApplicationState struct {
	ApplicationArgs
	ApplicationID string `pulumi:"applicationId"`
	Status        string `pulumi:"status"`
}

func (s *ApplicationState) Annotate(a infer.Annotator) {
	a.Describe(&s.ApplicationID, "The stable Dokploy application ID.")
	a.Describe(&s.Status, "The current application deployment status.")
}

type Application struct{ client clientFactory }

func (r *Application) Annotate(a infer.Annotator) {
	a.SetToken("index", "Application")
	a.Describe(&r, "A Dokploy application.")
}
func (a *ApplicationArgs) Annotate(annotator infer.Annotator) {
	annotator.Describe(&a.Name, "The application name.")
	annotator.Describe(&a.AppName, "The optional deployed application name.")
	annotator.Describe(&a.Description, "An optional application description.")
	annotator.Describe(&a.EnvironmentID, "The target environment ID.")
	annotator.Describe(&a.ServerID, "The optional server ID.")
	annotator.Describe(&a.Source, "The application source configuration.")
	annotator.Describe(&a.Environment, "Environment variables for the application.")
	annotator.Describe(&a.BuildArgs, "Build arguments for the application.")
	annotator.Describe(&a.BuildSecrets, "Build secrets for the application.")
	annotator.Describe(&a.CreateEnvFile, "Whether to create an environment file.")
}

func (r Application) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[ApplicationArgs], error) {
	inputs, failures, err := infer.DefaultCheck[ApplicationArgs](ctx, req.NewInputs)
	if err != nil || len(failures) != 0 {
		return infer.CheckResponse[ApplicationArgs]{Inputs: inputs, Failures: failures}, err
	}
	if inputs.Name == "" {
		failures = append(failures, p.CheckFailure{Property: "name", Reason: "name must not be empty"})
	}
	if inputs.EnvironmentID == "" && !req.NewInputs.Get("environmentId").HasComputed() {
		failures = append(failures, p.CheckFailure{Property: "environmentId", Reason: "environmentId must not be empty"})
	}
	if err := inputs.Source.validate(); err != nil {
		failures = append(failures, p.CheckFailure{Property: "source", Reason: err.Error()})
	}
	return infer.CheckResponse[ApplicationArgs]{Inputs: inputs, Failures: failures}, nil
}

func (r Application) Diff(_ context.Context, req infer.DiffRequest[ApplicationArgs, ApplicationState]) (infer.DiffResponse, error) {
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
	if !sameOptionalString(req.Inputs.BuildArgs, req.State.BuildArgs) {
		d["buildArgs"] = p.PropertyDiff{Kind: p.Update}
	}
	if !sameOptionalString(req.Inputs.BuildSecrets, req.State.BuildSecrets) {
		d["buildSecrets"] = p.PropertyDiff{Kind: p.Update}
	}
	if req.Inputs.CreateEnvFile != req.State.CreateEnvFile {
		d["createEnvFile"] = p.PropertyDiff{Kind: p.Update}
	}
	return infer.DiffResponse{HasChanges: len(d) > 0, DetailedDiff: d}, nil
}

func (r Application) Create(ctx context.Context, req infer.CreateRequest[ApplicationArgs]) (infer.CreateResponse[ApplicationState], error) {
	state := ApplicationState{ApplicationArgs: req.Inputs}
	if req.DryRun {
		return infer.CreateResponse[ApplicationState]{Output: state}, nil
	}
	api := r.client(ctx)
	body := generated.ApplicationCreateJSONRequestBody{Name: req.Inputs.Name, EnvironmentId: req.Inputs.EnvironmentID}
	if req.Inputs.AppName != nil {
		body.AppName = req.Inputs.AppName
	}
	if req.Inputs.Description != nil {
		body.Description = nullable.NewNullableWithValue(*req.Inputs.Description)
	}
	if req.Inputs.ServerID != nil {
		body.ServerId = nullable.NewNullableWithValue(*req.Inputs.ServerID)
	}
	response, err := api.ApplicationCreateWithResponse(ctx, body)
	if err != nil {
		return infer.CreateResponse[ApplicationState]{}, err
	}
	if response.JSON200 == nil || response.JSON200.ApplicationId == nil {
		return infer.CreateResponse[ApplicationState]{}, fmt.Errorf("application.create returned incomplete application")
	}
	state.ApplicationID = *response.JSON200.ApplicationId
	failSetup := func(setupErr error) (infer.CreateResponse[ApplicationState], error) {
		if _, cleanupErr := api.ApplicationDeleteWithResponse(ctx, generated.ApplicationDeleteJSONRequestBody{ApplicationId: state.ApplicationID}); cleanupErr != nil {
			p.GetLogger(ctx).Warningf("application cleanup failed for %s: %s", state.ApplicationID, sanitizeApplicationError(cleanupErr, req.Inputs))
		}
		return infer.CreateResponse[ApplicationState]{ID: state.ApplicationID, Output: state}, setupErr
	}
	if err := configureApplicationSource(ctx, api, state.ApplicationID, req.Inputs.Source); err != nil {
		return failSetup(sanitizeApplicationError(err, req.Inputs))
	}
	if err := configureApplicationBuild(ctx, api, state.ApplicationID, req.Inputs.Source); err != nil {
		return failSetup(sanitizeApplicationError(err, req.Inputs))
	}
	if err := configureApplicationEnvironment(ctx, api, state.ApplicationID, req.Inputs); err != nil {
		return failSetup(sanitizeApplicationError(err, req.Inputs))
	}
	if _, err := api.ApplicationDeployWithResponse(ctx, generated.ApplicationDeployJSONRequestBody{ApplicationId: state.ApplicationID}); err != nil {
		return infer.CreateResponse[ApplicationState]{ID: state.ApplicationID, Output: state}, initFailed(sanitizeApplicationError(err, req.Inputs))
	}
	if err := waitForDone(ctx, "application", state.ApplicationID, func(ctx context.Context) (string, error) { return applicationStatus(ctx, api, state.ApplicationID) }); err != nil {
		return infer.CreateResponse[ApplicationState]{ID: state.ApplicationID, Output: state}, initFailed(sanitizeApplicationError(err, req.Inputs))
	}
	state.Status = statusDone
	return infer.CreateResponse[ApplicationState]{ID: state.ApplicationID, Output: state}, nil
}

func configureApplicationEnvironment(ctx context.Context, api *client.Client, id string, args ApplicationArgs) error {
	body := generated.ApplicationSaveEnvironmentJSONRequestBody{ApplicationId: id, CreateEnvFile: args.CreateEnvFile, Env: nullable.NewNullNullable[string](), BuildArgs: nullable.NewNullNullable[string](), BuildSecrets: nullable.NewNullNullable[string]()}
	if args.Environment != nil {
		body.Env = nullable.NewNullableWithValue(*args.Environment)
	}
	if args.BuildArgs != nil {
		body.BuildArgs = nullable.NewNullableWithValue(*args.BuildArgs)
	}
	if args.BuildSecrets != nil {
		body.BuildSecrets = nullable.NewNullableWithValue(*args.BuildSecrets)
	}
	_, err := api.ApplicationSaveEnvironmentWithResponse(ctx, body)
	return sanitizeApplicationError(err, args)
}

func sanitizeApplicationError(err error, args ApplicationArgs) error {
	secrets := []string{}
	if args.Environment != nil {
		secrets = append(secrets, *args.Environment)
	}
	if args.BuildArgs != nil {
		secrets = append(secrets, *args.BuildArgs)
	}
	if args.BuildSecrets != nil {
		secrets = append(secrets, *args.BuildSecrets)
	}
	if args.Source.Docker != nil && args.Source.Docker.Password != nil {
		secrets = append(secrets, *args.Source.Docker.Password)
	}
	return sanitizeError(err, secrets...)
}

func applicationStatus(ctx context.Context, api *client.Client, id string) (string, error) {
	response, err := api.ApplicationOneWithResponse(ctx, &generated.ApplicationOneParams{ApplicationId: id})
	if err != nil {
		return "", err
	}
	if response.JSON200 == nil || response.JSON200.ApplicationStatus == nil {
		return "", fmt.Errorf("application.one returned incomplete application")
	}
	return *response.JSON200.ApplicationStatus, nil
}

func (r Application) Read(ctx context.Context, req infer.ReadRequest[ApplicationArgs, ApplicationState]) (infer.ReadResponse[ApplicationArgs, ApplicationState], error) {
	response, err := r.client(ctx).ApplicationOneWithResponse(ctx, &generated.ApplicationOneParams{ApplicationId: req.ID})
	if err != nil {
		if client.IsNotFound(err) {
			return infer.ReadResponse[ApplicationArgs, ApplicationState]{ID: ""}, nil
		}
		return infer.ReadResponse[ApplicationArgs, ApplicationState]{}, err
	}
	if response.JSON200 == nil || response.JSON200.ApplicationId == nil {
		return infer.ReadResponse[ApplicationArgs, ApplicationState]{}, fmt.Errorf("application.one returned incomplete application")
	}
	a := response.JSON200
	args := req.State.ApplicationArgs
	args.Name, args.EnvironmentID = value(a.Name), value(a.EnvironmentId)
	args.AppName, args.Description, args.ServerID = a.AppName, a.Description, a.ServerId
	if a.CreateEnvFile != nil {
		args.CreateEnvFile = *a.CreateEnvFile
	}
	decoded, err := decodeApplicationSource(a.AdditionalProperties, args.Source)
	if err != nil {
		return infer.ReadResponse[ApplicationArgs, ApplicationState]{}, err
	}
	args.Source = decoded
	state := ApplicationState{ApplicationArgs: args, ApplicationID: *a.ApplicationId, Status: value(a.ApplicationStatus)}
	return infer.ReadResponse[ApplicationArgs, ApplicationState]{ID: *a.ApplicationId, Inputs: args, State: state}, nil
}

func decodeApplicationSource(m map[string]interface{}, prior ApplicationSource) (ApplicationSource, error) {
	if m == nil {
		return ApplicationSource{}, fmt.Errorf("application.one omitted source data required to reconstruct application source")
	}
	kind := stringValue(m, "type", "sourceType")
	if kind == "" {
		if prior.Type != "" {
			kind = string(prior.Type)
		} else {
			return ApplicationSource{}, fmt.Errorf("application source data does not include source.type")
		}
	}
	result := ApplicationSource{Type: ApplicationSourceType(kind)}
	switch result.Type {
	case SourceDocker:
		image := stringValue(m, "image", "dockerImage")
		if image == "" {
			return ApplicationSource{}, fmt.Errorf("application source data omits required docker image")
		}
		d := &DockerSource{Image: image, RegistryURL: stringPointer(m, "registryUrl", "registryURL"), Username: stringPointer(m, "username")}
		if prior.Type == SourceDocker && prior.Docker != nil {
			d.Password = prior.Docker.Password
		}
		result.Docker = d
	case SourceGit:
		url := stringValue(m, "url", "customGitUrl")
		branch := stringValue(m, "branch", "customGitBranch")
		if url == "" || branch == "" {
			return ApplicationSource{}, fmt.Errorf("application source data omits required git url or branch")
		}
		result.Git = &GitApplicationSource{URL: url, Branch: branch, BuildPath: stringPointer(m, "buildPath", "customGitBuildPath"), WatchPaths: stringSlice(m, "watchPaths"), EnableSubmodules: boolValue(m, "enableSubmodules")}
		result.Git.Build = decodeBuild(m)
	case SourceGitLab:
		integration := stringValue(m, "integrationId", "gitlabId")
		owner, namespace, repo, branch := stringValue(m, "owner", "gitlabOwner"), stringValue(m, "namespace", "gitlabPathNamespace"), stringValue(m, "repository", "gitlabRepository"), stringValue(m, "branch", "gitlabBranch")
		if integration == "" || owner == "" || namespace == "" || repo == "" || branch == "" {
			return ApplicationSource{}, fmt.Errorf("application source data omits required gitlab source fields")
		}
		result.GitLab = &GitLabAppSource{IntegrationID: integration, ProjectID: int(numberValue(m, "projectId", "gitlabProjectId")), Owner: owner, Namespace: namespace, Repository: repo, Branch: branch, BuildPath: stringPointer(m, "buildPath", "gitlabBuildPath"), WatchPaths: stringSlice(m, "watchPaths"), EnableSubmodules: boolValue(m, "enableSubmodules")}
		if result.GitLab.ProjectID == 0 {
			return ApplicationSource{}, fmt.Errorf("application source data omits required gitlab projectId")
		}
		result.GitLab.Build = decodeBuild(m)
	default:
		return ApplicationSource{}, fmt.Errorf("application source data has unsupported source.type %q", kind)
	}
	return result, nil
}

func decodeBuild(m map[string]interface{}) ApplicationBuild {
	b := ApplicationBuild{Type: BuildType(stringValue(m, "buildType")), Dockerfile: stringPointer(m, "dockerfile"), DockerContextPath: stringPointer(m, "dockerContextPath"), DockerBuildStage: stringPointer(m, "dockerBuildStage")}
	if b.Type == "" {
		b.Type = BuildNixpacks
	}
	return b
}
func stringValue(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok {
			return v
		}
	}
	return ""
}
func stringPointer(m map[string]interface{}, keys ...string) *string {
	v := stringValue(m, keys...)
	if v == "" {
		return nil
	}
	return &v
}
func numberValue(m map[string]interface{}, keys ...string) float64 {
	for _, k := range keys {
		switch v := m[k].(type) {
		case float64:
			return v
		case jsonNumber:
			n, _ := strconv.ParseFloat(string(v), 64)
			return n
		}
	}
	return 0
}

type jsonNumber string

func boolValue(m map[string]interface{}, key string) bool { v, _ := m[key].(bool); return v }
func stringSlice(m map[string]interface{}, key string) []string {
	values, _ := m[key].([]interface{})
	result := make([]string, 0, len(values))
	for _, v := range values {
		if s, ok := v.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

func (r Application) Update(ctx context.Context, req infer.UpdateRequest[ApplicationArgs, ApplicationState]) (infer.UpdateResponse[ApplicationState], error) {
	state := ApplicationState{ApplicationArgs: req.Inputs, ApplicationID: req.ID, Status: req.State.Status}
	if req.DryRun {
		return infer.UpdateResponse[ApplicationState]{Output: state}, nil
	}
	metadataChanged := req.Inputs.Name != req.State.Name || !sameOptionalString(req.Inputs.AppName, req.State.AppName) || !sameOptionalString(req.Inputs.Description, req.State.Description)
	runtimeChanged := !reflect.DeepEqual(req.Inputs.Source, req.State.Source) || !sameOptionalString(req.Inputs.Environment, req.State.Environment) || !sameOptionalString(req.Inputs.BuildArgs, req.State.BuildArgs) || !sameOptionalString(req.Inputs.BuildSecrets, req.State.BuildSecrets) || req.Inputs.CreateEnvFile != req.State.CreateEnvFile
	if metadataChanged {
		body := generated.ApplicationUpdateJSONRequestBody{ApplicationId: req.ID, AppName: req.Inputs.AppName, Name: &req.Inputs.Name, Description: nullable.NewNullNullable[string]()}
		if req.Inputs.Description != nil {
			body.Description = nullable.NewNullableWithValue(*req.Inputs.Description)
		}
		if _, err := r.client(ctx).ApplicationUpdateWithResponse(ctx, body); err != nil {
			return infer.UpdateResponse[ApplicationState]{}, sanitizeApplicationError(err, req.Inputs)
		}
	}
	if runtimeChanged {
		api := r.client(ctx)
		if err := configureApplicationSource(ctx, api, req.ID, req.Inputs.Source); err != nil {
			return infer.UpdateResponse[ApplicationState]{Output: state}, sanitizeApplicationError(err, req.Inputs)
		}
		if err := configureApplicationBuild(ctx, api, req.ID, req.Inputs.Source); err != nil {
			return infer.UpdateResponse[ApplicationState]{Output: state}, sanitizeApplicationError(err, req.Inputs)
		}
		if err := configureApplicationEnvironment(ctx, api, req.ID, req.Inputs); err != nil {
			return infer.UpdateResponse[ApplicationState]{Output: state}, sanitizeApplicationError(err, req.Inputs)
		}
		if _, err := api.ApplicationRedeployWithResponse(ctx, generated.ApplicationRedeployJSONRequestBody{ApplicationId: req.ID}); err != nil {
			return infer.UpdateResponse[ApplicationState]{Output: state}, sanitizeApplicationError(err, req.Inputs)
		}
		if err := waitForDone(ctx, "application", req.ID, func(ctx context.Context) (string, error) { return applicationStatus(ctx, api, req.ID) }); err != nil {
			return infer.UpdateResponse[ApplicationState]{Output: state}, sanitizeApplicationError(err, req.Inputs)
		}
		state.Status = statusDone
	}
	return infer.UpdateResponse[ApplicationState]{Output: state}, nil
}

func (r Application) Delete(ctx context.Context, req infer.DeleteRequest[ApplicationState]) (infer.DeleteResponse, error) {
	_, err := r.client(ctx).ApplicationDeleteWithResponse(ctx, generated.ApplicationDeleteJSONRequestBody{ApplicationId: req.ID})
	if client.IsNotFound(err) {
		return infer.DeleteResponse{}, nil
	}
	return infer.DeleteResponse{}, err
}

func (r Application) WireDependencies(f infer.FieldSelector, args *ApplicationArgs, state *ApplicationState) {
	deps := []infer.InputField{
		f.InputField(&args.Name), f.InputField(&args.AppName), f.InputField(&args.Description), f.InputField(&args.EnvironmentID), f.InputField(&args.ServerID), f.InputField(&args.CreateEnvFile),
	}
	f.OutputField(&state.ApplicationID).DependsOn(deps...)
	f.OutputField(&state.Status).DependsOn(deps...)
	f.OutputField(&state.Environment).DependsOn(f.InputField(&args.Environment).Secret())
	f.OutputField(&state.BuildArgs).DependsOn(f.InputField(&args.BuildArgs).Secret())
	f.OutputField(&state.BuildSecrets).DependsOn(f.InputField(&args.BuildSecrets).Secret())
	f.OutputField(&state.Source).DependsOn(f.InputField(&args.Source).Secret())
}
