package dokploy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
)

func TestWaitForDone(t *testing.T) {
	old := waitPollInterval
	waitPollInterval = time.Millisecond
	t.Cleanup(func() { waitPollInterval = old })
	statuses := []string{"idle", "running", "done"}
	err := waitForDone(t.Context(), "application", "a1", func(context.Context) (string, error) {
		status := statuses[0]
		statuses = statuses[1:]
		return status, nil
	})
	require.NoError(t, err)
}

func TestWaitForDoneReturnsFailure(t *testing.T) {
	require.EqualError(t, waitForDone(t.Context(), "compose", "c1", func(context.Context) (string, error) {
		return "error", nil
	}), "compose c1 deployment failed")
}

func TestWaitForDoneRejectsUnknownStatus(t *testing.T) {
	require.EqualError(t, waitForDone(t.Context(), "application", "a1", func(context.Context) (string, error) {
		return "weird", nil
	}), `application a1 deployment returned unknown status "weird"`)
}

func TestWaitForDoneReturnsNonTransientReadError(t *testing.T) {
	want := errors.New("bad request")
	require.ErrorIs(t, waitForDone(t.Context(), "application", "a1", func(context.Context) (string, error) {
		return "", want
	}), want)
}

func TestWaitForDoneRetriesTransientReadError(t *testing.T) {
	old := waitPollInterval
	waitPollInterval = time.Millisecond
	t.Cleanup(func() { waitPollInterval = old })
	attempts := 0
	err := waitForDone(t.Context(), "application", "a1", func(context.Context) (string, error) {
		attempts++
		if attempts == 1 {
			return "", &client.APIError{StatusCode: 503, Message: "busy"}
		}
		return "done", nil
	})
	require.NoError(t, err)
	require.Equal(t, 2, attempts)
}

func TestWaitForDoneHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, waitForDone(ctx, "application", "a1", func(context.Context) (string, error) {
		t.Fatal("read should not be called")
		return "", nil
	}), context.Canceled)
}

func TestInitFailed(t *testing.T) {
	err := initFailed(errors.New("could not create"))
	require.Equal(t, infer.ResourceInitFailedError{Reasons: []string{"could not create"}}, err)
}
