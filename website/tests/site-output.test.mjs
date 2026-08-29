import assert from "node:assert/strict";
import test from "node:test";
import { assertAnchorExists, candidateSiteTargets } from "../scripts/site-output.mjs";

const DIST = "/tmp/site-dist";
const source = `${DIST}/guides/apps/index.html`;

test("resolves relative URLs against the source page and offers file candidates", () => {
  assert.deepEqual(candidateSiteTargets(DIST, source, "../../reference/project/"), [
    `${DIST}/reference/project/index.html`,
  ]);
  assert.deepEqual(candidateSiteTargets(DIST, source, "../../reference/project/index.html"), [
    `${DIST}/reference/project/index.html`, `${DIST}/reference/project/index.html/index.html`,
  ]);
});

test("enforces the project base and skips external URLs", () => {
  assert.deepEqual(candidateSiteTargets(DIST, source, "https://example.com/docs"), []);
  assert.throws(() => candidateSiteTargets(DIST, source, "/reference/project/"), /escapes base/);
});

test("checks anchor IDs literally, including regex-sensitive IDs", () => {
  assert.doesNotThrow(() => assertAnchorExists('<h2 id="a+b">Title</h2>', "a+b"));
  assert.throws(() => assertAnchorExists('<h2 id="other">Title</h2>', "a+b"), /Missing anchor/);
});
