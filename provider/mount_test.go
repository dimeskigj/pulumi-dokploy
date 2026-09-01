package dokploy

import (
	"net/http"
	"testing"

	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

func TestMountCheckRequiresValidTypeAndTarget(t *testing.T) {
	checked, err := (Mount{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{
		"type": property.New("bind"), "mountPath": property.New("/data"), "hostPath": property.New("/host"), "applicationId": property.New("a1"),
	})})
	require.NoError(t, err)
	require.Empty(t, checked.Failures)
	require.Equal(t, "bind", checked.Inputs.Type)
	checked, err = (Mount{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{
		"type": property.New("unknown"), "mountPath": property.New("/data"), "applicationId": property.New("a1"),
	})})
	require.NoError(t, err)
	require.NotEmpty(t, checked.Failures)
}

func TestMountCreateReadsMountThenRedeploysTarget(t *testing.T) {
	s := newScriptedServer(t,
		expectPOST("/api/mounts.create", `{"content":null,"filePath":null,"hostPath":"/host","mountPath":"/data","serviceId":"a1","serviceType":"application","type":"bind","volumeName":null}`, `{"mountId":"m1"}`),
		expectGET("/api/mounts.one", map[string][]string{"mountId": {"m1"}}, http.StatusOK, `{"mountId":"m1","mountPath":"/data","hostPath":"/host","type":"bind","serviceType":"application","applicationId":"a1"}`),
		expectGET("/api/application.one", map[string][]string{"applicationId": {"a1"}}, http.StatusOK, `{"applicationId":"a1","applicationStatus":"done"}`),
		expectPOST("/api/application.redeploy", `{"applicationId":"a1"}`, `{}`),
		expectGET("/api/application.one", map[string][]string{"applicationId": {"a1"}}, http.StatusOK, `{"applicationId":"a1","applicationStatus":"done"}`),
	)
	r := Mount{client: fixedClient(s.API())}
	got, err := r.Create(t.Context(), infer.CreateRequest[MountArgs]{Inputs: MountArgs{Type: "bind", MountPath: "/data", HostPath: stringPtr("/host"), ApplicationID: stringPtr("a1")}})
	require.NoError(t, err)
	require.Equal(t, "m1", got.ID)
	require.Equal(t, "bind", got.Output.Type)
}
