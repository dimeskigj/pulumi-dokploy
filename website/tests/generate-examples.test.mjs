import assert from "node:assert/strict";
import test from "node:test";
import { loadExamples, renderCompleteExamples } from "../scripts/generate-examples.mjs";

test("loads all complete examples in canonical order", async () => {
  const examples = await loadExamples(new URL("../../examples/", import.meta.url));
  assert.deepEqual(examples.map(({ language }) => language), [
    "typescript", "python", "go", "csharp", "java", "yaml",
  ]);
  assert.ok(examples.every(({ code }) => code.endsWith("\n")));
});

test("renders one synchronized six-language component", async () => {
  const examples = await loadExamples(new URL("../../examples/", import.meta.url));
  const mdx = renderCompleteExamples(examples);
  assert.equal((mdx.match(/language:/g) ?? []).length, 6);
  assert.match(mdx, /<LanguageTabs examples=\{examples\} \/>/);
  assert.doesNotMatch(mdx, /dokploy\.example\.com|x-api-key:/);
});
