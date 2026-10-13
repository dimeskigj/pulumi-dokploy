# Task 1 implementation report

## Status

Implemented Task 1: source save-operation generation and complete application builder support. Implementation commit: `49acb6d feat: add complete application builder support`.

## TDD evidence

- **RED — contract and validation:** `mise exec -- /usr/bin/env DOKPLOY_ACCEPTANCE=0 go test ./openapi/cmd/normalize ./provider -run 'TestApplication(SourceSaveOperationContracts|Build)' -count=1` exited 1 as expected. The contract test failed because all three save-operation paths were absent from normalized output. Builder validation rejected `heroku_buildpacks`, `paketo_buildpacks`, `static`, and `railpack` with the old supported-types error.
- **RED — API contract:** After adding request/read tests, `mise exec -- /usr/bin/env DOKPLOY_ACCEPTANCE=0 go test ./provider -run 'TestApplicationBuild' -count=1` exited 1 at compilation as expected: new builder constants and `ApplicationBuild` fields did not yet exist.
- **GREEN — focused contract/build tests:** `mise exec -- /usr/bin/env DOKPLOY_ACCEPTANCE=0 go test ./provider ./openapi/cmd/normalize -run 'TestApplication(SourceSaveOperationContracts|Build)' -count=1` passed: `ok` for both packages.
- **GREEN — application lifecycle regression tests:** `mise exec -- /usr/bin/env DOKPLOY_ACCEPTANCE=0 go test ./provider -run '^TestApplication' -count=1` passed.

## Implementation

- Added the Github, Gitea, and Bitbucket save-provider operations to the normalization allowlist, corrected their acknowledgments to boolean, and regenerated the normalized OpenAPI document and generated Go client.
- Added all six builder constants and optional Heroku/Railpack version, publish directory, and SPA settings. Validation accepts only each builder's applicable options, retains Dockerfile's nonempty requirement, and does not pin version strings.
- Added nil-safe build selection for Git/GitLab variants; build requests now send every option as a value or explicit null, preserve explicit false, and only skip build configuration for Docker. Missing/unsupported variants return errors.
- Read reconstruction now accepts prior build settings, limits decoded options to the selected builder, normalizes implicit false while retaining prior explicit false, and keeps the existing Nixpacks fallback.
- Added contract, validation, request shape, read/import, false/default, clearing, variant error, and runtime-diff tests; updated expected request bodies for nullable fields.

## Files changed

- `openapi/operations.txt`
- `openapi/corrections.json`
- `openapi/dokploy.json` (generated)
- `internal/client/generated/generated.gen.go` (generated)
- `openapi/cmd/normalize/main_test.go`
- `provider/application_source.go`
- `provider/application.go`
- `provider/application_test.go`
- `provider/application_build_test.go`

## Verification

- Required full offline check: `mise exec -- /usr/bin/env DOKPLOY_ACCEPTANCE=0 go test ./openapi/cmd/normalize ./provider ./internal/client/...` passed. Output: normalize, provider, client `ok`; generated client reports `[no test files]`.
- `mise exec -- /usr/bin/env DOKPLOY_ACCEPTANCE=0 make test_provider` passed across provider and internal packages; the conditional release-snapshot test skipped because no local snapshot was supplied.
- `mise exec -- /usr/bin/env DOKPLOY_ACCEPTANCE=0 make lint` passed with `0 issues`.
- `mise exec -- /usr/bin/env DOKPLOY_ACCEPTANCE=0 make provider` passed and built `bin/pulumi-resource-dokploy`.
- Post-commit reproducibility: `mise exec -- /usr/bin/env DOKPLOY_ACCEPTANCE=0 make check_openapi` passed; regeneration ended with `git diff --exit-code -- openapi/dokploy.json internal/client/generated/generated.gen.go` returning clean.
- `git diff --check` passed; post-check `git status --short` was clean.
- No live tests, SDK generation, publication, push, or merge was performed.

## Self-review and concerns

Reviewed the complete source/test and generated OpenAPI/client diff. Changes are limited to Task 1 surfaces. Existing build/source API fields remain additive; requests explicitly clear unset builder options; error paths avoid dereferencing absent repository variants. No unresolved concerns.
