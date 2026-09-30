# Task 2 report

## Result

Implemented the Schedule argument/state models, annotations, Check validation, target/update diff classification, and focused tests. Provider registration, provider metadata, and token/schema tests are deferred to Task 3 because `infer.Resource` requires the complete CRUD lifecycle. Direct schema generation is likewise unavailable before registration.

`appName` and `serviceName` are treated as target identity and therefore replace-on-change, alongside `scheduleType` and target IDs. This is encoded in Pulumi tags and Diff.

## TDD and verification

- Initial RED: `mise exec -- go test ./provider -run 'TestSchedule(SchemaSurface|CheckTypesAndTargets|CheckDefersComputedTargets|DiffReplacementAndUpdates)' -count=1` failed to compile because resource types were not yet implemented and because this repository uses `property.Map` for Check inputs. Subsequent corrected tests exposed all pointer fields without `,optional` tags as incorrectly required; adding those tags made the focused tests pass.
- GREEN: `mise exec -- go test ./provider -run 'TestSchedule(CheckTypesAndTargets|CheckDefersComputedTargets|CheckRejectsMissingRequiredFieldsAndInvalidShell|CheckRejectsIncompatibleTargets|CheckRequiresTypeSpecificTarget|DiffReplacementAndUpdates)' -count=1` — PASS.
- `mise exec -- go test ./provider -count=1` — FAIL. This broad command unexpectedly exercised existing live `TestLiveTier2Workloads` (MountDispatch/postgres redeploy failure) and unrelated `TestRegistryMetadata` (missing expected value). `TestSchemaSecretsAndDefaults` also failed because I temporarily removed the provider metadata description while undoing the deferred Task 3 metadata change; that original description has been restored. No further broad tests run.
- `git diff --check` — PASS.

No live acceptance test was intentionally requested; the broad provider suite nevertheless entered the live Tier 2 test. No further live tests were run.

## Files

- `provider/schedule.go`
- `provider/schedule_test.go`

Schema secret/default/tag assertions and provider token registration remain for Task 3 once CRUD enables registration.
