import assert from "node:assert/strict";
import { readFile, stat } from "node:fs/promises";
import path from "node:path";
import test from "node:test";
import { BASE, ORIGIN, assertAnchorExists, candidateSiteTargets, collectFiles } from "../scripts/site-output.mjs";

const DIST = path.resolve(new URL("../dist/", import.meta.url).pathname);

async function exists(file) {
  try {
    await stat(file);
    return true;
  } catch {
    return false;
  }
}

test("built site has valid internal links, base paths, routes, and search index", async () => {
  assert.ok(await exists(path.join(DIST, "index.html")));
  assert.ok(await exists(path.join(DIST, "reference/application/index.html")));
  assert.ok(await exists(path.join(DIST, "examples/complete/index.html")));
  assert.ok(await exists(path.join(DIST, "pagefind")));

  const htmlFiles = (await collectFiles(DIST)).filter((file) => file.endsWith(".html") && path.relative(DIST, file) !== "404.html");
  for (const sourceFile of htmlFiles) {
    const html = await readFile(sourceFile, "utf8");
    const sourceRoute = path.relative(DIST, sourceFile).replaceAll(path.sep, "/");
    const attributes = [...html.matchAll(/\b(?:href|src)=(['"])(.*?)\1/g)].map((match) => match[2]);
    for (const rawUrl of attributes) {
      if (!rawUrl || rawUrl.startsWith("#") || /^(?:mailto:|tel:)/i.test(rawUrl)) continue;
      const parsed = new URL(rawUrl, `${ORIGIN}${BASE}/${sourceRoute}`);
      if (parsed.origin !== ORIGIN) continue;
      const targets = candidateSiteTargets(DIST, sourceFile, rawUrl);
      let target;
      for (const candidate of targets) {
        if (await exists(candidate)) {
          target = candidate;
          break;
        }
      }
      assert.ok(target, `Missing target for ${rawUrl} in ${sourceRoute}`);
      if (parsed.hash) {
        const targetHtml = await readFile(target, "utf8");
        const id = decodeURIComponent(parsed.hash.slice(1));
        assertAnchorExists(targetHtml, id);
      }
    }
  }
});
