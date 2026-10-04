//go:build all || nodejs || python || dotnet || go || java || yaml

package examples

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func loadCanonicalYAML(t *testing.T) map[string]any {
	t.Helper()
	data, err := os.ReadFile("yaml/Pulumi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatalf("canonical YAML is invalid: %v", err)
	}
	return document
}

func validateYAMLResourceProperties(document map[string]any, resources map[string]any) error {
	rawResources, ok := document["resources"].(map[string]any)
	if !ok {
		return fmt.Errorf("resources must be an object")
	}
	resourceNames := make([]string, 0, len(rawResources))
	for name := range rawResources {
		resourceNames = append(resourceNames, name)
	}
	sort.Strings(resourceNames)
	for _, name := range resourceNames {
		resource, ok := rawResources[name].(map[string]any)
		if !ok {
			return fmt.Errorf("resource %q must be an object", name)
		}
		token, ok := resource["type"].(string)
		if !ok || token == "" {
			return fmt.Errorf("resource %q must have a type token", name)
		}
		schemaResource, ok := resources[token].(map[string]any)
		if !ok {
			return fmt.Errorf("resource %q has unknown type token %q", name, token)
		}
		inputProperties, ok := schemaResource["inputProperties"].(map[string]any)
		if !ok {
			return fmt.Errorf("resource %q (%q) has malformed schema input properties", name, token)
		}
		properties, ok := resource["properties"].(map[string]any)
		if !ok {
			return fmt.Errorf("resource %q (%q) properties must be an object", name, token)
		}
		propertyNames := make([]string, 0, len(properties))
		for property := range properties {
			propertyNames = append(propertyNames, property)
		}
		sort.Strings(propertyNames)
		for _, property := range propertyNames {
			if _, ok := inputProperties[property]; !ok {
				return fmt.Errorf("resource %q (%q) has unknown property %q", name, token, property)
			}
		}
	}
	return nil
}

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
	if len(resources) != 25 {
		t.Fatalf("canonical YAML has %d managed resources, want 25", len(resources))
	}
	want := map[string]int{
		"dokploy:index:Project":      1,
		"dokploy:index:Environment":  1,
		"dokploy:index:Application":  2,
		"dokploy:index:Compose":      1,
		"dokploy:index:Postgres":     1,
		"dokploy:index:MySQL":        1,
		"dokploy:index:MariaDB":      1,
		"dokploy:index:MongoDB":      1,
		"dokploy:index:Redis":        1,
		"dokploy:index:Domain":       2,
		"dokploy:index:Destination":  1,
		"dokploy:index:Backup":       1,
		"dokploy:index:VolumeBackup": 2,
		"dokploy:index:Schedule":     1,
		"dokploy:index:SSHKey":       1,
		"dokploy:index:Registry":     1,
		"dokploy:index:Tag":          1,
		"dokploy:index:ProjectTag":   1,
		"dokploy:index:Mount":        3,
		"dokploy:index:Server":       1,
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

func TestCanonicalServerExample(t *testing.T) {
	document := loadCanonicalYAML(t)
	resources := document["resources"].(map[string]any)
	server := resources["remoteServer"].(map[string]any)
	if server["type"] != "dokploy:index:Server" {
		t.Fatalf("remoteServer type = %v", server["type"])
	}
	properties := server["properties"].(map[string]any)
	for key, want := range map[string]any{"ipAddress": "192.0.2.10", "sshKeyId": "${sshKey.sshKeyId}", "serverType": "deploy", "enableDockerCleanup": false} {
		if properties[key] != want {
			t.Errorf("remoteServer %s = %v, want %v", key, properties[key], want)
		}
	}
	outputs := document["outputs"].(map[string]any)
	if outputs["remoteServerId"] != "${remoteServer.serverId}" {
		t.Errorf("remoteServerId = %v", outputs["remoteServerId"])
	}
	for name, raw := range resources {
		if name == "remoteServer" {
			continue
		}
		resource, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		props, _ := resource["properties"].(map[string]any)
		if props != nil && props["serverId"] == "${remoteServer.serverId}" {
			t.Errorf("workload %q targets unready remoteServer", name)
		}
	}
}

func TestGeneratedServerExamplesInstantiateAndExportIdentity(t *testing.T) {
	want := map[string][]string{
		"nodejs/index.ts":    {`new dokploy.Server("remoteServer"`, `export const remoteServerId = remoteServer.serverId`},
		"python/__main__.py": {`dokploy.Server("remoteServer"`, `pulumi.export("remoteServerId", remote_server.server_id)`},
		"go/main.go":         {`dokploy.NewServer(ctx, "remoteServer"`, `ctx.Export("remoteServerId", remoteServer.ServerId)`},
		"dotnet/Program.cs":  {`new Dokploy.Server("remoteServer"`, `["remoteServerId"] = remoteServer.ServerId`},
		"java/src/main/java/generated_program/App.java": {`new Server("remoteServer", ServerArgs.builder()`, `ctx.export("remoteServerId", remoteServer.serverId())`},
	}
	for path, markers := range want {
		t.Run(path, func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, marker := range markers {
				if !strings.Contains(string(data), marker) {
					t.Errorf("generated program %s is missing %q", path, marker)
				}
			}
		})
	}
}

func TestGenerationDoesNotAllocateCacheForOtherMakeTargets(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "mktemp-called")
	stub := filepath.Join(dir, "mktemp")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nprintf called > \"$MKTemp_MARKER\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	path := dir + string(os.PathListSeparator) + os.Getenv("PATH")
	cmd := exec.Command("make", "--no-print-directory", "-n", "provider")
	cmd.Dir = ".."
	cmd.Env = append(os.Environ(), "PATH="+path, "MKTemp_MARKER="+marker)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("make provider dry run: %v\n%s", err, out)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("non-generation target invoked mktemp (stat error %v)", err)
	}
}

func TestGenerationCacheFailurePreservesExistingExamples(t *testing.T) {
	root := t.TempDir()
	existing := filepath.Join(root, "examples", "nodejs", "index.ts")
	if err := os.MkdirAll(filepath.Dir(existing), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(existing, []byte("preserve me"), 0600); err != nil {
		t.Fatal(err)
	}
	makeMarker := filepath.Join(root, "make-called")
	fakeMake := filepath.Join(root, "make")
	if err := os.WriteFile(fakeMake, []byte("#!/bin/sh\nprintf called > \"$MAKE_MARKER\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	script, err := filepath.Abs("../scripts/gen-examples.sh")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", script, fakeMake, root)
	cmd.Env = append(os.Environ(), "TMPDIR="+filepath.Join(root, "does-not-exist"), "MAKE_MARKER="+makeMarker)
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("cache allocation unexpectedly succeeded: %s", out)
	}
	data, err := os.ReadFile(existing)
	if err != nil || string(data) != "preserve me" {
		t.Fatalf("existing example was changed: %q, %v", data, err)
	}
	if _, err := os.Stat(makeMarker); !os.IsNotExist(err) {
		t.Fatalf("make was invoked after cache allocation failure (stat error %v)", err)
	}
}

func TestGenerationHelperCleansOnlyAllocatedCache(t *testing.T) {
	root := t.TempDir()
	cacheRecord := filepath.Join(root, "cache-path")
	fakeMake := filepath.Join(root, "make")
	body := "#!/bin/sh\ntest -n \"$PULUMI_HOME\" && test -d \"$PULUMI_HOME\" || exit 9\ntest -n \"$PULUMI_HOME_OWNER_TOKEN\" || exit 10\ntest \"$(cat \"$PULUMI_HOME/.pulumi-dokploy-example-owner\")\" = \"$PULUMI_HOME_OWNER_TOKEN\" || exit 11\nprintf '%s' \"$PULUMI_HOME\" > \"$CACHE_RECORD\"\n"
	if err := os.WriteFile(fakeMake, []byte(body), 0755); err != nil {
		t.Fatal(err)
	}
	script, err := filepath.Abs("../scripts/gen-examples.sh")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", script, fakeMake, root)
	cmd.Env = append(os.Environ(), "TMPDIR="+root, "CACHE_RECORD="+cacheRecord)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generation helper: %v\n%s", err, out)
	}
	data, err := os.ReadFile(cacheRecord)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(string(data)); !os.IsNotExist(err) {
		t.Fatalf("owned temporary cache remains or was not cleaned: %q (stat %v)", data, err)
	}
}

func TestInnerGenerationTargetRejectsUnsafeCacheBeforeSideEffects(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name          string
		cacheValue    string
		unset         bool
		populated     bool
		matchingOwner bool
	}{
		{name: "unset", unset: true},
		{name: "empty", cacheValue: ""},
		{name: "invalid", cacheValue: filepath.Join(t.TempDir(), "not-a-directory")},
		{name: "existing-empty-shared-cache", cacheValue: filepath.Join(t.TempDir(), "shared-cache")},
		{name: "existing-populated-shared-cache", cacheValue: filepath.Join(t.TempDir(), "populated-cache"), populated: true},
		{name: "populated-shared-cache-with-matching-owner", cacheValue: filepath.Join(t.TempDir(), "published-plugin-cache"), populated: true, matchingOwner: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			if tt.populated || strings.Contains(tt.name, "shared-cache") {
				if err := os.MkdirAll(tt.cacheValue, 0700); err != nil {
					t.Fatal(err)
				}
			}
			var sentinel string
			if tt.populated {
				sentinel = filepath.Join(tt.cacheValue, "plugins", "resource-dokploy-0.3.0")
				if err := os.MkdirAll(filepath.Dir(sentinel), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(sentinel, []byte("shared plugin must remain untouched"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			const ownerToken = "copied-owner-token"
			if tt.matchingOwner {
				if err := os.WriteFile(filepath.Join(tt.cacheValue, ".pulumi-dokploy-example-owner"), []byte(ownerToken), 0600); err != nil {
					t.Fatal(err)
				}
			}
			marker := filepath.Join(tmp, "plugin-install-called")
			stubBin := filepath.Join(tmp, "bin")
			if err := os.MkdirAll(stubBin, 0755); err != nil {
				t.Fatal(err)
			}
			mise := filepath.Join(stubBin, "mise")
			if err := os.WriteFile(mise, []byte("#!/bin/sh\nprintf called > \"$PLUGIN_MARKER\"\nexit 1\n"), 0755); err != nil {
				t.Fatal(err)
			}
			existing := filepath.Join(root, "examples", "nodejs", "index.ts")
			original, err := os.ReadFile(existing)
			if err != nil {
				t.Fatal(err)
			}
			overrides := filepath.Join(tmp, "override.mk")
			if err := os.WriteFile(overrides, []byte("provider:\n\t@true\n"), 0600); err != nil {
				t.Fatal(err)
			}
			args := []string{"-j4", "--no-print-directory", "-C", root, "-f", "Makefile", "-f", overrides, "gen_examples_in_cache"}
			if !tt.unset {
				args = append(args, "PULUMI_HOME="+tt.cacheValue)
			}
			if tt.matchingOwner {
				args = append(args, "PULUMI_HOME_OWNER_TOKEN="+ownerToken)
			}
			cmd := exec.Command("make", args...)
			cmd.Env = append(os.Environ(), "PATH="+stubBin+string(os.PathListSeparator)+os.Getenv("PATH"), "PLUGIN_MARKER="+marker)
			if tt.unset {
				cmd.Env = append(cmd.Env, "PULUMI_HOME=")
			}
			if out, err := cmd.CombinedOutput(); err == nil {
				t.Fatalf("unsafe cache accepted: %s", out)
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("plugin installation ran before cache validation (stat error %v)", err)
			}
			if tt.populated {
				got, err := os.ReadFile(sentinel)
				if err != nil || string(got) != "shared plugin must remain untouched" {
					t.Fatalf("shared cache fixture changed: %q, %v", got, err)
				}
			}
			if tt.matchingOwner {
				got, err := os.ReadFile(filepath.Join(tt.cacheValue, ".pulumi-dokploy-example-owner"))
				if err != nil || string(got) != ownerToken {
					t.Fatalf("shared cache owner marker changed: %q, %v", got, err)
				}
			}
			got, err := os.ReadFile(existing)
			if err != nil || string(got) != string(original) {
				t.Fatalf("existing generated example changed: %v", err)
			}
		})
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

func TestValidateYAMLResourcePropertiesRejectsUnknownProperty(t *testing.T) {
	document := map[string]any{"resources": map[string]any{
		"project": map[string]any{
			"type":       "dokploy:index:Project",
			"properties": map[string]any{"name": "ok", "invalidProperty": true},
		},
	}}
	err := validateYAMLResourceProperties(document, schemaResources(t))
	if err == nil || !strings.Contains(err.Error(), "invalidProperty") {
		t.Fatalf("validation error = %v, want invalidProperty", err)
	}
}

func TestValidateYAMLResourcePropertiesRejectsMalformedResources(t *testing.T) {
	tests := []struct {
		name     string
		document map[string]any
		want     string
	}{
		{name: "missing resources", document: map[string]any{}, want: `resources`},
		{name: "non-object resources", document: map[string]any{"resources": "private-payload"}, want: `resources`},
		{name: "unknown token with empty properties", document: map[string]any{"resources": map[string]any{"safe-name": map[string]any{"type": "safe:unknown:Token", "properties": map[string]any{}}}}, want: `safe:unknown:Token`},
		{name: "unknown token with missing properties", document: map[string]any{"resources": map[string]any{"safe-name": map[string]any{"type": "safe:unknown:Token"}}}, want: `safe:unknown:Token`},
		{name: "non-object resource", document: map[string]any{"resources": map[string]any{"safe-name": "private-payload"}}, want: `safe-name`},
		{name: "non-object properties", document: map[string]any{"resources": map[string]any{"safe-name": map[string]any{"type": "dokploy:index:Project", "properties": "private-payload"}}}, want: `properties`},
		{name: "missing token", document: map[string]any{"resources": map[string]any{"safe-name": map[string]any{"properties": map[string]any{}}}}, want: `safe-name`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateYAMLResourceProperties(tt.document, schemaResources(t))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("validation error = %v, want an error containing %q", err, tt.want)
			}
			if strings.Contains(err.Error(), "private-payload") {
				t.Fatalf("validation error exposes payload: %v", err)
			}
		})
	}
}

func TestValidateYAMLResourcePropertiesAcceptsCanonicalYAML(t *testing.T) {
	document := loadCanonicalYAML(t)
	if err := validateYAMLResourceProperties(document, schemaResources(t)); err != nil {
		t.Fatal(err)
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
