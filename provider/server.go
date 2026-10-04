package dokploy

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/oapi-codegen/nullable"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

type ServerArgs struct {
	Name                string  `pulumi:"name"`
	Description         *string `pulumi:"description,optional"`
	IPAddress           string  `pulumi:"ipAddress"`
	Port                int     `pulumi:"port,optional"`
	Username            string  `pulumi:"username,optional"`
	SSHKeyID            *string `pulumi:"sshKeyId,optional"`
	ServerType          string  `pulumi:"serverType,optional"`
	EnableDockerCleanup bool    `pulumi:"enableDockerCleanup,optional"`
}
type ServerState struct {
	ServerArgs
	ServerID       string  `pulumi:"serverId"`
	OrganizationID *string `pulumi:"organizationId,optional"`
	Status         *string `pulumi:"status,optional"`
}
type Server struct{ client clientFactory }

const (
	serverTypeDeploy = "deploy"
	serverTypeBuild  = "build"
)

func (r *Server) Annotate(a infer.Annotator) {
	a.SetToken("index", "Server")
	a.Describe(r, "A remote server record in Dokploy; does not bootstrap or destroy the remote machine.")
}
func (a *ServerArgs) Annotate(n infer.Annotator) {
	n.Describe(&a.Name, "Server name.")
	n.Describe(&a.Description, "Optional description; omission clears the description.")
	n.Describe(&a.IPAddress, "Hostname or IP address of the remote server.")
	n.Describe(&a.Port, "SSH port (1–65535); defaults to 22.")
	n.SetDefault(&a.Port, 22)
	n.Describe(&a.Username, "SSH username; defaults to root.")
	n.SetDefault(&a.Username, "root")
	n.Describe(&a.SSHKeyID, "Managed SSH key ID; omission clears the key.")
	n.Describe(&a.ServerType, "Server role: deploy or build; defaults to deploy.")
	n.SetDefault(&a.ServerType, serverTypeDeploy)
	n.Describe(&a.EnableDockerCleanup, "Enable Dokploy's recurring Docker cleanup schedule; defaults to false.")
	n.SetDefault(&a.EnableDockerCleanup, false)
}
func (s *ServerState) Annotate(n infer.Annotator) {
	n.Describe(&s.ServerID, "Stable Dokploy server ID.")
	n.Describe(&s.OrganizationID, "Owning Dokploy organization ID.")
	n.Describe(&s.Status, "Dokploy server status (not a readiness guarantee).")
}

func (r Server) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[ServerArgs], error) {
	a, failures, err := infer.DefaultCheck[ServerArgs](ctx, req.NewInputs)
	if err != nil || len(failures) > 0 {
		return infer.CheckResponse[ServerArgs]{Inputs: a, Failures: failures}, err
	}
	add := func(k, msg string) {
		if !req.NewInputs.Get(k).HasComputed() {
			failures = append(failures, p.CheckFailure{Property: k, Reason: msg})
		}
	}
	for _, k := range []string{"username", "serverType"} {
		if v, ok := req.NewInputs.GetOk(k); ok && v.IsString() && v.AsString() == "" {
			add(k, k+" must not be blank")
		}
	}
	if v, ok := req.NewInputs.GetOk("port"); ok && v.IsNumber() && v.AsNumber() == 0 {
		add("port", "port must be between 1 and 65535")
	}
	for _, f := range []struct{ k, v string }{{"name", a.Name}, {"ipAddress", a.IPAddress}, {"username", a.Username}} {
		if strings.TrimSpace(f.v) == "" {
			add(f.k, f.k+" must not be blank")
		}
	}
	if a.SSHKeyID != nil && strings.TrimSpace(*a.SSHKeyID) == "" {
		add("sshKeyId", "sshKeyId must not be blank")
	}
	if a.Port < 1 || a.Port > 65535 {
		add("port", "port must be between 1 and 65535")
	}
	if a.ServerType != serverTypeDeploy && a.ServerType != serverTypeBuild {
		add("serverType", "serverType must be deploy or build")
	}
	return infer.CheckResponse[ServerArgs]{Inputs: a, Failures: failures}, nil
}
func (r Server) Diff(_ context.Context, req infer.DiffRequest[ServerArgs, ServerState]) (infer.DiffResponse, error) {
	a, b := req.Inputs, req.State.ServerArgs
	d := map[string]p.PropertyDiff{}
	for _, field := range []struct {
		k       string
		changed bool
	}{{"name", a.Name != b.Name}, {"description", !sameOptionalString(a.Description, b.Description)}, {"ipAddress", a.IPAddress != b.IPAddress}, {"port", a.Port != b.Port}, {"username", a.Username != b.Username}, {"sshKeyId", !sameOptionalString(a.SSHKeyID, b.SSHKeyID)}, {"serverType", a.ServerType != b.ServerType}, {"enableDockerCleanup", a.EnableDockerCleanup != b.EnableDockerCleanup}} {
		if field.changed {
			d[field.k] = p.PropertyDiff{Kind: p.Update}
		}
	}
	return infer.DiffResponse{HasChanges: len(d) > 0, DetailedDiff: d}, nil
}
func (r Server) Create(ctx context.Context, req infer.CreateRequest[ServerArgs]) (infer.CreateResponse[ServerState], error) {
	s := ServerState{ServerArgs: req.Inputs}
	if req.DryRun {
		return infer.CreateResponse[ServerState]{Output: s}, nil
	}
	a := req.Inputs
	api := r.client(ctx)
	resp, err := api.ServerCreateWithResponse(ctx, generated.ServerCreateJSONRequestBody{Name: a.Name, Description: serverNullable(a.Description), IpAddress: a.IPAddress, Port: a.Port, Username: a.Username, SshKeyId: serverNullable(a.SSHKeyID), ServerType: generated.ServerCreateRequestServerType(a.ServerType), EnableDockerCleanup: &a.EnableDockerCleanup})
	if err != nil {
		return infer.CreateResponse[ServerState]{}, fmt.Errorf("server.create failed: %w", safeServerError(err))
	}
	if resp.JSON200 == nil || strings.TrimSpace(resp.JSON200.ServerId) == "" {
		return infer.CreateResponse[ServerState]{}, errors.New("server.create returned no acknowledged server identity; inspect the server before retrying")
	}
	id := resp.JSON200.ServerId
	s.ServerID = id
	observed, err := readServer(ctx, api, id)
	if err != nil {
		return infer.CreateResponse[ServerState]{ID: id, Output: s}, initFailed(fmt.Errorf("server.create acknowledged but readback failed: %w", safeServerError(err)))
	}
	return infer.CreateResponse[ServerState]{ID: id, Output: observed}, nil
}
func (r Server) Read(ctx context.Context, req infer.ReadRequest[ServerArgs, ServerState]) (infer.ReadResponse[ServerArgs, ServerState], error) {
	s, err := readServer(ctx, r.client(ctx), req.ID)
	if client.IsNotFound(err) {
		return infer.ReadResponse[ServerArgs, ServerState]{}, nil
	}
	if err != nil {
		return infer.ReadResponse[ServerArgs, ServerState]{}, fmt.Errorf("server.one failed: %w", safeServerError(err))
	}
	return infer.ReadResponse[ServerArgs, ServerState]{ID: req.ID, Inputs: s.ServerArgs, State: s}, nil
}
func (r Server) Update(ctx context.Context, req infer.UpdateRequest[ServerArgs, ServerState]) (infer.UpdateResponse[ServerState], error) {
	s := ServerState{ServerArgs: req.Inputs, ServerID: req.ID, OrganizationID: req.State.OrganizationID, Status: req.State.Status}
	if req.DryRun {
		return infer.UpdateResponse[ServerState]{Output: s}, nil
	}
	a := req.Inputs
	api := r.client(ctx)
	resp, err := api.ServerUpdateWithResponse(ctx, generated.ServerUpdateJSONRequestBody{ServerId: req.ID, Name: a.Name, Description: serverNullable(a.Description), IpAddress: a.IPAddress, Port: a.Port, Username: a.Username, SshKeyId: serverNullable(a.SSHKeyID), ServerType: generated.ServerUpdateRequestServerType(a.ServerType), EnableDockerCleanup: &a.EnableDockerCleanup})
	if err != nil {
		return infer.UpdateResponse[ServerState]{Output: req.State}, fmt.Errorf("server.update failed: %w", safeServerError(err))
	}
	if resp.JSON200 == nil || resp.JSON200.ServerId != req.ID {
		return infer.UpdateResponse[ServerState]{Output: req.State}, errors.New("server.update returned an unconfirmed server identity")
	}
	observed, err := readServer(ctx, api, req.ID)
	if err != nil {
		return infer.UpdateResponse[ServerState]{Output: s}, initFailed(fmt.Errorf("server.update acknowledged but readback failed: %w", safeServerError(err)))
	}
	return infer.UpdateResponse[ServerState]{Output: observed}, nil
}
func (r Server) Delete(ctx context.Context, req infer.DeleteRequest[ServerState]) (infer.DeleteResponse, error) {
	api := r.client(ctx)
	_, err := api.ServerRemoveWithResponse(ctx, generated.ServerRemoveJSONRequestBody{ServerId: req.ID})
	if client.IsNotFound(err) {
		_, checkErr := readServer(ctx, api, req.ID)
		if client.IsNotFound(checkErr) {
			return infer.DeleteResponse{}, nil
		}
		if checkErr == nil {
			return infer.DeleteResponse{}, errors.New("server.remove reported absence but server.one found the record")
		}
		return infer.DeleteResponse{}, fmt.Errorf("server.remove absence could not be confirmed: %w", safeServerError(checkErr))
	}
	if err != nil {
		return infer.DeleteResponse{}, fmt.Errorf("server.remove failed: %w", safeServerError(err))
	}
	return infer.DeleteResponse{}, nil
}
func (r Server) WireDependencies(f infer.FieldSelector, a *ServerArgs, s *ServerState) {
	f.OutputField(&s.ServerID).DependsOn(f.InputField(&a.Name).Computed())
	f.OutputField(&s.OrganizationID).DependsOn(f.InputField(&a.Name).Computed())
	f.OutputField(&s.Status).DependsOn(f.InputField(&a.Name).Computed())
}

func serverNullable(v *string) nullable.Nullable[string] {
	if v == nil {
		return nullable.NewNullNullable[string]()
	}
	return nullable.NewNullableWithValue(*v)
}
func safeServerError(err error) error {
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
	return errors.New("request failed; upstream details withheld")
}
func readServer(ctx context.Context, api *client.Client, id string) (ServerState, error) {
	resp, err := api.ServerOneWithResponse(ctx, &generated.ServerOneParams{ServerId: id})
	if err != nil {
		return ServerState{}, err
	}
	v := resp.JSON200
	if v == nil || strings.TrimSpace(v.ServerId) == "" || v.ServerId != id {
		return ServerState{}, errors.New("server.one returned an incomplete or mismatched server identity")
	}
	if v.Name == nil || v.IpAddress == nil || v.Port == nil || v.Username == nil || v.ServerType == nil || v.EnableDockerCleanup == nil || v.OrganizationId == nil || v.ServerStatus == nil {
		return ServerState{}, errors.New("server.one returned incomplete server configuration or metadata")
	}
	a := ServerArgs{Name: *v.Name, Description: nullableValue(v.Description), IPAddress: *v.IpAddress, Port: *v.Port, Username: *v.Username, SSHKeyID: nullableValue(v.SshKeyId), ServerType: *v.ServerType, EnableDockerCleanup: *v.EnableDockerCleanup}
	return ServerState{ServerArgs: a, ServerID: id, OrganizationID: v.OrganizationId, Status: v.ServerStatus}, nil
}
