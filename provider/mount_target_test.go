package dokploy

import (
	"fmt"
	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/oapi-codegen/nullable"
	"testing"

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
