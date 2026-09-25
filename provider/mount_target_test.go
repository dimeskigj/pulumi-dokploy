package dokploy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/oapi-codegen/nullable"

	"github.com/stretchr/testify/require"
)

func TestMountArgsFromReconstructsAndValidatesTarget(t *testing.T) {
	app := "a1"
	m := generated.Mount{MountId: "m1", MountPath: stringPtr("/data"), Type: stringPtr("bind"), ServiceType: stringPtr("application"), ApplicationId: nullable.NewNullableWithValue(app)}
	args, err := mountArgsFrom(&m, MountArgs{})
	require.NoError(t, err)
	require.Equal(t, "bind", args.Type)
	require.Equal(t, app, *args.ApplicationID)

	m.ServiceType = stringPtr("unsupported")
	_, err = mountArgsFrom(&m, MountArgs{})
	require.EqualError(t, err, `mounts.one returned unsupported serviceType "unsupported"`)
}

func TestMountArgsFromPreservesRedactedContent(t *testing.T) {
	content := "retained-secret"
	m := generated.Mount{MountId: "m1", MountPath: stringPtr("/data"), Type: stringPtr("file"), ServiceType: stringPtr("application"), ApplicationId: nullable.NewNullableWithValue("a1")}
	args, err := mountArgsFrom(&m, MountArgs{Content: &content})
	require.NoError(t, err)
	require.NotNil(t, args.Content)
	require.Equal(t, content, *args.Content)
}

func TestMountTargetResolvesExactlyOneTypedID(t *testing.T) {
	for _, test := range []struct {
		name, serviceType, id string
		args                  MountArgs
	}{
		{"application", "application", "a1", MountArgs{ApplicationID: stringPtr("a1")}},
		{"compose", "compose", "c1", MountArgs{ComposeID: stringPtr("c1")}},
		{"postgres", "postgres", "p1", MountArgs{PostgresID: stringPtr("p1")}},
		{"mysql", "mysql", "m1", MountArgs{MySQLID: stringPtr("m1")}},
		{"mariadb", "mariadb", "m1", MountArgs{MariaDBID: stringPtr("m1")}},
		{"redis", "redis", "r1", MountArgs{RedisID: stringPtr("r1")}},
	} {
		t.Run(test.name, func(t *testing.T) {
			target, err := mountTargetFor(test.args)
			require.NoError(t, err)
			require.Equal(t, test.serviceType, target.serviceType)
			require.Equal(t, test.id, target.serviceID)
		})
	}
}

func TestMountTargetRejectsZeroOrMultipleIDs(t *testing.T) {
	_, err := mountTargetFor(MountArgs{})
	require.EqualError(t, err, "exactly one target ID must be set")
	_, err = mountTargetFor(MountArgs{ApplicationID: stringPtr("a1"), RedisID: stringPtr("r1")})
	require.EqualError(t, err, "exactly one target ID must be set")
}

func TestMountArgsFromRejectsAmbiguousTargets(t *testing.T) {
	m := generated.Mount{MountId: "m1", MountPath: stringPtr("/data"), Type: stringPtr("bind"), ServiceType: stringPtr("application"), ApplicationId: nullable.NewNullableWithValue("a1"), RedisId: nullable.NewNullableWithValue("r1")}
	_, err := mountArgsFrom(&m, MountArgs{})
	require.EqualError(t, err, "mounts.one returned ambiguous target IDs")
}

func TestSanitizeMountErrorRedactsRetainedContent(t *testing.T) {
	content := "top-secret-content"
	err := sanitizeMountError(fmt.Errorf("request failed: %s", content), MountArgs{Content: &content})
	require.NotContains(t, err.Error(), content)
}

func TestDeployMountTargetSkipsConfirmedMissingTarget(t *testing.T) {
	s := newScriptedServer(t, expectGET("/api/application.one", map[string][]string{"applicationId": {"a1"}}, 404, `{}`))
	target, err := mountTargetFor(MountArgs{ApplicationID: stringPtr("a1")})
	require.NoError(t, err)
	exists, err := deployMountTarget(t.Context(), s.API(), target)
	require.NoError(t, err)
	require.False(t, exists)
}

func TestDeployMountTargetIdentifiesFailureStage(t *testing.T) {
	secret := "private-host-payload-error"
	for _, tc := range []struct {
		stage  mountRedeployStage
		status func(context.Context, *client.Client) (string, error)
		deploy func(context.Context, *client.Client) error
	}{
		{stage: mountRedeployPreflight, status: func(context.Context, *client.Client) (string, error) { return "", errors.New(secret) }, deploy: func(context.Context, *client.Client) error { return nil }},
		{stage: mountRedeployDeploy, status: func(context.Context, *client.Client) (string, error) { return "present", nil }, deploy: func(context.Context, *client.Client) error { return &client.APIError{StatusCode: 503, Message: secret} }},
		{stage: mountRedeployReadiness, status: func(context.Context, *client.Client) (string, error) { return "present", nil }, deploy: func(context.Context, *client.Client) error { return nil }},
	} {
		target := mountTarget{serviceType: "postgres", serviceID: "secret-id", status: tc.status, deploy: tc.deploy}
		if tc.stage == mountRedeployReadiness {
			calls := 0
			target.status = func(context.Context, *client.Client) (string, error) {
				calls++
				if calls == 1 {
					return "present", nil
				}
				return "unknown-status", errors.New(secret)
			}
		}
		_, err := deployMountTarget(context.Background(), nil, target)
		require.Error(t, err)
		var failure *mountRedeployFailure
		require.ErrorAs(t, err, &failure)
		require.Equal(t, tc.stage, failure.stage)
		require.NotContains(t, err.Error(), secret)
		require.NotContains(t, err.Error(), "secret-id")
	}
}

func TestDeployMountTargetFailureChainDoesNotExposeCause(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		wantCode   string
	}{
		{name: "safe code retained", code: "BAD_REQUEST", wantCode: "BAD_REQUEST"},
		{name: "unsafe code omitted", code: "api-code-host-secret"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const messageSecret = "api-message-host-payload-secret"
			const operationSecret = "operation-leak-sentinel"
			target := mountTarget{
				serviceType: "postgres",
				serviceID:   "private-target-id",
				status:      func(context.Context, *client.Client) (string, error) { return "present", nil },
				deploy: func(context.Context, *client.Client) error {
					return &client.APIError{StatusCode: 503, Code: tc.code, Message: messageSecret, Operation: operationSecret}
				},
			}
			_, err := deployMountTarget(context.Background(), nil, target)
			require.Error(t, err)
			secrets := []string{messageSecret, "private-target-id", operationSecret}
			if tc.wantCode == "" {
				secrets = append(secrets, tc.code)
			}
			assertSafeMountRedeployErrorChain(t, err, secrets...)
			var apiErr *client.APIError
			require.ErrorAs(t, err, &apiErr)
			require.Equal(t, 503, apiErr.StatusCode)
			require.Equal(t, tc.wantCode, apiErr.Code)
			require.Empty(t, apiErr.Message)
			require.Empty(t, apiErr.Operation)

			updateErr := classifyMountUpdateFailure("redeploy", err)
			assertSafeMountRedeployErrorChain(t, updateErr, secrets...)
			var classified *mountUpdateFailure
			require.ErrorAs(t, updateErr, &classified)
			require.Equal(t, mountUpdateStatusFailed, classified.status)
		})
	}
}

func TestDeployMountTargetFailureChainSanitizesTransportAndReadinessErrors(t *testing.T) {
	const secret = "transport-host-payload-secret"
	timeout := syntheticMountTargetNetError{message: secret, timeout: true}
	target := mountTarget{
		serviceType: "postgres",
		serviceID:   "private-readiness-id",
		status: func(_ context.Context, _ *client.Client) (string, error) {
			return "invalid-state", timeout
		},
		deploy: func(context.Context, *client.Client) error { return nil },
	}
	_, err := deployMountTarget(context.Background(), nil, target)
	require.Error(t, err)
	assertSafeMountRedeployErrorChain(t, err, secret, "private-readiness-id")
	var networkErr net.Error
	require.ErrorAs(t, err, &networkErr)
	require.True(t, networkErr.Timeout())
	updateErr := classifyMountUpdateFailure("redeploy", err)
	assertSafeMountRedeployErrorChain(t, updateErr, secret, "private-readiness-id")
	var updateFailure *mountUpdateFailure
	require.ErrorAs(t, updateErr, &updateFailure)
	require.Equal(t, mountUpdateStatusTimeout, updateFailure.status)
}

func TestDeployMountTargetFailureChainSanitizesWaitForDoneTargetID(t *testing.T) {
	calls := 0
	target := mountTarget{
		serviceType: "postgres",
		serviceID:   "wait-for-done-private-id",
		status: func(context.Context, *client.Client) (string, error) {
			calls++
			if calls == 1 {
				return "present", nil
			}
			return "invalid-target-state", nil
		},
		deploy: func(context.Context, *client.Client) error { return nil },
	}
	_, err := deployMountTarget(context.Background(), nil, target)
	require.Error(t, err)
	assertSafeMountRedeployErrorChain(t, err, "wait-for-done-private-id")
	var failure *mountRedeployFailure
	require.ErrorAs(t, err, &failure)
	require.Equal(t, mountRedeployReadiness, failure.stage)
}

func TestDeployMountTargetFailureChainPreservesContextClassification(t *testing.T) {
	target := mountTarget{
		serviceType: "postgres",
		serviceID:   "private-context-id",
		status:      func(context.Context, *client.Client) (string, error) { return "present", nil },
		deploy:      func(context.Context, *client.Client) error { return context.DeadlineExceeded },
	}
	_, err := deployMountTarget(context.Background(), nil, target)
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NotContains(t, err.Error(), "private-context-id")
}

type syntheticMountTargetNetError struct {
	message string
	timeout bool
}

func (e syntheticMountTargetNetError) Error() string   { return e.message }
func (e syntheticMountTargetNetError) Timeout() bool   { return e.timeout }
func (e syntheticMountTargetNetError) Temporary() bool { return false }

func assertSafeMountRedeployErrorChain(t *testing.T, err error, secrets ...string) {
	t.Helper()
	for current := err; current != nil; current = errors.Unwrap(current) {
		for _, secret := range secrets {
			require.NotContains(t, current.Error(), secret)
		}
	}
}
