#!/usr/bin/env python3
"""Apply the repository's explicit CI policy to pinned CI Management output."""

import hashlib
from pathlib import Path
import re


ROOT = Path(__file__).resolve().parents[1]
WORKFLOWS = ROOT / ".github" / "workflows"
GENERATED_WORKFLOW_NAMES = (
    "build.yml", "command-dispatch.yml", "comment-on-stale-issues.yml",
    "community-moderation.yml", "export-repo-secrets.yml", "lint.yml",
    "prerelease.yml", "pull-request.yml", "release_command.yml", "release.yml",
    "run-acceptance-tests.yml", "weekly-pulumi-update.yml", "pages.yml",
)
WORKFLOW_JOB_POLICY = {
    "build.yml": {
        "prerequisites": True, "build_sdks": True, "tag_release_if_labeled_needs_release": False,
        "test": True, "publish": True, "publish_sdk": True, "lint": True,
    },
    "command-dispatch.yml": {"command-dispatch-for-testing": False},
    "comment-on-stale-issues.yml": {"cleanup": False},
    "community-moderation.yml": {"warn_codegen": False},
    "export-repo-secrets.yml": {"export-to-esc": False},
    "lint.yml": {"lint": True},
    "prerelease.yml": {
        "prerequisites": True, "build_sdks": True, "test": True, "publish": True,
        "publish_sdk": True, "publish_java_sdk": True, "publish_go_sdk": True,
    },
    "pull-request.yml": {"comment-on-pr": False},
    "release_command.yml": {"should_release": False},
    "release.yml": {
        "prerequisites": True, "build_sdks": True, "test": True, "publish": True,
        "publish_sdk": True, "publish_java_sdk": True, "publish_go_sdk": True,
        "dispatch_docs_build": False,
    },
    "run-acceptance-tests.yml": {
        "comment-notification": False, "prerequisites": True, "build_sdks": True,
        "test": True, "sentinel": False, "lint": True,
    },
    "weekly-pulumi-update.yml": {"weekly-pulumi-update": True},
    "pages.yml": {"build": False, "deploy": False},
}
EXPECTED_BUILD_GATES = (
    "make check_openapi && make check_codegen",
    "make test_race",
    "make build_sdks",
    "make test_examples",
    "make govulncheck",
    "make license",
    "make docs_check",
)
RELEASE_ACTION = "pulumi/action-release-by-pr-label@a90569296b805a3179b81c1860f5777073cc7aa2"
PUBLISH_GO_ACTION = "pulumi/publish-go-sdk-action@0a153fa3c54227a3c0f7c97a57033ecfe94ab3c2"
GO_VERSION = "1.25.13"
KNOWN_MISE_SHA256 = {
    "mise.toml": "6e4d48dfc6ba5bc647f8de223f35044793ae73aea8ff52f5e989a1dd597f1b08",
    "mise.test.toml": "25d93429d74aefa3c4c59cafc6c33dbbd00b1a876714855eaaadc7a6c8ded0c9",
}
BUILD_PUSH_WITH_PATHS_IGNORE = """  push:
    branches:
    - master
    - main
    - feature-**
    paths-ignore:
    - CHANGELOG.md
    tags-ignore:
    - v*
    - sdk/*
    - "**"
  pull_request: {}
  workflow_dispatch: {}
"""
BUILD_PUSH_NORMALIZED = BUILD_PUSH_WITH_PATHS_IGNORE.replace(
    "    paths-ignore:\n    - CHANGELOG.md\n", ""
).replace("    - \"**\"\n", "")
PAGES_WORKFLOW = """name: pages
on:
  push:
    branches: [main]
  workflow_dispatch: {}

permissions:
  contents: read

concurrency:
  group: pages
  cancel-in-progress: true

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
    - uses: actions/setup-node@249970729cb0ef3589644e2896645e5dc5ba9c38 # v6
      with:
        node-version: 24.18.0
        cache: npm
        cache-dependency-path: website/package-lock.json
    - uses: actions/configure-pages@983d7736d9b0ae728b81ab479565c72886d7745b # v5
    - run: npm ci --prefix website
    - run: npm --prefix website run build
    - uses: actions/upload-pages-artifact@7b1f4a764d45c48632c6b24a0339c27f5614fb0b # v4
      with:
        path: website/dist
  deploy:
    environment:
      name: github-pages
      url: ${{ steps.deployment.outputs.page_url }}
    needs: build
    permissions:
      pages: write
      id-token: write
    runs-on: ubuntu-latest
    steps:
    - id: deployment
      uses: actions/deploy-pages@d6db90164ac5ed86f2b6aed7e0febac5b3c0c03e # v4
"""


def normalize_trailing_whitespace(text: str) -> str:
    return "\n".join(line.rstrip() for line in text.splitlines()) + "\n"


def normalize_pages_workflow(path: Path) -> None:
    if not path.exists() or normalize_trailing_whitespace(path.read_text()) != PAGES_WORKFLOW:
        path.write_text(PAGES_WORKFLOW)


def replace_exact(text: str, old: str, new: str) -> str:
    old_count = text.count(old)
    new_count = text.count(new)
    if old_count == 1 and new_count == 0:
        return text.replace(old, new, 1)
    if old_count == 0 and new_count == 1:
        return text
    raise SystemExit(f"expected exactly one old xor normalized pattern: old={old_count}, new={new_count}")


def exact_once(path: Path, old: str, new: str) -> None:
    path.write_text(replace_exact(path.read_text(), old, new))


def exact_optional(path: Path, old: str, new: str) -> None:
    text = path.read_text()
    old_count = text.count(old)
    new_count = text.count(new)
    if old_count == 0 and new_count == 0:
        raise SystemExit(f"expected generated pattern not found in {path}: {old!r}")
    if old_count and new_count:
        raise SystemExit(f"generated pattern is partially normalized in {path}")
    if old_count > 1 or new_count > 1:
        raise SystemExit(f"generated pattern is ambiguous in {path}")
    if old_count:
        path.write_text(text.replace(old, new, 1))


def exact_remove(path: Path, old: str) -> None:
    text = path.read_text()
    count = text.count(old)
    if count == 0:
        return
    if count != 1:
        raise SystemExit(f"expected at most one removable generated pattern: count={count}")
    path.write_text(text.replace(old, "", 1))


def executable_run_lines(text: str) -> list[str]:
    """Return literal commands from YAML run fields, excluding shell setup/comments."""
    lines = text.splitlines()
    commands = []
    index = 0
    while index < len(lines):
        match = re.match(r"^(\s*)(?:-\s*)?run:\s*(.*)$", lines[index])
        if not match:
            index += 1
            continue
        indent, value = len(match.group(1)), match.group(2).strip()
        script = []
        if value and value not in {"|", "|-", "|+", ">", ">-", ">+"}:
            script.append(value)
        index += 1
        while index < len(lines):
            candidate = lines[index]
            if candidate.strip() and len(candidate) - len(candidate.lstrip()) <= indent:
                break
            script.append(candidate.strip())
            index += 1
        dead_conditional = False
        for command in script:
            command = command.strip()
            if not command or command.startswith("#"):
                continue
            if re.fullmatch(r"if\s+false;\s*then", command):
                dead_conditional = True
                continue
            if dead_conditional:
                if command == "fi":
                    dead_conditional = False
                continue
            if command in {"set -e", "set -o errexit", "set -o pipefail", "set -euo pipefail"}:
                continue
            commands.append(command)
    return commands


def require_exact_run_values(text: str, expected: tuple[str, ...]) -> None:
    commands = executable_run_lines(text)
    for value in expected:
        if commands.count(value) != 1:
            raise SystemExit(f"expected exactly one executable run value {value!r}, found {commands.count(value)}")


def _job_sections(text: str) -> dict[str, str]:
    jobs = text.find("jobs:\n")
    if jobs < 0:
        raise SystemExit("workflow has no jobs section")
    job_text = text[jobs + 6:]
    boundary = re.search(r"^[^ \t\n].*:$", job_text, re.MULTILINE)
    if boundary:
        job_text = job_text[:boundary.start()]
    matches = list(re.finditer(r"^  ([A-Za-z0-9_-]+):$", job_text, re.MULTILINE))
    sections = {}
    for number, match in enumerate(matches):
        start = jobs + 6 + match.start()
        end = jobs + 6 + matches[number + 1].start() if number + 1 < len(matches) else jobs + 6 + len(job_text)
        sections[match.group(1)] = text[start:end]
    return sections


def validate_workflow_jobs(name: str, text: str, policy: dict[str, bool]) -> None:
    sections = _job_sections(text)
    if set(sections) != set(policy):
        raise SystemExit(f"workflow job policy mismatch for {name}: actual={sorted(sections)} policy={sorted(policy)}")
    versions = re.findall(r"^\s*GOVERSION:\s*[\"']?([^\"'\s]+)", text, re.MULTILINE)
    if any(version != GO_VERSION for version in versions):
        raise SystemExit(f"stale GOVERSION pin in {name}")
    job_versions_by_name = {
        job: re.findall(r"^\s*GOVERSION:\s*[\"']?([^\"'\s]+)", section, re.MULTILINE)
        for job, section in sections.items()
    }
    has_job_specific_versions = any(job_versions_by_name.values())
    for job, executes_go in policy.items():
        section = sections[job]
        job_versions = job_versions_by_name[job]
        if executes_go:
            if has_job_specific_versions and job_versions != [GO_VERSION]:
                raise SystemExit(f"Go-executing workflow {name}/{job} lacks per-job GOVERSION {GO_VERSION}")
            if not has_job_specific_versions and versions.count(GO_VERSION) == 0:
                raise SystemExit(f"Go-executing workflow {name}/{job} lacks GOVERSION {GO_VERSION}")
        if not executes_go:
            if re.search(r"^\s*(?:-\s*)?uses:.*(?:setup-go|setup-tools)", section, re.MULTILINE):
                raise SystemExit(f"non-Go workflow job invokes Go/project setup in {name}/{job}")
            for command in executable_run_lines(section):
                if re.search(r"\b(?:go|make)\b", command):
                    raise SystemExit(f"non-Go workflow job invokes Go/make in {name}/{job}")


def validate_workflow_policy(root: Path = ROOT) -> None:
    workflows = root / ".github" / "workflows"
    actual = {path.name for path in workflows.glob("*") if path.suffix in {".yml", ".yaml"}}
    expected = set(GENERATED_WORKFLOW_NAMES)
    if actual != expected:
        raise SystemExit(f"generated workflow filename set mismatch: actual={sorted(actual)} expected={sorted(expected)}")
    for name in GENERATED_WORKFLOW_NAMES:
        text = (workflows / name).read_text()
        if name == "pages.yml" and normalize_trailing_whitespace(text) != PAGES_WORKFLOW:
            raise SystemExit("pages workflow does not match canonical content")
        validate_workflow_jobs(name, text, WORKFLOW_JOB_POLICY[name])


def normalize_build_trigger(text: str) -> str:
    return replace_exact(text, BUILD_PUSH_WITH_PATHS_IGNORE, BUILD_PUSH_NORMALIZED)


def ensure_build_pull_request(text: str) -> str:
    old = "  pull_request: {}\n"
    if text.count(old) == 0 and text.count("  workflow_dispatch: {}\n") == 1:
        return text.replace("  workflow_dispatch: {}\n", "  pull_request: {}\n  workflow_dispatch: {}\n", 1)
    if text.count(old) == 1 and text.count("  workflow_dispatch: {}\n") == 1:
        return text
    raise SystemExit("expected exactly one build pull_request/workflow_dispatch trigger state")


def insert_in_job(path: Path, job: str, marker: str, insertion: str) -> None:
    text = path.read_text()
    start = text.find(f"  {job}:\n")
    if start < 0:
        raise SystemExit(f"expected job not found in {path}: {job}")
    next_job = re.search(r"\n  [^ ]", text[start + 3 :])
    end = start + 3 + next_job.start() if next_job else len(text)
    section = text[start:end]
    if insertion in section:
        return
    if section.count(marker) != 1:
        raise SystemExit(f"expected one insertion marker in {path} job {job}")
    path.write_text(text[:start] + section.replace(marker, insertion + marker, 1) + text[end:])


def trim_explicit_outputs() -> None:
    paths = [WORKFLOWS / name for name in GENERATED_WORKFLOW_NAMES]
    paths += [ROOT / ".github" / "ISSUE_TEMPLATE" / "bug.yaml"]
    paths += [ROOT / name for name in ("CODE-OF-CONDUCT.md", "CONTRIBUTING.md", "SECURITY.md")]
    for path in paths:
        if not path.exists():
            raise SystemExit(f"expected generated output not found: {path}")
        path.write_text("\n".join(line.rstrip() for line in path.read_text().splitlines()) + "\n")


def remove_validated_generated_mise() -> None:
    header = "# WARNING: This file is autogenerated - changes will be overwritten when regenerated by https://github.com/pulumi/ci-mgmt"
    candidates = (ROOT / ".config" / "mise.toml", ROOT / ".config" / "mise.test.toml")
    if not any(path.exists() for path in candidates):
        return
    validated = []
    for path in candidates:
        if not path.exists():
            raise SystemExit(f"refusing to remove missing generated mise file: {path}")
        content = path.read_bytes()
        try:
            lines = content.decode().splitlines()
        except UnicodeDecodeError as error:
            raise SystemExit(f"refusing to remove unrecognized generated mise file: {path}") from error
        if not lines or lines[0] != header:
            raise SystemExit(f"refusing to remove unrecognized generated mise file: {path}")
        if "[tools]" not in lines:
            raise SystemExit(f"refusing to remove malformed generated mise file: {path}")
        digest = hashlib.sha256(content).hexdigest()
        if digest != KNOWN_MISE_SHA256[path.name]:
            raise SystemExit(f"refusing to remove modified generated mise file: {path}")
        validated.append(path)
    for path in validated:
        path.unlink()


def main() -> None:
    # These are the only generator-owned files this policy is allowed to touch.
    pages = WORKFLOWS / "pages.yml"
    normalize_pages_workflow(pages)
    trim_explicit_outputs()
    remove_validated_generated_mise()
    build = WORKFLOWS / "build.yml"
    build.write_text(ensure_build_pull_request(build.read_text()))
    build.write_text(normalize_build_trigger(build.read_text()))
    exact_optional(build, "pulumi/action-release-by-pr-label@main", RELEASE_ACTION)
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
    build_text = build.read_text()
    if gate not in build_text:
        marker = "    - name: Build Provider\n"
        if build_text.count(marker) != 1:
            raise SystemExit(f"expected one build insertion marker in {build}")
        build.write_text(build_text.replace(marker, gate + marker, 1))
    docs_gate = """    - name: Setup Node for docs
      uses: actions/setup-node@249970729cb0ef3589644e2896645e5dc5ba9c38 # v6
      with:
        node-version: 24.18.0
        cache: npm
        cache-dependency-path: website/package-lock.json
    - name: Check documentation
      run: make docs_check
"""
    insert_in_job(
        build,
        "prerequisites",
        "    - name: Check OpenAPI and generated SDKs\n",
        docs_gate,
    )

    acceptance = WORKFLOWS / "run-acceptance-tests.yml"
    exact_remove(acceptance, "  pull_request:\n    paths-ignore:\n    - CHANGELOG.md\n")
    exact_once(
        acceptance,
        "  prerequisites:\n    runs-on:",
        "  prerequisites:\n    environment: dokploy-acceptance\n    runs-on:",
    )
    credential_step = """    - name: Require Dokploy acceptance credentials
      env:
        DOKPLOY_ENDPOINT: ${{ steps.esc-secrets.outputs.DOKPLOY_ENDPOINT }}
        DOKPLOY_API_KEY: ${{ steps.esc-secrets.outputs.DOKPLOY_API_KEY }}
      run: test -n "$DOKPLOY_ENDPOINT" && test -n "$DOKPLOY_API_KEY"
"""
    insert_in_job(acceptance, "prerequisites", "    - name: Setup Tools\n", credential_step)

    command_dispatch = WORKFLOWS / "command-dispatch.yml"
    exact_once(command_dispatch, "repository: pulumi/pulumi-dokploy", "repository: dimeskigj/pulumi-dokploy")

    release_command = WORKFLOWS / "release_command.yml"
    exact_optional(release_command, "pulumi/action-release-by-pr-label@main", RELEASE_ACTION)

    release = WORKFLOWS / "release.yml"
    exact_optional(
        release,
        "args: -p 3 release --clean --timeout 60m0s",
        "args: -p 3 -f .goreleaser.yml release --clean --timeout 60m0s",
    )

    for name in ("release.yml", "prerelease.yml"):
        exact_optional(WORKFLOWS / name, "pulumi/publish-go-sdk-action@v1", PUBLISH_GO_ACTION)

    for name in (".goreleaser.yml", ".goreleaser.prerelease.yml"):
        path = ROOT / name
        exact_optional(
            path,
            "github.com/pulumi/pulumi-dokploy/provider/pkg/version.Version={{.Tag}}",
            "github.com/dimeskigj/pulumi-dokploy/provider.Version={{.Tag}}",
        )

    for name in ("build.yml", "prerelease.yml", "release.yml", "run-acceptance-tests.yml", "weekly-pulumi-update.yml"):
        exact_once(WORKFLOWS / name, 'GOVERSION: "1.21.x"', f'GOVERSION: "{GO_VERSION}"')
    exact_once(
        WORKFLOWS / "lint.yml",
        """env:
  PULUMI_API: https://api.pulumi-staging.io
""",
        """env:
  GOVERSION: "1.25.13"
  PULUMI_API: https://api.pulumi-staging.io
""",
    )

    lint = ROOT / ".golangci.yml"
    exact_once(
        lint,
        """  enable:
    - goconst
    - gosec
    - lll
    - misspell
    - nakedret
    - revive
    - unconvert
""",
        """  enable: [goconst, gosec, misspell, nakedret, unconvert]
  disable: [lll, revive]
""",
    )
    exact_once(lint, """  enable:
    - gci
    - gofmt
""", """  enable:
    - gofmt
""")
    exact_once(
        lint,
        """        path: pkg/
        text: "var-naming" # https://github.com/pulumi/ci-mgmt/issues/2100
formatters:
""",
        """        path: pkg/
        text: "var-naming" # https://github.com/pulumi/ci-mgmt/issues/2100
      - linters:
          - goconst
        path: '.*_test\\.go'
formatters:
""",
    )

    if "paths-ignore:" in build.read_text() or "pull_request:" not in build.read_text():
        raise SystemExit("build workflow trigger normalization failed")
    if "pull_request:" in acceptance.read_text():
        raise SystemExit("acceptance workflow must not trigger on pull_request")
    require_exact_run_values(build.read_text(), EXPECTED_BUILD_GATES)
    validate_workflow_policy()


if __name__ == "__main__":
    main()
