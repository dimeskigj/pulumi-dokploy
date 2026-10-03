package dokploy

import (
	"context"

	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/pulumi/pulumi-go-provider/infer"
)

// GetSSHKeyArgs identifies an existing Dokploy SSH key.
type GetSSHKeyArgs struct {
	SSHKeyID string `pulumi:"sshKeyId"`
}

func (a *GetSSHKeyArgs) Annotate(n infer.Annotator) {
	n.Describe(&a.SSHKeyID, "The ID of the existing Dokploy SSH key to read.")
}

// GetSSHKeyResult contains only read-only, non-secret metadata.
type GetSSHKeyResult struct {
	SSHKeyID       string  `pulumi:"sshKeyId"`
	Name           string  `pulumi:"name"`
	Description    *string `pulumi:"description,optional"`
	PublicKey      *string `pulumi:"publicKey,optional"`
	OrganizationID *string `pulumi:"organizationId,optional"`
}

func (out *GetSSHKeyResult) Annotate(n infer.Annotator) {
	n.Describe(&out.SSHKeyID, "The stable Dokploy SSH key ID.")
	n.Describe(&out.Name, "The SSH key name.")
	n.Describe(&out.Description, "The optional description metadata.")
	n.Describe(&out.PublicKey, "The optional public key metadata.")
	n.Describe(&out.OrganizationID, "The optional organization ID metadata.")
}

type GetSSHKey struct{ client clientFactory }

func (r *GetSSHKey) Annotate(n infer.Annotator) {
	n.SetToken("index", "getSSHKey")
	n.Describe(&r, "Read non-secret metadata for an existing Dokploy SSH key by ID without managing it.")
}
func (r GetSSHKey) Invoke(ctx context.Context, req infer.FunctionRequest[GetSSHKeyArgs]) (infer.FunctionResponse[GetSSHKeyResult], error) {
	var empty infer.FunctionResponse[GetSSHKeyResult]
	id := req.Input.SSHKeyID
	if err := validateLookupID("sshKeyId", id); err != nil {
		return empty, err
	}
	response, err := r.client(ctx).SshKeyOne(ctx, &generated.SshKeyOneParams{SshKeyId: id})
	obj, err := lookupDecode[lookupSSHKey]("sshKey.one", response, err)
	if err != nil {
		return empty, err
	}
	if err := validateLookupIdentity("sshKey.one", id, obj.SshKeyId, obj.Name); err != nil {
		return empty, err
	}
	return infer.FunctionResponse[GetSSHKeyResult]{Output: GetSSHKeyResult{SSHKeyID: id, Name: *obj.Name, Description: obj.Description, PublicKey: obj.PublicKey, OrganizationID: obj.OrganizationId}}, nil
}
