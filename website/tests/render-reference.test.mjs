import assert from "node:assert/strict";
import { mkdir, mkdtemp, readFile, readdir, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { renderConfiguration, renderResource, renderTypes, replaceGeneratedDirectory } from "../scripts/render-reference.mjs";

test("renders generated notice and input/output metadata", () => {
  const mdx = renderResource({
    token: "dokploy:index:Application",
    name: "Application",
    slug: "application",
    description: "A Dokploy application.",
    inputs: [{
      name: "environmentId", type: "string", typeHref: null, description: "Environment.",
      required: true, secret: false, replaceOnChanges: true,
      defaultValue: null, environment: [],
    }],
    outputs: [],
  });
  assert.match(mdx, /title: Application/);
  assert.match(mdx, /Generated from `schema\.json`/);
  assert.match(mdx, /replaceOnChanges: true/);
});

test("renders configuration properties as JSON component props", () => {
  const config = [{ name: "apiKey", description: 'A "key".', required: false }];
  const mdx = renderConfiguration({ config });
  assert.match(mdx, /title: Configuration/);
  assert.ok(mdx.includes(`<PropertyTable properties={${JSON.stringify(config)}} />`));
});

test("renders complex types with stable slug anchors", () => {
  const mdx = renderTypes({ types: [{ name: "ZedType", slug: "zed-type", description: "Zed.", properties: [] }, { name: "AlphaType", slug: "alpha-type", description: "Alpha.", properties: [] }] });
  assert.ok(mdx.indexOf("## AlphaType {#alpha-type}") < mdx.indexOf("## ZedType {#zed-type}"));
});

test("replaces the generated directory atomically", async () => {
  const parent = await mkdtemp(join(tmpdir(), "render-reference-"));
  const target = join(parent, "reference");
  const writeTarget = join(parent, "write-target");
  await mkdir(target);
  await writeFile(join(target, "old.mdx"), "old");

  try {
    await assert.rejects(
      replaceGeneratedDirectory(target, { "new.mdx": "new" }, async () => {
        throw new Error("write failed");
      }),
      /write failed/,
    );
    assert.deepEqual(await readdir(target), ["old.mdx"]);
    assert.equal(await readFile(join(target, "old.mdx"), "utf8"), "old");

    await replaceGeneratedDirectory(target, { "new.mdx": "new" });
    assert.deepEqual(await readdir(target), ["new.mdx"]);
    assert.equal(await readFile(join(target, "new.mdx"), "utf8"), "new");
    await replaceGeneratedDirectory(writeTarget, { "another.mdx": "another" });
    assert.equal(await readFile(join(writeTarget, "another.mdx"), "utf8"), "another");
  } finally {
    await rm(parent, { recursive: true, force: true });
  }
});
