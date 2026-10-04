import assert from "node:assert/strict";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { formatType, loadSchema, parseSchema, slugFromToken } from "../scripts/reference-model.mjs";

const schema = {
  name: "dokploy",
  config: {
    variables: {
      apiKey: {
        type: "string",
        description: "API key.",
        secret: true,
        defaultInfo: { environment: ["DOKPLOY_API_KEY"] },
      },
    },
  },
  types: {
    "dokploy:index:Source": {
      type: "object",
      description: "Source.",
      properties: { branch: { type: "string", description: "Branch." } },
      required: ["branch"],
    },
  },
  resources: {
    "dokploy:index:Application": {
      description: "Application.",
      inputProperties: {
        environmentId: {
          type: "string",
          description: "Environment.",
          replaceOnChanges: true,
        },
        source: { $ref: "#/types/dokploy:index:Source", description: "Source." },
      },
      requiredInputs: ["environmentId", "source"],
      properties: {
        applicationId: { type: "string", description: "ID." },
      },
      required: ["applicationId"],
    },
  },
};

test("normalizes config, inputs, outputs, and complex types", () => {
  const model = parseSchema(schema, { expectedResources: new Set(["Application"]) });
  assert.deepEqual(model.config[0], {
    name: "apiKey",
    type: "string",
    typeHref: null,
    description: "API key.",
    required: false,
    secret: true,
    replaceOnChanges: false,
    defaultValue: null,
    environment: ["DOKPLOY_API_KEY"],
  });
  assert.equal(model.resources[0].slug, "application");
  assert.equal(model.resources[0].inputs[0].replaceOnChanges, true);
  assert.equal(model.resources[0].inputs[1].typeHref, "../types/#source");
  assert.equal(model.types[0].properties[0].required, true);
  assert.deepEqual(model.functions, []);
});

test("normalizes function contracts deterministically and validates descriptions/references", () => {
  const project = { description: "Read project metadata.", inputs: { properties: { projectId: { type: "string", description: "ID." } }, required: ["projectId"] }, outputs: { properties: { name: { type: "string", description: "Name." }, projectId: { type: "string", description: "ID." }, description: { type: "string", description: "Description." } }, required: ["name", "projectId"] } };
  const fixture = { ...schema, functions: { "dokploy:index:getProject": project, "dokploy:index:getEnvironment": { ...structuredClone(project), description: "Read environment metadata." } } };
  const model = parseSchema(fixture, { expectedResources: new Set(["Application"]) });
  assert.deepEqual(model.functions.map(({ name }) => name), ["getEnvironment", "getProject"]);
  const result = model.functions[1];
  assert.equal(result.name, "getProject");
  assert.equal(result.slug, "get-project");
  assert.equal(result.inputs[0].name, "projectId");
  assert.equal(result.inputs[0].required, true);
  assert.deepEqual(result.outputs.map(({ name, type, required }) => ({ name, type, required })), [
    { name: "description", type: "string", required: false },
    { name: "name", type: "string", required: true },
    { name: "projectId", type: "string", required: true },
  ]);
  const badDescription = structuredClone(fixture);
  delete badDescription.functions["dokploy:index:getProject"].outputs.properties.name.description;
  assert.throws(() => parseSchema(badDescription, { expectedResources: new Set(["Application"]) }), /Missing description.*getProject.*name/);
  const badReference = structuredClone(fixture);
  badReference.functions["dokploy:index:getProject"].inputs.properties.projectId = { $ref: "#/types/dokploy:index:Missing", description: "ID." };
  assert.throws(() => parseSchema(badReference, { expectedResources: new Set(["Application"]) }), /Dangling type reference/);
});

test("formats every schema type used by the provider", () => {
  assert.equal(formatType({ type: "string" }), "string");
  assert.equal(formatType({ type: "integer" }), "integer");
  assert.equal(formatType({ type: "boolean" }), "boolean");
  assert.equal(formatType({ type: "array", items: { type: "string" } }), "string[]");
  assert.equal(formatType({ $ref: "#/types/dokploy:index:Source" }), "Source");
});

test("derives stable lowercase slugs", () => {
  assert.equal(slugFromToken("dokploy:index:Postgres"), "postgres");
  assert.equal(slugFromToken("dokploy:index:getSSHKey"), "get-ssh-key");
  assert.equal(slugFromToken("dokploy:index:getMySQL"), "get-mysql");
});

test("rejects tokens that are not exactly dokploy:index:identifier", () => {
  for (const token of [
    "dokploy:other:Postgres",
    "dokploy:index:Postgres:Extra",
    "dokploy:index:",
    "other:index:Postgres",
    "dokploy:index:Postgres/unsafe",
  ]) {
    assert.throws(() => slugFromToken(token), /Invalid Pulumi token/);
  }
});

test("loads and validates the real provider schema", async () => {
  const model = parseSchema(
    await loadSchema(new URL("../../provider/cmd/pulumi-resource-dokploy/schema.json", import.meta.url)),
  );
   assert.equal(model.resources.length, 20);
  assert.equal(model.config.find(({ name }) => name === "apiKey").secret, true);
   assert.equal(
    model.resources
      .find(({ name }) => name === "Application")
      .inputs.find(({ name }) => name === "environmentId").replaceOnChanges,
     true,
   );
   for (const resource of ["SSHKey", "Registry", "Tag", "ProjectTag", "Mount", "Schedule", "Port"]) {
     assert.ok(model.resources.some(({ name }) => name === resource), `${resource} is published`);
   }
   assert.equal(model.resources.find(({ name }) => name === "SSHKey").inputs.find(({ name }) => name === "privateKey").secret, true);
   assert.equal(model.resources.find(({ name }) => name === "SSHKey").inputs.find(({ name }) => name === "privateKey").replaceOnChanges, true);
   assert.equal(model.resources.find(({ name }) => name === "ProjectTag").inputs.find(({ name }) => name === "projectId").replaceOnChanges, true);
   assert.deepEqual(model.functions.map(({ name }) => name), ["getApplication", "getCompose", "getEnvironment", "getMariaDB", "getMongoDB", "getMySQL", "getPostgres", "getProject", "getRedis", "getRegistry", "getSSHKey", "getServer"]);
   for (const fn of model.functions) {
     assert.equal(fn.inputs.length, 1);
     assert.equal(fn.inputs[0].required, true);
     assert.equal(fn.outputs.find(({ name }) => name === "name").required, true);
     assert.equal(fn.outputs.find(({ name }) => name === fn.inputs[0].name).required, true);
     assert.ok(fn.outputs.every(({ description }) => description));
   }
});

test("rejects duplicate normalized resource names", () => {
  const resources = {
    ...schema.resources,
    "dokploy:index:Nested:Application": schema.resources["dokploy:index:Application"],
  };
  assert.throws(
    () => parseSchema({ ...schema, resources }, { expectedResources: new Set(["Application"]) }),
    /Unexpected resource set/,
  );
});

test("rejects resource tokens that do not exactly match expected tokens", () => {
  const resources = { "dokploy:other:Application": schema.resources["dokploy:index:Application"] };
  assert.throws(
    () => parseSchema({ ...schema, resources }, { expectedResources: new Set(["Application"]) }),
    /Unexpected resource set/,
  );
});

test("rejects missing descriptions and dangling type references", () => {
  const missingDescription = structuredClone(schema);
  missingDescription.resources["dokploy:index:Application"].description = "";
  assert.throws(
    () => parseSchema(missingDescription, { expectedResources: new Set(["Application"]) }),
    /Missing description/,
  );

  const danglingReference = structuredClone(schema);
  danglingReference.resources["dokploy:index:Application"].inputProperties.source.$ref =
    "#/types/dokploy:index:Missing";
  assert.throws(
    () => parseSchema(danglingReference, { expectedResources: new Set(["Application"]) }),
    /Dangling type reference/,
  );
});

test("includes the source path in validation errors for loaded schemas", async () => {
  const directory = await mkdtemp(join(tmpdir(), "reference-model-"));
  const path = join(directory, "malformed-schema.json");
  try {
    await writeFile(path, JSON.stringify({ ...schema, name: "not-dokploy" }));
    const loaded = await loadSchema(path);
    assert.throws(() => parseSchema(loaded), new RegExp(`not-dokploy.*${path}`));
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});
