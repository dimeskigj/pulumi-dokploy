import assert from "node:assert/strict";
import { readdir } from "node:fs/promises";
import path from "node:path";

export const BASE = "/pulumi-dokploy";
export const ORIGIN = "https://gjorgjidimeski.github.io";

export async function collectFiles(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  const files = [];
  for (const entry of entries) {
    const file = path.join(directory, entry.name);
    if (entry.isDirectory()) files.push(...await collectFiles(file));
    else files.push(file);
  }
  return files;
}

export function candidateSiteTargets(dist, sourceFile, rawUrl) {
  const sourceRoute = path.relative(dist, sourceFile)
    .replaceAll(path.sep, "/")
    .replace(/index\.html$/, "");
  const url = new URL(rawUrl, `${ORIGIN}${BASE}/${sourceRoute}`);
  if (url.origin !== ORIGIN) return [];
  assert.ok(url.pathname === BASE || url.pathname.startsWith(`${BASE}/`), `URL escapes base: ${rawUrl}`);
  const relative = decodeURIComponent(url.pathname.slice(BASE.length)).replace(/^\//, "");
  const target = path.join(dist, relative);
  return relative.endsWith("/") || relative === ""
    ? [path.join(target, "index.html")]
    : [target, path.join(target, "index.html")];
}

export function assertAnchorExists(html, id) {
  const escaped = id.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  assert.match(html, new RegExp(`\\bid=(['"])${escaped}\\1`), `Missing anchor ${id}`);
}
