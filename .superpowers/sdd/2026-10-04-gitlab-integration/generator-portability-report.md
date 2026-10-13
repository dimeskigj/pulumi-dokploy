# Generator portability regression report

## Result

Restored ordinary `${TMPDIR:-/tmp}` behavior and removed machine-specific converter paths/policy. Converter resolution now honors an executable explicit override, otherwise copies a cached exact-version executable from the caller's Pulumi home (`PULUMI_HOME`, or `$HOME/.pulumi`), otherwise asks the Pulumi CLI to install exact version `1.38.7` into the isolated home. All resolution and installation remains before generated translation deletion.

## Verification

- `TMPDIR=/tmp/opencode /tmp/opencode/gitlab-integration-tools-fwQ0lcJ2/bin/safe-run python3 scripts/test_gen_examples.py` — exit 0, 5 executable behavior tests pass; tests use disposable fake repositories/homes and fake CLI commands.
- Same SAFE wrapper with `python3 scripts/test_normalize_generated.py` — exit 0, 3 tests pass.
- Same SAFE wrapper with `sh -n scripts/gen-examples.sh` — exit 0.
- Same SAFE wrapper with `git diff --check` — exit 0.
- `TMPDIR=/tmp/opencode PULUMI_YAML_CONVERTER_BINARY="$HOME/.pulumi/plugins/converter-yaml-v1.38.7/pulumi-converter-yaml" /tmp/opencode/gitlab-integration-tools-fwQ0lcJ2/bin/safe-run make gen_examples` — exit 0. Canonical codegen/example generation ran with converter 1.38.7 and isolated Pulumi home; generated diff was clean.

OpenAPI/codegen checks and full SDK/example/docs checks were not run separately. No OpenAPI/output edits were made. No credentials, live Dokploy calls, or authenticated downloads were used.

## Follow-up: YAML generator source-test portability

Controller RED reproduced: `TMPDIR=/tmp/opencode /tmp/opencode/gitlab-integration-tools-fwQ0lcJ2/bin/safe-run go test ./examples -tags=yaml -run 'TestGenExamplesUsesIsolatedCanonicalGenerator|TestGeneratedExamplesContainPortAndManagedGitLab' -count=1` — exit 1 at `examples/yaml_test.go:464`, requiring literal `/tmp/opencode/*`. Replaced only that conflicting source-contract assertion with checks for the portable `${TMPDIR:-/tmp}` default, override variable, and invalid-override diagnostic. Existing one-recipe, isolated installation, fixture preservation, translation, normalization, pinned-version, and preflight-order assertions remain.

New GREEN and follow-up checks (all actual tool invocations used `SAFE=/tmp/opencode/gitlab-integration-tools-fwQ0lcJ2/bin/safe-run`, `TMPDIR=/tmp/opencode`):

- Focused YAML tests above — exit 0.
- `python3 scripts/test_gen_examples.py` — exit 0, 5 tests.
- `python3 scripts/test_normalize_generated.py` — exit 0, 3 tests.
- `go test ./examples -tags=yaml -count=1` — exit 0.
- Isolated-home `make test_examples` (preseeded only with cached converter 1.38.7) — exit 0 across Go, Python, Node.js, .NET, and Java examples. .NET emitted two nullable warnings; Java emitted Javadoc/deprecation warnings.
- `make docs_check` — exit 0 (generated docs check, Astro checks, 68 website tests, site build, and built-site tests). npm reported 20 dependency audit vulnerabilities; Astro warned `src/icons` is absent. Neither caused failure.

This follow-up edits only `examples/yaml_test.go` plus this report. The untracked controller-authored GitHub integration design spec was left untouched.

## Changed paths

- `scripts/gen-examples.sh`
- `scripts/test_gen_examples.py`
- `.superpowers/sdd/2026-10-04-gitlab-integration/generator-portability-report.md`
