package dokploy

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

func failureProperties(failures []p.CheckFailure) []string {
	properties := make([]string, 0, len(failures))
	for _, failure := range failures {
		properties = append(properties, failure.Property)
	}
	return properties
}

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

func TestMountCheckRejectsWrongTypeFieldsAndEmptyValues(t *testing.T) {
	checked, err := (Mount{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{
		"type": property.New("bind"), "mountPath": property.New("/data"), "hostPath": property.New(""),
		"volumeName": property.New("wrong"), "filePath": property.New("also-wrong"), "content": property.New("secret"), "applicationId": property.New("a1"),
	})})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"hostPath", "volumeName", "filePath", "content"}, failureProperties(checked.Failures))
}

func TestMountCheckAllowsExplicitEmptyFileContent(t *testing.T) {
	checked, err := (Mount{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{
		"type": property.New("file"), "mountPath": property.New("/data"), "filePath": property.New("/config"), "content": property.New(""), "applicationId": property.New("a1"),
	})})
	require.NoError(t, err)
	require.Empty(t, checked.Failures)
}

func TestMountCheckDefersTypeDependentValidationWhenComputed(t *testing.T) {
	checked, err := (Mount{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{
		"type": property.New(property.Computed), "mountPath": property.New("/data"), "applicationId": property.New(property.Computed),
	})})
	require.NoError(t, err)
	require.Empty(t, checked.Failures)
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
	got, err := r.Create(t.Context(), infer.CreateRequest[MountArgs]{Inputs: MountArgs{Type: mountTypeBind, MountPath: "/data", HostPath: stringPtr("/host"), ApplicationID: stringPtr("a1")}})
	require.NoError(t, err)
	require.Equal(t, "m1", got.ID)
	require.Equal(t, "bind", got.Output.Type)
}

func TestMountCreateMalformedReadbackRetainsPartialState(t *testing.T) {
	s := newScriptedServer(t,
		expectPOST("/api/mounts.create", `{"content":null,"filePath":null,"hostPath":"/host","mountPath":"/data","serviceId":"a1","serviceType":"application","type":"bind","volumeName":null}`, `{"mountId":"m1"}`),
		expectGET("/api/mounts.one", map[string][]string{"mountId": {"m1"}}, http.StatusOK, `{"mountId":"m1","mountPath":"/data","serviceType":"application","applicationId":"a1"}`),
	)
	r := Mount{client: fixedClient(s.API())}
	got, err := r.Create(t.Context(), infer.CreateRequest[MountArgs]{Inputs: MountArgs{Type: "bind", MountPath: "/data", HostPath: stringPtr("/host"), ApplicationID: stringPtr("a1")}})
	require.Error(t, err)
	require.Equal(t, "m1", got.ID)
	require.Equal(t, "/data", got.Output.MountPath)
}

func TestMountDeleteSanitizesEachOperationError(t *testing.T) {
	secret := "delete-secret"
	cases := []struct {
		name         string
		expectations []scriptedRequest
	}{
		{"lookup", []scriptedRequest{expectGET("/api/mounts.one", map[string][]string{"mountId": {"m1"}}, http.StatusBadRequest, `{"message":"delete-secret"}`)}},
		{"removal", []scriptedRequest{
			expectGET("/api/mounts.one", map[string][]string{"mountId": {"m1"}}, http.StatusOK, `{"mountId":"m1"}`),
			{
				Method:   http.MethodPost,
				Path:     "/api/mounts.remove",
				Body:     json.RawMessage(`{"mountId":"m1"}`),
				Status:   http.StatusBadRequest,
				Response: []byte(`{"message":"delete-secret"}`),
			},
		}},
		{"redeploy", []scriptedRequest{
			expectGET("/api/mounts.one", map[string][]string{"mountId": {"m1"}}, http.StatusOK, `{"mountId":"m1"}`),
			expectPOST("/api/mounts.remove", `{"mountId":"m1"}`, `{}`),
			expectGET("/api/application.one", map[string][]string{"applicationId": {"a1"}}, http.StatusOK, `{"applicationId":"a1","applicationStatus":"done"}`),
			{Method: http.MethodPost, Path: "/api/application.redeploy", Body: json.RawMessage(`{"applicationId":"a1"}`), Status: http.StatusBadRequest, Response: []byte(`{"message":"delete-secret"}`)},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newScriptedServer(t, tc.expectations...)
			_, err := (Mount{client: fixedClient(s.API())}).Delete(t.Context(), infer.DeleteRequest[MountState]{ID: "m1", State: MountState{MountArgs: MountArgs{ApplicationID: stringPtr("a1"), Content: &secret}}})
			require.Error(t, err)
			require.NotContains(t, err.Error(), secret)
		})
	}
}

func TestMountUpdateMalformedReadbackRetainsPartialState(t *testing.T) {
	s := newScriptedServer(t,
		expectPOST("/api/mounts.update", `{"applicationId":"a1","composeId":null,"content":null,"filePath":null,"hostPath":"/host","mariadbId":null,"mountId":"m1","mountPath":"/data","mysqlId":null,"postgresId":null,"redisId":null,"serviceType":"application","type":"bind","volumeName":null}`, `{}`),
		expectGET("/api/mounts.one", map[string][]string{"mountId": {"m1"}}, http.StatusOK, `{"mountId":"m1","mountPath":"/data","serviceType":"application","applicationId":"a1"}`),
	)
	r := Mount{client: fixedClient(s.API())}
	got, err := r.Update(t.Context(), infer.UpdateRequest[MountArgs, MountState]{ID: "m1", Inputs: MountArgs{Type: "bind", MountPath: "/data", HostPath: stringPtr("/host"), ApplicationID: stringPtr("a1")}})
	require.Error(t, err)
	require.Equal(t, "m1", got.Output.MountID)
	require.Equal(t, "/data", got.Output.MountPath)
}

func TestMountReadSanitizesCurrentAndPriorContent(t *testing.T) {
	current, prior := "current-file-content", "prior-file-content"
	s := newScriptedServer(t, expectGET("/api/mounts.one", map[string][]string{"mountId": {"m1"}}, http.StatusBadRequest, `{"message":"current-file-content prior-file-content"}`))
	_, err := (Mount{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[MountArgs, MountState]{ID: "m1", Inputs: MountArgs{Content: &current}, State: MountState{MountArgs: MountArgs{Content: &prior}}})
	require.Error(t, err)
	require.NotContains(t, err.Error(), current)
	require.NotContains(t, err.Error(), prior)
}

func TestMountUpdateSanitizesCurrentAndPriorContent(t *testing.T) {
	current, prior := "current-update-content", "prior-update-content"
	s := newScriptedServer(t, scriptedRequest{Method: http.MethodPost, Path: "/api/mounts.update", Body: json.RawMessage(`{"applicationId":"a1","composeId":null,"content":"current-update-content","filePath":"/config","hostPath":null,"mariadbId":null,"mountId":"m1","mountPath":"/data","mysqlId":null,"postgresId":null,"redisId":null,"serviceType":"application","type":"file","volumeName":null}`), Status: http.StatusBadRequest, Response: []byte(`{"message":"current-update-content prior-update-content"}`)})
	_, err := (Mount{client: fixedClient(s.API())}).Update(t.Context(), infer.UpdateRequest[MountArgs, MountState]{ID: "m1", Inputs: MountArgs{Type: "file", MountPath: "/data", FilePath: stringPtr("/config"), Content: &current, ApplicationID: stringPtr("a1")}, State: MountState{MountArgs: MountArgs{Content: &prior}}})
	require.Error(t, err)
	require.NotContains(t, err.Error(), current)
	require.NotContains(t, err.Error(), prior)
}

func TestMountUpdateClassifiesEachFailurePhaseWithoutExposingCause(t *testing.T) {
	secret := "private-host-and-payload-sentinel"
	cases := []struct {
		name         string
		expectations []scriptedRequest
		inputs       MountArgs
		want         string
	}{
		{"target validation", nil, MountArgs{Type: mountTypeBind, MountPath: "/data"}, "phase=target"},
		{"update request", []scriptedRequest{{Method: http.MethodPost, Path: "/api/mounts.update", Body: json.RawMessage(`{"applicationId":"a1","composeId":null,"content":null,"filePath":null,"hostPath":"/host","mariadbId":null,"mountId":"m1","mountPath":"/data","mysqlId":null,"postgresId":null,"redisId":null,"serviceType":"application","type":"bind","volumeName":null}`), Status: http.StatusBadRequest, Response: []byte(`{"message":""}`)}}, MountArgs{Type: mountTypeBind, MountPath: "/data", HostPath: stringPtr("/host"), ApplicationID: stringPtr("a1")}, "phase=update"},
		{"readback", []scriptedRequest{expectPOST("/api/mounts.update", `{"applicationId":"a1","composeId":null,"content":null,"filePath":null,"hostPath":"/host","mariadbId":null,"mountId":"m1","mountPath":"/data","mysqlId":null,"postgresId":null,"redisId":null,"serviceType":"application","type":"bind","volumeName":null}`, `{}`), expectGET("/api/mounts.one", map[string][]string{"mountId": {"m1"}}, http.StatusBadRequest, `{"message":"`+secret+`"}`)}, MountArgs{Type: mountTypeBind, MountPath: "/data", HostPath: stringPtr("/host"), ApplicationID: stringPtr("a1")}, "phase=readback"},
		{"redeploy", []scriptedRequest{expectPOST("/api/mounts.update", `{"applicationId":"a1","composeId":null,"content":null,"filePath":null,"hostPath":"/host","mariadbId":null,"mountId":"m1","mountPath":"/data","mysqlId":null,"postgresId":null,"redisId":null,"serviceType":"application","type":"bind","volumeName":null}`, `{}`), expectGET("/api/mounts.one", map[string][]string{"mountId": {"m1"}}, http.StatusOK, `{"mountId":"m1","mountPath":"/data","hostPath":"/host","type":"bind","serviceType":"application","applicationId":"a1"}`), expectGET("/api/application.one", map[string][]string{"applicationId": {"a1"}}, http.StatusOK, `{"applicationId":"a1","applicationStatus":"done"}`), {Method: http.MethodPost, Path: "/api/application.redeploy", Body: json.RawMessage(`{"applicationId":"a1"}`), Status: http.StatusBadRequest, Response: []byte(`{}`)}}, MountArgs{Type: mountTypeBind, MountPath: "/data", HostPath: stringPtr("/host"), ApplicationID: stringPtr("a1")}, "phase=redeploy"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var apiClient clientFactory
			if tc.expectations != nil {
				s := newScriptedServer(t, tc.expectations...)
				apiClient = fixedClient(s.API())
			}
			_, err := (Mount{client: apiClient}).Update(t.Context(), infer.UpdateRequest[MountArgs, MountState]{ID: "m1", Inputs: tc.inputs})
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.want)
			require.NotContains(t, err.Error(), secret)
		})
	}
}

type syntheticMountTimeout struct{}

func (syntheticMountTimeout) Error() string   { return "timeout secret sentinel" }
func (syntheticMountTimeout) Timeout() bool   { return true }
func (syntheticMountTimeout) Temporary() bool { return true }

func TestMountUpdateTimeoutClassificationPreservesCause(t *testing.T) {
	cause := fmt.Errorf("transport: %w", syntheticMountTimeout{})
	got := classifyMountUpdateFailure("update", cause)
	require.Contains(t, got.Error(), "status=timeout")
	require.ErrorIs(t, got, cause)
	var timeout net.Error
	require.ErrorAs(t, got, &timeout)
}

func TestMountUpdateRedeployErrorChainIsSanitizedAndClassified(t *testing.T) {
	const secret = "api-host-payload-private-sentinel"
	s := newScriptedServer(t,
		expectPOST("/api/mounts.update", `{"applicationId":"a1","composeId":null,"content":null,"filePath":null,"hostPath":"/host","mariadbId":null,"mountId":"m1","mountPath":"/data","mysqlId":null,"postgresId":null,"redisId":null,"serviceType":"application","type":"bind","volumeName":null}`, `{}`),
		expectGET("/api/mounts.one", map[string][]string{"mountId": {"m1"}}, http.StatusOK, `{"mountId":"m1","mountPath":"/data","hostPath":"/host","type":"bind","serviceType":"application","applicationId":"a1"}`),
		expectGET("/api/application.one", map[string][]string{"applicationId": {"a1"}}, http.StatusOK, `{"applicationId":"a1","applicationStatus":"done"}`),
		scriptedRequest{Method: http.MethodPost, Path: "/api/application.redeploy", Body: json.RawMessage(`{"applicationId":"a1"}`), Status: http.StatusServiceUnavailable, Response: []byte(`{"code":"BAD_REQUEST","message":"` + secret + `"}`)},
	)
	_, err := (Mount{client: fixedClient(s.API())}).Update(t.Context(), infer.UpdateRequest[MountArgs, MountState]{ID: "m1", Inputs: MountArgs{Type: mountTypeBind, MountPath: "/data", HostPath: stringPtr("/host"), ApplicationID: stringPtr("a1")}})
	require.Error(t, err)
	assertSafeMountRedeployErrorChain(t, err, secret, "a1", "/host")
	var updateFailure *mountUpdateFailure
	require.ErrorAs(t, err, &updateFailure)
	require.Equal(t, "redeploy", updateFailure.phase)
	var redeployFailure *mountRedeployFailure
	require.ErrorAs(t, err, &redeployFailure)
	require.Equal(t, mountRedeployDeploy, redeployFailure.stage)
	var apiErr *client.APIError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, http.StatusServiceUnavailable, apiErr.StatusCode)
	require.Equal(t, "BAD_REQUEST", apiErr.Code)
}
