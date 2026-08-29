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

test("property table uses Starlight's own Badge component for metadata", async () => {
  const source = await readFile(new URL("../src/components/PropertyTable.astro", import.meta.url), "utf8");
  assert.match(source, /import \{ Badge \} from "@astrojs\/starlight\/components"/);
});
