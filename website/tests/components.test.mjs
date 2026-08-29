import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

test("property metadata has explicit accessible labels", async () => {
  const source = await readFile(new URL("../src/components/PropertyTable.astro", import.meta.url), "utf8");
  for (const label of ["Required", "Secret", "Replaces resource"]) {
    assert.match(source, new RegExp(label));
  }
});

test("language tabs enforce all supported languages", async () => {
  const source = await readFile(new URL("../src/components/LanguageTabs.astro", import.meta.url), "utf8");
  for (const language of ["typescript", "python", "go", "csharp", "java", "yaml"]) {
    assert.match(source, new RegExp(`"${language}"`));
  }
  assert.match(source, /syncKey="language"/);
});

test("home hero links preserve the configured Pages base", async () => {
  const source = await readFile(new URL("../src/components/HomeHero.astro", import.meta.url), "utf8");
  assert.match(source, /import \{ base \} from "\.\.\/site-config\.mjs"/);
  assert.match(source, /withBase\("getting-started"\)/);
  assert.match(source, /withBase\("reference"\)/);
  assert.doesNotMatch(source, /href="\/(getting-started|reference)\//);
});

test("light theme uses dark readable text for success and accent metadata", async () => {
  const source = await readFile(new URL("../src/styles/global.css", import.meta.url), "utf8");
  assert.match(source, /--sl-color-accent-high: #075b69/);
  assert.match(source, /--dokploy-success: #087a52/);
  assert.match(source, /--dokploy-table-heading: #b8f7ff/);
  assert.match(source, /:root\[data-theme="dark"\][\s\S]*--sl-color-accent-high: #b8f7ff/);
  assert.match(source, /\.property-table thead th[\s\S]*var\(--dokploy-table-heading\)/);
});
