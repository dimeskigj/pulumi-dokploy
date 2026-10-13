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

## Changed paths

- `scripts/gen-examples.sh`
- `scripts/test_gen_examples.py`
- `.superpowers/sdd/2026-10-04-gitlab-integration/generator-portability-report.md`
