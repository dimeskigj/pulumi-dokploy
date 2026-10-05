# Final GitLab integration fix wave

Base: `b8cce32`. Commit: the focused commit containing this report (recorded in final response).

## Scope and evidence

- `provider/gitlab_integration.go`: reject query (including bare `?`) and fragment (including bare `#`) on public/internal base URLs in both Check and apply validation. Keep callback validation separate; accept encoded path delimiters and internal basic auth. Errors use fixed, value-free text.
- `provider/gitlab_integration_test.go`: Check and concrete Create/Update rejection cases for both fields and all four component forms; verify no client invocation, safe messages, valid encoded paths/basic auth/callback, plus three required-input empty cases.
- `provider/gitlab_integration_lifecycle_test.go`: successful update changes `name` and confirms the changed value from post-update readback.
- `internal/client/gitlab_contract_test.go`: assert refresh token presence/exact decoded value; cover empty create, malformed update, JSON remove acknowledgments.
- `examples/yaml_test.go`: reject both OAuth token properties in the canonical managed resource.

RED: `/tmp/opencode/gitlab-integration-tools-ses-ef758d7d/safe-run go test ./provider -run '^TestGitLabIntegrationBaseURLComponents$' -count=1` exited 1: all eight public/internal query, bare query, fragment, and bare fragment Check cases unexpectedly had `Failures: []` (line 141 before formatting).

GREEN: `/tmp/opencode/gitlab-integration-tools-ses-ef758d7d/safe-run go test ./provider ./internal/client -run 'TestGitLabIntegration|TestGitLabGeneratedClientContract' -count=1` passed both packages; `/tmp/opencode/gitlab-integration-tools-ses-ef758d7d/safe-run go test -tags yaml ./examples -run '^TestCanonicalYAMLGitLabIntegration$' -count=1` passed. Initial combined attempt without `-tags yaml` reported build constraints exclude all Go files in examples; initial broader focused attempt caught an overly broad assertion against the `applicationSecret` property name, corrected before GREEN.

Broader verification (all via the same safe-run launcher):

- `sh -c 'GOLANGCI_LINT_CACHE=/tmp/opencode/gitlab-integration-tools-ses-ef758d7d/lint-cache-final make lint'` in fresh isolated cache: `0 issues`.
- `make test_provider`: passed (`provider`, `internal/client`; generated and provider/cmd have no tests).
- `make test_race`: passed (`provider`, `internal/client`; generated and provider/cmd have no tests).
- `make provider`: passed; built local provider binary (ignored output).
- `go test -tags yaml ./examples -count=1`: passed.
- `git diff --check`: passed.

Self-review: No schema field, annotation, default, OpenAPI, SDK or website description changed; schema/codegen/docs regeneration was deliberately not run (it replaces generated directories and is unnecessary for value validation). Inspected full scoped diff and status; only five feature source/test paths and this report are in scope. URL parser component checks distinguish raw delimiters from percent-encoded path content; errors never interpolate raw URL or secret. No live acceptance or release tasks were run. Remaining limitation: no live server validation (intentionally prohibited); existing behavioral tests and isolated checks cover this fix. SDD documents retained per no-cleanup instruction; controller handles any future document decision.
