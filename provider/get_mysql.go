package dokploy

import (
	"context"

	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/pulumi/pulumi-go-provider/infer"
)

type GetMySQLArgs struct {
	MySQLID string `pulumi:"mysqlId"`
}

func (a *GetMySQLArgs) Annotate(n infer.Annotator) {
	n.Describe(&a.MySQLID, "The ID of the existing Dokploy MySQL database to read.")
}

type GetMySQLResult struct {
	MySQLID       string  `pulumi:"mysqlId"`
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

func (o *GetMySQLResult) Annotate(n infer.Annotator) {
	n.Describe(&o.MySQLID, "The stable Dokploy MySQL ID.")
	n.Describe(&o.Name, "The MySQL database name.")
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

type GetMySQL struct{ client clientFactory }

func (r *GetMySQL) Annotate(n infer.Annotator) {
	n.SetToken("index", "getMySQL")
	n.Describe(&r, "Read non-secret metadata for an existing Dokploy MySQL database by ID without managing it.")
}
func (r GetMySQL) Invoke(ctx context.Context, req infer.FunctionRequest[GetMySQLArgs]) (infer.FunctionResponse[GetMySQLResult], error) {
	var empty infer.FunctionResponse[GetMySQLResult]
	id := req.Input.MySQLID
	if err := validateLookupID("mysqlId", id); err != nil {
		return empty, err
	}
	response, err := r.client(ctx).MysqlOneWithResponse(ctx, &generated.MysqlOneParams{MysqlId: id})
	if err != nil {
		return empty, lookupError("mysql.one", err)
	}
	if response == nil || response.JSON200 == nil {
		return empty, lookupResponseError("mysql.one")
	}
	obj := response.JSON200
	if err := validateLookupIdentity("mysql.one", id, obj.MysqlId, obj.Name); err != nil {
		return empty, err
	}
	status, err := lookupOptionalString("mysql.one", obj.AdditionalProperties, "applicationStatus")
	if err != nil {
		return empty, err
	}
	return infer.FunctionResponse[GetMySQLResult]{Output: GetMySQLResult{MySQLID: id, Name: *obj.Name, Description: obj.Description, AppName: obj.AppName, EnvironmentID: obj.EnvironmentId, ServerID: obj.ServerId, DockerImage: lookupDatabaseImage(obj.DockerImage, obj.Image), Status: status, ExternalPort: obj.ExternalPort, DatabaseName: obj.DatabaseName, DatabaseUser: obj.DatabaseUser}}, nil
}
