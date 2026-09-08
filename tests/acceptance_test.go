package tests

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func TestAccLifecycleSmoke(t *testing.T) {
	if os.Getenv("DOKPLOY_ACCEPTANCE") != "1" {
		t.Skip("set DOKPLOY_ACCEPTANCE=1 to run live acceptance tests")
	}
	endpoint := os.Getenv("DOKPLOY_ENDPOINT")
	apiKey := os.Getenv("DOKPLOY_API_KEY")
	if endpoint == "" || apiKey == "" {
		t.Skip("live Dokploy credentials are not configured")
	}
	if err := requirePulumiCLI(true, exec.LookPath); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	runLifecycleSmoke(t, ctx, liveConfig{Endpoint: endpoint, APIKey: apiKey, NameSuffix: uuid.NewString()})
}

func TestPulumiCLIAvailabilityHelper(t *testing.T) {
	t.Run("present", func(t *testing.T) {
		if !pulumiCLIAvailable(func(string) (string, error) { return "/usr/bin/pulumi", nil }) {
			t.Fatal("present Pulumi CLI was reported unavailable")
		}
	})
	t.Run("missing", func(t *testing.T) {
		if pulumiCLIAvailable(func(string) (string, error) { return "", exec.ErrNotFound }) {
			t.Fatal("missing Pulumi CLI was reported available")
		}
	})
}

func TestRequirePulumiCLI(t *testing.T) {
	lookupMissing := func(string) (string, error) { return "", exec.ErrNotFound }
	lookupPresent := func(string) (string, error) { return "/usr/bin/pulumi", nil }
	tests := []struct {
		name           string
		acceptance     bool
		lookPath       func(string) (string, error)
		wantErrMessage string
	}{
		{name: "disabled acceptance does not require CLI", lookPath: lookupMissing},
		{name: "enabled acceptance requires CLI", acceptance: true, lookPath: lookupMissing, wantErrMessage: "Pulumi CLI prerequisite is unavailable"},
		{name: "enabled acceptance accepts available CLI", acceptance: true, lookPath: lookupPresent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := requirePulumiCLI(tt.acceptance, tt.lookPath)
			if tt.wantErrMessage == "" {
				if err != nil {
					t.Fatalf("requirePulumiCLI() error = %v, want nil", err)
				}
				return
			}
			if err == nil || err.Error() != tt.wantErrMessage {
				t.Fatalf("requirePulumiCLI() error = %v, want %q", err, tt.wantErrMessage)
			}
		})
	}
}

func requirePulumiCLI(acceptanceEnabled bool, lookPath func(string) (string, error)) error {
	if !acceptanceEnabled || pulumiCLIAvailable(lookPath) {
		return nil
	}
	return errors.New("Pulumi CLI prerequisite is unavailable")
}

func TestLifecycleSmokeProgram(t *testing.T) {
	variants := []struct {
		name    string
		program func(liveConfig) pulumi.RunFunc
		values  lifecycleRevisionValues
	}{
		{name: "revision one", program: lifecycleRevisionOne, values: lifecycleRevisionOneValues()},
		{name: "revision two", program: lifecycleRevisionTwo, values: lifecycleRevisionTwoValues()},
	}
	for _, variant := range variants {
		t.Run(variant.name, func(t *testing.T) {
			mocks := &captureLifecycleResources{}
			err := pulumi.RunErr(variant.program(liveConfig{NameSuffix: "mock"}), pulumi.WithMocks("lifecycle", variant.name, mocks))
			if err != nil {
				t.Fatal(err)
			}
			project := mocks.resource("dokploy:index:Project", acceptanceEntityName("project", "mock"))
			assertString(t, project, "description", variant.values.description)
			environment := mocks.resource("dokploy:index:Environment", acceptanceEntityName("environment", "mock"))
			assertString(t, environment, "name", acceptanceEntityName(variant.values.environment, "mock"))
			assertString(t, environment, "projectId", "project-mock-id")
			tag := mocks.resource("dokploy:index:Tag", acceptanceEntityName("tag", "mock"))
			assertString(t, tag, "color", variant.values.color)
			association := mocks.resource("dokploy:index:ProjectTag", acceptanceEntityName("project-tag", "mock"))
			assertString(t, association, "projectId", "project-mock-id")
			assertString(t, association, "tagId", "tag-mock-id")
		})
	}
}

func TestLifecycleSmokeProgramKeepsResourceIdentityAcrossRevisions(t *testing.T) {
	mocks := &captureLifecycleResources{}
	cfg := liveConfig{NameSuffix: "mock"}
	for _, program := range []pulumi.RunFunc{lifecycleRevisionOne(cfg), lifecycleRevisionTwo(cfg)} {
		if err := pulumi.RunErr(program, pulumi.WithMocks("lifecycle", "identity", mocks)); err != nil {
			t.Fatal(err)
		}
	}
	for _, entity := range []struct{ token, kind string }{
		{"dokploy:index:Project", "project"},
		{"dokploy:index:Environment", "environment"},
		{"dokploy:index:Tag", "tag"},
		{"dokploy:index:ProjectTag", "project-tag"},
	} {
		name := acceptanceEntityName(entity.kind, cfg.NameSuffix)
		history := mocks.idHistory(entity.token, name)
		if len(history) != 2 || history[0] != history[1] {
			t.Errorf("%s identity history = %#v, want two stable values", entity.kind, history)
		}
	}
	ids := map[string]bool{}
	for _, entity := range []struct{ token, kind string }{
		{"dokploy:index:Project", "project"},
		{"dokploy:index:Environment", "environment"},
		{"dokploy:index:Tag", "tag"},
		{"dokploy:index:ProjectTag", "project-tag"},
	} {
		ids[mocks.idHistory(entity.token, acceptanceEntityName(entity.kind, cfg.NameSuffix))[0]] = true
	}
	if len(ids) != 4 {
		t.Fatalf("distinct logical resources did not receive distinct IDs: %#v", ids)
	}
}

func TestAcceptanceEntityNameIncludesPrefixAndRunSuffix(t *testing.T) {
	for _, kind := range []string{"project", "environment", "tag", "project-tag"} {
		got := acceptanceEntityName(kind, "run-123")
		want := "pulumi-acceptance-" + kind + "-run-123"
		if got != want {
			t.Errorf("%s name = %q, want %q", kind, got, want)
		}
	}
}

func TestAcceptanceStackAndProjectNamesIncludePrefixAndRunSuffix(t *testing.T) {
	if got, want := acceptanceStackName("run-123"), "pulumi-acceptance-stack-run-123"; got != want {
		t.Errorf("stack name = %q, want %q", got, want)
	}
	if got, want := acceptanceProjectName("run-123"), "pulumi-acceptance-project-run-123"; got != want {
		t.Errorf("project name = %q, want %q", got, want)
	}
}

func TestSanitizeAcceptanceDiagnosticRedactsConfiguredValues(t *testing.T) {
	t.Setenv("DOKPLOY_REGISTRY_USERNAME", "registry-user-sentinel")
	t.Setenv("DOKPLOY_GITLAB_TOKEN", "gitlab-token-sentinel")
	cfg := liveConfig{Endpoint: "https://dokploy.example.invalid", APIKey: "api-key-sentinel", SecretSentinels: []string{"password-sentinel"}}
	diagnostic := sanitizeAcceptanceDiagnostic("up revision two failed at "+cfg.Endpoint+" with "+cfg.APIKey+" and password-sentinel registry-user-sentinel gitlab-token-sentinel", cfg)
	if diagnostic != "up revision two failed at [REDACTED] with [REDACTED] and [REDACTED] [REDACTED] [REDACTED]" {
		t.Fatalf("sanitized diagnostic = %q", diagnostic)
	}
}

func TestLifecycleSmokeCleanupUsesIndependentContexts(t *testing.T) {
	var destroyDeadline, removeDeadline time.Time
	destroyErr, removeErr := cleanupLifecycleStack(5*time.Millisecond,
		func(ctx context.Context) error {
			destroyDeadline, _ = ctx.Deadline()
			<-ctx.Done()
			return ctx.Err()
		},
		func(ctx context.Context) error {
			removeDeadline, _ = ctx.Deadline()
			return nil
		})
	if destroyErr == nil || removeErr != nil {
		t.Fatalf("cleanup errors = %v, %v; want destroy timeout and successful removal", destroyErr, removeErr)
	}
	if destroyDeadline.IsZero() || removeDeadline.IsZero() || !removeDeadline.After(destroyDeadline) {
		t.Fatalf("cleanup deadlines were not independent: destroy=%v remove=%v", destroyDeadline, removeDeadline)
	}
}

type captureLifecycleResources struct {
	mu        sync.Mutex
	resources map[string]resource.PropertyMap
	ids       map[string]string
	histories map[string][]string
}

func (m *captureLifecycleResources) Call(pulumi.MockCallArgs) (resource.PropertyMap, error) {
	return resource.PropertyMap{}, nil
}

func (m *captureLifecycleResources) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.resources == nil {
		m.resources = map[string]resource.PropertyMap{}
	}
	if m.ids == nil {
		m.ids = map[string]string{}
	}
	if m.histories == nil {
		m.histories = map[string][]string{}
	}
	inputs := args.Inputs.Copy()
	switch args.TypeToken {
	case "dokploy:index:Project":
		inputs["projectId"] = resource.NewStringProperty("project-mock-id")
	case "dokploy:index:Environment":
		inputs["environmentId"] = resource.NewStringProperty("environment-mock-id")
		inputs["projectId"] = resource.NewStringProperty("project-mock-id")
	case "dokploy:index:Tag":
		inputs["tagId"] = resource.NewStringProperty("tag-mock-id")
	}
	m.resources[args.TypeToken+"/"+args.Name] = inputs
	key := args.TypeToken + "/" + args.Name
	if _, ok := m.ids[key]; !ok {
		m.ids[key] = "mock-resource-id-" + strconv.Itoa(len(m.ids)+1)
	}
	m.histories[key] = append(m.histories[key], m.ids[key])
	return m.ids[key], inputs, nil
}

func (m *captureLifecycleResources) resource(token, name string) resource.PropertyMap {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.resources[token+"/"+name]
}

func (m *captureLifecycleResources) idHistory(token, name string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.histories[token+"/"+name]...)
}

func assertString(t *testing.T, values resource.PropertyMap, key, want string) {
	t.Helper()
	value, ok := values[resource.PropertyKey(key)]
	if !ok {
		t.Fatalf("missing %s", key)
	}
	if got := value.StringValue(); got != want {
		t.Errorf("%s = %q, want %q", key, got, want)
	}
}
