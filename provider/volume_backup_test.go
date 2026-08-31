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

func TestVolumeBackupTargetValidation(t *testing.T) {
	for name, args := range map[string]VolumeBackupArgs{
		"none":                     {Name: "vb", VolumeName: "vol", Prefix: "p", DestinationID: "d1", CronExpression: "0 0 * * *"},
		"both":                     {Name: "vb", VolumeName: "vol", Prefix: "p", DestinationID: "d1", CronExpression: "0 0 * * *", ApplicationID: stringPtr("a1"), ComposeID: stringPtr("c1")},
		"compose without service":  {Name: "vb", VolumeName: "vol", Prefix: "p", DestinationID: "d1", CronExpression: "0 0 * * *", ComposeID: stringPtr("c1")},
		"service with application": {Name: "vb", VolumeName: "vol", Prefix: "p", DestinationID: "d1", CronExpression: "0 0 * * *", ApplicationID: stringPtr("a1"), ServiceName: stringPtr("web")},
	} {
		t.Run(name, func(t *testing.T) {
			r := VolumeBackup{}
			checked, err := r.Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{
				"name": property.New(args.Name), "volumeName": property.New(args.VolumeName), "prefix": property.New(args.Prefix),
				"destinationId": property.New(args.DestinationID), "cronExpression": property.New(args.CronExpression),
				"applicationId": optionalStringProperty(args.ApplicationID), "composeId": optionalStringProperty(args.ComposeID),
				"serviceName": optionalStringProperty(args.ServiceName),
			})})
			require.NoError(t, err)
			require.NotEmpty(t, checked.Failures)
		})
	}
}

func TestVolumeBackupCheckDefaultsEnabled(t *testing.T) {
	got, err := (VolumeBackup{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{
		"name": property.New("vb"), "volumeName": property.New("vol"), "prefix": property.New("p"),
		"destinationId": property.New("d1"), "cronExpression": property.New("0 0 * * *"), "applicationId": property.New("a1"),
	})})
	require.NoError(t, err)
	require.Empty(t, got.Failures)
	require.True(t, got.Inputs.Enabled)
}

func TestVolumeBackupCheckDefersTargetValidationWhileComputed(t *testing.T) {
	checked, err := (VolumeBackup{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{
		"name": property.New("vb"), "volumeName": property.New("vol"), "prefix": property.New("p"),
		"destinationId": property.New("d1"), "cronExpression": property.New("0 0 * * *"), "applicationId": property.New(property.Computed),
	})})
	require.NoError(t, err)
	require.Empty(t, checked.Failures)
}

func TestVolumeBackupDiff(t *testing.T) {
	old := VolumeBackupArgs{Name: "vb", VolumeName: "vol", Prefix: "p", DestinationID: "d1", CronExpression: "0 0 * * *", ApplicationID: stringPtr("a1")}
	in := VolumeBackupArgs{Name: "vb2", VolumeName: "vol", Prefix: "p2", DestinationID: "d2", CronExpression: "0 1 * * *", ApplicationID: stringPtr("a1"), KeepLatestCount: intPtr(3)}
	d, err := (VolumeBackup{}).Diff(t.Context(), infer.DiffRequest[VolumeBackupArgs, VolumeBackupState]{Inputs: in, State: VolumeBackupState{VolumeBackupArgs: old}})
	require.NoError(t, err)
	require.Equal(t, p.Update, d.DetailedDiff["name"].Kind)
	require.Equal(t, p.Update, d.DetailedDiff["destinationId"].Kind)
	require.Equal(t, p.Update, d.DetailedDiff["keepLatestCount"].Kind)
	require.NotContains(t, d.DetailedDiff, "applicationId")

	old.ApplicationID, in.ApplicationID, in.ComposeID, in.ServiceName = stringPtr("a1"), nil, stringPtr("c1"), stringPtr("web")
	replace, err := (VolumeBackup{}).Diff(t.Context(), infer.DiffRequest[VolumeBackupArgs, VolumeBackupState]{Inputs: in, State: VolumeBackupState{VolumeBackupArgs: old}})
	require.NoError(t, err)
	require.Equal(t, p.UpdateReplace, replace.DetailedDiff["applicationId"].Kind)
	require.Equal(t, p.UpdateReplace, replace.DetailedDiff["composeId"].Kind)
	require.Equal(t, p.UpdateReplace, replace.DetailedDiff["serviceName"].Kind)
}

func TestVolumeBackupCreateApplication(t *testing.T) {
	s := newScriptedServer(t,
		expectPOST("/api/volumeBackups.create",
			`{"name":"vb","volumeName":"vol","prefix":"p-","destinationId":"d1","cronExpression":"0 0 * * *","enabled":true,"applicationId":"a1","composeId":null,"serviceName":null,"keepLatestCount":null,"serviceType":"application"}`,
			`{"volumeBackupId":"vb1"}`),
	)
	got, err := (VolumeBackup{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[VolumeBackupArgs]{Inputs: VolumeBackupArgs{
		Name: "vb", VolumeName: "vol", Prefix: "p-", DestinationID: "d1", CronExpression: "0 0 * * *", Enabled: true, ApplicationID: stringPtr("a1"),
	}})
	require.NoError(t, err)
	require.Equal(t, "vb1", got.ID)
}

func TestVolumeBackupCreateCompose(t *testing.T) {
	s := newScriptedServer(t,
		expectPOST("/api/volumeBackups.create",
			`{"name":"vb","volumeName":"vol","prefix":"p-","destinationId":"d1","cronExpression":"0 0 * * *","enabled":true,"applicationId":null,"composeId":"c1","serviceName":"web","keepLatestCount":5,"serviceType":"compose","turnOff":true}`,
			`{"volumeBackupId":"vb1"}`),
	)
	turnOff := true
	got, err := (VolumeBackup{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[VolumeBackupArgs]{Inputs: VolumeBackupArgs{
		Name: "vb", VolumeName: "vol", Prefix: "p-", DestinationID: "d1", CronExpression: "0 0 * * *", Enabled: true,
		ComposeID: stringPtr("c1"), ServiceName: stringPtr("web"), KeepLatestCount: intPtr(5), TurnOff: &turnOff,
	}})
	require.NoError(t, err)
	require.Equal(t, "vb1", got.ID)
}

func TestVolumeBackupReadReconstructsApplicationAndCompose(t *testing.T) {
	s := newScriptedServer(t,
		expectGET("/api/volumeBackups.one", map[string][]string{"volumeBackupId": {"vb1"}}, http.StatusOK,
			`{"volumeBackupId":"vb1","name":"vb","volumeName":"vol","prefix":"p-","destinationId":"d1","cronExpression":"0 0 * * *","enabled":true,"serviceType":"application","applicationId":"a1"}`),
		expectGET("/api/volumeBackups.one", map[string][]string{"volumeBackupId": {"vb2"}}, http.StatusOK,
			`{"volumeBackupId":"vb2","name":"vb","volumeName":"vol","prefix":"p-","destinationId":"d1","cronExpression":"0 0 * * *","enabled":true,"serviceType":"compose","composeId":"c1","serviceName":"web"}`),
	)
	r := VolumeBackup{client: fixedClient(s.API())}
	app, err := r.Read(t.Context(), infer.ReadRequest[VolumeBackupArgs, VolumeBackupState]{ID: "vb1"})
	require.NoError(t, err)
	require.Equal(t, "a1", *app.Inputs.ApplicationID)
	require.Nil(t, app.Inputs.ComposeID)

	compose, err := r.Read(t.Context(), infer.ReadRequest[VolumeBackupArgs, VolumeBackupState]{ID: "vb2"})
	require.NoError(t, err)
	require.Equal(t, "c1", *compose.Inputs.ComposeID)
	require.Equal(t, "web", *compose.Inputs.ServiceName)
	require.Nil(t, compose.Inputs.ApplicationID)
}

func TestVolumeBackupReadPreservesPriorEnabledWhenAPIOmitsIt(t *testing.T) {
	s := newScriptedServer(t, expectGET("/api/volumeBackups.one", map[string][]string{"volumeBackupId": {"vb1"}}, http.StatusOK,
		`{"volumeBackupId":"vb1","name":"vb","volumeName":"vol","prefix":"p-","destinationId":"d1","cronExpression":"0 0 * * *","serviceType":"application","applicationId":"a1"}`))
	read, err := (VolumeBackup{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[VolumeBackupArgs, VolumeBackupState]{ID: "vb1", State: VolumeBackupState{VolumeBackupArgs: VolumeBackupArgs{Enabled: true}}})
	require.NoError(t, err)
	require.True(t, read.Inputs.Enabled)
}

func TestVolumeBackupUpdate(t *testing.T) {
	s := newScriptedServer(t,
		expectPOST("/api/volumeBackups.update",
			`{"volumeBackupId":"vb1","name":"vb2","volumeName":"vol","prefix":"p2-","destinationId":"d2","cronExpression":"0 1 * * *","enabled":false,"applicationId":"a1","composeId":null,"serviceName":null,"keepLatestCount":null,"serviceType":"application"}`,
			`{"volumeBackupId":"vb1"}`),
	)
	_, err := (VolumeBackup{client: fixedClient(s.API())}).Update(t.Context(), infer.UpdateRequest[VolumeBackupArgs, VolumeBackupState]{ID: "vb1", Inputs: VolumeBackupArgs{
		Name: "vb2", VolumeName: "vol", Prefix: "p2-", DestinationID: "d2", CronExpression: "0 1 * * *", Enabled: false, ApplicationID: stringPtr("a1"),
	}})
	require.NoError(t, err)
}

func TestVolumeBackupReadNotFoundAndDeleteNotFound(t *testing.T) {
	s := newScriptedServer(t,
		expectGET("/api/volumeBackups.one", map[string][]string{"volumeBackupId": {"missing"}}, http.StatusNotFound, `{"code":"NOT_FOUND"}`),
		scriptedRequest{Method: http.MethodPost, Path: "/api/volumeBackups.delete", Body: json.RawMessage(`{"volumeBackupId":"missing"}`), Status: http.StatusNotFound, Response: []byte(`{"code":"NOT_FOUND"}`)},
	)
	r := VolumeBackup{client: fixedClient(s.API())}
	read, err := r.Read(t.Context(), infer.ReadRequest[VolumeBackupArgs, VolumeBackupState]{ID: "missing"})
	require.NoError(t, err)
	require.Empty(t, read.ID)
	_, err = r.Delete(t.Context(), infer.DeleteRequest[VolumeBackupState]{ID: "missing"})
	require.NoError(t, err)
}

func TestVolumeBackupProviderRegistration(t *testing.T) {
	spec, err := p.GetSchema(t.Context(), Name, Version, Provider())
	require.NoError(t, err)
	require.Contains(t, spec.Resources, "dokploy:index:VolumeBackup")
}
