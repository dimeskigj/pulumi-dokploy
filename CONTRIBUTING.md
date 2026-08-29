# Contributing

1. Install the versions in `.mise.toml` with `mise install`.
2. Make changes in provider source and tests; use TDD for behavior changes.
3. Run `make lint`, `make check_openapi`, `make codegen`, `make build_sdks`, and `make test`.
4. Run `make ci-mgmt` after changing `.ci-mgmt.yaml`; do not hand-edit generated workflows.
5. Ensure `git diff --check` is clean and include generated schema/SDK changes in the commit.

Pull requests should explain user-visible behavior and include regression tests. Acceptance tests
require protected repository secrets and are run manually by maintainers.
