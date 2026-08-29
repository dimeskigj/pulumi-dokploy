//go:build all

package examples

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestCanonicalYAMLUsesGeneratedSchema(t *testing.T) {
	data, err := os.ReadFile("yaml/Pulumi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatalf("canonical YAML is invalid: %v", err)
	}

	resources, ok := document["resources"].(map[string]any)
	if !ok {
		t.Fatal("canonical YAML has no resources")
	}
	want := map[string]bool{
		"dokploy:index:Project":     false,
		"dokploy:index:Environment": false,
		"dokploy:index:Application": false,
		"dokploy:index:Compose":     false,
		"dokploy:index:Postgres":    false,
		"dokploy:index:Redis":       false,
		"dokploy:index:Domain":      false,
	}
	for name, raw := range resources {
		resource, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("resource %q is not an object", name)
		}
		typeName, _ := resource["type"].(string)
		if _, exists := want[typeName]; exists {
			want[typeName] = true
		}
		if strings.Contains(typeName, "Domain") && resource["properties"] == nil {
			t.Errorf("resource %q has no properties", name)
		}
	}
	for typeName, found := range want {
		if !found {
			t.Errorf("canonical YAML is missing %s", typeName)
		}
		if _, ok := schemaResources(t)[typeName]; !ok {
			t.Errorf("%s is not present in generated provider schema", typeName)
		}
	}
}

func TestGeneratedLanguageExamplesExist(t *testing.T) {
	for _, language := range []string{"nodejs", "python", "go", "dotnet", "java"} {
		if _, err := os.Stat(language); err != nil {
			t.Errorf("generated %s example is missing: %v", language, err)
		}
	}
}
