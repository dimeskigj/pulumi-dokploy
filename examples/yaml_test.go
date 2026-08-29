//go:build all

package examples

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	if len(resources) != 8 {
		t.Fatalf("canonical YAML has %d managed resources, want 8", len(resources))
	}
	want := map[string]int{
		"dokploy:index:Project":     1,
		"dokploy:index:Environment": 1,
		"dokploy:index:Application": 1,
		"dokploy:index:Compose":     1,
		"dokploy:index:Postgres":    1,
		"dokploy:index:Redis":       1,
		"dokploy:index:Domain":      2,
	}
	counts := map[string]int{}
	for name, raw := range resources {
		resource, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("resource %q is not an object", name)
		}
		typeName, _ := resource["type"].(string)
		if _, exists := want[typeName]; !exists {
			t.Errorf("resource %q has unknown type token %q", name, typeName)
		} else {
			counts[typeName]++
		}
		if strings.Contains(typeName, "Domain") && resource["properties"] == nil {
			t.Errorf("resource %q has no properties", name)
		}
	}
	for typeName, expected := range want {
		if counts[typeName] != expected {
			t.Errorf("canonical YAML has %d %s resources, want %d", counts[typeName], typeName, expected)
		}
		if _, ok := schemaResources(t)[typeName]; !ok {
			t.Errorf("%s is not present in generated provider schema", typeName)
		}
	}
}

func TestCanonicalYAMLActuallyBindsWithPulumi(t *testing.T) {
	out := t.TempDir()
	cmd := exec.Command("mise", "exec", "pulumi@3.259.0", "--", "pulumi", "convert", "--from", "yaml", "--language", "yaml", "--cwd", "yaml", "--out", out, "--generate-only")
	cmd.Dir = "."
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Pulumi YAML binding failed: %v\n%s", err, output)
	}
}

func TestGeneratedProjectRuntimes(t *testing.T) {
	for language, wantRuntime := range map[string]string{
		"nodejs": "nodejs",
		"python": "python",
		"go":     "go",
		"dotnet": "dotnet",
		"java":   "java",
	} {
		t.Run(language, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(language, "Pulumi.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			var manifest struct {
				Runtime string `yaml:"runtime"`
			}
			if err := yaml.Unmarshal(data, &manifest); err != nil {
				t.Fatalf("generated %s manifest is invalid: %v", language, err)
			}
			if manifest.Runtime != wantRuntime {
				t.Errorf("generated %s manifest runtime = %q, want %q", language, manifest.Runtime, wantRuntime)
			}
		})
	}
}

func TestCanonicalYAMLRejectsUnknownProperty(t *testing.T) {
	canonical, err := os.ReadFile("yaml/Pulumi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	variant := strings.Replace(string(canonical), "      name: dokploy-mvp\n", "      invalidProperty: true\n      name: dokploy-mvp\n", 1)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Pulumi.yaml"), []byte(variant), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "out")
	cmd := exec.Command("mise", "exec", "pulumi@3.259.0", "--", "pulumi", "convert", "--from", "yaml", "--language", "yaml", "--strict", "--cwd", root, "--out", out, "--generate-only")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("Pulumi YAML binding unexpectedly accepted invalidProperty")
	}
	if !strings.Contains(string(output), "invalidProperty") {
		t.Fatalf("binding error omitted invalid property: %v\n%s", err, output)
	}
}

func TestGeneratedArtifactsArePortableAndDocumentAlternatives(t *testing.T) {
	root, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	workspaceRoot := filepath.Dir(root)
	for _, language := range []string{"nodejs", "python", "go", "dotnet", "java"} {
		languageRoot := filepath.Join(root, language)
		if err := filepath.Walk(languageRoot, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if info.IsDir() {
				if path != languageRoot && map[string]bool{"bin": true, "obj": true, "target": true, "node_modules": true, "__pycache__": true}[info.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if strings.Contains(string(data), workspaceRoot) {
				return fmt.Errorf("%s contains workspace absolute path", path)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		readme, err := os.ReadFile(filepath.Join(languageRoot, "README.md"))
		if err != nil {
			t.Fatal(err)
		}
		text := string(readme)
		if !strings.Contains(text, "Git source alternative") || !strings.Contains(text, "GitLab source alternative") {
			t.Errorf("%s README omits inactive source alternatives", language)
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
