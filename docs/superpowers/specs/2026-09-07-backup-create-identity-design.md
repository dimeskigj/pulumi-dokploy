# Backup Create Identity Design

## Problem

Dokploy's `backup.create` endpoint returns HTTP 200 with an empty response body, and Dokploy exposes no global backup listing endpoint. The only available discovery path is the target database's nested `backups` collection.

The provider currently snapshots backup IDs before creation, submits the create request, reads the collection once, and accepts the only new ID. This fails under eventual consistency and can attribute the wrong backup when another actor creates a backup concurrently. A failed discovery happens after the remote create succeeded, so retries can create duplicates.

## Goal

Recover the created backup ID when it becomes uniquely identifiable from observable Dokploy state, while never knowingly adopting an unrelated or ambiguous backup.

## Non-Goals

- Guarantee identity when multiple externally created backups have identical observable fields.
- Coordinate separate provider processes or external Dokploy users.
- Change the Pulumi schema or resource token.
- Add process-local locking that gives a false impression of global serialization.
- Modify generated OpenAPI code manually.

## Approach

Replace one-shot ID-set comparison with bounded polling over parsed backup records.

Before creation, read the target's nested backups and record every existing non-empty backup ID. After `backup.create` succeeds, repeatedly read the same target and inspect only records whose IDs were absent from the pre-create snapshot.

A new record is a candidate only when all observable create fields equal the requested inputs:

- `schedule`
- `enabled`, when present in the response
- `prefix`
- `destinationId`
- `database`
- `databaseType`
- target-specific ID (`postgresId`, `mysqlId`, `mariadbId`, or `mongoId`), when present
- `keepLatestCount`, including null versus a concrete value, when present

The provider succeeds only when exactly one new matching candidate exists. Unrelated new records are ignored. Zero matching candidates cause another poll until the operation context expires. Multiple matching candidates remain ambiguous and result in a safe error rather than arbitrary adoption.

No local mutex will be added. It would prevent only same-process races and would not address independent Pulumi operations, Dokploy users, or automation.

## Components

### Parsed observation

`provider/backup.go` will define a small internal representation for fields observable in nested backup values. Parsing must tolerate missing optional fields because Dokploy response shapes can omit values, but malformed values for fields that are present must not be treated as a match.

The target reader will return observations keyed by backup ID rather than a boolean ID set. Existing target dispatch and incomplete-target error behavior remains unchanged.

### Candidate matching

Matching will be a pure helper. It receives an observation, the requested database type and target ID, and `BackupArgs`. This makes field semantics directly unit-testable and prevents polling behavior from obscuring matching errors.

An omitted optional response field is treated as unknown rather than a mismatch only where the API demonstrably omits that field. Required identifying fields must be present and equal. The implementation must prefer false negatives over false positives.

### Bounded discovery

Discovery will use a package variable for the poll interval, following `waitPollInterval`, so tests can run without real delays. The Pulumi operation context supplies the deadline; the helper does not create a longer independent timeout.

Each cycle will:

1. Check context cancellation.
2. Read target backups.
3. Remove pre-existing IDs.
4. Filter exact matches.
5. Return the ID for exactly one match.
6. Return an ambiguity error for multiple exact matches.
7. Wait for the poll interval when no match is visible.

Read errors are returned immediately. Existing client-level bounded retries already handle transient GET failures.

## Error Semantics

If the create request fails, preserve current behavior and return that error without an ID.

If discovery is canceled or reaches the Pulumi deadline, return an error that states that `backup.create` succeeded but no unique matching backup became visible for the target. Wrap the context error so callers can still use `errors.Is`.

If multiple exact candidates are observed, return an ambiguity error listing the candidate count but not arbitrary payload values. No candidate is adopted or deleted.

The provider cannot return partial state without a trustworthy ID. Documentation must be explicit that an ambiguous or timed-out create may leave an unmanaged backup and that users should inspect Dokploy before retrying.

## Testing

Scripted-server tests will cover:

- Immediate visibility of one exact matching backup.
- Delayed visibility after one or more empty observations.
- An unrelated concurrent backup followed by one exact match.
- One exact match alongside unrelated new backups.
- Multiple exact matching backups producing an ambiguity error.
- Context deadline while no candidate appears.
- Context cancellation while waiting.
- Matching and discovery for PostgreSQL, MySQL, MariaDB, and MongoDB.
- Optional `enabled` and `keepLatestCount` matching semantics.
- Malformed nested values never being accepted as candidates.

The existing provider suite and race suite must continue to pass. Live backup acceptance remains a final verification step and requires the explicit Dokploy acceptance environment.

## Upstream Follow-Up

The robust long-term contract is for Dokploy to return the created `backupId` or accept an idempotency/correlation token. The provider workaround must remain isolated so it can be removed when that API capability becomes available.
