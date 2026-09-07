Status: completed

Commit: 13aff91 fix: accept active organization primary IDs

Tests: RED confirmed with `go test ./provider -run 'TestSSHKey|TestTask9OrganizationActiveShapeClassification' -count=1`; GREEN focused tests and `go test ./... -count=1` passed.

Concerns: `make generate_openapi` could not run because `mise` is unavailable; artifacts were regenerated with the equivalent direct `go run` commands. No live calls were made.

P2 repair evidence: Added combined-field cases for empty, valid, wrong-type, and null primary IDs with valid legacy fallback; valid primary ID precedence remains asserted. `classifyOrganizationActiveShape` now returns the legacy flat label whenever the primary is not a non-empty string, while retaining secret-safe labels and nested rejection.

Verification: `go test ./provider -run 'TestTask9OrganizationActiveShapeClassification' -count=1` passed; `go test ./provider -run 'TestSSHKey|TestTask9OrganizationActiveShapeClassification' -count=1` passed; `go test ./... -count=1` passed; `git diff --check` passed.

Fixture root cause: `liveSSHKeyPair` generated Ed25519 keys but encoded the private key as generic PKCS#8 (`BEGIN PRIVATE KEY`), which Dokploy rejects for SSH-key creation; this was a fixture-format defect, not provider request mapping.

Fixture evidence: RSA 2048 keys now use PKCS#1 `RSA PRIVATE KEY` PEM and `ssh-rsa` authorized-key output. `TestLiveSSHKeyPairUsesDokployKeyFormats` parses both envelopes and compares the derived authorized public key to the supplied public key without printing key values.

Verification: `go test ./provider -run 'TestLiveSSHKeyPairUsesDokployKeyFormats' -count=1` passed; `go test ./provider -run 'TestSSHKey|TestLiveSSHKeyPairUsesDokployKeyFormats|TestTask9OrganizationActiveShapeClassification' -count=1` passed; `go test ./... -count=1` passed; `git diff --check` passed. No live calls or `.env` reads were made.

Empty-create discovery: Added `sshKey.all` to the normalized contract as an `SSHKeyList` response and changed `sshKey.create` to no content; regenerated `openapi/dokploy.json` and `internal/client/generated/generated.gen.go`. SSH create now snapshots IDs before create, performs the empty-body create, lists afterward, selects exactly one new matching-name ID, and returns sanitized initialization failures for absent or ambiguous candidates. Pre-existing IDs are excluded, and discovered IDs are returned with partial state when `sshKey.one` fails.

TDD evidence: New scripted-server tests cover empty response discovery, no candidate, ambiguous candidates, pre-existing same-name exclusion, partial state after post-create read failure, and no-content parser behavior through create. The first focused run failed because the old create implementation dereferenced removed `JSON200`; subsequent candidate-init assertions failed until errors were wrapped as `ResourceInitFailedError`; final focused and full suites passed. No list payloads, IDs, or key material are logged.

Verification: `go test ./provider -run 'TestSSHKeyCreate|TestSSHKeyAPIErrorsRedact' -count=1` passed; `go test ./openapi/cmd/normalize ./provider -run 'TestNormalizeUsesProductionOperationsAndCorrections|TestSSHKey|TestTask9OrganizationActiveShapeClassification|TestLiveSSHKeyPairUsesDokployKeyFormats' -count=1` passed; `go test ./... -count=1` passed; `git diff --check` passed. No live calls or `.env` reads were made.
