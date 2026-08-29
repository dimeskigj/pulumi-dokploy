//go:build all

package examples

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func repoPath(t *testing.T, parts ...string) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(append([]string{wd, ".."}, parts...)...)
}

func providerSchema(t *testing.T) map[string]any {
	t.Helper()
	data, err := os.ReadFile(repoPath(t, "provider", "cmd", "pulumi-resource-dokploy", "schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	return schema
}

func schemaResources(t *testing.T) map[string]any {
	t.Helper()
	resources, ok := providerSchema(t)["resources"].(map[string]any)
	if !ok {
		t.Fatal("provider schema has no resources")
	}
	return resources
}
