import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const read = (path) => readFileSync(new URL(path, import.meta.url), "utf8");

test("GitHub guide documents manual setup, relation import, and Dokploy-only deletion", () => {
  const guide = read("../src/content/docs/guides/github-integrations.mdx");
  for (const phrase of ["authorize", "githubId", "pulumi import", "githubAppName", "does not create", "does not delete", "unmanaged workloads"]) {
    assert.match(guide, new RegExp(phrase, "i"));
  }
});

test("canonical YAML keeps GitHub integration out of the default resource graph", () => {
  const yaml = read("../../examples/yaml/Pulumi.yaml");
  assert.doesNotMatch(yaml, /^\s+type: dokploy:index:GitHubIntegration$/m);
  assert.match(yaml, /options \(import:/i);
  assert.match(yaml, /githubId/);
});
