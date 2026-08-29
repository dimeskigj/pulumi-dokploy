#!/usr/bin/env python3
"""Apply the repository's deterministic CI policy after CI Management generation."""

from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
WORKFLOWS = ROOT / ".github" / "workflows"
RELEASE_ACTION = "pulumi/action-release-by-pr-label@a90569296b805a3179b81c1860f5777073cc7aa2"


def replace_once(path: Path, old: str, new: str) -> None:
    text = path.read_text()
    if old not in text:
        if new in text:
            return
        raise SystemExit(f"expected generator output not found in {path}: {old!r}")
    path.write_text(text.replace(old, new, 1))


def main() -> None:
    # This checkout pins its toolchain in the root mise file; discard the
    # template's optional vfox config so local and CI generation use one source.
    for generated_mise in (ROOT / ".config" / "mise.toml", ROOT / ".config" / "mise.test.toml"):
        generated_mise.unlink(missing_ok=True)
    for generated in (ROOT / ".github").rglob("*"):
        if generated.is_file():
            generated.write_text("\n".join(line.rstrip() for line in generated.read_text().splitlines()) + "\n")
    for document in (ROOT / "CODE-OF-CONDUCT.md", ROOT / "CONTRIBUTING.md", ROOT / "SECURITY.md"):
        if document.exists():
            document.write_text("\n".join(line.rstrip() for line in document.read_text().splitlines()) + "\n")
    (ROOT / ".golangci.yml").write_text(
        '''version: "2"
linters:
  enable: [goconst, gosec, misspell, nakedret, unconvert]
  disable:
    # lll and revive are style/documentation policy, not correctness gates.
    - lll
    - revive
  exclusions:
    generated: lax
    rules:
      - linters: [goconst]
        path: '.*_test\\.go'
formatters:
  enable: [gofmt]
'''
    )
    build = WORKFLOWS / "build.yml"
    build_text = build.read_text()
    build_text = build_text.replace("    paths-ignore:\n    - CHANGELOG.md\n", "")
    build_text = build_text.replace("    - \"**\"\n", "")
    if "  pull_request: {}\n" not in build_text:
        build_text = build_text.replace("  workflow_dispatch: {}\n", "  pull_request: {}\n  workflow_dispatch: {}\n", 1)
    build_text = build_text.replace("pulumi/action-release-by-pr-label@main", RELEASE_ACTION)
    build_text = build_text.replace("args: -p 3 release --clean --timeout 60m0s", "args: -p 3 -f .goreleaser.yml release --clean --timeout 60m0s")
    gate = """    - name: Check OpenAPI and generated SDKs
      run: make check_openapi && make check_codegen
    - name: Run race tests
      run: make test_race
    - name: Build all SDKs
      run: make build_sdks
    - name: Test all examples
      run: make test_examples
    - name: Vulnerability scan
      run: make govulncheck
    - name: License scan
      run: make license
"""
    marker = "    - name: Build Provider\n"
    if "    - name: Check OpenAPI and generated SDKs\n" not in build_text:
        build_text = build_text.replace(marker, gate + marker, 1)
    build.write_text(build_text)

    acceptance = WORKFLOWS / "run-acceptance-tests.yml"
    acceptance_text = acceptance.read_text()
    acceptance_text = acceptance_text.replace("  pull_request:\n    paths-ignore:\n    - CHANGELOG.md\n", "", 1)
    acceptance_text = acceptance_text.replace("  prerequisites:\n    runs-on:", "  prerequisites:\n    environment: dokploy-acceptance\n    runs-on:", 1)
    credential_step = """    - name: Require Dokploy acceptance credentials
      env:
        DOKPLOY_ENDPOINT: ${{ steps.esc-secrets.outputs.DOKPLOY_ENDPOINT }}
        DOKPLOY_API_KEY: ${{ steps.esc-secrets.outputs.DOKPLOY_API_KEY }}
      run: test -n "$DOKPLOY_ENDPOINT" && test -n "$DOKPLOY_API_KEY"
"""
    if "    - name: Require Dokploy acceptance credentials\n" not in acceptance_text:
        acceptance_text = acceptance_text.replace("    - name: Setup Tools\n", credential_step + "    - name: Setup Tools\n", 1)
    acceptance.write_text(acceptance_text)

    command_dispatch = WORKFLOWS / "command-dispatch.yml"
    replace_once(command_dispatch, "repository: pulumi/pulumi-dokploy", "repository: gjorgjidimeski/pulumi-dokploy")
    release_command = WORKFLOWS / "release_command.yml"
    release_command_text = release_command.read_text().replace("pulumi/action-release-by-pr-label@main", RELEASE_ACTION)
    release_command.write_text(release_command_text)
    release = WORKFLOWS / "release.yml"
    release_text = release.read_text().replace(
        "args: -p 3 release --clean --timeout 60m0s",
        "args: -p 3 -f .goreleaser.yml release --clean --timeout 60m0s",
    )
    release.write_text(release_text)

    for name in (".goreleaser.yml", ".goreleaser.prerelease.yml"):
        path = ROOT / name
        text = path.read_text().replace(
            "github.com/pulumi/pulumi-dokploy/provider/pkg/version.Version={{.Tag}}",
            "github.com/gjorgjidimeski/pulumi-dokploy/provider.Version={{.Tag}}",
        )
        path.write_text(text)

    if "paths-ignore:" in build.read_text() or "pull_request:" not in build.read_text():
        raise SystemExit("build workflow trigger normalization failed")
    if "pull_request:" in acceptance.read_text():
        raise SystemExit("acceptance workflow must not trigger on pull_request")
    if RELEASE_ACTION not in release_command.read_text() or "@main" in release_command.read_text():
        raise SystemExit("release action pinning failed")


if __name__ == "__main__":
    main()
