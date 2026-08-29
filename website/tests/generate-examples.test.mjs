import assert from "node:assert/strict";
import { mkdir, mkdtemp, readFile, readdir, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { loadExamples, renderCompleteExamples, writeCompleteExamples } from "../scripts/generate-examples.mjs";

const SOURCE_PATHS = [
  "nodejs/index.ts", "python/__main__.py", "go/main.go", "dotnet/Program.cs",
  "java/src/main/java/generated_program/App.java", "yaml/Pulumi.yaml",
];

async function fixtureRoot(contents = {}) {
  const root = await mkdtemp(path.join(os.tmpdir(), "dokploy-examples-"));
  for (const sourcePath of SOURCE_PATHS) {
    const file = path.join(root, sourcePath);
    await mkdir(path.dirname(file), { recursive: true });
    await writeFile(file, contents[sourcePath] ?? `example for ${sourcePath}\n`);
  }
  return root;
}

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

test("uses scalar and secret configuration accessors for endpoint and API key", async () => {
  const examples = await loadExamples(new URL("../../examples/", import.meta.url));
  const source = Object.fromEntries(examples.map(({ language, code }) => [language, code]));
  assert.match(source.typescript, /config\.require\("dokploy:endpoint"\)/);
  assert.match(source.typescript, /config\.requireSecret\("dokploy:apiKey"\)/);
  assert.match(source.python, /config\.require\("dokploy:endpoint"\)/);
  assert.match(source.python, /config\.require_secret\("dokploy:apiKey"\)/);
  assert.match(source.go, /cfg\.Require\("dokploy:endpoint"\)/);
  assert.match(source.go, /cfg\.RequireSecret\("dokploy:apiKey"\)/);
  assert.match(source.csharp, /config\.Require\("dokploy:endpoint"\)/);
  assert.match(source.csharp, /config\.RequireSecret\("dokploy:apiKey"\)/);
  assert.match(source.java, /config\.require\("dokploy:endpoint"\)/);
  assert.match(source.java, /config\.requireSecret\("dokploy:apiKey"\)/);
  assert.match(source.yaml, /dokploy:apiKey:\n\s+secret: true\n\s+value: replace-with-a-dokploy-api-key/);
  for (const code of [source.typescript, source.python, source.go, source.csharp]) {
    assert.doesNotMatch(code, /requireObject|require_object|RequireObject/);
  }
});

test("preserves canonical YAML output names in every tracked language", async () => {
  const examples = await loadExamples(new URL("../../examples/", import.meta.url));
  for (const { language, code } of examples) {
    for (const name of ["gitlabIntegration", "gitlabProject", "gitlabOwner", "gitlabNamespace", "gitlabRepository", "gitBranch"]) {
      assert.doesNotMatch(code, new RegExp(`(?:export const|ctx\\.export\\("${name})0`), `${language} has suffixed ${name}`);
    }
  }
});

test("rejects missing, empty, unsafe endpoint, PEM, and API-key example fixtures", async () => {
  const cases = [
    ["missing", async (root) => rm(path.join(root, "go/main.go"), { force: true })],
    ["empty", async (root) => writeFile(path.join(root, "go/main.go"), "\n")],
    ["unsafe endpoint", async (root) => writeFile(path.join(root, "go/main.go"), "https://dokploy.example.com\n")],
    ["PEM", async (root) => writeFile(path.join(root, "go/main.go"), "-----BEGIN PRIVATE KEY-----\n")],
    ["API key header", async (root) => writeFile(path.join(root, "go/main.go"), "x-api-key: leaked\n")],
  ];
  for (const [name, mutate] of cases) {
    const root = await fixtureRoot();
    await mutate(root);
    await assert.rejects(loadExamples(root), /Missing|empty|Unsafe|unsafe/i, name);
    await rm(root, { recursive: true, force: true });
  }
});

test("writes complete output atomically after successful loading", async () => {
  const root = await fixtureRoot();
  const targetDirectory = await mkdtemp(path.join(os.tmpdir(), "dokploy-output-"));
  const target = path.join(targetDirectory, "complete.mdx");
  await writeCompleteExamples(`${root}${path.sep}`, target);
  assert.match(await readFile(target, "utf8"), /<LanguageTabs examples=\{examples\} \/>/);
  assert.deepEqual(await readdir(targetDirectory), ["complete.mdx"]);
  await rm(root, { recursive: true, force: true });
  await rm(targetDirectory, { recursive: true, force: true });
});
