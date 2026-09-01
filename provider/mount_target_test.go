package dokploy

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
