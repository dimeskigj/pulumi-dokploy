import assert from "node:assert/strict";
import test from "node:test";

test("Astro is configured for the project Pages URL", async () => {
  const { default: config } = await import("../astro.config.mjs");
  assert.equal(config.site, "https://gjorgjidimeski.github.io");
  assert.equal(config.base, "/pulumi-dokploy");
  assert.equal(config.output, "static");
  assert.equal(config.integrations.length, 1);
});
