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

test("formats notification string maps", () => {
  assert.equal(formatType({ type: "object", additionalProperties: { type: "string" } }), "map<string, string>");
  assert.throws(() => formatType({ type: "object", additionalProperties: { type: "integer" } }), /Unsupported Pulumi property type/);
  assert.throws(() => formatType({ type: "object", properties: { key: { type: "string" } } }), /Unsupported Pulumi property type/);
