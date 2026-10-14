import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const read = (path) => readFileSync(new URL(path, import.meta.url), "utf8");

test("Gitea guide documents manual authorization, secret configuration, and relation import", () => {
  const guide = read("../src/content/docs/guides/gitea-integrations.mdx");
  for (const phrase of ["clientSecret", "giteaInternalUrl", "giteaId", "pulumi import", "authorize", "does not create", "does not validate", "does not delete", "unmanaged workloads", "organizationName", "null"]) {
    assert.match(guide, new RegExp(phrase, "i"));
  }
  assert.match(guide, /config set --secret/i);
  assert.match(guide, /https:\/\/[^\s"`]+/);
  assert.doesNotMatch(guide, /Gitea(?:Integration)? source type/i);
});
