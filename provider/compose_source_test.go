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

func TestComposeInferredSchemaHasVariantSpecificComposePaths(t *testing.T) {
	spec, err := p.GetSchema(t.Context(), Name, Version, Provider())
	if err != nil {
		t.Fatal(err)
	}
	compose := spec.Resources["dokploy:index:Compose"]
	sourceRef := strings.TrimPrefix(compose.InputProperties["source"].Ref, "#/types/")
	source := spec.Types[sourceRef]
	rawRef := strings.TrimPrefix(source.Properties["raw"].Ref, "#/types/")
	gitRef := strings.TrimPrefix(source.Properties["git"].Ref, "#/types/")
	gitlabRef := strings.TrimPrefix(source.Properties["gitlab"].Ref, "#/types/")
	if _, ok := spec.Types[rawRef].Properties["composeFile"]; !ok {
		t.Fatal("raw schema lacks composeFile")
	}
	if _, ok := spec.Types[rawRef].Properties["composePath"]; ok {
		t.Fatal("raw schema unexpectedly exposes composePath")
	}
	for _, ref := range []string{gitRef, gitlabRef} {
		if _, ok := spec.Types[ref].Properties["composePath"]; !ok {
			t.Fatalf("repository schema %q lacks composePath", ref)
		}
	}
}
