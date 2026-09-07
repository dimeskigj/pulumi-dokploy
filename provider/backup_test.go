package dokploy

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

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

func TestBackupObservationMatchesCreate(t *testing.T) {
	trueValue := true
	three := 3
	args := BackupArgs{
		Schedule: "0 0 * * *", Enabled: true, Prefix: "p-",
		DestinationID: "d1", Database: "app", KeepLatestCount: &three,
		PostgresID: stringPtr("pg1"),
	}
	base := backupObservation{
		ID: "b1", Valid: true, Schedule: "0 0 * * *", Enabled: &trueValue,
		Prefix: "p-", DestinationID: "d1", Database: "app",
		DatabaseType: backupDatabaseTypePostgres, TargetID: "pg1",
		KeepLatestCount: &three,
	}

	tests := []struct {
		name   string
		mutate func(*backupObservation)
		want   bool
	}{
		{name: "exact", want: true},
		{name: "other schedule", mutate: func(v *backupObservation) { v.Schedule = "0 1 * * *" }},
		{name: "other destination", mutate: func(v *backupObservation) { v.DestinationID = "d2" }},
		{name: "other database", mutate: func(v *backupObservation) { v.Database = "other" }},
		{name: "other prefix", mutate: func(v *backupObservation) { v.Prefix = "other-" }},
		{name: "other type", mutate: func(v *backupObservation) { v.DatabaseType = backupDatabaseTypeMySQL }},
		{name: "other target", mutate: func(v *backupObservation) { v.TargetID = "pg2" }},
		{name: "other enabled", mutate: func(v *backupObservation) { disabled := false; v.Enabled = &disabled }},
		{name: "other retention", mutate: func(v *backupObservation) { four := 4; v.KeepLatestCount = &four }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := base
			if test.mutate != nil {
				test.mutate(&got)
			}
			require.Equal(t, test.want, got.matchesCreate(backupDatabaseTypePostgres, "pg1", args))
		})
	}

	t.Run("omitted enabled is unknown and accepted", func(t *testing.T) {
		withoutEnabled := base
		withoutEnabled.Enabled = nil
		require.True(t, withoutEnabled.matchesCreate(backupDatabaseTypePostgres, "pg1", args))
	})
	t.Run("omitted retention only matches omitted request", func(t *testing.T) {
		withoutRetention := base
		withoutRetention.KeepLatestCount = nil
		withoutRetentionArgs := args
		withoutRetentionArgs.KeepLatestCount = nil
		require.True(t, withoutRetention.matchesCreate(backupDatabaseTypePostgres, "pg1", withoutRetentionArgs))
		require.False(t, withoutRetention.matchesCreate(backupDatabaseTypePostgres, "pg1", args))
	})
}

func TestBackupCreateResolvesIDFromTargetDiffAfterEmptyCreateResponse(t *testing.T) {
	oldPoll := backupCreatePollInterval
	backupCreatePollInterval = time.Millisecond
	t.Cleanup(func() { backupCreatePollInterval = oldPoll })
	s := newScriptedServer(t,
		expectGET("/api/postgres.one", map[string][]string{"postgresId": {"pg1"}}, http.StatusOK, `{"postgresId":"pg1","backups":[{"backupId":"existing","schedule":"0 0 1 * *","enabled":true,"prefix":"old-","destinationId":"d1","database":"old","databaseType":"postgres","postgresId":"pg1","keepLatestCount":null}]}`),
		expectPOST("/api/backup.create", `{"schedule":"0 0 * * *","enabled":true,"prefix":"p-","destinationId":"d1","database":"app","databaseType":"postgres","postgresId":"pg1","keepLatestCount":null}`, ``),
		expectGET("/api/postgres.one", map[string][]string{"postgresId": {"pg1"}}, http.StatusOK, `{"postgresId":"pg1","backups":[{"backupId":"existing","schedule":"0 0 1 * *","enabled":true,"prefix":"old-","destinationId":"d1","database":"old","databaseType":"postgres","postgresId":"pg1","keepLatestCount":null},{"backupId":"new1","schedule":"0 0 * * *","enabled":true,"prefix":"p-","destinationId":"d1","database":"app","databaseType":"postgres","postgresId":"pg1","keepLatestCount":null}]}`),
	)
	got, err := (Backup{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[BackupArgs]{Inputs: BackupArgs{
		Schedule: "0 0 * * *", Enabled: true, Prefix: "p-", DestinationID: "d1", Database: "app", PostgresID: stringPtr("pg1"),
	}})
	require.NoError(t, err)
	require.Equal(t, "new1", got.ID)
}

func TestBackupCreateForEachDatabaseType(t *testing.T) {
	oldPoll := backupCreatePollInterval
	backupCreatePollInterval = time.Millisecond
	t.Cleanup(func() { backupCreatePollInterval = oldPoll })
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
				expectGET(tc.endpoint, map[string][]string{tc.idQuery: {tc.targetID}}, http.StatusOK, `{"backups":[{"backupId":"new1","schedule":"0 0 * * *","enabled":true,"prefix":"p-","destinationId":"d1","database":"app","databaseType":"`+tc.databaseType+`","`+tc.idQuery+`":"`+tc.targetID+`","keepLatestCount":null}]}`),
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
	oldPoll := backupCreatePollInterval
	backupCreatePollInterval = 10 * time.Millisecond
	t.Cleanup(func() { backupCreatePollInterval = oldPoll })
	unchanged := `{"backups":[{"backupId":"existing","schedule":"0 0 1 * *","enabled":true,"prefix":"old-","destinationId":"d1","database":"old","databaseType":"postgres","postgresId":"pg1","keepLatestCount":null}]}`
	s := newScriptedServer(t,
		expectGET("/api/postgres.one", map[string][]string{"postgresId": {"pg1"}}, http.StatusOK, unchanged),
		expectPOST("/api/backup.create", `{"schedule":"0 0 * * *","enabled":true,"prefix":"p-","destinationId":"d1","database":"app","databaseType":"postgres","postgresId":"pg1","keepLatestCount":null}`, ``),
		expectGET("/api/postgres.one", map[string][]string{"postgresId": {"pg1"}}, http.StatusOK, unchanged),
		expectGET("/api/postgres.one", map[string][]string{"postgresId": {"pg1"}}, http.StatusOK, unchanged),
		expectGET("/api/postgres.one", map[string][]string{"postgresId": {"pg1"}}, http.StatusOK, unchanged),
	)
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Millisecond)
	defer cancel()
	_, err := (Backup{client: fixedClient(s.API())}).Create(ctx, infer.CreateRequest[BackupArgs]{Inputs: BackupArgs{
		Schedule: "0 0 * * *", Enabled: true, Prefix: "p-", DestinationID: "d1", Database: "app", PostgresID: stringPtr("pg1"),
	}})
	require.ErrorContains(t, err, "backup.create succeeded but no unique matching backup became visible")
}

func TestBackupCreateErrorsWhenMultipleNewBackupsFound(t *testing.T) {
	oldPoll := backupCreatePollInterval
	backupCreatePollInterval = time.Millisecond
	t.Cleanup(func() { backupCreatePollInterval = oldPoll })
	s := newScriptedServer(t,
		expectGET("/api/postgres.one", map[string][]string{"postgresId": {"pg1"}}, http.StatusOK, `{"backups":[]}`),
		expectPOST("/api/backup.create", `{"schedule":"0 0 * * *","enabled":true,"prefix":"p-","destinationId":"d1","database":"app","databaseType":"postgres","postgresId":"pg1","keepLatestCount":null}`, ``),
		expectGET("/api/postgres.one", map[string][]string{"postgresId": {"pg1"}}, http.StatusOK, `{"backups":[{"backupId":"new1","schedule":"0 0 * * *","enabled":true,"prefix":"p-","destinationId":"d1","database":"app","databaseType":"postgres","postgresId":"pg1","keepLatestCount":null},{"backupId":"new2","schedule":"0 0 * * *","enabled":true,"prefix":"p-","destinationId":"d1","database":"app","databaseType":"postgres","postgresId":"pg1","keepLatestCount":null}]}`),
	)
	_, err := (Backup{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[BackupArgs]{Inputs: BackupArgs{
		Schedule: "0 0 * * *", Enabled: true, Prefix: "p-", DestinationID: "d1", Database: "app", PostgresID: stringPtr("pg1"),
	}})
	require.ErrorContains(t, err, "2 matching backups")
}

const backupCreateTarget = `{"postgresId":"pg1","backups":`
const backupCreateArgsJSON = `{"schedule":"0 0 * * *","enabled":true,"prefix":"p-","destinationId":"d1","database":"app","databaseType":"postgres","postgresId":"pg1","keepLatestCount":null}`
const backupCreateExact = `{"backupId":"new1","schedule":"0 0 * * *","enabled":true,"prefix":"p-","destinationId":"d1","database":"app","databaseType":"postgres","postgresId":"pg1","keepLatestCount":null}`
const backupCreateUnrelated = `{"backupId":"other","schedule":"0 1 * * *","enabled":true,"prefix":"other-","destinationId":"d2","database":"other","databaseType":"postgres","postgresId":"pg1","keepLatestCount":null}`

func backupCreateTargetFor(targetID string) string {
	return `{"postgresId":"` + targetID + `","backups":`
}

func backupCreateRequest(t *testing.T, ctx context.Context, s *scriptedServer) (infer.CreateResponse[BackupState], error) {
	t.Helper()
	return (Backup{client: fixedClient(s.API())}).Create(ctx, infer.CreateRequest[BackupArgs]{Inputs: BackupArgs{
		Schedule: "0 0 * * *", Enabled: true, Prefix: "p-", DestinationID: "d1", Database: "app", PostgresID: stringPtr("pg1"),
	}})
}

func TestBackupCreateWaitsForDelayedVisibility(t *testing.T) {
	oldPoll := backupCreatePollInterval
	backupCreatePollInterval = time.Millisecond
	t.Cleanup(func() { backupCreatePollInterval = oldPoll })
	unchanged := backupCreateTarget + `[]}`
	s := newScriptedServer(t,
		expectGET("/api/postgres.one", map[string][]string{"postgresId": {"pg1"}}, http.StatusOK, unchanged),
		expectPOST("/api/backup.create", backupCreateArgsJSON, ``),
		expectGET("/api/postgres.one", map[string][]string{"postgresId": {"pg1"}}, http.StatusOK, unchanged),
		expectGET("/api/postgres.one", map[string][]string{"postgresId": {"pg1"}}, http.StatusOK, unchanged),
		expectGET("/api/postgres.one", map[string][]string{"postgresId": {"pg1"}}, http.StatusOK, backupCreateTarget+`[`+backupCreateExact+`]}`),
	)
	got, err := backupCreateRequest(t, t.Context(), s)
	require.NoError(t, err)
	require.Equal(t, "new1", got.ID)
}

func TestBackupCreate_IgnoresUnrelatedCandidate(t *testing.T) {
	oldPoll := backupCreatePollInterval
	backupCreatePollInterval = time.Millisecond
	t.Cleanup(func() { backupCreatePollInterval = oldPoll })
	s := newScriptedServer(t,
		expectGET("/api/postgres.one", map[string][]string{"postgresId": {"pg1"}}, http.StatusOK, backupCreateTarget+`[]}`),
		expectPOST("/api/backup.create", backupCreateArgsJSON, ``),
		expectGET("/api/postgres.one", map[string][]string{"postgresId": {"pg1"}}, http.StatusOK, backupCreateTarget+`[`+backupCreateUnrelated+`]}`),
		expectGET("/api/postgres.one", map[string][]string{"postgresId": {"pg1"}}, http.StatusOK, backupCreateTarget+`[`+backupCreateUnrelated+`,`+backupCreateExact+`]}`),
	)
	got, err := backupCreateRequest(t, t.Context(), s)
	require.NoError(t, err)
	require.Equal(t, "new1", got.ID)
}

func TestBackupCreate_SelectsUniqueMatchAmongNewBackups(t *testing.T) {
	oldPoll := backupCreatePollInterval
	backupCreatePollInterval = time.Millisecond
	t.Cleanup(func() { backupCreatePollInterval = oldPoll })
	s := newScriptedServer(t,
		expectGET("/api/postgres.one", map[string][]string{"postgresId": {"pg1"}}, http.StatusOK, backupCreateTarget+`[]}`),
		expectPOST("/api/backup.create", backupCreateArgsJSON, ``),
		expectGET("/api/postgres.one", map[string][]string{"postgresId": {"pg1"}}, http.StatusOK, backupCreateTarget+`[`+backupCreateExact+`,`+backupCreateUnrelated+`]}`),
	)
	got, err := backupCreateRequest(t, t.Context(), s)
	require.NoError(t, err)
	require.Equal(t, "new1", got.ID)
}

func TestBackupCreate_RejectsMultipleExactMatches(t *testing.T) {
	oldPoll := backupCreatePollInterval
	backupCreatePollInterval = time.Millisecond
	t.Cleanup(func() { backupCreatePollInterval = oldPoll })
	second := `{"backupId":"new2","schedule":"0 0 * * *","enabled":true,"prefix":"p-","destinationId":"d1","database":"app","databaseType":"postgres","postgresId":"pg1","keepLatestCount":null}`
	s := newScriptedServer(t,
		expectGET("/api/postgres.one", map[string][]string{"postgresId": {"pg1"}}, http.StatusOK, backupCreateTarget+`[]}`),
		expectPOST("/api/backup.create", backupCreateArgsJSON, ``),
		expectGET("/api/postgres.one", map[string][]string{"postgresId": {"pg1"}}, http.StatusOK, backupCreateTarget+`[`+backupCreateExact+`,`+second+`]}`),
	)
	got, err := backupCreateRequest(t, t.Context(), s)
	require.ErrorContains(t, err, "2 matching backups")
	require.Empty(t, got.ID)
}

func TestBackupCreateAmbiguityErrorOmitsTargetID(t *testing.T) {
	sentinel := "sentinel-target-id"
	oldPoll := backupCreatePollInterval
	backupCreatePollInterval = time.Millisecond
	t.Cleanup(func() { backupCreatePollInterval = oldPoll })
	second := strings.ReplaceAll(`{"backupId":"new2","schedule":"0 0 * * *","enabled":true,"prefix":"p-","destinationId":"d1","database":"app","databaseType":"postgres","postgresId":"pg1","keepLatestCount":null}`, "pg1", sentinel)
	exact := strings.ReplaceAll(backupCreateExact, "pg1", sentinel)
	target := backupCreateTargetFor(sentinel)
	s := newScriptedServer(t,
		expectGET("/api/postgres.one", map[string][]string{"postgresId": {sentinel}}, http.StatusOK, target+`[]}`),
		expectPOST("/api/backup.create", strings.ReplaceAll(backupCreateArgsJSON, "pg1", sentinel), ``),
		expectGET("/api/postgres.one", map[string][]string{"postgresId": {sentinel}}, http.StatusOK, target+`[`+exact+`,`+second+`]}`),
	)
	got, err := (Backup{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[BackupArgs]{Inputs: BackupArgs{
		Schedule: "0 0 * * *", Enabled: true, Prefix: "p-", DestinationID: "d1", Database: "app", PostgresID: stringPtr(sentinel),
	}})
	require.ErrorContains(t, err, "2 matching backups")
	require.NotContains(t, err.Error(), sentinel)
	require.Empty(t, got.ID)
}

func TestBackupCreate_Deadline(t *testing.T) {
	oldPoll := backupCreatePollInterval
	backupCreatePollInterval = 10 * time.Millisecond
	t.Cleanup(func() { backupCreatePollInterval = oldPoll })
	unchanged := backupCreateTarget + `[]}`
	expectations := []scriptedRequest{
		expectGET("/api/postgres.one", map[string][]string{"postgresId": {"pg1"}}, http.StatusOK, unchanged),
		expectPOST("/api/backup.create", backupCreateArgsJSON, ``),
	}
	for i := 0; i < 3; i++ {
		expectations = append(expectations, expectGET("/api/postgres.one", map[string][]string{"postgresId": {"pg1"}}, http.StatusOK, unchanged))
	}
	s := newScriptedServer(t, expectations...)
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Millisecond)
	defer cancel()
	_, err := backupCreateRequest(t, ctx, s)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.ErrorContains(t, err, "backup.create succeeded but no unique matching backup became visible")
}

func TestBackupCreateDeadlineErrorOmitsTargetID(t *testing.T) {
	sentinel := "sentinel-target-id"
	oldPoll := backupCreatePollInterval
	backupCreatePollInterval = 10 * time.Millisecond
	t.Cleanup(func() { backupCreatePollInterval = oldPoll })
	unchanged := backupCreateTargetFor(sentinel) + `[]}`
	expectations := []scriptedRequest{
		expectGET("/api/postgres.one", map[string][]string{"postgresId": {sentinel}}, http.StatusOK, unchanged),
		expectPOST("/api/backup.create", strings.ReplaceAll(backupCreateArgsJSON, "pg1", sentinel), ``),
	}
	for i := 0; i < 3; i++ {
		expectations = append(expectations, expectGET("/api/postgres.one", map[string][]string{"postgresId": {sentinel}}, http.StatusOK, unchanged))
	}
	s := newScriptedServer(t, expectations...)
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Millisecond)
	defer cancel()
	_, err := (Backup{client: fixedClient(s.API())}).Create(ctx, infer.CreateRequest[BackupArgs]{Inputs: BackupArgs{
		Schedule: "0 0 * * *", Enabled: true, Prefix: "p-", DestinationID: "d1", Database: "app", PostgresID: stringPtr(sentinel),
	}})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NotContains(t, err.Error(), sentinel)
}

func TestBackupCreate_Cancellation(t *testing.T) {
	oldPoll := backupCreatePollInterval
	backupCreatePollInterval = 10 * time.Millisecond
	t.Cleanup(func() { backupCreatePollInterval = oldPoll })
	unchanged := backupCreateTarget + `[]}`
	expectations := []scriptedRequest{
		expectGET("/api/postgres.one", map[string][]string{"postgresId": {"pg1"}}, http.StatusOK, unchanged),
		expectPOST("/api/backup.create", backupCreateArgsJSON, ``),
	}
	for i := 0; i < 3; i++ {
		expectations = append(expectations, expectGET("/api/postgres.one", map[string][]string{"postgresId": {"pg1"}}, http.StatusOK, unchanged))
	}
	s := newScriptedServer(t, expectations...)
	ctx, cancel := context.WithCancel(t.Context())
	go func() {
		time.Sleep(25 * time.Millisecond)
		cancel()
	}()
	_, err := backupCreateRequest(t, ctx, s)
	require.ErrorIs(t, err, context.Canceled)
	require.True(t, errors.Is(err, context.Canceled))
	require.ErrorContains(t, err, "backup.create succeeded but no unique matching backup became visible")
}

func TestBackupCreateCancellationErrorOmitsTargetID(t *testing.T) {
	sentinel := "sentinel-target-id"
	oldPoll := backupCreatePollInterval
	backupCreatePollInterval = 10 * time.Millisecond
	t.Cleanup(func() { backupCreatePollInterval = oldPoll })
	unchanged := backupCreateTargetFor(sentinel) + `[]}`
	expectations := []scriptedRequest{
		expectGET("/api/postgres.one", map[string][]string{"postgresId": {sentinel}}, http.StatusOK, unchanged),
		expectPOST("/api/backup.create", strings.ReplaceAll(backupCreateArgsJSON, "pg1", sentinel), ``),
	}
	for i := 0; i < 3; i++ {
		expectations = append(expectations, expectGET("/api/postgres.one", map[string][]string{"postgresId": {sentinel}}, http.StatusOK, unchanged))
	}
	s := newScriptedServer(t, expectations...)
	ctx, cancel := context.WithCancel(t.Context())
	go func() {
		time.Sleep(25 * time.Millisecond)
		cancel()
	}()
	_, err := (Backup{client: fixedClient(s.API())}).Create(ctx, infer.CreateRequest[BackupArgs]{Inputs: BackupArgs{
		Schedule: "0 0 * * *", Enabled: true, Prefix: "p-", DestinationID: "d1", Database: "app", PostgresID: stringPtr(sentinel),
	}})
	require.ErrorIs(t, err, context.Canceled)
	require.NotContains(t, err.Error(), sentinel)
}

func TestBackupCreateDoesNotAdoptMalformedPreExistingBackup(t *testing.T) {
	oldPoll := backupCreatePollInterval
	backupCreatePollInterval = 10 * time.Millisecond
	t.Cleanup(func() { backupCreatePollInterval = oldPoll })
	malformed := backupCreateTarget + `[{"backupId":"existing"}]}`
	completed := backupCreateTarget + `[{"backupId":"existing","schedule":"0 0 * * *","enabled":true,"prefix":"p-","destinationId":"d1","database":"app","databaseType":"postgres","postgresId":"pg1","keepLatestCount":null}]}`
	s := newScriptedServer(t,
		expectGET("/api/postgres.one", map[string][]string{"postgresId": {"pg1"}}, http.StatusOK, malformed),
		expectPOST("/api/backup.create", backupCreateArgsJSON, ``),
		expectGET("/api/postgres.one", map[string][]string{"postgresId": {"pg1"}}, http.StatusOK, completed),
	)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Millisecond)
	defer cancel()
	_, err := backupCreateRequest(t, ctx, s)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestBackupCreateDiscoveryErrorOmitsTargetAndAPIMessage(t *testing.T) {
	sentinelTarget := "sentinel-target-id"
	sentinelMessage := "secret server payload"
	s := newScriptedServer(t, scriptedRequest{
		Method:   http.MethodGet,
		Path:     "/api/postgres.one",
		Query:    map[string][]string{"postgresId": {sentinelTarget}},
		Status:   http.StatusBadRequest,
		Response: []byte(`{"code":"UPSTREAM-` + sentinelTarget + `","message":"` + sentinelMessage + `"}`),
	})
	_, err := (Backup{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[BackupArgs]{Inputs: BackupArgs{
		Schedule: "0 0 * * *", Enabled: true, Prefix: "p-", DestinationID: "d1", Database: "app", PostgresID: stringPtr(sentinelTarget),
	}})
	require.EqualError(t, err, "backup.create could not read target backups")
	require.NotContains(t, err.Error(), sentinelTarget)
	require.NotContains(t, err.Error(), sentinelMessage)
}

func TestBackupCreatePostCreateDiscoveryErrorIsSafe(t *testing.T) {
	sentinelTarget := "sentinel-target-id"
	sentinelMessage := "secret server payload"
	s := newScriptedServer(t,
		expectGET("/api/postgres.one", map[string][]string{"postgresId": {sentinelTarget}}, http.StatusOK, backupCreateTargetFor(sentinelTarget)+`[]}`),
		expectPOST("/api/backup.create", strings.ReplaceAll(backupCreateArgsJSON, "pg1", sentinelTarget), ""),
		scriptedRequest{
			Method: http.MethodGet, Path: "/api/postgres.one", Query: map[string][]string{"postgresId": {sentinelTarget}},
			Status: http.StatusBadRequest, Response: []byte(`{"message":"` + sentinelMessage + `"}`),
		},
	)
	_, err := (Backup{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[BackupArgs]{Inputs: BackupArgs{
		Schedule: "0 0 * * *", Enabled: true, Prefix: "p-", DestinationID: "d1", Database: "app", PostgresID: stringPtr(sentinelTarget),
	}})
	require.EqualError(t, err, "backup.create could not read target backups")
	require.NotContains(t, err.Error(), sentinelTarget)
	require.NotContains(t, err.Error(), sentinelMessage)
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
