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

func TestBackupTargetValidation(t *testing.T) {
	for name, args := range map[string]BackupArgs{
		"none":     {Schedule: "0 0 * * *", Prefix: "p", DestinationID: "d1", Database: "app"},
		"multiple": {Schedule: "0 0 * * *", Prefix: "p", DestinationID: "d1", Database: "app", PostgresID: stringPtr("pg1"), MySQLID: stringPtr("my1")},
	} {
		t.Run(name, func(t *testing.T) {
			r := Backup{}
			checked, err := r.Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{
				"schedule": property.New(args.Schedule), "prefix": property.New(args.Prefix), "destinationId": property.New(args.DestinationID),
				"database": property.New(args.Database), "postgresId": optionalStringProperty(args.PostgresID), "mysqlId": optionalStringProperty(args.MySQLID),
			})})
			require.NoError(t, err)
			require.NotEmpty(t, checked.Failures)
		})
	}
}

func TestBackupCheckDefaultsEnabledAndValidatesRequiredFields(t *testing.T) {
	got, err := (Backup{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{
		"schedule": property.New("0 0 * * *"), "prefix": property.New("p"), "destinationId": property.New("d1"),
		"database": property.New("app"), "postgresId": property.New("pg1"),
	})})
	require.NoError(t, err)
	require.Empty(t, got.Failures)
	require.True(t, got.Inputs.Enabled)

	disabled, err := (Backup{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{
		"schedule": property.New("0 0 * * *"), "prefix": property.New("p"), "destinationId": property.New("d1"),
		"database": property.New("app"), "postgresId": property.New("pg1"), "enabled": property.New(false),
	})})
	require.NoError(t, err)
	require.False(t, disabled.Inputs.Enabled)

	empty, err := (Backup{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{})})
	require.NoError(t, err)
	require.NotEmpty(t, empty.Failures)
}

func TestBackupCheckDefersTargetValidationWhileComputed(t *testing.T) {
	checked, err := (Backup{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{
		"schedule": property.New("0 0 * * *"), "prefix": property.New("p"), "destinationId": property.New("d1"),
		"database": property.New("app"), "postgresId": property.New(property.Computed),
	})})
	require.NoError(t, err)
	require.Empty(t, checked.Failures)
}

func TestBackupDiff(t *testing.T) {
	old := BackupArgs{Schedule: "0 0 * * *", Prefix: "p", DestinationID: "d1", Database: "app", PostgresID: stringPtr("pg1")}
	in := BackupArgs{Schedule: "0 1 * * *", Prefix: "p2", DestinationID: "d2", Database: "app2", PostgresID: stringPtr("pg1"), KeepLatestCount: intPtr(3)}
	d, err := (Backup{}).Diff(t.Context(), infer.DiffRequest[BackupArgs, BackupState]{Inputs: in, State: BackupState{BackupArgs: old}})
	require.NoError(t, err)
	require.Equal(t, p.Update, d.DetailedDiff["schedule"].Kind)
	require.Equal(t, p.Update, d.DetailedDiff["destinationId"].Kind)
	require.Equal(t, p.Update, d.DetailedDiff["keepLatestCount"].Kind)
	require.NotContains(t, d.DetailedDiff, "postgresId")

	old.PostgresID, in.PostgresID, in.MySQLID = stringPtr("pg1"), nil, stringPtr("my1")
	replace, err := (Backup{}).Diff(t.Context(), infer.DiffRequest[BackupArgs, BackupState]{Inputs: in, State: BackupState{BackupArgs: old}})
	require.NoError(t, err)
	require.Equal(t, p.UpdateReplace, replace.DetailedDiff["postgresId"].Kind)
	require.Equal(t, p.UpdateReplace, replace.DetailedDiff["mysqlId"].Kind)
}

func TestBackupCreateResolvesIDFromTargetDiffAfterEmptyCreateResponse(t *testing.T) {
	s := newScriptedServer(t,
		expectGET("/api/postgres.one", map[string][]string{"postgresId": {"pg1"}}, http.StatusOK, `{"postgresId":"pg1","backups":[{"backupId":"existing"}]}`),
		expectPOST("/api/backup.create", `{"schedule":"0 0 * * *","enabled":true,"prefix":"p-","destinationId":"d1","database":"app","databaseType":"postgres","postgresId":"pg1","keepLatestCount":null}`, ``),
		expectGET("/api/postgres.one", map[string][]string{"postgresId": {"pg1"}}, http.StatusOK, `{"postgresId":"pg1","backups":[{"backupId":"existing"},{"backupId":"new1"}]}`),
	)
	got, err := (Backup{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[BackupArgs]{Inputs: BackupArgs{
		Schedule: "0 0 * * *", Enabled: true, Prefix: "p-", DestinationID: "d1", Database: "app", PostgresID: stringPtr("pg1"),
	}})
	require.NoError(t, err)
	require.Equal(t, "new1", got.ID)
}

func TestBackupCreateForEachDatabaseType(t *testing.T) {
	for _, tc := range []struct {
		databaseType, endpoint, idQuery, targetID string
		args                                      BackupArgs
	}{
		{"mysql", "/api/mysql.one", "mysqlId", "my1", BackupArgs{MySQLID: stringPtr("my1")}},
		{"mariadb", "/api/mariadb.one", "mariadbId", "ma1", BackupArgs{MariaDBID: stringPtr("ma1")}},
		{"mongo", "/api/mongo.one", "mongoId", "mo1", BackupArgs{MongoID: stringPtr("mo1")}},
	} {
		t.Run(tc.databaseType, func(t *testing.T) {
			s := newScriptedServer(t,
				expectGET(tc.endpoint, map[string][]string{tc.idQuery: {tc.targetID}}, http.StatusOK, `{"backups":[]}`),
				expectPOST("/api/backup.create", `{"schedule":"0 0 * * *","enabled":true,"prefix":"p-","destinationId":"d1","database":"app","databaseType":"`+tc.databaseType+`","`+tc.idQuery+`":"`+tc.targetID+`","keepLatestCount":null}`, ``),
				expectGET(tc.endpoint, map[string][]string{tc.idQuery: {tc.targetID}}, http.StatusOK, `{"backups":[{"backupId":"new1"}]}`),
			)
			args := tc.args
			args.Schedule, args.Enabled, args.Prefix, args.DestinationID, args.Database = "0 0 * * *", true, "p-", "d1", "app"
			got, err := (Backup{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[BackupArgs]{Inputs: args})
			require.NoError(t, err)
			require.Equal(t, "new1", got.ID)
		})
	}
}

func TestBackupCreateErrorsWhenNoNewBackupFound(t *testing.T) {
	s := newScriptedServer(t,
		expectGET("/api/postgres.one", map[string][]string{"postgresId": {"pg1"}}, http.StatusOK, `{"backups":[{"backupId":"existing"}]}`),
		expectPOST("/api/backup.create", `{"schedule":"0 0 * * *","enabled":true,"prefix":"p-","destinationId":"d1","database":"app","databaseType":"postgres","postgresId":"pg1","keepLatestCount":null}`, ``),
		expectGET("/api/postgres.one", map[string][]string{"postgresId": {"pg1"}}, http.StatusOK, `{"backups":[{"backupId":"existing"}]}`),
	)
	_, err := (Backup{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[BackupArgs]{Inputs: BackupArgs{
		Schedule: "0 0 * * *", Enabled: true, Prefix: "p-", DestinationID: "d1", Database: "app", PostgresID: stringPtr("pg1"),
	}})
	require.ErrorContains(t, err, "did not produce a new backup")
}

func TestBackupCreateErrorsWhenMultipleNewBackupsFound(t *testing.T) {
	s := newScriptedServer(t,
		expectGET("/api/postgres.one", map[string][]string{"postgresId": {"pg1"}}, http.StatusOK, `{"backups":[]}`),
		expectPOST("/api/backup.create", `{"schedule":"0 0 * * *","enabled":true,"prefix":"p-","destinationId":"d1","database":"app","databaseType":"postgres","postgresId":"pg1","keepLatestCount":null}`, ``),
		expectGET("/api/postgres.one", map[string][]string{"postgresId": {"pg1"}}, http.StatusOK, `{"backups":[{"backupId":"new1"},{"backupId":"new2"}]}`),
	)
	_, err := (Backup{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[BackupArgs]{Inputs: BackupArgs{
		Schedule: "0 0 * * *", Enabled: true, Prefix: "p-", DestinationID: "d1", Database: "app", PostgresID: stringPtr("pg1"),
	}})
	require.ErrorContains(t, err, "cannot determine which one was created")
}

func TestBackupReadReconstructsEachDatabaseType(t *testing.T) {
	for _, tc := range []struct {
		databaseType, field, expectedID string
	}{
		{"postgres", "postgresId", "pg1"}, {"mysql", "mysqlId", "my1"}, {"mariadb", "mariadbId", "ma1"}, {"mongo", "mongoId", "mo1"},
	} {
		t.Run(tc.databaseType, func(t *testing.T) {
			s := newScriptedServer(t, expectGET("/api/backup.one", map[string][]string{"backupId": {"b1"}}, http.StatusOK,
				`{"backupId":"b1","schedule":"0 0 * * *","enabled":true,"prefix":"p-","destinationId":"d1","database":"app","databaseType":"`+tc.databaseType+`","`+tc.field+`":"`+tc.expectedID+`"}`))
			read, err := (Backup{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[BackupArgs, BackupState]{ID: "b1"})
			require.NoError(t, err)
			m := map[string]*string{"postgresId": read.Inputs.PostgresID, "mysqlId": read.Inputs.MySQLID, "mariadbId": read.Inputs.MariaDBID, "mongoId": read.Inputs.MongoID}
			require.Equal(t, tc.expectedID, *m[tc.field])
		})
	}
}

func TestBackupReadPreservesPriorEnabledWhenAPIOmitsIt(t *testing.T) {
	s := newScriptedServer(t, expectGET("/api/backup.one", map[string][]string{"backupId": {"b1"}}, http.StatusOK,
		`{"backupId":"b1","schedule":"0 0 * * *","prefix":"p-","destinationId":"d1","database":"app","databaseType":"postgres","postgresId":"pg1"}`))
	read, err := (Backup{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[BackupArgs, BackupState]{ID: "b1", State: BackupState{BackupArgs: BackupArgs{Enabled: true}}})
	require.NoError(t, err)
	require.True(t, read.Inputs.Enabled)
}

func TestBackupUpdate(t *testing.T) {
	s := newScriptedServer(t,
		expectPOST("/api/backup.update", `{"backupId":"b1","schedule":"0 1 * * *","enabled":false,"prefix":"p2-","destinationId":"d2","database":"app2","databaseType":"postgres","keepLatestCount":5,"serviceName":null,"metadata":null}`, ``),
	)
	_, err := (Backup{client: fixedClient(s.API())}).Update(t.Context(), infer.UpdateRequest[BackupArgs, BackupState]{ID: "b1", Inputs: BackupArgs{
		Schedule: "0 1 * * *", Enabled: false, Prefix: "p2-", DestinationID: "d2", Database: "app2", PostgresID: stringPtr("pg1"), KeepLatestCount: intPtr(5),
	}})
	require.NoError(t, err)
}

func TestBackupReadNotFoundAndDeleteNotFound(t *testing.T) {
	s := newScriptedServer(t,
		expectGET("/api/backup.one", map[string][]string{"backupId": {"missing"}}, http.StatusNotFound, `{"code":"NOT_FOUND"}`),
		scriptedRequest{Method: http.MethodPost, Path: "/api/backup.remove", Body: json.RawMessage(`{"backupId":"missing"}`), Status: http.StatusNotFound, Response: []byte(`{"code":"NOT_FOUND"}`)},
	)
	r := Backup{client: fixedClient(s.API())}
	read, err := r.Read(t.Context(), infer.ReadRequest[BackupArgs, BackupState]{ID: "missing"})
	require.NoError(t, err)
	require.Empty(t, read.ID)
	_, err = r.Delete(t.Context(), infer.DeleteRequest[BackupState]{ID: "missing"})
	require.NoError(t, err)
}

func TestBackupProviderRegistration(t *testing.T) {
	spec, err := p.GetSchema(t.Context(), Name, Version, Provider())
	require.NoError(t, err)
	require.Contains(t, spec.Resources, "dokploy:index:Backup")
}
