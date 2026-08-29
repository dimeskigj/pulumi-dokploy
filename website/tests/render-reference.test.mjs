import assert from "node:assert/strict";
import { access, mkdir, mkdtemp, readFile, readdir, rename as fsRename, rm, writeFile } from "node:fs/promises";
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
  assert.match(mdx, /title: "Application"/);
  assert.match(mdx, /Generated from `schema\.json`/);
  assert.match(mdx, /\{\/\* environmentId replaceOnChanges: true \*\/\}/);
});

test("serializes hostile frontmatter title and description safely", () => {
  const title = 'Application: "unsafe"\nnext';
  const description = 'Description: "unsafe"\n---\nnext';
  const mdx = renderResource({ name: title, description, inputs: [], outputs: [] });
  assert.ok(mdx.startsWith(`---\ntitle: ${JSON.stringify(title)}\ndescription: ${JSON.stringify(description)}\n---\n`));
});

test("renders configuration properties as JSON component props", () => {
  const config = [{ name: "apiKey", description: 'A "key".', required: false }];
  const mdx = renderConfiguration({ config });
  assert.match(mdx, /title: "Configuration"/);
  assert.ok(mdx.includes(`<PropertyTable properties={${JSON.stringify(config)}} />`));
});

test("renders complex types with stable slug anchors", () => {
  const mdx = renderTypes({ types: [{ name: "ZedType", slug: "zed-type", description: "Zed.", properties: [] }, { name: "AlphaType", slug: "alpha-type", description: "Alpha.", properties: [] }] });
  assert.ok(mdx.indexOf('<h2 id="alpha-type">AlphaType</h2>') < mdx.indexOf('<h2 id="zed-type">ZedType</h2>'));
  assert.doesNotMatch(mdx, /\{#(?:alpha|zed)-type\}/);
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

function injectedFilesystem(renameOperation, rmOperation = rm) {
  return { access, mkdtemp, rename: renameOperation, rm: rmOperation };
}

test("restores the original target when installation rename fails", async () => {
  const parent = await mkdtemp(join(tmpdir(), "render-reference-install-failure-"));
  const target = join(parent, "reference");
  await mkdir(target);
  await writeFile(join(target, "old.mdx"), "old");
  let renameCount = 0;
  try {
    await assert.rejects(
      replaceGeneratedDirectory(target, { "new.mdx": "new" }, writeFile, injectedFilesystem(async (from, to) => {
        renameCount += 1;
        if (renameCount === 2) throw new Error("install rename failed");
        return fsRename(from, to);
      })),
      /install rename failed/,
    );
    assert.equal(await readFile(join(target, "old.mdx"), "utf8"), "old");
    assert.deepEqual(await readdir(target), ["old.mdx"]);
  } finally {
    await rm(parent, { recursive: true, force: true });
  }
});

test("preserves the backup when restoration fails", async () => {
  const parent = await mkdtemp(join(tmpdir(), "render-reference-restore-failure-"));
  const target = join(parent, "reference");
  await mkdir(target);
  await writeFile(join(target, "old.mdx"), "old");
  let renameCount = 0;
  try {
    await assert.rejects(
      replaceGeneratedDirectory(target, { "new.mdx": "new" }, writeFile, injectedFilesystem(async (from, to) => {
        renameCount += 1;
        if (renameCount >= 2) throw new Error(renameCount === 2 ? "install rename failed" : "restore rename failed");
        return fsRename(from, to);
      })),
      /install rename failed/,
    );
    const entries = await readdir(parent);
    const backups = entries.filter((entry) => entry.startsWith(".reference-backup-"));
    assert.equal(backups.length, 1);
    assert.equal(await readFile(join(parent, backups[0], "old.mdx"), "utf8"), "old");
  } finally {
    await rm(parent, { recursive: true, force: true });
  }
});

test("does not report cleanup failure after installing new output", async () => {
  const parent = await mkdtemp(join(tmpdir(), "render-reference-cleanup-failure-"));
  const target = join(parent, "reference");
  await mkdir(target);
  await writeFile(join(target, "old.mdx"), "old");
  let backupRemovals = 0;
  try {
    await replaceGeneratedDirectory(target, { "new.mdx": "new" }, writeFile, injectedFilesystem(fsRename, async (path, options) => {
      if (path.includes(".reference-backup-") && backupRemovals++ > 0) throw new Error("backup cleanup failed");
      return rm(path, options);
    }));
    assert.equal(await readFile(join(target, "new.mdx"), "utf8"), "new");
  } finally {
    await rm(parent, { recursive: true, force: true });
  }
});
