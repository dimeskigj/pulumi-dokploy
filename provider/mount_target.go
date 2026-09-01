package dokploy

import (
	"context"
	"fmt"
	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
)

type mountTarget struct {
	serviceType, serviceID string
	deploy                 func(context.Context, *client.Client) error
	status                 func(context.Context, *client.Client) (string, error)
}

func mountTargetFor(args MountArgs) (mountTarget, error) {
	type candidate struct {
		kind   string
		id     *string
		deploy func(context.Context, *client.Client) error
		status func(context.Context, *client.Client, string) (string, error)
	}
	cs := []candidate{
		{"application", args.ApplicationID, func(c context.Context, a *client.Client) error {
			_, e := a.ApplicationRedeployWithResponse(c, generated.ApplicationRedeployJSONRequestBody{ApplicationId: *args.ApplicationID})
			return e
		}, applicationStatus},
		{"compose", args.ComposeID, func(c context.Context, a *client.Client) error {
			_, e := a.ComposeRedeployWithResponse(c, generated.ComposeRedeployJSONRequestBody{ComposeId: *args.ComposeID})
			return e
		}, composeStatus},
		{"postgres", args.PostgresID, func(c context.Context, a *client.Client) error {
			_, e := a.PostgresDeployWithResponse(c, generated.PostgresDeployJSONRequestBody{PostgresId: *args.PostgresID})
			return e
		}, postgresStatus},
		{"mysql", args.MySQLID, func(c context.Context, a *client.Client) error {
			_, e := a.MysqlDeployWithResponse(c, generated.MysqlDeployJSONRequestBody{MysqlId: *args.MySQLID})
			return e
		}, mysqlStatus},
		{"mariadb", args.MariaDBID, func(c context.Context, a *client.Client) error {
			_, e := a.MariadbDeployWithResponse(c, generated.MariadbDeployJSONRequestBody{MariadbId: *args.MariaDBID})
			return e
		}, mariadbStatus},
		{"redis", args.RedisID, func(c context.Context, a *client.Client) error {
			_, e := a.RedisDeployWithResponse(c, generated.RedisDeployJSONRequestBody{RedisId: *args.RedisID})
			return e
		}, redisStatus},
	}
	var chosen *candidate
	for i := range cs {
		if cs[i].id != nil && *cs[i].id != "" {
			if chosen != nil {
				return mountTarget{}, fmt.Errorf("exactly one target ID must be set")
			}
			chosen = &cs[i]
		}
	}
	if chosen == nil {
		return mountTarget{}, fmt.Errorf("exactly one target ID must be set")
	}
	id := *chosen.id
	return mountTarget{serviceType: chosen.kind, serviceID: id, deploy: chosen.deploy, status: func(c context.Context, a *client.Client) (string, error) { return chosen.status(c, a, id) }}, nil
}

func deployMountTarget(ctx context.Context, api *client.Client, target mountTarget) (bool, error) {
	if _, err := target.status(ctx, api); err != nil {
		if client.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	if err := target.deploy(ctx, api); err != nil {
		return true, err
	}
	if err := waitForDone(ctx, target.serviceType, target.serviceID, func(c context.Context) (string, error) { return target.status(c, api) }); err != nil {
		return true, err
	}
	return true, nil
}
