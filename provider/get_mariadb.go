package dokploy

import (
	"context"

	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/pulumi/pulumi-go-provider/infer"
)

type GetMariaDBArgs struct {
	MariaDBID string `pulumi:"mariadbId"`
}

func (a *GetMariaDBArgs) Annotate(n infer.Annotator) {
	n.Describe(&a.MariaDBID, "The ID of the existing Dokploy MariaDB database to read.")
}

type GetMariaDBResult struct {
	MariaDBID     string  `pulumi:"mariadbId"`
	Name          string  `pulumi:"name"`
	Description   *string `pulumi:"description,optional"`
	AppName       *string `pulumi:"appName,optional"`
	EnvironmentID *string `pulumi:"environmentId,optional"`
	ServerID      *string `pulumi:"serverId,optional"`
	DockerImage   *string `pulumi:"dockerImage,optional"`
	Status        *string `pulumi:"status,optional"`
	ExternalPort  *int    `pulumi:"externalPort,optional"`
	DatabaseName  *string `pulumi:"databaseName,optional"`
	DatabaseUser  *string `pulumi:"databaseUser,optional"`
}

func (o *GetMariaDBResult) Annotate(n infer.Annotator) {
	n.Describe(&o.MariaDBID, "The stable Dokploy MariaDB ID.")
	n.Describe(&o.Name, "The MariaDB database name.")
	n.Describe(&o.Description, "The optional description metadata.")
	n.Describe(&o.AppName, "The optional application name metadata.")
	n.Describe(&o.EnvironmentID, "The optional environment ID metadata.")
	n.Describe(&o.ServerID, "The optional server ID metadata.")
	n.Describe(&o.DockerImage, "The optional Docker image metadata.")
	n.Describe(&o.Status, "The optional application status.")
	n.Describe(&o.ExternalPort, "The optional external port.")
	n.Describe(&o.DatabaseName, "The optional database name metadata.")
	n.Describe(&o.DatabaseUser, "The optional database username metadata.")
}

type GetMariaDB struct{ client clientFactory }

func (r *GetMariaDB) Annotate(n infer.Annotator) {
	n.SetToken("index", "getMariaDB")
	n.Describe(&r, "Read non-secret metadata for an existing Dokploy MariaDB database by ID without managing it.")
}
func (r GetMariaDB) Invoke(ctx context.Context, req infer.FunctionRequest[GetMariaDBArgs]) (infer.FunctionResponse[GetMariaDBResult], error) {
	var empty infer.FunctionResponse[GetMariaDBResult]
	id := req.Input.MariaDBID
	if err := validateLookupID("mariadbId", id); err != nil {
		return empty, err
	}
	response, err := r.client(ctx).MariadbOneWithResponse(ctx, &generated.MariadbOneParams{MariadbId: id})
	if err != nil {
		return empty, lookupError("mariadb.one", err)
	}
	if response == nil || response.JSON200 == nil {
		return empty, lookupResponseError("mariadb.one")
	}
	obj := response.JSON200
	if err := validateLookupIdentity("mariadb.one", id, obj.MariadbId, obj.Name); err != nil {
		return empty, err
	}
	status, err := lookupOptionalString("mariadb.one", obj.AdditionalProperties, "applicationStatus")
	if err != nil {
		return empty, err
	}
	return infer.FunctionResponse[GetMariaDBResult]{Output: GetMariaDBResult{MariaDBID: id, Name: *obj.Name, Description: obj.Description, AppName: obj.AppName, EnvironmentID: obj.EnvironmentId, ServerID: obj.ServerId, DockerImage: lookupDatabaseImage(obj.DockerImage, obj.Image), Status: status, ExternalPort: obj.ExternalPort, DatabaseName: obj.DatabaseName, DatabaseUser: obj.DatabaseUser}}, nil
}
