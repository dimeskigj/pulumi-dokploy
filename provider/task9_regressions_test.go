package dokploy

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/stretchr/testify/require"
)

func TestTask9UsesAcceptanceStopFileInWorkflowAndReport(t *testing.T) {
	root := filepath.Join("..", ".github", "workflows", "run-acceptance-tests.yml")
	workflow, err := os.ReadFile(root)
	require.NoError(t, err)
	report, err := os.ReadFile(filepath.Join("..", "docs", "bugs", "2026-09-05-live-acceptance-run.md"))
	require.NoError(t, err)
	for _, content := range [][]byte{workflow, report} {
		require.Contains(t, string(content), "DOKPLOY_ACCEPTANCE_STOP_FILE")
		require.NotContains(t, string(content), "STOP_MARKER")
	}
}

func TestTask9WorkloadDependencyRequiresCreatedIDAndDoneStatus(t *testing.T) {
	for _, test := range []struct {
		name   string
		id     string
		status string
		want   bool
	}{
		{name: "missing id", status: statusDone, want: false},
		{name: "missing status", id: "application-id", want: false},
		{name: "created and done", id: "application-id", status: statusDone, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, workloadDependencyReady(test.id, test.status))
		})
	}
}

func TestTask9ComposeDiffInputsChangeOnePropertyAtATime(t *testing.T) {
	base := ComposeArgs{
		Name:        "compose",
		Environment: stringPtr("BASE=1"),
		Source:      ComposeSource{Type: ComposeSourceRaw, Raw: &RawComposeSource{ComposeFile: "services: {}"}},
	}
	sourceInputs, environmentInputs := composeDiffInputs(base)
	require.NotEqual(t, base.Source.Type, sourceInputs.Source.Type)
	require.Equal(t, base.Environment, sourceInputs.Environment)
	require.Equal(t, base.Source.Type, environmentInputs.Source.Type)
	require.NotEqual(t, base.Environment, environmentInputs.Environment)
}

func TestTask9RegisteredCleanupDeletesAtMostOnce(t *testing.T) {
	state := &liveCleanupID{id: "backup-id"}
	var deleted []string
	remove := func(id string) error {
		deleted = append(deleted, id)
		return nil
	}
	require.NoError(t, runLiveCleanupOnce(state, remove))
	require.NoError(t, runLiveCleanupOnce(state, remove))
	require.Equal(t, []string{"backup-id"}, deleted)
	require.Empty(t, state.current())
}

func TestTask9CleanupFailureRetainsIDForDeferredRetry(t *testing.T) {
	state := &liveCleanupID{id: "backup-id"}
	attempts := 0
	wantErr := errors.New("temporary cleanup failure")

	err := runLiveCleanupOnce(state, func(string) error {
		attempts++
		return wantErr
	})

	require.ErrorIs(t, err, wantErr)
	require.Equal(t, "backup-id", state.current())
	require.Equal(t, 1, attempts)

	require.NoError(t, runLiveCleanupOnce(state, func(string) error {
		attempts++
		return nil
	}))
	require.Empty(t, state.current())
	require.Equal(t, 2, attempts)
}

func TestTask9OverlappingCleanupCallsRemoveOnceAndRetryAfterFailure(t *testing.T) {
	state := &liveCleanupID{id: "backup-id"}
	removeStarted := make(chan struct{})
	releaseRemove := make(chan struct{})
	removeCalls := make(chan struct{}, 2)
	firstResult := make(chan error, 1)

	go func() {
		firstResult <- runLiveCleanupOnce(state, func(string) error {
			removeCalls <- struct{}{}
			close(removeStarted)
			<-releaseRemove
			return errors.New("temporary cleanup failure")
		})
	}()
	<-removeStarted

	secondResult := make(chan error, 1)
	go func() {
		secondResult <- runLiveCleanupOnce(state, func(string) error {
			removeCalls <- struct{}{}
			return nil
		})
	}()
	require.NoError(t, <-secondResult)
	close(releaseRemove)
	require.Error(t, <-firstResult)
	require.Equal(t, "backup-id", state.current())

	require.NoError(t, runLiveCleanupOnce(state, func(string) error {
		removeCalls <- struct{}{}
		return nil
	}))
	require.Empty(t, state.current())
	require.Len(t, removeCalls, 2)
}

func TestTask9AlreadyAbsentCleanupClearsID(t *testing.T) {
	state := &liveCleanupID{id: "backup-id"}
	require.NoError(t, runLiveCleanupOnce(state, func(string) error {
		return &client.APIError{StatusCode: 404, Message: "absent"}
	}))
	require.Empty(t, state.current())
}

func TestTask9CleanupIDCanBeClearedAfterExplicitDelete(t *testing.T) {
	state := &liveCleanupID{id: "backup-id"}
	state.clear()
	var deleted []string
	require.NoError(t, runLiveCleanupOnce(state, func(id string) error {
		deleted = append(deleted, id)
		return nil
	}))
	require.Empty(t, deleted)
	require.True(t, strings.TrimSpace(state.current()) == "")
}

func TestTask9MountCasesAreIndependentAndSummarizedByType(t *testing.T) {
	require.Equal(t, []string{"bind", "volume", "file"}, mountCaseNames())
	resetLiveHarnessState()
	t.Cleanup(resetLiveHarnessState)
	for _, kind := range mountCaseNames() {
		recordLiveOutcome("mount/"+kind, "create-failed")
	}
	state := snapshotLiveHarnessState()
	require.Equal(t, []string{"mount/bind", "mount/volume", "mount/file"}, liveResultKinds(state.results))
}

func TestTask9FinalFeatureRowsMatchEvidence(t *testing.T) {
	report, err := os.ReadFile(filepath.Join("..", "docs", "bugs", "2026-09-05-live-acceptance-run.md"))
	require.NoError(t, err)
	text := string(report)
	require.Contains(t, text, "| 2 | Application | fail |")
	require.Contains(t, text, "| 2 | Compose | pass |")
	require.Contains(t, text, "| 1 | SSHKey | skip |")
	require.NotContains(t, text, "| 1 | SSHKey | fail |")
	for _, target := range []string{"application", "compose"} {
		for _, mount := range []string{"bind", "volume", "file"} {
			require.Contains(t, text, "| 2 | Mount/"+target+"/"+mount+" | fail |")
		}
	}
}

func TestTask9EnvironmentComparisonUsesStatusOnly(t *testing.T) {
	classification := classifyEnvironmentUpdateComparison(
		&client.APIError{StatusCode: 400, Message: "secret body"}, nil)
	require.Equal(t, "provider-failed-direct-succeeded", classification)
	require.NotContains(t, classification, "secret")
}

func TestTask9EnvironmentDirectRequestIncludesPriorProjectID(t *testing.T) {
	body := generated.EnvironmentUpdateJSONBody{EnvironmentId: "environment-id", Name: stringPtr("staging"), ProjectId: stringPtr("prior-project-id")}
	encoded, err := json.Marshal(body)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"projectId":"prior-project-id"`)
}

func TestTask9ProjectTagDelayedAssociationPollClassifiesWithoutValues(t *testing.T) {
	reads := 0
	appeared, classification := pollProjectTagAssociation(context.Background(), func(context.Context) (bool, error) {
		reads++
		return reads == 2, nil
	})
	require.True(t, appeared)
	require.Equal(t, "appeared-after-delay", classification)
}

func TestTask9OrganizationActiveShapeClassification(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		want string
	}{
		{name: "flat id", body: `{"id":"org"}`, want: "flat-id"},
		{name: "flat organization id", body: `{"organizationId":"org"}`, want: "flat-organization-id"},
		{name: "nested ids", body: `{"organization":{"id":"org","organizationId":"legacy"}}`, want: "nested-organization-object"},
		{name: "null", body: `{"id":null}`, want: "null-id"},
		{name: "missing", body: `{}`, want: "missing-id"},
		{name: "invalid json", body: `{`, want: "invalid-json"},
		{name: "empty string", body: `{"id":""}`, want: "empty-id"},
		{name: "wrong type", body: `{"id":42}`, want: "wrong-type-id"},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, classifyOrganizationActiveShape([]byte(test.body)))
		})
	}
}
