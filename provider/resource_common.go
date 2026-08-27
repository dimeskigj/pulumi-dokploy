package dokploy

import (
	"context"
	"fmt"
	"time"

	"github.com/gjorgjidimeski/pulumi-dokploy/internal/client"
	"github.com/pulumi/pulumi-go-provider/infer"
)

type clientFactory func(context.Context) *client.Client

var waitPollInterval = 2 * time.Second

func configuredClient(ctx context.Context) *client.Client {
	config := infer.GetConfig[Config](ctx)
	if config.client == nil {
		panic("Dokploy provider is not configured")
	}
	return config.client
}

func fixedClient(api *client.Client) clientFactory {
	return func(context.Context) *client.Client { return api }
}

func waitForDone(ctx context.Context, kind, id string, read func(context.Context) (string, error)) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		status, err := read(ctx)
		if err != nil {
			if !client.IsTransient(err) {
				return err
			}
		} else {
			switch status {
			case "done":
				return nil
			case "error":
				return fmt.Errorf("%s %s deployment failed", kind, id)
			case "idle", "running":
			default:
				return fmt.Errorf("%s %s deployment returned unknown status %q", kind, id, status)
			}
		}

		timer := time.NewTimer(waitPollInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func initFailed(err error) infer.ResourceInitFailedError {
	return infer.ResourceInitFailedError{Reasons: []string{err.Error()}}
}
