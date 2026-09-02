# Final review fix wave report

## Status

Implemented findings 1–5. Finding 6 remains documented as a generator constraint:
the Go converter emits `cfg.Get` for canonical YAML secret config values, so the
generated example retains secrecy by wrapping the value with `pulumi.ToSecret`
instead of hand-editing generated output.

## Fixes

- Mount validation now rejects fields for the wrong mount type and empty
  `hostPath`, `volumeName`, and `filePath`; explicit empty file content remains
  valid. Type- and value-dependent checks defer when inputs are computed.
- Mount read, update, create, delete, and deployment/readback errors sanitize
  both current and prior file content while preserving typed API errors (and
  therefore 404 handling).
- Added `postgresId` to canonical `postgresFileMount` and regenerated all five
  language examples plus the website complete example.
- Live SSH fixtures now generate authorized-key public text and PKCS#8 PEM
  private keys. Unit coverage parses both formats.
- The real OpenAPI artifact test now asserts the exact normalized operation set
  equals `operations.txt`.

## Red/green evidence

Initial focused validation run demonstrated the defects: wrong-type and empty
value cases produced no failures, and computed `type` incorrectly produced the
unsupported-type failure. After the implementation:

```text
mise exec -- go test ./provider -run 'TestMountCheckRejectsWrongTypeFieldsAndEmptyValues|TestMountCheckAllowsExplicitEmptyFileContent|TestMountCheckDefersTypeDependentValidationWhenComputed' -count=1
ok
mise exec -- go test ./provider -run 'TestMount|TestLiveSSHKeyPairUsesDokployKeyFormats' -count=1
ok
mise exec -- go test ./openapi/cmd/normalize -run TestNormalizeUsesProductionOperationsAndCorrections -count=1
ok
```

## Verification

All commands ran in this worktree:

```text
mise exec -- go test ./provider -run 'TestMount|TestLiveSSHKeyPairUsesDokployKeyFormats|TestSchema' -count=1  PASS
mise exec -- go test ./provider -count=1                                      PASS
mise exec -- go test ./openapi/cmd/normalize -count=1                          PASS
mise exec -- make check_openapi                                                PASS
mise exec -- go test -short -count=1 ./provider/... ./internal/...             PASS
mise exec -- make test_race                                                    PASS
mise exec -- make check_codegen                                                PASS
mise exec -- go test ./examples -tags=all -count=1                             PASS
mise exec -- make test_examples                                                 PASS
python3 -m unittest scripts/test_normalize_ci.py -v                            PASS (34 tests)
mise exec -- make lint                                                         PASS (0 issues)
mise exec -- make docs_check                                                    PASS (44 website tests)
mise exec -- make docs_build                                                    PASS (38 pages)
git diff --check                                                               PASS
```

The live Dokploy command was not run because no live credentials were available;
there is no claim of live API success. Generators emitted their existing Pulumi,
npm audit, and missing-icons warnings; none caused a verification failure.

## Commits

- `d26b091 fix: harden mount validation and generated examples`

The commit includes provider tests, generated examples, website output, the
normalizer test, and SSH format coverage. No unrelated files were changed.

## Residuals

- Live API verification remains unavailable without credentials.
- Finding 6 is retained as a documented technical constraint; generated Go
  output remains secret through `pulumi.ToSecret`.
