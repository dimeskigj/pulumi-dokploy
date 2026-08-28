package dokploy

import (
	"net/http"
	"testing"

	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
)

func TestComposeRawCreateOrdersEnvironmentBeforeDeploy(t *testing.T) {
	old := waitPollInterval
	waitPollInterval = 0
	t.Cleanup(func() { waitPollInterval = old })
	s := newScriptedServer(t,
		expectPOST("/api/compose.create", `{"composeFile":"services: {}","composeType":"docker-compose","environmentId":"e1","name":"demo"}`, `{"composeId":"c1"}`),
		expectPOST("/api/compose.saveEnvironment", `{"composeId":"c1","env":null}`, `true`),
		expectPOST("/api/compose.deploy", `{"composeId":"c1"}`, `"running"`),
		expectGET("/api/compose.one", map[string][]string{"composeId": {"c1"}}, http.StatusOK, `{"composeId":"c1","status":"done"}`),
	)
	got, err := (Compose{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[ComposeArgs]{Inputs: ComposeArgs{Name: "demo", EnvironmentID: "e1", Source: ComposeSource{Type: ComposeSourceRaw, Raw: &RawComposeSource{ComposeFile: "services: {}"}}}})
	require.NoError(t, err)
	require.Equal(t, "c1", got.ID)
	require.Equal(t, "done", got.Output.Status)
}

func TestComposeDeleteUsesConfiguredVolumeFlag(t *testing.T) {
	s := newScriptedServer(t, expectPOST("/api/compose.delete", `{"composeId":"c1","deleteVolumes":true}`, `true`))
	_, err := (Compose{client: fixedClient(s.API())}).Delete(t.Context(), infer.DeleteRequest[ComposeState]{ID: "c1", State: ComposeState{ComposeArgs: ComposeArgs{DeleteVolumesOnDestroy: true}}})
	require.NoError(t, err)
}
