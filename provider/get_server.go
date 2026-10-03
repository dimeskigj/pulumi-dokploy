package dokploy

import (
	"context"

	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/pulumi/pulumi-go-provider/infer"
)

// GetServerArgs identifies an existing Dokploy server.
type GetServerArgs struct {
	ServerID string `pulumi:"serverId"`
}

func (a *GetServerArgs) Annotate(n infer.Annotator) {
	n.Describe(&a.ServerID, "The ID of the existing Dokploy server to read.")
}

// GetServerResult contains only read-only, non-secret metadata.
type GetServerResult struct {
	ServerID       string  `pulumi:"serverId"`
	Name           string  `pulumi:"name"`
	Description    *string `pulumi:"description,optional"`
	IPAddress      *string `pulumi:"ipAddress,optional"`
	Port           *int    `pulumi:"port,optional"`
	Username       *string `pulumi:"username,optional"`
	OrganizationID *string `pulumi:"organizationId,optional"`
	SSHKeyID       *string `pulumi:"sshKeyId,optional"`
	ServerType     *string `pulumi:"serverType,optional"`
	Status         *string `pulumi:"status,optional"`
}

func (out *GetServerResult) Annotate(n infer.Annotator) {
	n.Describe(&out.ServerID, "The stable Dokploy server ID.")
	n.Describe(&out.Name, "The server name.")
	n.Describe(&out.Description, "The optional description metadata.")
	n.Describe(&out.IPAddress, "The optional IP address metadata.")
	n.Describe(&out.Port, "The optional port metadata.")
	n.Describe(&out.Username, "The optional username metadata.")
	n.Describe(&out.OrganizationID, "The optional organization ID metadata.")
	n.Describe(&out.SSHKeyID, "The optional SSH key ID metadata.")
	n.Describe(&out.ServerType, "The optional server type metadata.")
	n.Describe(&out.Status, "The optional status metadata.")
}

type GetServer struct{ client clientFactory }

func (r *GetServer) Annotate(n infer.Annotator) {
	n.SetToken("index", "getServer")
	n.Describe(&r, "Read non-secret metadata for an existing Dokploy server by ID without managing it.")
}
func (r GetServer) Invoke(ctx context.Context, req infer.FunctionRequest[GetServerArgs]) (infer.FunctionResponse[GetServerResult], error) {
	var empty infer.FunctionResponse[GetServerResult]
	id := req.Input.ServerID
	if err := validateLookupID("serverId", id); err != nil {
		return empty, err
	}
	response, err := r.client(ctx).ServerOne(ctx, &generated.ServerOneParams{ServerId: id})
	obj, err := lookupDecode[lookupServer]("server.one", response, err)
	if err != nil {
		return empty, err
	}
	if err := validateLookupIdentity("server.one", id, obj.ServerId, obj.Name); err != nil {
		return empty, err
	}
	return infer.FunctionResponse[GetServerResult]{Output: GetServerResult{ServerID: id, Name: *obj.Name, Description: obj.Description, IPAddress: obj.IpAddress, Port: obj.Port, Username: obj.Username, OrganizationID: obj.OrganizationId, SSHKeyID: obj.SshKeyId, ServerType: obj.ServerType, Status: obj.ServerStatus}}, nil
}
