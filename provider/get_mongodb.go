package dokploy

import (
	"context"

	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/pulumi/pulumi-go-provider/infer"
)

type GetMongoDBArgs struct {
	MongoID string `pulumi:"mongoId"`
}

func (a *GetMongoDBArgs) Annotate(n infer.Annotator) {
	n.Describe(&a.MongoID, "The ID of the existing Dokploy MongoDB database to read.")
}

type GetMongoDBResult struct {
	MongoID       string  `pulumi:"mongoId"`
	Name          string  `pulumi:"name"`
	Description   *string `pulumi:"description,optional"`
	AppName       *string `pulumi:"appName,optional"`
	EnvironmentID *string `pulumi:"environmentId,optional"`
	ServerID      *string `pulumi:"serverId,optional"`
	DockerImage   *string `pulumi:"dockerImage,optional"`
	Status        *string `pulumi:"status,optional"`
	ExternalPort  *int    `pulumi:"externalPort,optional"`
	DatabaseUser  *string `pulumi:"databaseUser,optional"`
	ReplicaSets   *bool   `pulumi:"replicaSets,optional"`
}

func (o *GetMongoDBResult) Annotate(n infer.Annotator) {
	n.Describe(&o.MongoID, "The stable Dokploy MongoDB ID.")
	n.Describe(&o.Name, "The MongoDB database name.")
	n.Describe(&o.Description, "The optional description metadata.")
	n.Describe(&o.AppName, "The optional application name metadata.")
	n.Describe(&o.EnvironmentID, "The optional environment ID metadata.")
	n.Describe(&o.ServerID, "The optional server ID metadata.")
	n.Describe(&o.DockerImage, "The optional Docker image metadata.")
	n.Describe(&o.Status, "The optional application status.")
	n.Describe(&o.ExternalPort, "The optional external port.")
	n.Describe(&o.DatabaseUser, "The optional database username metadata.")
	n.Describe(&o.ReplicaSets, "Whether replica sets are enabled, if supplied.")
}

type GetMongoDB struct{ client clientFactory }

func (r *GetMongoDB) Annotate(n infer.Annotator) {
	n.SetToken("index", "getMongoDB")
	n.Describe(&r, "Read non-secret metadata for an existing Dokploy MongoDB database by ID without managing it.")
}
func (r GetMongoDB) Invoke(ctx context.Context, req infer.FunctionRequest[GetMongoDBArgs]) (infer.FunctionResponse[GetMongoDBResult], error) {
	var empty infer.FunctionResponse[GetMongoDBResult]
	id := req.Input.MongoID
	if err := validateLookupID("mongoId", id); err != nil {
		return empty, err
	}
	response, err := r.client(ctx).MongoOne(ctx, &generated.MongoOneParams{MongoId: id})
	obj, err := lookupDecode[lookupMongo]("mongo.one", response, err)
	if err != nil {
		return empty, err
	}
	if err := validateLookupIdentity("mongo.one", id, obj.MongoId, obj.Name); err != nil {
		return empty, err
	}
	return infer.FunctionResponse[GetMongoDBResult]{Output: GetMongoDBResult{MongoID: id, Name: *obj.Name, Description: obj.Description, AppName: obj.AppName, EnvironmentID: obj.EnvironmentId, ServerID: obj.ServerId, DockerImage: lookupImage(obj.DockerImage, obj.Image), Status: obj.ApplicationStatus, ExternalPort: obj.ExternalPort, DatabaseUser: obj.DatabaseUser, ReplicaSets: obj.ReplicaSets}}, nil
}
