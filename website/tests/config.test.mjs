import assert from "node:assert/strict";
import test from "node:test";
import { base, output, site } from "../src/site-config.mjs";

test("site configuration uses the project Pages URL", () => {
  assert.equal(site, "https://dimeskigj.github.io");
  assert.equal(base, "/pulumi-dokploy");
  assert.equal(output, "static");
});
