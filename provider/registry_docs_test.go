package dokploy

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func readProjectFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(content)
}

func projectSection(t *testing.T, document, heading string) string {
	t.Helper()
	start := strings.Index(document, heading)
	require.NotEqual(t, -1, start, heading)
	rest := document[start+len(heading):]
	end := strings.Index(rest, "\n## ")
	require.NotEqual(t, -1, end, heading)
	return document[start : start+len(heading)+end]
}

var registryLanguages = []string{"typescript", "python", "go", "csharp", "java", "yaml"}

func TestRegistryOverviewStructure(t *testing.T) {
	index := readProjectFile(t, "../docs/_index.md")
	require.True(t, strings.HasPrefix(index, "---\n"))
	parts := strings.SplitN(index, "---\n", 3)
	require.Len(t, parts, 3)
	frontMatter := map[string]string{}
	require.NoError(t, yaml.Unmarshal([]byte(parts[1]), &frontMatter))
	require.Equal(t, "package", frontMatter["layout"])
	require.Equal(t, "Dokploy", frontMatter["title"])
	require.Contains(t, frontMatter["meta_desc"], "Dokploy")
	require.Contains(t, frontMatter["meta_desc"], "Pulumi")
	require.NotRegexp(t, regexp.MustCompile(`(?m)^# `), parts[2])

	headings := []string{"## Installation", "## Example Usage", "## Configuration"}
	previous := -1
	for _, heading := range headings {
		position := strings.Index(index, heading)
		require.Greater(t, position, previous, heading)
		previous = position
	}
}

func TestRegistryMaintainerContactsAndOwnership(t *testing.T) {
	security := readProjectFile(t, "../SECURITY.md")
	conduct := readProjectFile(t, "../CODE-OF-CONDUCT.md")
	codeowners := readProjectFile(t, "../.github/CODEOWNERS")

	for name, content := range map[string]string{"SECURITY.md": security, "CODE-OF-CONDUCT.md": conduct} {
		require.Contains(t, content, "contact@dimeski.net", name)
		require.NotContains(t, content, "code-of-conduct@pulumi.com", name)
	}
	require.Regexp(t, regexp.MustCompile(`(?is)do not report security vulnerabilities in public (?:issues|pull requests).*email.*privately`), security)
	require.Regexp(t, regexp.MustCompile(`(?is)never include.*(?:api keys|private keys|passwords|credentials).*public (?:issues|pull requests|examples|logs|test fixtures)`), security)
	require.Equal(t, "* @dimeskigj\n", codeowners)
}

func TestRegistryPublicationRunbook(t *testing.T) {
	runbook := readProjectFile(t, "../docs/registry-publication-runbook.md")
	for _, marker := range []string{
		"workflow run release-smoke.yml -f version=0.2.2",
		`"repoSlug": "dimeskigj/pulumi-dokploy"`,
		`"schemaFile": "provider/cmd/pulumi-resource-dokploy/schema.json"`,
		`"dimeskigj"`, "publisher-names.json", "maintainer-approved public display name",
		"/check", "/preview", "fact-sheet", "six language tabs", "logo",
		"Registry CODEOWNER", "public Registry page",
	} {
		require.Contains(t, runbook, marker)
	}
	require.GreaterOrEqual(t, strings.Count(runbook, "- [ ]"), 8)
	require.NotRegexp(t, regexp.MustCompile(`(?im)^\s*- \[x\]`), runbook)
	require.NotRegexp(t, regexp.MustCompile(`(?im)^\s*- \[X\]`), runbook)
	for _, action := range []string{
		"Dispatch the read-only release smoke workflow:",
		"Record the run URL and require",
		"Fork or check out `pulumi/registry`",
		"Add a `publisher-names.json` entry",
		"Run the current lint/check commands",
		"Open the upstream pull request",
		"Resolve every fact-sheet finding",
		"Ask a Pulumi maintainer",
		"Inspect the preview:",
		"Obtain approval from a Pulumi Registry CODEOWNER.",
		"Merge only through the upstream maintainer process.",
		"After deployment, open the public Registry page",
	} {
		line := regexp.MustCompile(`(?m)^- \[ \] ` + regexp.QuoteMeta(action) + `.*$`).FindString(runbook)
		require.NotEmpty(t, line, "external action must be its own unchecked item: %s", action)
	}
	require.NotContains(t, runbook, "Registry PR has been opened")
}

func TestRegistryReadinessLedgerSeparatesEvidenceStates(t *testing.T) {
	ledger := readProjectFile(t, "../docs/provider-registry-readiness.md")
	for _, heading := range []string{
		"## Repository-complete", "## Publicly available", "## External pending",
	} {
		require.Contains(t, ledger, heading)
	}
	for _, marker := range []string{
		"v0.2.2", "docs/_index.md", "logoUrl", "contact@dimeski.net",
		".github/CODEOWNERS", "release-smoke", "community-packages/package-list.json",
		"publisher-names.json", "fact-sheet", "preview", "Registry CODEOWNER",
	} {
		require.Contains(t, ledger, marker)
	}
	require.Contains(t, ledger, "not yet Registry-ready")
	require.NotContains(t, ledger, "corrected release must be published")
	pending := projectSection(t, ledger, "## External pending")
	for _, marker := range []string{
		"Successful `release-smoke` dispatch", "community-packages/package-list.json",
		"publisher-names.json", "fact-sheet", "/check", "/preview", "Registry CODEOWNER",
	} {
		require.Contains(t, pending, marker)
	}
	require.Equal(t, 5, strings.Count(pending, "- [ ]"))
	require.NotContains(t, pending, "- [x]")
}

func TestBuildDotnetCreatesVersionFileForCleanCheckout(t *testing.T) {
	makefile := readProjectFile(t, "../Makefile")
	start := strings.Index(makefile, "build_dotnet:")
	require.NotEqual(t, -1, start, "Makefile must define build_dotnet")
	buildDotnet := makefile[start:]
	end := strings.Index(buildDotnet, "\nbuild_java:")
	require.NotEqual(t, -1, end, "build_dotnet must precede build_java")
	buildDotnet = buildDotnet[:end]
	buildDotnet = strings.Replace(buildDotnet,
		"cd sdk/dotnet && dotnet build --nologo -p:Version=$(VERSION_GENERIC)",
		"test \"$$(cat sdk/dotnet/version.txt)\" = \"$(VERSION_GENERIC)\"", 1)
	directory := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(directory, "sdk", "dotnet"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "Makefile"), []byte("VERSION_GENERIC ?= clean-checkout\n"+buildDotnet), 0o600))
	command := exec.Command("make", "-f", "Makefile", "build_dotnet")
	command.Dir = directory
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	version, err := os.ReadFile(filepath.Join(directory, "sdk", "dotnet", "version.txt"))
	require.NoError(t, err)
	require.Equal(t, "clean-checkout", string(version))
}

func TestRegistryOverviewLanguageChoosers(t *testing.T) {
	index := readProjectFile(t, "../docs/_index.md")
	chooser := `{{< chooser language "typescript,python,go,csharp,java,yaml" >}}`
	require.Equal(t, 2, strings.Count(index, chooser))
	require.NotContains(t, index, "hcl")
	require.Equal(t, 2, strings.Count(index, "{{< /chooser >}}"))
	for _, language := range registryLanguages {
		open := "{{% choosable language " + language + " %}}"
		require.Equal(t, 2, strings.Count(index, open), language)
	}
	require.Equal(t, [][]string{
		{"typescript", "python", "go", "csharp", "java", "yaml"},
		{"typescript", "python", "go", "csharp", "java", "yaml"},
	}, chooserLanguages(index))
	require.Equal(t, 12, strings.Count(index, "{{% /choosable %}}"))
	for _, section := range []string{"## Installation", "## Example Usage"} {
		body := projectSection(t, index, section)
		for _, language := range registryLanguages {
			open := "{{% choosable language " + language + " %}}"
			start := strings.Index(body, open)
			require.NotEqual(t, -1, start)
			end := strings.Index(body[start+len(open):], "{{% /choosable %}}")
			require.NotEqual(t, -1, end)
			tab := body[start : start+len(open)+end]
			newline := strings.Index(tab, "\n")
			require.NotEqual(t, -1, newline, language+" tab must have a body")
			require.NotEmpty(t, strings.TrimSpace(tab[newline:]), language+" tab must contain content")
		}
	}
	installationMarkers := map[string]string{
		"typescript": "npm install @dimeskigj/pulumi-dokploy",
		"python":     "pip install pulumi-dokploy",
		"go":         "go get github.com/dimeskigj/pulumi-dokploy/sdk/go/dokploy",
		"csharp":     "dotnet add package Dimeskigj.Pulumi.Dokploy",
		"java":       "<groupId>net.dimeski.pulumi</groupId>",
		"yaml":       "pulumi package add github.com/dimeskigj/pulumi-dokploy dokploy",
	}
	exampleMarkers := map[string]string{
		"typescript": "new dokploy.Project",
		"python":     "pulumi_dokploy.Project",
		"go":         "dokploy.NewProject",
		"csharp":     "new Project(\"example\"",
		"java":       "new Project(\"example\"",
		"yaml":       "type: dokploy:index:Project",
	}
	for language, marker := range installationMarkers {
		require.Contains(t, chooserTab(t, projectSection(t, index, "## Installation"), language), marker)
		require.Contains(t, chooserTab(t, projectSection(t, index, "## Example Usage"), language), exampleMarkers[language])
	}
}

func chooserTab(t *testing.T, section, language string) string {
	t.Helper()
	open := "{{% choosable language " + language + " %}}"
	start := strings.Index(section, open)
	require.NotEqual(t, -1, start, language)
	contentStart := start + len(open)
	end := strings.Index(section[contentStart:], "{{% /choosable %}}")
	require.NotEqual(t, -1, end, language)
	return section[contentStart : contentStart+end]
}

func chooserLanguages(document string) [][]string {
	const prefix = `{{< chooser language "`
	var languages [][]string
	for remaining := document; ; {
		start := strings.Index(remaining, prefix)
		if start < 0 {
			return languages
		}
		remaining = remaining[start+len(prefix):]
		end := strings.Index(remaining, `" >}}`)
		if end < 0 {
			return languages
		}
		languages = append(languages, strings.Split(remaining[:end], ","))
		remaining = remaining[end+len(`" >}}`):]
	}
}

func TestRegistryOverviewCoordinatesExamplesAndConfiguration(t *testing.T) {
	index := readProjectFile(t, "../docs/_index.md")
	for _, marker := range []string{
		"npm install @dimeskigj/pulumi-dokploy",
		"pip install pulumi-dokploy",
		"go get github.com/dimeskigj/pulumi-dokploy/sdk/go/dokploy",
		"dotnet add package Dimeskigj.Pulumi.Dokploy",
		"<groupId>net.dimeski.pulumi</groupId>",
		"implementation 'net.dimeski.pulumi:dokploy:",
		"pulumi package add github.com/dimeskigj/pulumi-dokploy dokploy",
		"new dokploy.Project", "pulumi_dokploy.Project", "dokploy.NewProject",
		"new Project", "new Project(\"example\"", "type: dokploy:index:Project",
		"pulumi config set dokploy:endpoint https://dokploy.example.invalid",
		"pulumi config set --secret dokploy:apiKey your-api-key",
		"`endpoint` (Required, Not secret)", "`apiKey` (Required, Secret)",
		"DOKPLOY_ENDPOINT", "DOKPLOY_API_KEY", "community-maintained",
	} {
		require.Contains(t, index, marker)
	}
	require.NotContains(t, index, "official Dokploy")
	require.NotContains(t, index, "official Pulumi")
}

func TestRegistryDocumentationCoordinatesStayConsistent(t *testing.T) {
	index := readProjectFile(t, "../docs/_index.md")
	installation := readProjectFile(t, "../docs/installation-configuration.md")
	for _, marker := range []string{
		"@dimeskigj/pulumi-dokploy", "pulumi-dokploy",
		"Dimeskigj.Pulumi.Dokploy", "net.dimeski.pulumi:dokploy",
		"github.com/dimeskigj/pulumi-dokploy/sdk/go/dokploy",
		"dokploy:endpoint", "dokploy:apiKey", "DOKPLOY_ENDPOINT", "DOKPLOY_API_KEY",
	} {
		require.Contains(t, index, marker)
		require.Contains(t, installation, marker)
	}
}

func TestRegistryFrontMatterAndSupportLinks(t *testing.T) {
	index := readProjectFile(t, "../docs/_index.md")
	installation := readProjectFile(t, "../docs/installation-configuration.md")
	for name, content := range map[string]string{"index": index, "installation": installation} {
		t.Run(name, func(t *testing.T) {
			parts := strings.SplitN(content, "---\n", 3)
			require.Len(t, parts, 3)
			frontMatter := map[string]string{}
			require.NoError(t, yaml.Unmarshal([]byte(parts[1]), &frontMatter))
			require.Equal(t, "package", frontMatter["layout"])
			require.NotEmpty(t, frontMatter["meta_desc"])
			require.NotEmpty(t, frontMatter["title"])
		})
	}
	for _, link := range []string{
		"https://github.com/dimeskigj/pulumi-dokploy",
		"https://github.com/dimeskigj/pulumi-dokploy/issues",
		"https://github.com/dimeskigj/pulumi-dokploy/blob/main/CONTRIBUTING.md",
	} {
		require.Contains(t, index, link)
	}
}

func TestRegistryInstallationDetails(t *testing.T) {
	installation := readProjectFile(t, "../docs/installation-configuration.md")
	require.Regexp(t, regexp.MustCompile(`(?s)<groupId>net\.dimeski\.pulumi</groupId>\s*<artifactId>dokploy</artifactId>\s*<version>\$\{DOKPLOY_VERSION\}</version>`), installation)
	require.Contains(t, installation, "pluginDownloadURL")
	require.Contains(t, installation, "github://api.github.com/dimeskigj/pulumi-dokploy")
}

func TestRegistryExamplesDoNotExposeSecrets(t *testing.T) {
	index := readProjectFile(t, "../docs/_index.md")
	configuration := strings.Index(index, "Configure the Dokploy endpoint and API key")
	require.NotEqual(t, -1, configuration)
	examples := index[:configuration]
	require.NotContains(t, examples, "apiKey")
	require.NotContains(t, examples, "DOKPLOY_API_KEY")
	require.NotContains(t, examples, "your-api-key")
}

func TestRegistryLicense(t *testing.T) {
	license := readProjectFile(t, "../LICENSE")
	require.Contains(t, license, "TERMS AND CONDITIONS FOR USE, REPRODUCTION, AND DISTRIBUTION")
	for _, section := range []string{"1. Definitions.", "2. Grant of Copyright License.", "3. Grant of Patent License.", "4. Redistribution.", "5. Submission of Contributions.", "6. Trademarks.", "7. Disclaimer of Warranty.", "8. Limitation of Liability.", "9. Accepting Warranty or Additional Liability."} {
		require.Contains(t, license, section)
	}
	require.Contains(t, license, "APPENDIX: How to apply the Apache License to your work")
}

func TestRegistryLogoAssetAndAttribution(t *testing.T) {
	logo := readProjectFile(t, "../website/public/logo.svg")
	attribution := readProjectFile(t, "../website/public/logo-LICENSE.md")
	require.Contains(t, logo, "<svg")
	require.Contains(t, logo, "viewBox=")
	lowerLogo := strings.ToLower(logo)
	for _, forbidden := range []string{"<script", "javascript:", "http://", "https://", "data:image/", "<image", "<foreignobject"} {
		require.NotContains(t, lowerLogo, forbidden)
	}
	for _, marker := range []string{"## Source", "## License", "## Modifications", "SPDX-License-Identifier:"} {
		require.Contains(t, attribution, marker)
	}
	require.Regexp(t, regexp.MustCompile(`SPDX-License-Identifier: (MIT|Apache-2\.0|BSD-2-Clause|BSD-3-Clause|CC0-1\.0)`), attribution)
}

func TestCodegenRemovesGeneratedDotnetPackageIcon(t *testing.T) {
	makefile := readProjectFile(t, "../Makefile")
	project := readProjectFile(t, "../sdk/dotnet/Dimeskigj.Pulumi.Dokploy.csproj")
	codegen := makefile[strings.Index(makefile, "codegen: provider"):strings.Index(makefile, "build_go:")]
	require.Contains(t, codegen, "remove-dotnet-package-icon.py sdk/dotnet/Dimeskigj.Pulumi.Dokploy.csproj")
	require.Contains(t, codegen, "rm -f sdk/dotnet/logo.png")
	require.NotContains(t, codegen, "generate-logo-png.py")
	require.NotContains(t, project, "PackageIcon")
	require.NotContains(t, project, "logo.png")

	directory := t.TempDir()
	generated := filepath.Join(directory, "generated.csproj")
	generatedProject := `<Project>
  <PropertyGroup>
    <PackageIcon>logo.png</PackageIcon>
  </PropertyGroup>
  <ItemGroup>
    <None Include="logo.png">
      <Pack>True</Pack>
      <PackagePath></PackagePath>
    </None>
  </ItemGroup>
</Project>
`
	require.NoError(t, os.WriteFile(generated, []byte(generatedProject), 0o600))
	command := exec.Command("python3", "scripts/remove-dotnet-package-icon.py", generated)
	command.Dir = ".."
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	cleanProject, err := os.ReadFile(generated)
	require.NoError(t, err)
	require.NotContains(t, string(cleanProject), "logo.png")

	command = exec.Command("python3", "scripts/remove-dotnet-package-icon.py", generated)
	command.Dir = ".."
	output, err = command.CombinedOutput()
	require.NoError(t, err, string(output), "icon removal must be idempotent")
	require.NoError(t, os.WriteFile(generated, []byte(`<Project><PackageIcon>unexpected.png</PackageIcon></Project>`), 0o600))
	command = exec.Command("python3", "scripts/remove-dotnet-package-icon.py", generated)
	command.Dir = ".."
	_, err = command.CombinedOutput()
	require.Error(t, err, "unexpected generated icon references must fail closed")
}

func TestRepositorySetupDeclaresPortableJavaBuildTools(t *testing.T) {
	mise := readProjectFile(t, "../.mise.toml")
	require.Regexp(t, regexp.MustCompile(`(?m)^java\s*=\s*\["temurin-11\.0\.32\+101",\s*"temurin-17\.0\.20\+8"\]\s*$`), mise)
	require.Regexp(t, regexp.MustCompile(`(?m)^gradle\s*=\s*"8\.14\.3"\s*$`), mise)
	require.Regexp(t, regexp.MustCompile(`(?m)^maven\s*=\s*"3\.9\.11"\s*$`), mise)
	action := readProjectFile(t, "../.github/actions/setup-tools/action.yml")
	require.Contains(t, action, "uses: jdx/mise-action@")
	require.Contains(t, action, "install: true")
	makefile := readProjectFile(t, "../Makefile")
	require.Contains(t, makefile, "build_sdks: build_go build_python build_nodejs build_dotnet build_java")
	for _, selection := range []string{
		"mise exec java@temurin-11.0.32+101 gradle@8.14.3 --",
		"mise exec java@temurin-17.0.20+8 maven@3.9.11 --",
	} {
		require.Contains(t, makefile, selection)
	}
	require.Contains(t, makefile, "-Djdk.tls.client.protocols=TLSv1.2 -Dhttps.protocols=TLSv1.2")
}
