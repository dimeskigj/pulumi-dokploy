package dokploy

import (
	"strings"
	"testing"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
)

func TestComposeSourceValidateRequiresExactlyOneConfiguredVariant(t *testing.T) {
	validGit := &GitComposeSource{URL: "https://example.test/repo", Branch: "main"}
	tests := []struct {
		name   string
		source ComposeSource
		want   string
	}{
		{"missing variant", ComposeSource{Type: ComposeSourceRaw}, "source.raw is required when source.type is raw"},
		{"raw extra git", ComposeSource{Type: ComposeSourceRaw, Raw: &RawComposeSource{ComposeFile: "services: {}"}, Git: validGit}, "source.git must be omitted when source.type is raw"},
		{"git missing", ComposeSource{Type: ComposeSourceGit}, "source.git is required when source.type is git"},
		{"git empty url", ComposeSource{Type: ComposeSourceGit, Git: &GitComposeSource{Branch: "main"}}, "source.git.url must not be empty"},
		{"gitlab missing", ComposeSource{Type: ComposeSourceGitLab}, "source.gitlab is required when source.type is gitlab"},
		{"unknown", ComposeSource{Type: "other"}, "source.type must be one of raw, git, or gitlab"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.source.validate(); err == nil || err.Error() != tc.want {
				t.Fatalf("validate() = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestComposeSourceDefaults(t *testing.T) {
	source := property.New(map[string]property.Value{"type": property.New("raw"), "raw": property.New(map[string]property.Value{"composeFile": property.New("services: {}")})})
	got, err := (Compose{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{"name": property.New("demo"), "environmentId": property.New("e1"), "source": source})})
	if err != nil || len(got.Failures) != 0 {
		t.Fatalf("Check() = %#v, %v", got.Failures, err)
	}
	if got.Inputs.ComposeType != ComposeDocker {
		t.Fatalf("defaults = %#v", got.Inputs)
	}
}

func TestComposeGitSourceDefaultsComposePath(t *testing.T) {
	source := property.New(map[string]property.Value{
		"type": property.New("git"),
		"git":  property.New(map[string]property.Value{"url": property.New("https://example.test/repo"), "branch": property.New("main")}),
	})
	got, err := (Compose{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{"name": property.New("demo"), "environmentId": property.New("e1"), "source": source})})
	if err != nil || len(got.Failures) != 0 {
		t.Fatalf("Check() = %#v, %v", got.Failures, err)
	}
	if got.Inputs.Source.Git.ComposePath != "./docker-compose.yml" {
		t.Fatalf("compose path = %q", got.Inputs.Source.Git.ComposePath)
	}
}

func TestComposeGitLabSourceDefaultsComposePath(t *testing.T) {
	source := property.New(map[string]property.Value{
		"type": property.New("gitlab"),
		"gitlab": property.New(map[string]property.Value{
			"integrationId": property.New("i1"), "projectId": property.New(float64(42)),
			"owner": property.New("owner"), "namespace": property.New("namespace"),
			"repository": property.New("repo"), "branch": property.New("main"),
		}),
	})
	got, err := (Compose{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{"name": property.New("demo"), "environmentId": property.New("e1"), "source": source})})
	if err != nil || len(got.Failures) != 0 {
		t.Fatalf("Check() = %#v, %v", got.Failures, err)
	}
	if got.Inputs.Source.GitLab.ComposePath != "./docker-compose.yml" {
		t.Fatalf("compose path = %q", got.Inputs.Source.GitLab.ComposePath)
	}
}

func TestComposeInferredSchemaHasVariantSpecificComposePaths(t *testing.T) {
	spec, err := p.GetSchema(t.Context(), Name, Version, Provider())
	if err != nil {
		t.Fatal(err)
	}
	compose := spec.Resources["dokploy:index:Compose"]
	if compose.InputProperties == nil {
		t.Fatal("Compose input properties are nil")
	}
	sourceProperty, ok := compose.InputProperties["source"]
	if !ok || sourceProperty.Ref == "" {
		t.Fatal("Compose source property is missing its type reference")
	}
	sourceRef := strings.TrimPrefix(sourceProperty.Ref, "#/types/")
	source, ok := spec.Types[sourceRef]
	if !ok || source.Properties == nil {
		t.Fatalf("source type %q is missing properties", sourceRef)
	}
	for _, variant := range []string{"raw", "git", "gitlab"} {
		if _, ok := source.Properties[variant]; !ok {
			t.Fatalf("source schema lacks %s variant", variant)
		}
		if source.Properties[variant].Ref == "" {
			t.Fatalf("source schema variant %s lacks type reference", variant)
		}
	}
	raw, ok := spec.Types[strings.TrimPrefix(source.Properties["raw"].Ref, "#/types/")]
	if !ok || raw.Properties == nil {
		t.Fatal("raw source type is missing properties")
	}
	composeFile, ok := raw.Properties["composeFile"]
	if !ok || composeFile.Type != "string" || !contains(raw.Required, "composeFile") {
		t.Fatal("raw schema lacks composeFile")
	}
	if _, ok := raw.Properties["composePath"]; ok {
		t.Fatal("raw schema unexpectedly exposes composePath")
	}
	for _, variant := range []string{"git", "gitlab"} {
		ref := strings.TrimPrefix(source.Properties[variant].Ref, "#/types/")
		typ, ok := spec.Types[ref]
		if !ok || typ.Properties == nil {
			t.Fatalf("%s source type is missing properties", variant)
		}
		path, ok := typ.Properties["composePath"]
		if !ok || path.Type != "string" || contains(typ.Required, "composePath") {
			t.Fatalf("repository schema %q lacks composePath", ref)
		}
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
