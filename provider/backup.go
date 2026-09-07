package dokploy

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/oapi-codegen/nullable"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

const (
	backupDatabaseTypePostgres = "postgres"
	backupDatabaseTypeMySQL    = "mysql"
	backupDatabaseTypeMariaDB  = "mariadb"
	backupDatabaseTypeMongo    = "mongo"
)

type BackupArgs struct {
	Schedule        string  `pulumi:"schedule"`
	Enabled         bool    `pulumi:"enabled,optional"`
	Prefix          string  `pulumi:"prefix"`
	DestinationID   string  `pulumi:"destinationId"`
	Database        string  `pulumi:"database"`
	KeepLatestCount *int    `pulumi:"keepLatestCount,optional"`
	PostgresID      *string `pulumi:"postgresId,optional" provider:"replaceOnChanges"`
	MySQLID         *string `pulumi:"mysqlId,optional" provider:"replaceOnChanges"`
	MariaDBID       *string `pulumi:"mariadbId,optional" provider:"replaceOnChanges"`
	MongoID         *string `pulumi:"mongoId,optional" provider:"replaceOnChanges"`
}

type BackupState struct {
	BackupArgs
	BackupID string `pulumi:"backupId"`
}

func (s *BackupState) Annotate(a infer.Annotator) {
	a.Describe(&s.BackupID, "The stable Dokploy backup ID.")
}

type Backup struct{ client clientFactory }

func (r *Backup) Annotate(a infer.Annotator) {
	a.SetToken("index", "Backup")
	a.Describe(&r, "A scheduled Dokploy database backup. Exactly one of postgresId, mysqlId, mariadbId, or mongoId must be set.")
}
func (a *BackupArgs) Annotate(annotator infer.Annotator) {
	annotator.Describe(&a.Schedule, "The backup cron schedule.")
	annotator.Describe(&a.Enabled, "Whether the backup schedule is enabled.")
	annotator.Describe(&a.Prefix, "The backup file prefix.")
	annotator.Describe(&a.DestinationID, "The destination ID backups are stored to.")
	annotator.Describe(&a.Database, "The database name inside the target instance to back up.")
	annotator.Describe(&a.KeepLatestCount, "The optional number of most recent backups to retain.")
	annotator.Describe(&a.PostgresID, "The target Postgres ID.")
	annotator.Describe(&a.MySQLID, "The target MySQL ID.")
	annotator.Describe(&a.MariaDBID, "The target MariaDB ID.")
	annotator.Describe(&a.MongoID, "The target MongoDB ID.")
	annotator.SetDefault(&a.Enabled, true)
}

func (a BackupArgs) validate() error {
	set := 0
	for _, id := range []*string{a.PostgresID, a.MySQLID, a.MariaDBID, a.MongoID} {
		if id != nil && *id != "" {
			set++
		}
	}
	if set != 1 {
		return fmt.Errorf("exactly one of postgresId, mysqlId, mariadbId, or mongoId must be set")
	}
	return nil
}

func (a BackupArgs) databaseType() (string, string, error) {
	switch {
	case a.PostgresID != nil && *a.PostgresID != "":
		return backupDatabaseTypePostgres, *a.PostgresID, nil
	case a.MySQLID != nil && *a.MySQLID != "":
		return backupDatabaseTypeMySQL, *a.MySQLID, nil
	case a.MariaDBID != nil && *a.MariaDBID != "":
		return backupDatabaseTypeMariaDB, *a.MariaDBID, nil
	case a.MongoID != nil && *a.MongoID != "":
		return backupDatabaseTypeMongo, *a.MongoID, nil
	default:
		return "", "", fmt.Errorf("exactly one of postgresId, mysqlId, mariadbId, or mongoId must be set")
	}
}

func (r Backup) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[BackupArgs], error) {
	_, explicitEnabled := req.NewInputs.AsMap()["enabled"]
	var explicitEnabledValue bool
	if explicitEnabled {
		explicitEnabledValue = req.NewInputs.Get("enabled").AsBool()
	}
	in, failures, err := infer.DefaultCheck[BackupArgs](ctx, req.NewInputs)
	if err != nil || len(failures) != 0 {
		return infer.CheckResponse[BackupArgs]{Inputs: in, Failures: failures}, err
	}
	if !explicitEnabled {
		in.Enabled = true
	} else {
		in.Enabled = explicitEnabledValue
	}
	for _, field := range []struct {
		name, value string
	}{{"schedule", in.Schedule}, {"prefix", in.Prefix}, {"destinationId", in.DestinationID}, {"database", in.Database}} {
		if field.value == "" && !req.NewInputs.Get(field.name).HasComputed() {
			failures = append(failures, p.CheckFailure{Property: field.name, Reason: fmt.Sprintf("%s must not be empty", field.name)})
		}
	}
	targetComputed := false
	for _, name := range []string{"postgresId", "mysqlId", "mariadbId", "mongoId"} {
		if req.NewInputs.Get(name).HasComputed() {
			targetComputed = true
		}
	}
	if !targetComputed {
		if err := in.validate(); err != nil {
			failures = append(failures, p.CheckFailure{Property: "target", Reason: err.Error()})
		}
	}
	return infer.CheckResponse[BackupArgs]{Inputs: in, Failures: failures}, nil
}

func (r Backup) Diff(_ context.Context, req infer.DiffRequest[BackupArgs, BackupState]) (infer.DiffResponse, error) {
	in, old := req.Inputs, req.State.BackupArgs
	d := map[string]p.PropertyDiff{}
	for _, field := range []struct {
		name    string
		changed bool
	}{
		{"postgresId", !sameOptionalString(in.PostgresID, old.PostgresID)}, {"mysqlId", !sameOptionalString(in.MySQLID, old.MySQLID)},
		{"mariadbId", !sameOptionalString(in.MariaDBID, old.MariaDBID)}, {"mongoId", !sameOptionalString(in.MongoID, old.MongoID)},
	} {
		if field.changed {
			d[field.name] = p.PropertyDiff{Kind: p.UpdateReplace}
		}
	}
	for _, field := range []struct {
		name    string
		changed bool
	}{
		{"schedule", in.Schedule != old.Schedule}, {"enabled", in.Enabled != old.Enabled}, {"prefix", in.Prefix != old.Prefix},
		{"destinationId", in.DestinationID != old.DestinationID}, {"database", in.Database != old.Database},
		{"keepLatestCount", !sameOptionalInt(in.KeepLatestCount, old.KeepLatestCount)},
	} {
		if field.changed {
			d[field.name] = p.PropertyDiff{Kind: p.Update}
		}
	}
	return infer.DiffResponse{HasChanges: len(d) > 0, DetailedDiff: d}, nil
}

type backupObservation struct {
	ID              string
	Valid           bool
	Schedule        string
	Enabled         *bool
	Prefix          string
	DestinationID   string
	Database        string
	DatabaseType    string
	TargetID        string
	KeepLatestCount *int
}

func (v backupObservation) matchesCreate(databaseType, targetID string, a BackupArgs) bool {
	if !v.Valid {
		return false
	}
	if v.Schedule != a.Schedule || v.Prefix != a.Prefix || v.DestinationID != a.DestinationID ||
		v.Database != a.Database || v.DatabaseType != databaseType || v.TargetID != targetID {
		return false
	}
	if v.Enabled != nil && *v.Enabled != a.Enabled {
		return false
	}
	if (v.KeepLatestCount == nil) != (a.KeepLatestCount == nil) {
		return false
	}
	return v.KeepLatestCount == nil || *v.KeepLatestCount == *a.KeepLatestCount
}

func backupStringField(obj map[string]interface{}, name string) (string, bool, bool) {
	value, present := obj[name]
	if !present {
		return "", false, true
	}
	stringValue, ok := value.(string)
	return stringValue, true, ok && stringValue != ""
}

func backupIntField(value interface{}) (*int, bool) {
	if value == nil {
		return nil, true
	}
	var number float64
	switch value := value.(type) {
	case float64:
		number = value
	case float32:
		number = float64(value)
	case int:
		return &value, true
	case int64:
		converted := int(value)
		return &converted, int64(converted) == value
	default:
		return nil, false
	}
	if math.IsNaN(number) || math.IsInf(number, 0) || math.Trunc(number) != number {
		return nil, false
	}
	maxInt := float64(int(^uint(0) >> 1))
	minInt := -maxInt - 1
	if number < minInt || number > maxInt {
		return nil, false
	}
	converted := int(number)
	if float64(converted) != number {
		return nil, false
	}
	return &converted, true
}

var errBackupDiscovery = errors.New("backup.create could not read target backups")

func backupDiscoveryError(err error) error {
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("%w: %w", errBackupDiscovery, context.Canceled)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: %w", errBackupDiscovery, context.DeadlineExceeded)
	}
	return errBackupDiscovery
}

// backupObservationsForTarget lists backups currently attached to a database
// instance by reading them off its nested "backups" property, since Dokploy
// exposes no backup.all listing endpoint.
func backupObservationsForTarget(ctx context.Context, api *client.Client, databaseType, targetID string) (map[string]backupObservation, error) {
	var additional map[string]interface{}
	switch databaseType {
	case backupDatabaseTypePostgres:
		resp, err := api.PostgresOneWithResponse(ctx, &generated.PostgresOneParams{PostgresId: targetID})
		if err != nil {
			return nil, backupDiscoveryError(err)
		}
		if resp.JSON200 == nil {
			return nil, errBackupDiscovery
		}
		additional = resp.JSON200.AdditionalProperties
	case backupDatabaseTypeMySQL:
		resp, err := api.MysqlOneWithResponse(ctx, &generated.MysqlOneParams{MysqlId: targetID})
		if err != nil {
			return nil, backupDiscoveryError(err)
		}
		if resp.JSON200 == nil {
			return nil, errBackupDiscovery
		}
		additional = resp.JSON200.AdditionalProperties
	case backupDatabaseTypeMariaDB:
		resp, err := api.MariadbOneWithResponse(ctx, &generated.MariadbOneParams{MariadbId: targetID})
		if err != nil {
			return nil, backupDiscoveryError(err)
		}
		if resp.JSON200 == nil {
			return nil, errBackupDiscovery
		}
		additional = resp.JSON200.AdditionalProperties
	case backupDatabaseTypeMongo:
		resp, err := api.MongoOneWithResponse(ctx, &generated.MongoOneParams{MongoId: targetID})
		if err != nil {
			return nil, backupDiscoveryError(err)
		}
		if resp.JSON200 == nil {
			return nil, errBackupDiscovery
		}
		additional = resp.JSON200.AdditionalProperties
	default:
		return nil, errBackupDiscovery
	}
	observations := map[string]backupObservation{}
	backups, present := additional["backups"]
	if !present || backups == nil {
		return nil, errBackupDiscovery
	}
	list, ok := backups.([]interface{})
	if !ok {
		return nil, errBackupDiscovery
	}
	for _, item := range list {
		obj, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		id, present, valid := backupStringField(obj, "backupId")
		if !present || !valid {
			continue
		}
		observation := backupObservation{ID: id, Valid: true}
		schedule, present, valid := backupStringField(obj, "schedule")
		if !present || !valid {
			observation.Valid = false
		}
		prefix, present, valid := backupStringField(obj, "prefix")
		if !present || !valid {
			observation.Valid = false
		}
		destinationID, present, valid := backupStringField(obj, "destinationId")
		if !present || !valid {
			observation.Valid = false
		}
		database, present, valid := backupStringField(obj, "database")
		if !present || !valid {
			observation.Valid = false
		}
		databaseTypeValue, present, valid := backupStringField(obj, "databaseType")
		if !present || !valid {
			observation.Valid = false
		}
		observedTargetID, present, valid := backupStringField(obj, map[string]string{
			backupDatabaseTypePostgres: backupDatabaseTypePostgres + "Id",
			backupDatabaseTypeMySQL:    "mysqlId",
			backupDatabaseTypeMariaDB:  "mariadbId",
			backupDatabaseTypeMongo:    "mongoId",
		}[databaseType])
		if !present {
			observedTargetID = targetID
		} else if !valid {
			observation.Valid = false
		}
		var enabled *bool
		if value, present := obj["enabled"]; present {
			value, ok := value.(bool)
			if !ok {
				observation.Valid = false
			} else {
				enabled = &value
			}
		}
		var keepLatestCount *int
		if value, present := obj["keepLatestCount"]; present {
			var ok bool
			keepLatestCount, ok = backupIntField(value)
			if !ok {
				observation.Valid = false
			}
		}
		observation.Schedule, observation.Enabled, observation.Prefix = schedule, enabled, prefix
		observation.DestinationID, observation.Database, observation.DatabaseType = destinationID, database, databaseTypeValue
		observation.TargetID, observation.KeepLatestCount = observedTargetID, keepLatestCount
		observations[id] = observation
	}
	return observations, nil
}

func backupCreateBody(databaseType, targetID string, a BackupArgs) generated.BackupCreateJSONRequestBody {
	b := generated.BackupCreateJSONRequestBody{
		Schedule: a.Schedule, Prefix: a.Prefix, DestinationId: a.DestinationID, Database: a.Database,
		DatabaseType: generated.BackupCreateJSONBodyDatabaseType(databaseType), Enabled: nullable.NewNullableWithValue(a.Enabled),
		KeepLatestCount: nullable.NewNullNullable[float32](),
	}
	if a.KeepLatestCount != nil {
		b.KeepLatestCount = nullable.NewNullableWithValue(float32(*a.KeepLatestCount))
	}
	switch databaseType {
	case backupDatabaseTypePostgres:
		b.PostgresId = nullable.NewNullableWithValue(targetID)
	case backupDatabaseTypeMySQL:
		b.MysqlId = nullable.NewNullableWithValue(targetID)
	case backupDatabaseTypeMariaDB:
		b.MariadbId = nullable.NewNullableWithValue(targetID)
	case backupDatabaseTypeMongo:
		b.MongoId = nullable.NewNullableWithValue(targetID)
	}
	return b
}

var backupCreatePollInterval = 2 * time.Second

func waitForCreatedBackup(ctx context.Context, api *client.Client, databaseType, targetID string, before map[string]backupObservation, args BackupArgs) (string, error) {
	wrapContextError := func(err error) (string, error) {
		return "", fmt.Errorf(
			"backup.create succeeded but no unique matching backup became visible for database type %s: %w",
			databaseType, err,
		)
	}
	for {
		if err := ctx.Err(); err != nil {
			return wrapContextError(err)
		}
		after, err := backupObservationsForTarget(ctx, api, databaseType, targetID)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return wrapContextError(ctxErr)
			}
			return "", err
		}
		matches := make([]string, 0, len(after))
		for id, observation := range after {
			if _, existed := before[id]; !existed && observation.matchesCreate(databaseType, targetID, args) {
				matches = append(matches, id)
			}
		}
		if len(matches) == 1 {
			return matches[0], nil
		}
		if len(matches) > 1 {
			return "", fmt.Errorf("backup.create found %d matching backups for database type %s", len(matches), databaseType)
		}

		timer := time.NewTimer(backupCreatePollInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return wrapContextError(ctx.Err())
		case <-timer.C:
		}
	}
}

// Create works around backup.create returning HTTP 200 with an empty body
// (confirmed against a live Dokploy instance: unlike every other create
// endpoint in this provider, it echoes back nothing, not even the new
// backupId). Since there is no backup.all listing endpoint either, the only
// way to recover the new ID is to snapshot the target database's nested
// backups list before and after the call and diff them.
func (r Backup) Create(ctx context.Context, req infer.CreateRequest[BackupArgs]) (infer.CreateResponse[BackupState], error) {
	state := BackupState{BackupArgs: req.Inputs}
	if req.DryRun {
		return infer.CreateResponse[BackupState]{Output: state}, nil
	}
	api := r.client(ctx)
	databaseType, targetID, err := req.Inputs.databaseType()
	if err != nil {
		return infer.CreateResponse[BackupState]{}, err
	}
	before, err := backupObservationsForTarget(ctx, api, databaseType, targetID)
	if err != nil {
		return infer.CreateResponse[BackupState]{}, err
	}
	if _, err := api.BackupCreateWithResponse(ctx, backupCreateBody(databaseType, targetID, req.Inputs)); err != nil {
		return infer.CreateResponse[BackupState]{}, err
	}
	newID, err := waitForCreatedBackup(ctx, api, databaseType, targetID, before, req.Inputs)
	if err != nil {
		return infer.CreateResponse[BackupState]{}, err
	}
	state.BackupID = newID
	return infer.CreateResponse[BackupState]{ID: state.BackupID, Output: state}, nil
}

func (r Backup) Read(ctx context.Context, req infer.ReadRequest[BackupArgs, BackupState]) (infer.ReadResponse[BackupArgs, BackupState], error) {
	resp, err := r.client(ctx).BackupOneWithResponse(ctx, &generated.BackupOneParams{BackupId: req.ID})
	if err != nil {
		if client.IsNotFound(err) {
			return infer.ReadResponse[BackupArgs, BackupState]{ID: ""}, nil
		}
		return infer.ReadResponse[BackupArgs, BackupState]{}, err
	}
	if resp.JSON200 == nil || resp.JSON200.BackupId == nil {
		return infer.ReadResponse[BackupArgs, BackupState]{}, fmt.Errorf("backup.one returned incomplete backup")
	}
	v := resp.JSON200
	a := req.State.BackupArgs
	a.Schedule, a.Prefix, a.DestinationID, a.Database = value(v.Schedule), value(v.Prefix), value(v.DestinationId), value(v.Database)
	a.KeepLatestCount = v.KeepLatestCount
	a.Enabled = req.State.Enabled
	if v.Enabled != nil {
		a.Enabled = *v.Enabled
	}
	a.PostgresID, a.MySQLID, a.MariaDBID, a.MongoID = nil, nil, nil, nil
	switch value(v.DatabaseType) {
	case backupDatabaseTypePostgres:
		a.PostgresID = v.PostgresId
	case backupDatabaseTypeMySQL:
		a.MySQLID = v.MysqlId
	case backupDatabaseTypeMariaDB:
		a.MariaDBID = v.MariadbId
	case backupDatabaseTypeMongo:
		a.MongoID = v.MongoId
	}
	state := BackupState{BackupArgs: a, BackupID: *v.BackupId}
	return infer.ReadResponse[BackupArgs, BackupState]{ID: *v.BackupId, Inputs: a, State: state}, nil
}

func (r Backup) Update(ctx context.Context, req infer.UpdateRequest[BackupArgs, BackupState]) (infer.UpdateResponse[BackupState], error) {
	state := BackupState{BackupArgs: req.Inputs, BackupID: req.ID}
	if req.DryRun {
		return infer.UpdateResponse[BackupState]{Output: state}, nil
	}
	databaseType, _, err := req.Inputs.databaseType()
	if err != nil {
		return infer.UpdateResponse[BackupState]{Output: state}, err
	}
	b := generated.BackupUpdateJSONRequestBody{
		BackupId: req.ID, Schedule: req.Inputs.Schedule, Prefix: req.Inputs.Prefix, DestinationId: req.Inputs.DestinationID,
		Database: req.Inputs.Database, DatabaseType: generated.BackupUpdateJSONBodyDatabaseType(databaseType),
		Enabled: nullable.NewNullableWithValue(req.Inputs.Enabled), KeepLatestCount: nullable.NewNullNullable[float32](),
		ServiceName: nullable.NewNullNullable[string](), Metadata: nullable.NewNullNullable[interface{}](),
	}
	if req.Inputs.KeepLatestCount != nil {
		b.KeepLatestCount = nullable.NewNullableWithValue(float32(*req.Inputs.KeepLatestCount))
	}
	if _, err := r.client(ctx).BackupUpdateWithResponse(ctx, b); err != nil {
		return infer.UpdateResponse[BackupState]{Output: state}, err
	}
	return infer.UpdateResponse[BackupState]{Output: state}, nil
}

func (r Backup) Delete(ctx context.Context, req infer.DeleteRequest[BackupState]) (infer.DeleteResponse, error) {
	_, err := r.client(ctx).BackupRemoveWithResponse(ctx, generated.BackupRemoveJSONRequestBody{BackupId: req.ID})
	if client.IsNotFound(err) {
		return infer.DeleteResponse{}, nil
	}
	return infer.DeleteResponse{}, err
}

func (r Backup) WireDependencies(f infer.FieldSelector, args *BackupArgs, state *BackupState) {
	deps := []infer.InputField{
		f.InputField(&args.Schedule), f.InputField(&args.Enabled), f.InputField(&args.Prefix), f.InputField(&args.DestinationID),
		f.InputField(&args.Database), f.InputField(&args.KeepLatestCount), f.InputField(&args.PostgresID), f.InputField(&args.MySQLID),
		f.InputField(&args.MariaDBID), f.InputField(&args.MongoID),
	}
	f.OutputField(&state.BackupID).DependsOn(deps...)
}
