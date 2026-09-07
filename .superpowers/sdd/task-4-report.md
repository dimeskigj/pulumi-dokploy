Status: completed

Commit: 13aff91 fix: accept active organization primary IDs

Tests: RED confirmed with `go test ./provider -run 'TestSSHKey|TestTask9OrganizationActiveShapeClassification' -count=1`; GREEN focused tests and `go test ./... -count=1` passed.

Concerns: `make generate_openapi` could not run because `mise` is unavailable; artifacts were regenerated with the equivalent direct `go run` commands. No live calls were made.

P2 repair evidence: Added combined-field cases for empty, valid, wrong-type, and null primary IDs with valid legacy fallback; valid primary ID precedence remains asserted. `classifyOrganizationActiveShape` now returns the legacy flat label whenever the primary is not a non-empty string, while retaining secret-safe labels and nested rejection.

Verification: `go test ./provider -run 'TestTask9OrganizationActiveShapeClassification' -count=1` passed; `go test ./provider -run 'TestSSHKey|TestTask9OrganizationActiveShapeClassification' -count=1` passed; `go test ./... -count=1` passed; `git diff --check` passed.
