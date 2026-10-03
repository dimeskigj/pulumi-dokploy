package dokploy

import (
	"context"

	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/pulumi/pulumi-go-provider/infer"
)

type GetPostgresArgs struct {
	PostgresID string `pulumi:"postgresId"`
}

func (a *GetPostgresArgs) Annotate(n infer.Annotator) {
	n.Describe(&a.PostgresID, "The ID of the existing Dokploy Postgres database to read.")
}

type GetPostgresResult struct {
	PostgresID    string  `pulumi:"postgresId"`
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

func (o *GetPostgresResult) Annotate(n infer.Annotator) {
	n.Describe(&o.PostgresID, "The stable Dokploy Postgres ID.")
	n.Describe(&o.Name, "The Postgres database name.")
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

type GetPostgres struct{ client clientFactory }

func (r *GetPostgres) Annotate(n infer.Annotator) {
	n.SetToken("index", "getPostgres")
	n.Describe(&r, "Read non-secret metadata for an existing Dokploy Postgres database by ID without managing it.")
}
func (r GetPostgres) Invoke(ctx context.Context, req infer.FunctionRequest[GetPostgresArgs]) (infer.FunctionResponse[GetPostgresResult], error) {
	var empty infer.FunctionResponse[GetPostgresResult]
	id := req.Input.PostgresID
	if err := validateLookupID("postgresId", id); err != nil {
		return empty, err
	}
	response, err := r.client(ctx).PostgresOne(ctx, &generated.PostgresOneParams{PostgresId: id})
	obj, err := lookupDecode[lookupPostgres]("postgres.one", response, err)
	if err != nil {
		return empty, err
	}
	if err := validateLookupIdentity("postgres.one", id, obj.PostgresId, obj.Name); err != nil {
		return empty, err
	}
	return infer.FunctionResponse[GetPostgresResult]{Output: GetPostgresResult{PostgresID: id, Name: *obj.Name, Description: obj.Description, AppName: obj.AppName, EnvironmentID: obj.EnvironmentId, ServerID: obj.ServerId, DockerImage: lookupImage(obj.DockerImage, obj.Image), Status: obj.ApplicationStatus, ExternalPort: obj.ExternalPort, DatabaseName: obj.DatabaseName, DatabaseUser: obj.DatabaseUser}}, nil
}
