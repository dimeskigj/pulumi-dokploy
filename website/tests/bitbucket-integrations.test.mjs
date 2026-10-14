import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const read = (path) => readFileSync(new URL(path, import.meta.url), "utf8");

test("Bitbucket guide documents exclusive secret modes and conditional identity", () => {
  const guide = read("../src/content/docs/guides/bitbucket-integrations.mdx");
  for (const phrase of ["apiToken", "appPassword", "bitbucketEmail", "bitbucketUsername", "exactly one", "secret", "bitbucketId", "pulumi import"]) {
    assert.match(guide, new RegExp(phrase, "i"));
  }
  assert.match(guide, /config set --secret/i);
  assert.match(guide, /workspace/i);
  assert.match(guide, /empty string/i);
  assert.match(guide, /not guaranteed/i);
  assert.match(guide, /does not validate/i);
  assert.match(guide, /does not delete/i);
  assert.match(guide, /unmanaged workloads/i);
  assert.doesNotMatch(guide, /current Bitbucket.{0,50}compatible/i);
});
