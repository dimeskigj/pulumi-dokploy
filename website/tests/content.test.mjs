import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const required = [
  "index.mdx",
  "getting-started/installation.mdx",
  "getting-started/first-deployment.mdx",
  "concepts/projects-and-environments.mdx",
  "concepts/sources.mdx",
  "concepts/lifecycle-and-state.mdx",
  "concepts/secrets.mdx",
  "guides/applications.mdx",
  "guides/compose.mdx",
  "guides/databases.mdx",
  "guides/domains.mdx",
  "guides/imports.mdx",
  "guides/troubleshooting.mdx",
  "contributing.mdx",
];

test("all curated pages exist with title and description", async () => {
  for (const path of required) {
    const source = await readFile(new URL(`../src/content/docs/${path}`, import.meta.url), "utf8");
    assert.match(source, /^---\n[\s\S]*title:/);
    assert.match(source, /description:/);
  }
});

test("secret and destructive lifecycle guidance is explicit", async () => {
  const secrets = await readFile(new URL("../src/content/docs/concepts/secrets.mdx", import.meta.url), "utf8");
  assert.match(secrets, /pulumi config set --secret/);
  assert.match(secrets, /never log/i);
  const compose = await readFile(new URL("../src/content/docs/guides/compose.mdx", import.meta.url), "utf8");
  assert.match(compose, /deleteVolumesOnDestroy/);
  assert.match(compose, /false/);
});
