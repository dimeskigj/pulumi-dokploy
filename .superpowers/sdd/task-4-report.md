Status: completed

Commit: 13aff91 fix: accept active organization primary IDs

Tests: RED confirmed with `go test ./provider -run 'TestSSHKey|TestTask9OrganizationActiveShapeClassification' -count=1`; GREEN focused tests and `go test ./... -count=1` passed.

Concerns: `make generate_openapi` could not run because `mise` is unavailable; artifacts were regenerated with the equivalent direct `go run` commands. No live calls were made.
