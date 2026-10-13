package dokploy

import (
	"encoding/json"
	"net/http"
	"testing"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

func TestApplicationAutoDeployCreate(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value *bool
		want  string
	}{
		{name: "omitted"},
		{name: "true", value: boolPtr(true), want: `{"applicationId":"a1","autoDeploy":true}`},
		{name: "false", value: boolPtr(false), want: `{"applicationId":"a1","autoDeploy":false}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			oldInterval := waitPollInterval
			waitPollInterval = 0
			t.Cleanup(func() { waitPollInterval = oldInterval })
			expectations := []scriptedRequest{expectPOST("/api/application.create", `{"name":"demo","environmentId":"e1"}`, `{"applicationId":"a1"}`)}
			if tc.value != nil {
				expectations = append(expectations, expectPOST("/api/application.update", tc.want, `true`))
			}
			expectations = append(expectations,
				expectPOST("/api/application.saveDockerProvider", `{"applicationId":"a1","dockerImage":"nginx","password":"","registryUrl":"","username":""}`, `true`),
				expectPOST("/api/application.saveEnvironment", `{"applicationId":"a1","buildArgs":null,"buildSecrets":null,"createEnvFile":false,"env":null}`, `true`),
				expectPOST("/api/application.deploy", `{"applicationId":"a1"}`, `"running"`),
				expectGET("/api/application.one", map[string][]string{"applicationId": {"a1"}}, http.StatusOK, `{"applicationId":"a1","applicationStatus":"done"}`),
			)
			s := newScriptedServer(t, expectations...)
			args := ApplicationArgs{Name: "demo", EnvironmentID: "e1", AutoDeploy: tc.value, Source: ApplicationSource{Type: SourceDocker, Docker: &DockerSource{Image: "nginx"}}}
			got, err := (Application{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[ApplicationArgs]{Inputs: args})
			require.NoError(t, err)
			require.Equal(t, tc.value, got.Output.AutoDeploy)
		})
	}
}

func TestApplicationAutoDeployUpdateConfigurationOnly(t *testing.T) {
	for _, tc := range []struct {
		name       string
		old, input *bool
		metadata   bool
		want       string
	}{
		{name: "false to true", old: boolPtr(false), input: boolPtr(true), want: `{"applicationId":"a1","autoDeploy":true,"description":null,"name":"demo"}`},
		{name: "true to false", old: boolPtr(true), input: boolPtr(false), want: `{"applicationId":"a1","autoDeploy":false,"description":null,"name":"demo"}`},
		{name: "shared metadata", old: boolPtr(false), input: boolPtr(true), metadata: true, want: `{"applicationId":"a1","autoDeploy":true,"description":"updated","name":"demo"}`},
		{name: "metadata only", old: boolPtr(false), input: boolPtr(false), metadata: true, want: `{"applicationId":"a1","description":"updated","name":"demo"}`},
		{name: "omission preserves observed value", old: boolPtr(true)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var s *scriptedServer
			if tc.name == "omission preserves observed value" {
				s = newScriptedServer(t)
			} else {
				s = newScriptedServer(t, expectPOST("/api/application.update", tc.want, `true`))
			}
			oldArgs := ApplicationArgs{Name: "demo", EnvironmentID: "e1", AutoDeploy: tc.old, Source: ApplicationSource{Type: SourceDocker, Docker: &DockerSource{Image: "nginx"}}}
			newArgs := oldArgs
			newArgs.AutoDeploy = tc.input
			if tc.metadata {
				newArgs.Description = stringPtr("updated")
			}
			got, err := (Application{client: fixedClient(s.API())}).Update(t.Context(), infer.UpdateRequest[ApplicationArgs, ApplicationState]{ID: "a1", Inputs: newArgs, State: ApplicationState{ApplicationArgs: oldArgs}})
			require.NoError(t, err)
			want := tc.input
			if want == nil {
				want = tc.old
			}
			require.Equal(t, want, got.Output.AutoDeploy)
		})
	}
}

func TestApplicationAutoDeployReadAndDiff(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		prior, want    *bool
	}{
		{name: "true", response: `"autoDeploy":true`, want: boolPtr(true)},
		{name: "false", response: `"autoDeploy":false`, want: boolPtr(false)},
		{name: "null preserves prior", response: `"autoDeploy":null`, prior: boolPtr(true), want: boolPtr(true)},
		{name: "missing preserves prior", prior: boolPtr(false), want: boolPtr(false)},
		{name: "id only import", response: `"autoDeploy":false`, want: boolPtr(false)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newScriptedServer(t, expectGET("/api/application.one", map[string][]string{"applicationId": {"a1"}}, http.StatusOK, `{"applicationId":"a1","name":"demo","environmentId":"e1","applicationStatus":"done","type":"docker","dockerImage":"nginx"`+optionalComma(tc.response)+`}`))
			prior := ApplicationState{ApplicationArgs: ApplicationArgs{AutoDeploy: tc.prior}}
			got, err := (Application{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: "a1", State: prior})
			require.NoError(t, err)
			require.Equal(t, tc.want, got.Inputs.AutoDeploy)
		})
	}

	for _, tc := range []struct {
		name       string
		old, input *bool
		changed    bool
	}{
		{name: "explicit change", old: boolPtr(false), input: boolPtr(true), changed: true},
		{name: "same explicit value", old: boolPtr(true), input: boolPtr(true)},
		{name: "omission after true", old: boolPtr(true)},
		{name: "omission after false", old: boolPtr(false)},
	} {
		t.Run("diff/"+tc.name, func(t *testing.T) {
			diff, err := (Application{}).Diff(t.Context(), infer.DiffRequest[ApplicationArgs, ApplicationState]{Inputs: ApplicationArgs{Name: "demo", EnvironmentID: "e1", AutoDeploy: tc.input}, State: ApplicationState{ApplicationArgs: ApplicationArgs{Name: "demo", EnvironmentID: "e1", AutoDeploy: tc.old}}})
			require.NoError(t, err)
			require.Equal(t, tc.changed, diff.HasChanges)
			if tc.changed {
				require.Equal(t, p.Update, diff.DetailedDiff["autoDeploy"].Kind)
			}
		})
	}

	checked, err := (Application{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{
		"name":          property.New("demo"),
		"environmentId": property.New("e1"),
		"source":        property.New(map[string]property.Value{"type": property.New("docker"), "docker": property.New(map[string]property.Value{"image": property.New("nginx")})}),
		"autoDeploy":    property.New(property.Computed),
	})})
	require.NoError(t, err)
	require.Empty(t, checked.Failures)

	spec, err := p.GetSchema(t.Context(), Name, Version, Provider())
	require.NoError(t, err)
	prop := schemaProperty(spec, "dokploy:index:Application", "autoDeploy")
	require.Equal(t, "boolean", prop.Type)
	require.NotContains(t, spec.Resources["dokploy:index:Application"].RequiredInputs, "autoDeploy")
}

func TestApplicationAutoDeployPreviewAndRuntimeUpdate(t *testing.T) {
	r := Application{}
	_, err := r.Create(t.Context(), infer.CreateRequest[ApplicationArgs]{DryRun: true, Inputs: ApplicationArgs{Name: "demo", EnvironmentID: "e1", AutoDeploy: boolPtr(false)}})
	require.NoError(t, err)
	_, err = r.Update(t.Context(), infer.UpdateRequest[ApplicationArgs, ApplicationState]{DryRun: true, ID: "a1", Inputs: ApplicationArgs{Name: "demo", EnvironmentID: "e1", AutoDeploy: boolPtr(false)}})
	require.NoError(t, err)

	oldInterval := waitPollInterval
	waitPollInterval = 0
	t.Cleanup(func() { waitPollInterval = oldInterval })
	s := newScriptedServer(t,
		expectPOST("/api/application.update", `{"applicationId":"a1","autoDeploy":false,"description":null,"name":"demo"}`, `true`),
		expectPOST("/api/application.saveDockerProvider", `{"applicationId":"a1","dockerImage":"redis","password":"","registryUrl":"","username":""}`, `true`),
		expectPOST("/api/application.saveEnvironment", `{"applicationId":"a1","buildArgs":null,"buildSecrets":null,"createEnvFile":false,"env":null}`, `true`),
		expectPOST("/api/application.redeploy", `{"applicationId":"a1"}`, `"running"`),
		expectGET("/api/application.one", map[string][]string{"applicationId": {"a1"}}, http.StatusOK, `{"applicationId":"a1","applicationStatus":"done"}`),
	)
	oldArgs := ApplicationArgs{Name: "demo", EnvironmentID: "e1", AutoDeploy: boolPtr(true), Source: ApplicationSource{Type: SourceDocker, Docker: &DockerSource{Image: "nginx"}}}
	newArgs := ApplicationArgs{Name: "demo", EnvironmentID: "e1", AutoDeploy: boolPtr(false), Source: ApplicationSource{Type: SourceDocker, Docker: &DockerSource{Image: "redis"}}}
	_, err = (Application{client: fixedClient(s.API())}).Update(t.Context(), infer.UpdateRequest[ApplicationArgs, ApplicationState]{ID: "a1", Inputs: newArgs, State: ApplicationState{ApplicationArgs: oldArgs}})
	require.NoError(t, err)
}

func TestApplicationAutoDeployCreateSetupFailureRedactsSecretsAndCleansUp(t *testing.T) {
	secret := "AUTO-SECRET"
	s := newScriptedServer(t,
		expectPOST("/api/application.create", `{"name":"demo","environmentId":"e1"}`, `{"applicationId":"a1"}`),
		scriptedRequest{Method: http.MethodPost, Path: "/api/application.update", Body: json.RawMessage(`{"applicationId":"a1","autoDeploy":false}`), Status: http.StatusBadRequest, Response: []byte(`{"code":"SETUP_FAILED","message":"AUTO-SECRET"}`)},
		expectPOST("/api/application.delete", `{"applicationId":"a1"}`, `true`),
	)
	args := ApplicationArgs{Name: "demo", EnvironmentID: "e1", AutoDeploy: boolPtr(false), Environment: &secret, Source: ApplicationSource{Type: SourceDocker, Docker: &DockerSource{Image: "nginx"}}}
	got, err := (Application{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[ApplicationArgs]{Inputs: args})
	require.Error(t, err)
	require.Equal(t, "a1", got.ID)
	require.NotContains(t, err.Error(), secret)
}

func TestApplicationAutoDeployUpdateFailureRedactsPriorAndCurrentSecrets(t *testing.T) {
	oldSecret, newSecret := "PRIOR-AUTO-SECRET", "CURRENT-AUTO-SECRET"
	s := newScriptedServer(t, scriptedRequest{Method: http.MethodPost, Path: "/api/application.update", Body: json.RawMessage(`{"applicationId":"a1","autoDeploy":false,"description":null,"name":"demo"}`), Status: http.StatusBadRequest, Response: []byte(`{"code":"UPDATE_FAILED","message":"PRIOR-AUTO-SECRET CURRENT-AUTO-SECRET"}`)})
	oldArgs := ApplicationArgs{Name: "demo", EnvironmentID: "e1", AutoDeploy: boolPtr(true), Environment: &oldSecret}
	newArgs := ApplicationArgs{Name: "demo", EnvironmentID: "e1", AutoDeploy: boolPtr(false), Environment: &newSecret}
	_, err := (Application{client: fixedClient(s.API())}).Update(t.Context(), infer.UpdateRequest[ApplicationArgs, ApplicationState]{ID: "a1", Inputs: newArgs, State: ApplicationState{ApplicationArgs: oldArgs}})
	require.Error(t, err)
	require.NotContains(t, err.Error(), oldSecret)
	require.NotContains(t, err.Error(), newSecret)
}

func boolPtr(v bool) *bool { return &v }

func optionalComma(value string) string {
	if value == "" {
		return ""
	}
	return "," + value
}
