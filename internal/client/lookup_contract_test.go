package client

import (
	"encoding/json"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/stretchr/testify/require"
)

func TestLookupMetadataDecodes(t *testing.T) {
	var db generated.Postgres
	require.NoError(t, json.Unmarshal([]byte(`{"postgresId":"p1","name":"db","dockerImage":"canonical","image":"legacy","applicationStatus":"done","future":{"secret":"ignored"}}`), &db))
	require.Equal(t, "legacy", *db.Image)
	image, err := db.DockerImage.Get()
	require.NoError(t, err)
	require.Equal(t, "canonical", image)
	require.Equal(t, "done", db.AdditionalProperties["applicationStatus"])
	require.Equal(t, map[string]interface{}{"secret": "ignored"}, db.AdditionalProperties["future"])
	var absent generated.Postgres
	require.NoError(t, json.Unmarshal([]byte(`{"postgresId":"p1","dockerImage":null,"applicationStatus":null}`), &absent))
	require.True(t, absent.DockerImage.IsNull())
	require.Nil(t, absent.AdditionalProperties["applicationStatus"])
	var environment generated.Environment
	require.NoError(t, json.Unmarshal([]byte(`{"environmentId":"e1","isDefault":false}`), &environment))
	require.NotNil(t, environment.IsDefault)
	require.False(t, *environment.IsDefault)
}
