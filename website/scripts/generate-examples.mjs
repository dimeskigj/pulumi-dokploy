import { readFile, rename, writeFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const SOURCES = [
  ["typescript", "TypeScript", "nodejs/index.ts"],
  ["python", "Python", "python/__main__.py"],
  ["go", "Go", "go/main.go"],
  ["csharp", "C#", "dotnet/Program.cs"],
  ["java", "Java", "java/src/main/java/generated_program/App.java"],
  ["yaml", "YAML", "yaml/Pulumi.yaml"],
];

const UNSAFE = [
  /https:\/\/dokploy\.example\.com(?:[/?#\s]|$)/,
  /-----BEGIN [^-]*PRIVATE KEY-----/,
  /-----BEGIN [^-]*-----/,
  /x-api-key:/i,
];

function normalizeCode(code, sourcePath) {
  const normalized = code.replaceAll("\r\n", "\n").replaceAll("\r", "\n");
  if (!normalized.trim()) throw new Error(`Example source is empty: ${sourcePath}`);
  if (!normalized.endsWith("\n")) return `${normalized}\n`;
  return normalized;
}

export async function loadExamples(root) {
  const rootUrl = root instanceof URL ? root : pathToFileURL(root);
  const examples = [];
  for (const [language, label, relativePath] of SOURCES) {
    const sourcePath = relativePath.replaceAll("\\", "/");
    let code;
    try {
      code = normalizeCode(await readFile(new URL(sourcePath, rootUrl), "utf8"), sourcePath);
    } catch (error) {
      if (error.code === "ENOENT") throw new Error(`Missing example source: ${sourcePath}`);
      throw error;
    }
    if (UNSAFE.some((pattern) => pattern.test(code))) {
      throw new Error(`Unsafe content in example source: ${sourcePath}`);
    }
    examples.push({ language, label, code, sourcePath });
  }
  return examples;
}

export function renderCompleteExamples(examples) {
  if (examples.length !== SOURCES.length || examples.some((example, index) => example.language !== SOURCES[index][0])) {
    throw new Error("Complete examples must contain the six canonical languages in order");
  }
  const serialized = `[\n${examples.map((example) => `  { language: ${JSON.stringify(example.language)}, label: ${JSON.stringify(example.label)}, code: ${JSON.stringify(example.code).replaceAll("<", "\\u003C")}, sourcePath: ${JSON.stringify(example.sourcePath)} },`).join("\n")}\n]`;
  return `---
title: Complete provider example
description: A complete Dokploy deployment in six Pulumi languages.
---

import LanguageTabs from "../../../components/LanguageTabs.astro";

This page is generated from the tracked provider examples so every language stays synchronized with the canonical YAML source.

The generated examples use invalid placeholders for endpoints and credentials. Supply real values through secret configuration before deployment.

export const examples = ${serialized};

<LanguageTabs examples={examples} />
`;
}

export async function writeCompleteExamples(root = new URL("../../examples/", import.meta.url), target = join(dirname(fileURLToPath(import.meta.url)), "../src/content/docs/examples/complete.mdx")) {
  const content = renderCompleteExamples(await loadExamples(root));
  const temporary = `${target}.${process.pid}.tmp`;
  await writeFile(temporary, content, "utf8");
  await rename(temporary, target);
}

if (process.argv[1] && pathToFileURL(process.argv[1]).href === import.meta.url) {
  try {
    await writeCompleteExamples();
  } catch (error) {
    console.error(`Example generation failed: ${error.message}`);
    process.exitCode = 1;
  }
}
