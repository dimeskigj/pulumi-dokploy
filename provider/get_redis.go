package dokploy

import (
	"context"

	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/pulumi/pulumi-go-provider/infer"
)

type GetRedisArgs struct {
	RedisID string `pulumi:"redisId"`
}

func (a *GetRedisArgs) Annotate(n infer.Annotator) {
	n.Describe(&a.RedisID, "The ID of the existing Dokploy Redis database to read.")
}

type GetRedisResult struct {
	RedisID       string  `pulumi:"redisId"`
	Name          string  `pulumi:"name"`
	Description   *string `pulumi:"description,optional"`
	AppName       *string `pulumi:"appName,optional"`
	EnvironmentID *string `pulumi:"environmentId,optional"`
	ServerID      *string `pulumi:"serverId,optional"`
	DockerImage   *string `pulumi:"dockerImage,optional"`
	Status        *string `pulumi:"status,optional"`
	ExternalPort  *int    `pulumi:"externalPort,optional"`
}

func (o *GetRedisResult) Annotate(n infer.Annotator) {
	n.Describe(&o.RedisID, "The stable Dokploy Redis ID.")
	n.Describe(&o.Name, "The Redis database name.")
	n.Describe(&o.Description, "The optional description metadata.")
	n.Describe(&o.AppName, "The optional application name metadata.")
	n.Describe(&o.EnvironmentID, "The optional environment ID metadata.")
	n.Describe(&o.ServerID, "The optional server ID metadata.")
	n.Describe(&o.DockerImage, "The optional Docker image metadata.")
	n.Describe(&o.Status, "The optional application status.")
	n.Describe(&o.ExternalPort, "The optional external port.")
}

type GetRedis struct{ client clientFactory }

func (r *GetRedis) Annotate(n infer.Annotator) {
	n.SetToken("index", "getRedis")
	n.Describe(&r, "Read non-secret metadata for an existing Dokploy Redis database by ID without managing it.")
}
func (r GetRedis) Invoke(ctx context.Context, req infer.FunctionRequest[GetRedisArgs]) (infer.FunctionResponse[GetRedisResult], error) {
	var empty infer.FunctionResponse[GetRedisResult]
	id := req.Input.RedisID
	if err := validateLookupID("redisId", id); err != nil {
		return empty, err
	}
	response, err := r.client(ctx).RedisOne(ctx, &generated.RedisOneParams{RedisId: id})
	obj, err := lookupDecode[lookupRedis]("redis.one", response, err)
	if err != nil {
		return empty, err
	}
	if err := validateLookupIdentity("redis.one", id, obj.RedisId, obj.Name); err != nil {
		return empty, err
	}
	return infer.FunctionResponse[GetRedisResult]{Output: GetRedisResult{RedisID: id, Name: *obj.Name, Description: obj.Description, AppName: obj.AppName, EnvironmentID: obj.EnvironmentId, ServerID: obj.ServerId, DockerImage: lookupImage(obj.DockerImage, obj.Image), Status: obj.ApplicationStatus, ExternalPort: obj.ExternalPort}}, nil
}
