# Task 3 implementation report

## Delivered

- Curated `website/src/content/docs/guides/servers.mdx` covers configuration-only registration, SSH/readiness limits, cleanup opt-in, in-place updates, inactive failures, partial identity, workload delete refusal, record/history deletion versus VM preservation, import, and safe placement.
- Added Server reference/sidebar/guide routes and import guidance; updated provider README's workload deployment lifecycle wording and Server lifecycle/import guidance.
- Added registration-only YAML fixture using reserved `192.0.2.10`, existing SSHKey output, explicit deploy/cleanup false, and Server ID output. It deliberately does not place workloads on the new server.
- Added website/schema and provider documentation regressions; generated Server reference and examples page via repository generation target; copied YAML README to five generated languages per target convention.

## TDD evidence

- RED: `mise exec -- node --test website/tests/content.test.mjs website/tests/reference-model.test.mjs` failed as expected: missing curated Servers page/sidebar and `Unexpected resource set` due schema Server not in renderer allowlist. Initial targeted Go commands indicated no matching tests existed yet.
- GREEN: after source/test additions, `mise exec -- node --test website/tests/content.test.mjs website/tests/reference-model.test.mjs` passed 29/29; `mise exec -- go test -short ./provider -run TestServerDocumentation -count=1` passed; `mise exec -- go test ./examples -tags=yaml -run TestCanonicalServerExample -count=1` passed.

## Generation and verification

- `mise exec -- make gen_examples` built provider/schema/SDKs and installed the dev plugin, then the five Pulumi conversions reported that `dokploy:index:Schedule` and `dokploy:index:Server` could not be found in provider `dokploy`. The Make target continued despite those converter errors. The generated language code was not trustworthy, so its partial output was discarded; no generated language source files were committed. The generated language READMEs were refreshed from canonical YAML README.
- `mise exec -- make docs_generate` completed and generated `website/src/content/docs/reference/server.mdx` and complete example page.
- `mise exec -- npm --prefix website run check`: passed, 0 diagnostics and 50/50 website tests.
- `mise exec -- npm --prefix website run build`: passed (42 pages). Astro emitted existing missing `src/icons` warning.
- `mise exec -- go test ./examples -tags=yaml -run TestCanonicalServerExample -count=1`: passed.
- `mise exec -- make docs_check` passed post-commit: generated drift check clean, website check 50/50 tests, build succeeded (Astro emitted the existing missing `src/icons` warning), and built-site tests passed 2/2. Pre-commit generated drift failure was expected because generated outputs were not yet committed.
- `git diff --check`: passed before commit.
- `mise exec -- make test_examples` passed: YAML-tagged Go tests, Go example compile, Python compile, Node TypeScript compile, .NET build, Java SDK publication to local Maven, and Java example package all succeeded. SDK/.NET/Java emitted existing compiler/Javadoc warnings but exited successfully.

## Self-review / concerns

- The canonical YAML and YAML output contain `remoteServer`, and the Server docs/reference are generated. However, the CLI could not resolve Server/Schedule during language conversion, so the five language program sources do not yet contain generated Server resources. This is an unresolved generation blocker and means the all-five-language example portion is incomplete. Do not hand-edit those generated programs; resolve provider plugin selection/schema discovery and rerun `make gen_examples`.
- The temporary controller-owned `.mise.toml` modification was left untouched and unstaged.
- No live compatibility testing was performed.
