package tests

import (
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func TestLiveAcceptanceReadmeContract(t *testing.T) {
	readmeBytes, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	readme := string(readmeBytes)
	sectionOrder := []string{
		"## Purpose",
		"## Test layers",
		"## Safety rules",
		"## Prerequisites",
		"## Local setup",
		"## Test commands",
		"## Optional coverage",
		"## Cleanup and stop behavior",
		"## Result classification",
		"## Reports",
	}
	previous := -1
	for _, heading := range sectionOrder {
		position := strings.Index(readme, heading)
		if position <= previous {
			t.Errorf("README section %q is out of order or missing", heading)
		}
		previous = position
	}
	for _, required := range []string{
		"Direct-provider lifecycle tests",
		"Pulumi Automation API smoke test",
		"TestLiveTier1ControlPlane",
		"TestLiveTier2Workloads",
		"TestLiveTier3Databases",
		"TestLiveTier4Backups",
		"TestAccLifecycleSmoke",
		"DOKPLOY_ACCEPTANCE=1",
		"DOKPLOY_ENDPOINT",
		"DOKPLOY_API_KEY",
		"DOKPLOY_ACCEPTANCE_STOP_FILE",
		"-parallel=1",
		"DOKPLOY_REGISTRY_URL",
		"DOKPLOY_GITLAB_INTEGRATION_ID",
		"DOKPLOY_ACCEPTANCE_ALLOW_REPLICAS",
		"DOKPLOY_CUSTOM_CERT_RESOLVER",
		"DOKPLOY_REGISTRY_USERNAME",
		"DOKPLOY_REGISTRY_PASSWORD",
		"DOKPLOY_REGISTRY_IMAGE_PREFIX",
		"DOKPLOY_GITLAB_PROJECT_ID",
		"DOKPLOY_GITLAB_OWNER",
		"DOKPLOY_GITLAB_NAMESPACE",
		"DOKPLOY_GITLAB_REPOSITORY",
		"DOKPLOY_GITLAB_BRANCH",
		"DOKPLOY_GITLAB_USERNAME",
		"DOKPLOY_GITLAB_TOKEN",
		"DOKPLOY_ACCEPTANCE_SERVER_ID",
		"docs/bugs/README.md",
		"docs/bugs/2026-09-05-live-acceptance-run.md",
	} {
		if !strings.Contains(readme, required) {
			t.Errorf("live acceptance README is missing %q", required)
		}
	}
	if !strings.Contains(readme, "active custom certificate") {
		t.Error("README does not identify custom certificate resolver support as active")
	}

	commandsStart := strings.Index(readme, "## Test commands")
	optionalCoverageStart := strings.Index(readme, "## Optional coverage")
	if commandsStart < 0 || optionalCoverageStart <= commandsStart {
		t.Fatal("README test command section boundaries are missing or out of order")
	}
	commandsSection := readme[commandsStart:optionalCoverageStart]
	commandBlockStart := strings.Index(commandsSection, "```bash")
	commandBlockEnd := -1
	if commandBlockStart >= 0 {
		commandBlockEnd = strings.Index(commandsSection[commandBlockStart+len("```bash"):], "```")
	}
	if commandBlockStart < 0 || commandBlockEnd < 0 {
		t.Fatal("README test commands section has no bash command block")
	}
	commandBlock := commandsSection[commandBlockStart+len("```bash") : commandBlockStart+len("```bash")+commandBlockEnd]
	expectedCommands := []string{
		"mise exec -- go test ./provider -run TestLiveTier1ControlPlane -parallel=1 -count=1 -v",
		"mise exec -- go test ./provider -run TestLiveTier2Workloads -parallel=1 -count=1 -v",
		"mise exec -- go test ./provider -run TestLiveTier3Databases -parallel=1 -count=1 -v",
		"mise exec -- go test ./provider -run TestLiveTier4Backups -parallel=1 -count=1 -v",
		"mise exec -- go test ./tests -run TestAccLifecycleSmoke -parallel=1 -count=1 -v",
	}
	for _, command := range expectedCommands {
		if strings.Count(commandBlock, command) != 1 {
			t.Errorf("README must contain exactly one serial command %q", command)
		}
	}
	if got := strings.Count(commandBlock, "mise exec -- go test "); got != len(expectedCommands) {
		t.Errorf("README has %d test commands, want exactly %d", got, len(expectedCommands))
	}

	for _, required := range []string{
		"fallback cleanup",
		"explicit resource absence",
		"Cleanup contexts are independent",
		"The stop marker is created only when cleanup fails or a confirmed server-health failure occurs.",
		"Do not run a later heavy tier",
		"when the marker exists",
		"**Pass:**",
		"**Skip:**",
		"**Provider defect:**",
		"**Dokploy server defect:**",
		"**Test environment limitation:**",
		"ordinary resource failure does not stop an independent tier",
		"Never run these tests against a production server",
		"Do not print credentials",
		"Redact credentials",
		"Process prerequisites",
		"Workflow/operator safety setup",
		"The stop marker is created only when cleanup fails or a confirmed server-health failure occurs.",
	} {
		if !strings.Contains(readme, required) {
			t.Errorf("README is missing safety or result contract %q", required)
		}
	}
	if strings.Contains(readme, "when a heavy operation cannot safely complete") {
		t.Error("README incorrectly treats a heavy-operation failure as a stop-marker condition")
	}
	for _, prohibited := range []string{"Go test code loads .env", "tests read .env", "parse .env"} {
		if strings.Contains(readme, prohibited) {
			t.Errorf("live acceptance README contains prohibited instruction %q", prohibited)
		}
	}
	if !strings.Contains(readme, "Go test code must not read\n`.env` or parse `.env`") {
		t.Error("README does not state the safe shell and test-code .env boundary")
	}
	contributingBytes, err := os.ReadFile("../CONTRIBUTING.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contributingBytes), "tests/README.md") {
		t.Error("CONTRIBUTING.md does not link to the live acceptance guide")
	}
}

func TestLiveDiagnosticSourceContract(t *testing.T) {
	files, err := filepath.Glob("../provider/live*_test.go")
	if err != nil {
		t.Fatal(err)
	}
	files = append(files, "acceptance_program_test.go")
	for _, path := range files {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		violations := liveDiagnosticViolations(path, content)
		for _, violation := range violations {
			t.Error(violation)
		}
	}
}

func liveDiagnosticViolations(path string, source []byte) []string {
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, path, source, 0)
	if err != nil {
		return []string{path + ": parse failed"}
	}
	violations := []string{}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil || !isLiveExecutionFunction(path, function.Name.Name) {
			continue
		}
		ast.Inspect(function.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			if selector, ok := call.Fun.(*ast.SelectorExpr); ok {
				if receiver, ok := selector.X.(*ast.Ident); ok && receiver.Name == "require" && (selector.Sel.Name == "NotEmpty" || selector.Sel.Name == "Contains" || (selector.Sel.Name == "Equal" && sensitiveAssertion(call))) {
					violations = append(violations, path+": ordinary require."+selector.Sel.Name+" in "+function.Name.Name)
				}
				if selector.Sel.Name == "Error" {
					if receiver, ok := selector.X.(*ast.Ident); ok && receiver.Name == "err" {
						violations = append(violations, path+": err.Error() in "+function.Name.Name)
					}
				}
			}
			if selector, ok := call.Fun.(*ast.SelectorExpr); ok && selector.Sel.Name == "Sprintf" && len(call.Args) > 0 {
				if format, ok := call.Args[0].(*ast.BasicLit); ok && strings.Contains(format.Value, "%#v") {
					violations = append(violations, path+": %#v formatting in "+function.Name.Name)
				}
			}
			return true
		})
	}
	return violations
}

func sensitiveAssertion(call *ast.CallExpr) bool {
	if len(call.Args) < 2 {
		return true
	}
	sensitive := false
	for _, argument := range call.Args[1:] {
		ast.Inspect(argument, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if !ok {
				return true
			}
			name := strings.ToLower(identifier.Name)
			for _, marker := range []string{"id", "name", "endpoint", "bucket", "region", "prefix", "schedule", "content", "password", "environment", "source", "state", "output"} {
				if strings.Contains(name, marker) {
					sensitive = true
				}
			}
			return true
		})
	}
	return sensitive
}

func isLiveExecutionFunction(path, name string) bool {
	if strings.HasSuffix(path, "live_harness_unit_test.go") {
		return strings.HasPrefix(name, "live") || strings.HasPrefix(name, "runLive")
	}
	if strings.HasSuffix(path, "acceptance_program_test.go") {
		return name == "runLifecycleSmoke" || strings.HasPrefix(name, "assertLifecycle")
	}
	if strings.HasPrefix(name, "Test") {
		return strings.HasPrefix(name, "TestLiveTier")
	}
	return strings.HasPrefix(name, "live") || strings.HasPrefix(name, "runLive") || strings.HasPrefix(name, "deleteAndRead") || strings.HasPrefix(name, "finishDatabase") || strings.HasPrefix(name, "cleanupDirect")
}

func TestAcceptanceFailureIsFieldOnly(t *testing.T) {
	if got := acceptanceFailure("refresh outputs", "projectId"); got != "Pulumi acceptance refresh outputs failed for field projectId" {
		t.Fatalf("acceptance failure = %q", got)
	}
}

func TestAcceptanceStageErrorIsSanitized(t *testing.T) {
	err := acceptanceStageError("stack-create", errors.New("secret-sentinel https://private.example resource-id-sentinel"))
	if got, want := err.Error(), "Pulumi acceptance failed at stage stack-create: category=process"; got != want {
		t.Fatalf("acceptance stage error = %q, want %q", got, want)
	}
}

func TestAcceptanceStageErrorRejectsUnknownStage(t *testing.T) {
	err := acceptanceStageError("secret-sentinel", errors.New("boom"))
	if got, want := err.Error(), "Pulumi acceptance failed at stage unknown: category=process"; got != want {
		t.Fatalf("acceptance stage error = %q, want %q", got, want)
	}
}

func TestAcceptanceStageErrorClassifiesContextAndNilErrors(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		category string
	}{
		{name: "timeout", err: context.DeadlineExceeded, category: "timeout"},
		{name: "canceled", err: context.Canceled, category: "canceled"},
		{name: "nil", category: "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := acceptanceStageError("preview-1", tt.err)
			want := "Pulumi acceptance failed at stage preview-1: category=" + tt.category
			if got := err.Error(); got != want {
				t.Fatalf("acceptance stage error = %q, want %q", got, want)
			}
		})
	}
}

func TestAcceptanceStageErrorAllowsEverySmokeStage(t *testing.T) {
	for stage := range acceptanceStages {
		err := acceptanceStageError(stage, errors.New("sensitive process detail"))
		want := "Pulumi acceptance failed at stage " + stage + ": category=process"
		if got := err.Error(); got != want {
			t.Errorf("stage %q produced %q, want %q", stage, got, want)
		}
	}
}

func TestAcceptanceRefreshOutputErrorIsStageSanitized(t *testing.T) {
	err := acceptanceRefreshOutputError("refresh-1", errors.New("secret-sentinel https://private.example"))
	if got, want := err.Error(), "Pulumi acceptance failed at stage refresh-1: category=process"; got != want {
		t.Fatalf("refresh output error = %q, want %q", got, want)
	}
}

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
			if got := mocks.customResourceCount(); got != 4 {
				t.Fatalf("captured custom resource count = %d, want four", got)
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
	var destroyDeadline, exportDeadline, removeDeadline, listDeadline time.Time
	destroyErr, exportErr, removeErr, listErr := cleanupLifecycleStack(5*time.Millisecond,
		func(ctx context.Context) (auto.DestroyResult, error) {
			destroyDeadline, _ = ctx.Deadline()
			<-ctx.Done()
			return auto.DestroyResult{}, errors.New("destroy failed")
		},
		func(ctx context.Context, _ auto.DestroyResult) error {
			exportDeadline, _ = ctx.Deadline()
			return errors.New("export failed")
		},
		func(ctx context.Context) error {
			removeDeadline, _ = ctx.Deadline()
			return nil
		},
		func(ctx context.Context) error {
			listDeadline, _ = ctx.Deadline()
			return nil
		})
	if destroyErr == nil || exportErr == nil || removeErr != nil || listErr != nil {
		t.Fatalf("cleanup errors = %v, %v, %v, %v; want destroy/export errors and successful removal/list", destroyErr, exportErr, removeErr, listErr)
	}
	if destroyDeadline.IsZero() || exportDeadline.IsZero() || removeDeadline.IsZero() || listDeadline.IsZero() || !exportDeadline.After(destroyDeadline) || !removeDeadline.After(exportDeadline) || !listDeadline.After(removeDeadline) {
		t.Fatalf("cleanup contexts were not independent: destroy=%v export=%v remove=%v list=%v", destroyDeadline, exportDeadline, removeDeadline, listDeadline)
	}
}

func TestLifecycleSmokeCleanupRunsPostDestroyValidationAndAllCleanupSteps(t *testing.T) {
	var order []string
	destroyErrWant := errors.New("destroy failed")
	postDestroyErr := errors.New("export validation failed")
	removeErrWant := errors.New("remove failed")
	listErrWant := errors.New("list failed")
	var removeDeadline, listDeadline time.Time
	destroyErr, exportErr, removeErr, listErr := cleanupLifecycleStack(1*time.Second,
		func(context.Context) (auto.DestroyResult, error) {
			order = append(order, "destroy")
			return auto.DestroyResult{}, destroyErrWant
		},
		func(context.Context, auto.DestroyResult) error {
			order = append(order, "export validation")
			return postDestroyErr
		},
		func(ctx context.Context) error {
			order = append(order, "remove")
			removeDeadline, _ = ctx.Deadline()
			return removeErrWant
		},
		func(ctx context.Context) error {
			order = append(order, "list")
			listDeadline, _ = ctx.Deadline()
			return listErrWant
		})
	if !errors.Is(destroyErr, destroyErrWant) || !errors.Is(exportErr, postDestroyErr) || !errors.Is(removeErr, removeErrWant) || !errors.Is(listErr, listErrWant) {
		t.Fatalf("cleanup errors = %v, %v, %v, %v; want destroy, post-destroy, remove, and list errors", destroyErr, exportErr, removeErr, listErr)
	}
	if got, want := strings.Join(order, ","), "destroy,export validation,remove,list"; got != want {
		t.Fatalf("cleanup order = %q, want %q", got, want)
	}
	if removeDeadline.IsZero() || listDeadline.IsZero() || !listDeadline.After(removeDeadline) {
		t.Fatalf("cleanup contexts were not independent: remove=%v, list=%v", removeDeadline, listDeadline)
	}
}

func TestLifecycleAggregateAllowsReadOperations(t *testing.T) {
	if err := validateLifecycleAggregate("revision one", map[string]int{"create": 1, "read": 3, "same": 2}); err != nil {
		t.Fatalf("validateLifecycleAggregate() = %v, want nil", err)
	}
}

func TestLifecycleAggregateAllowsExpectedUpdatesAndReads(t *testing.T) {
	if err := validateLifecycleAggregate("revision two", map[string]int{"update": 1, "read": 3, "noop": 1}); err != nil {
		t.Fatalf("validateLifecycleAggregate() = %v, want nil", err)
	}
}

func TestLifecycleAggregateRejectsUnexpectedCreates(t *testing.T) {
	err := validateLifecycleAggregate("revision two", map[string]int{"update": 1, "create": 1})
	if err == nil || !strings.Contains(err.Error(), "create") {
		t.Fatalf("validateLifecycleAggregate() = %v, want unexpected create error", err)
	}
}

func TestLifecycleAggregateRejectsUnsupportedOperations(t *testing.T) {
	err := validateLifecycleAggregate("revision two", map[string]int{"update": 1, "refresh": 1})
	if err == nil || !strings.Contains(err.Error(), "refresh") {
		t.Fatalf("validateLifecycleAggregate() = %v, want unsupported operation error", err)
	}
}

func TestLifecycleAggregateRejectsReplacement(t *testing.T) {
	err := validateLifecycleAggregate("revision two", map[string]int{"update": 1, "replace": 1})
	if err == nil || !strings.Contains(err.Error(), "replacement") {
		t.Fatalf("validateLifecycleAggregate() = %v, want replacement error", err)
	}
}

func TestLifecycleAggregateDiagnosticsAreSorted(t *testing.T) {
	err := validateLifecycleAggregate("revision two", map[string]int{"replace": 1, "delete": 1})
	if err == nil || !strings.HasPrefix(err.Error(), "revision two contains deletion") {
		t.Fatalf("validateLifecycleAggregate() = %v, want sorted deletion diagnostic", err)
	}
}

func TestLifecycleAggregateRejectsDeletion(t *testing.T) {
	err := validateLifecycleAggregate("revision two", map[string]int{"update": 1, "delete": 1})
	if err == nil || !strings.Contains(err.Error(), "deletion") {
		t.Fatalf("validateLifecycleAggregate() = %v, want deletion error", err)
	}
}

func TestLifecycleAggregateAdapters(t *testing.T) {
	preview := auto.PreviewResult{ChangeSummary: map[apitype.OpType]int{apitype.OpCreate: 1, apitype.OpRead: 3, apitype.OpSame: 2}}
	previewChanges := normalizeLifecycleChanges(preview.ChangeSummary)
	if previewChanges["create"] != 1 || previewChanges["read"] != 3 || previewChanges["same"] != 2 {
		t.Fatalf("preview adapter = %#v, want create=1, read=3, and same=2", previewChanges)
	}
	if err := validatePreviewAggregate(preview, "revision one"); err != nil {
		t.Fatalf("validatePreviewAggregate() = %v, want nil", err)
	}
	changes := map[string]int{"update": 1, "read": 3, "noop": 1}
	up := auto.UpResult{Summary: auto.UpdateSummary{ResourceChanges: &changes}}
	if err := validateUpdateAggregate(up, "revision two"); err != nil {
		t.Fatalf("update adapter = %v, want nil", err)
	}
	if err := validateUpdateAggregate(auto.UpResult{}, "revision two"); err == nil {
		t.Fatal("validateUpdateAggregate() = nil for missing resource changes")
	}
}

func TestLifecycleDestroyStateFiltersManagedResources(t *testing.T) {
	state, err := json.Marshal(map[string]any{"resources": []map[string]string{{"type": "pulumi:pulumi:Stack"}, {"type": "dokploy:index:Project"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := validateDestroyedState(apitype.UntypedDeployment{Deployment: state}); err == nil {
		t.Fatal("validateDestroyedState() = nil, want managed-resource error")
	}
	clean, err := json.Marshal(map[string]any{"resources": []map[string]string{{"type": "pulumi:pulumi:Stack"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := validateDestroyedState(apitype.UntypedDeployment{Deployment: clean}); err != nil {
		t.Fatalf("validateDestroyedState() = %v, want nil", err)
	}
}

func TestLifecycleStackRemovalRequiresAbsentName(t *testing.T) {
	stacks := []auto.StackSummary{{Name: "other"}}
	if err := validateStackRemoved(stacks, "pulumi-acceptance-stack-run"); err != nil {
		t.Fatalf("validateStackRemoved() = %v, want nil", err)
	}
	if err := validateStackRemoved(append(stacks, auto.StackSummary{Name: "pulumi-acceptance-stack-run"}), "pulumi-acceptance-stack-run"); err == nil {
		t.Fatal("validateStackRemoved() = nil, want stack-present error")
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

func (m *captureLifecycleResources) customResourceCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for key := range m.resources {
		if strings.HasPrefix(key, "dokploy:index:") && !strings.Contains(key, "/pulumi-acceptance-read-") {
			count++
		}
	}
	return count
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
