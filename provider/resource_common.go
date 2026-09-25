package dokploy

import (
	"context"
	"fmt"
	"time"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/pulumi/pulumi-go-provider/infer"
)

const (
	statusDone         = "done"
	defaultComposePath = "./docker-compose.yml"
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
			case statusDone:
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

func sameOptionalBool(a, b *bool) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// preferLiveSecret adopts a value a *.one read reported, falling back to the value
// already in state. An empty string counts as "not reported" so that a resource with no
// stored environment does not turn a nil input into an empty string, which would diff
// forever. The fallback is what preserves write-only secrets Dokploy never returns.
// preferLiveSecret adopts the value Dokploy reports, falling back to what state
// already holds. An empty string counts as "not reported": Dokploy returns one for
// both an unset field and a cleared one, and treating that as a value would churn
// every nil to "" on refresh. The cost is that clearing every variable in the
// Dokploy UI does not surface as drift.
func preferLiveSecret(live, prior *string) *string {
	if live != nil && *live != "" {
		value := *live
		return &value
	}
	return prior
}

func initFailed(err error) infer.ResourceInitFailedError {
	return infer.ResourceInitFailedError{Reasons: []string{err.Error()}}
}
