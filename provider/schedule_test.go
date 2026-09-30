package dokploy

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/google/uuid"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

// Deliberately never prints request bodies or dynamic IDs on failure.
func scheduleTestResource(t *testing.T, handler http.HandlerFunc) Schedule {
	t.Helper()
	s := httptest.NewServer(handler)
	t.Cleanup(s.Close)
	api, err := client.New(s.URL, "placeholder-key", client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}), client.WithHTTPClient(&http.Client{Transport: &http.Transport{DisableKeepAlives: true}}))
	require.NoError(t, err)
	return Schedule{client: fixedClient(api)}
}

func scheduleTestArgs() ScheduleArgs {
	return ScheduleArgs{Name: "disabled", CronExpression: "0 0 * * *", Command: "placeholder-command", ScheduleType: "dokploy-server", Script: ptr("placeholder-script"), Description: ptr("description"), Timezone: ptr("UTC"), OrganizationID: ptr("placeholder-org"), AppName: ptr("app"), ServiceName: ptr("service"), ShellType: ptr("sh")}
}

func TestScheduleCreateAndImportAllTypes(t *testing.T) {
	for _, typ := range []string{"application", "compose", "server", "dokploy-server"} {
		t.Run(typ, func(t *testing.T) {
			a := scheduleTestArgs()
			a.ScheduleType = typ
			switch typ {
			case "application":
				a.ApplicationID = ptr("placeholder-app")
			case "compose":
				a.ComposeID = ptr("placeholder-compose")
			case "server":
				a.ServerID = ptr("placeholder-server")
			}
			var id string
			var observed map[string]any
			calls := 0
			r := scheduleTestResource(t, func(w http.ResponseWriter, req *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				if calls == 1 {
					require.Equal(t, "/api/schedule.create", req.URL.Path)
					require.Equal(t, http.MethodPost, req.Method)
					require.NoError(t, json.NewDecoder(req.Body).Decode(&observed))
					id, _ = observed["scheduleId"].(string)
					_, err := uuid.Parse(id)
					require.True(t, err == nil)
					require.True(t, observed["enabled"] == false && observed["scheduleType"] == typ)
					for k, v := range map[string]any{"name": a.Name, "cronExpression": a.CronExpression, "command": a.Command, "script": *a.Script, "description": *a.Description, "timezone": *a.Timezone, "organizationId": *a.OrganizationID, "appName": *a.AppName, "serviceName": *a.ServiceName, "shellType": *a.ShellType} {
						require.True(t, observed[k] == v, "incorrect field: %s", k)
					}
					for k, v := range map[string]*string{"applicationId": a.ApplicationID, "composeId": a.ComposeID, "serverId": a.ServerID} {
						if v == nil {
							_, present := observed[k]
							require.False(t, present)
						} else {
							require.True(t, observed[k] == *v)
						}
					}
					_, present := observed["createdAt"]
					require.False(t, present)
					return // Bodyless acknowledgment must work with the actual typed client.
				}
				require.Equal(t, "/api/schedule.one", req.URL.Path)
				require.Equal(t, http.MethodGet, req.Method)
				require.True(t, req.URL.Query().Get("scheduleId") == id)
				observed["name"] = "refreshed"
				require.NoError(t, json.NewEncoder(w).Encode(observed))
			})
			got, err := r.Create(t.Context(), infer.CreateRequest[ScheduleArgs]{Inputs: a})
			require.NoError(t, err)
			require.True(t, got.ID == id && got.Output.ScheduleID == id)
			require.Equal(t, "refreshed", got.Output.Name)
			read, err := r.Read(t.Context(), infer.ReadRequest[ScheduleArgs, ScheduleState]{ID: id})
			require.NoError(t, err)
			require.True(t, read.Inputs.Command == a.Command && read.Inputs.ScheduleType == typ)
			require.Equal(t, 3, calls)
		})
	}
}

func TestScheduleCreateDryRunNoCalls(t *testing.T) {
	r := Schedule{client: func(context.Context) *client.Client { t.Fatal("preview called client"); return nil }}
	got, err := r.Create(t.Context(), infer.CreateRequest[ScheduleArgs]{Inputs: scheduleTestArgs(), DryRun: true})
	require.NoError(t, err)
	require.Empty(t, got.ID)
	require.False(t, got.Output.Enabled)
	up, err := r.Update(t.Context(), infer.UpdateRequest[ScheduleArgs, ScheduleState]{ID: "placeholder-id", Inputs: scheduleTestArgs(), DryRun: true})
	require.NoError(t, err)
	require.Equal(t, "placeholder-id", up.Output.ScheduleID)
}

func TestScheduleCreateBrokenAcknowledgmentBody(t *testing.T) {
	for _, confirmed := range []bool{false, true} {
		t.Run(map[bool]string{false: "partial-state", true: "confirmed"}[confirmed], func(t *testing.T) {
			calls := 0
			var id string
			var body map[string]any
			r := scheduleTestResource(t, func(w http.ResponseWriter, req *http.Request) {
				calls++
				switch calls {
				case 1:
					require.Equal(t, "/api/schedule.create", req.URL.Path)
					require.Equal(t, http.MethodPost, req.Method)
					require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
					id, _ = body["scheduleId"].(string)
					// Acknowledged headers, but a truncated body makes the generated
					// response parser's io.ReadAll fail with unexpected EOF.
					w.Header().Set("Content-Length", "100")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("{"))
				case 2:
					require.Equal(t, "/api/schedule.one", req.URL.Path)
					require.Equal(t, http.MethodGet, req.Method)
					require.True(t, req.URL.Query().Get("scheduleId") == id)
					if !confirmed {
						w.WriteHeader(http.StatusNotFound)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					require.NoError(t, json.NewEncoder(w).Encode(body))
				default:
					t.Error("unexpected request: creation must not retry or delete")
				}
			})
			a := scheduleTestArgs()
			got, err := r.Create(t.Context(), infer.CreateRequest[ScheduleArgs]{Inputs: a})
			if confirmed {
				require.NoError(t, err)
			} else {
				var partial infer.ResourceInitFailedError
				require.ErrorAs(t, err, &partial)
				for _, secret := range []string{id, a.Command, *a.Script} {
					require.False(t, strings.Contains(err.Error(), secret))
				}
			}
			require.True(t, id != "" && got.ID == id && got.Output.ScheduleID == id)
			require.True(t, got.Output.Command == a.Command && got.Output.Script != nil && *got.Output.Script == *a.Script)
			require.Equal(t, 2, calls)
		})
	}
}

func TestScheduleCreateReadbackFailuresKeepPartialID(t *testing.T) {
	for _, body := range []string{"404", "null", "", "{}", `{"scheduleId":"unrelated"}`, "transport"} {
		t.Run(body, func(t *testing.T) {
			calls := 0
			id := ""
			r := scheduleTestResource(t, func(w http.ResponseWriter, req *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				if calls == 1 {
					var b map[string]any
					require.NoError(t, json.NewDecoder(req.Body).Decode(&b))
					id, _ = b["scheduleId"].(string)
					return
				}
				require.Equal(t, 2, calls)
				require.Equal(t, "/api/schedule.one", req.URL.Path)
				if body == "transport" {
					c, _, err := w.(http.Hijacker).Hijack()
					require.NoError(t, err)
					require.NoError(t, c.Close())
					return
				}
				if body == "404" {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				_, _ = w.Write([]byte(body))
			})
			got, err := r.Create(t.Context(), infer.CreateRequest[ScheduleArgs]{Inputs: scheduleTestArgs()})
			var partial infer.ResourceInitFailedError
			require.ErrorAs(t, err, &partial)
			require.True(t, id != "" && got.ID == id && got.Output.ScheduleID == id)
			require.False(t, strings.Contains(err.Error(), id))
			require.Equal(t, 2, calls)
			require.True(t, got.Output.Command == scheduleTestArgs().Command)
		})
	}
}

func TestScheduleReadDriftNullAbsentAndNotFound(t *testing.T) {
	for _, tc := range []struct {
		body        string
		status      int
		bad, absent bool
	}{
		{`{"scheduleId":"placeholder-id","name":"drift","cronExpression":"new cron","command":"new command","scheduleType":"dokploy-server","enabled":true,"script":null,"timezone":null}`, 200, false, false},
		{"null", 200, true, false}, {"{}", 200, true, false}, {"invalid", 200, true, false}, {`{"scheduleId":"other"}`, 200, true, false}, {"", 404, false, true},
	} {
		r := scheduleTestResource(t, func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		})
		got, err := r.Read(t.Context(), infer.ReadRequest[ScheduleArgs, ScheduleState]{ID: "placeholder-id", State: ScheduleState{ScheduleArgs: scheduleTestArgs()}})
		if tc.bad {
			require.Error(t, err)
			continue
		}
		require.NoError(t, err)
		if tc.absent {
			require.Empty(t, got.ID)
			continue
		}
		require.Equal(t, "drift", got.Inputs.Name)
		require.Equal(t, "new cron", got.Inputs.CronExpression)
		require.True(t, got.Inputs.Enabled)
		require.Nil(t, got.Inputs.Script)
		require.Nil(t, got.Inputs.Timezone)
		require.Equal(t, "description", *got.Inputs.Description)
	}
}

func TestScheduleReadEnabledPresence(t *testing.T) {
	for _, priorEnabled := range []bool{true, false} {
		priorName := "empty-import"
		prior := ScheduleState{}
		if priorEnabled {
			priorName = "prior-enabled"
			prior.ScheduleArgs = scheduleTestArgs()
			prior.Enabled = true
		}
		for _, field := range []struct {
			name, json string
			invalid    bool
			want       bool
		}{
			{"null", `,"enabled":null`, true, false},
			{"omitted", "", false, priorEnabled},
			{"false", `,"enabled":false`, false, false},
			{"true", `,"enabled":true`, false, true},
		} {
			t.Run(priorName+"/"+field.name, func(t *testing.T) {
				r := scheduleTestResource(t, func(w http.ResponseWriter, req *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"scheduleId":"placeholder-id","name":"n","command":"c","cronExpression":"cron","scheduleType":"dokploy-server"` + field.json + `}`))
				})
				got, err := r.Read(t.Context(), infer.ReadRequest[ScheduleArgs, ScheduleState]{ID: "placeholder-id", State: prior})
				if field.invalid {
					require.Error(t, err)
					require.Empty(t, got.ID)
					return
				}
				require.NoError(t, err)
				require.Equal(t, "placeholder-id", got.ID)
				require.Equal(t, field.want, got.Inputs.Enabled)
				require.Equal(t, field.want, got.State.Enabled)
			})
		}
	}
}

func TestScheduleReadClearsExplicitNullPointerFields(t *testing.T) {
	r := scheduleTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"scheduleId":"placeholder-id","name":"n","command":"c","cronExpression":"cron","scheduleType":"dokploy-server","appName":null,"shellType":null,"applicationId":null,"composeId":null,"serverId":null,"organizationId":null,"serviceName":null,"description":null}`))
	})
	prior := scheduleTestArgs()
	prior.ApplicationID, prior.ComposeID, prior.ServerID = ptr("placeholder-app"), ptr("placeholder-compose"), ptr("placeholder-server")
	got, err := r.Read(t.Context(), infer.ReadRequest[ScheduleArgs, ScheduleState]{ID: "placeholder-id", State: ScheduleState{ScheduleArgs: prior}})
	require.NoError(t, err)
	for _, v := range []*string{got.Inputs.AppName, got.Inputs.ShellType, got.Inputs.ApplicationID, got.Inputs.ComposeID, got.Inputs.ServerID, got.Inputs.OrganizationID, got.Inputs.ServiceName, got.Inputs.Description} {
		require.Nil(t, v)
	}
	// Omitted secret fields retain prior values rather than being cleared.
	require.True(t, got.Inputs.Script != nil && *got.Inputs.Script == *prior.Script)
}

func TestScheduleUpdateRejectsMismatchedReadback(t *testing.T) {
	calls := 0
	r := scheduleTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 2 {
			_, _ = w.Write([]byte(`{"scheduleId":"unrelated","name":"n","command":"c","cronExpression":"cron","scheduleType":"dokploy-server"}`))
		}
	})
	got, err := r.Update(t.Context(), infer.UpdateRequest[ScheduleArgs, ScheduleState]{ID: "placeholder-id", Inputs: scheduleTestArgs()})
	require.Error(t, err)
	require.True(t, got.Output.ScheduleID == "placeholder-id")
	require.NotContains(t, err.Error(), "unrelated")
	require.Equal(t, 2, calls)
}

func TestScheduleErrorsClassifyTransportSafely(t *testing.T) {
	err := sanitizeScheduleError(errors.New("https://private.example.invalid/placeholder-id placeholder-command"), scheduleTestArgs())
	require.NotContains(t, err.Error(), "private.example.invalid")
	require.NotContains(t, err.Error(), "placeholder-id")
	require.NotContains(t, err.Error(), "placeholder-command")
	require.ErrorIs(t, sanitizeScheduleError(context.Canceled, ScheduleArgs{}), context.Canceled)
	require.ErrorIs(t, sanitizeScheduleError(context.DeadlineExceeded, ScheduleArgs{}), context.DeadlineExceeded)
}

func TestScheduleUpdateAndDelete(t *testing.T) {
	a := scheduleTestArgs()
	a.Script = nil
	a.Description = nil
	a.Enabled = true
	calls := 0
	r := scheduleTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		switch calls {
		case 1:
			require.Equal(t, "/api/schedule.update", req.URL.Path)
			var b map[string]any
			require.NoError(t, json.NewDecoder(req.Body).Decode(&b))
			for k, v := range map[string]any{"scheduleId": "placeholder-id", "name": a.Name, "cronExpression": a.CronExpression, "command": a.Command, "scheduleType": a.ScheduleType, "enabled": true, "script": nil, "description": nil, "applicationId": nil, "composeId": nil, "serverId": nil, "timezone": *a.Timezone, "organizationId": *a.OrganizationID, "appName": *a.AppName, "serviceName": *a.ServiceName, "shellType": *a.ShellType} {
				v2, present := b[k]
				require.True(t, present && v2 == v, "incorrect field: %s", k)
			}
		case 2:
			require.Equal(t, "/api/schedule.one", req.URL.Path)
			require.True(t, req.URL.Query().Get("scheduleId") == "placeholder-id")
			_, _ = w.Write([]byte(`{"scheduleId":"placeholder-id","name":"refreshed","command":"placeholder-command","cronExpression":"cron","scheduleType":"dokploy-server"}`))
		case 3, 4:
			require.Equal(t, "/api/schedule.delete", req.URL.Path)
			var b map[string]string
			require.NoError(t, json.NewDecoder(req.Body).Decode(&b))
			require.True(t, b["scheduleId"] == "placeholder-id")
			if calls == 4 {
				w.WriteHeader(404)
			}
		default:
			t.Error("unexpected API call")
		}
	})
	up, err := r.Update(t.Context(), infer.UpdateRequest[ScheduleArgs, ScheduleState]{ID: "placeholder-id", Inputs: a})
	require.NoError(t, err)
	require.Equal(t, "refreshed", up.Output.Name)
	for range 2 {
		_, err = r.Delete(t.Context(), infer.DeleteRequest[ScheduleState]{ID: "placeholder-id"})
		require.NoError(t, err)
	}
	require.Equal(t, 4, calls)
}

func TestScheduleErrorsAreRedacted(t *testing.T) {
	for _, phase := range []string{"create", "create-readback", "read", "update", "update-readback", "delete"} {
		t.Run(phase, func(t *testing.T) {
			a := scheduleTestArgs()
			old := a
			old.Command = "placeholder-old-command"
			old.Script = ptr("placeholder-old-script")
			calls := 0
			id := "placeholder-id"
			r := scheduleTestResource(t, func(w http.ResponseWriter, req *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				if phase == "create" || phase == "create-readback" && calls == 1 {
					var b map[string]any
					require.NoError(t, json.NewDecoder(req.Body).Decode(&b))
					id, _ = b["scheduleId"].(string)
				}
				if (phase == "update-readback" || phase == "create-readback") && calls == 1 {
					return
				}
				w.WriteHeader(400)
				require.NoError(t, json.NewEncoder(w).Encode(map[string]string{"code": id + " " + a.Command, "message": *a.Script + " " + old.Command + " " + *old.Script + " https://private.example.invalid/path"}))
			})
			var err error
			switch phase {
			case "create", "create-readback":
				_, err = r.Create(t.Context(), infer.CreateRequest[ScheduleArgs]{Inputs: a})
			case "read":
				_, err = r.Read(t.Context(), infer.ReadRequest[ScheduleArgs, ScheduleState]{ID: id, Inputs: a, State: ScheduleState{ScheduleArgs: old}})
			case "update", "update-readback":
				_, err = r.Update(t.Context(), infer.UpdateRequest[ScheduleArgs, ScheduleState]{ID: id, Inputs: a, State: ScheduleState{ScheduleArgs: old}})
			case "delete":
				_, err = r.Delete(t.Context(), infer.DeleteRequest[ScheduleState]{ID: id, State: ScheduleState{ScheduleArgs: a}})
			}
			require.Error(t, err)
			for _, v := range []string{id, a.Command, *a.Script, old.Command, *old.Script, "private.example.invalid"} {
				require.False(t, strings.Contains(err.Error(), v))
			}
			var apiErr *client.APIError
			if errors.As(err, &apiErr) {
				require.Empty(t, apiErr.Code)
				require.Empty(t, apiErr.Message)
				require.Empty(t, apiErr.Operation)
			}
		})
	}
}

func TestScheduleCheckTypesAndTargets(t *testing.T) {
	for _, tc := range []struct {
		typ                  string
		app, compose, server bool
		valid                bool
	}{{"application", true, false, false, true}, {"compose", false, true, false, true}, {"server", false, false, true, true}, {"dokploy-server", false, false, false, true}, {"other", false, false, false, false}} {
		a := ScheduleArgs{Name: "n", CronExpression: "* * * * *", Command: "echo", ScheduleType: tc.typ}
		if tc.app {
			a.ApplicationID = ptr("a")
		}
		if tc.compose {
			a.ComposeID = ptr("c")
		}
		if tc.server {
			a.ServerID = ptr("s")
		}
		values := map[string]property.Value{"name": property.New(a.Name), "cronExpression": property.New(a.CronExpression), "command": property.New(a.Command), "scheduleType": property.New(a.ScheduleType)}
		if a.ApplicationID != nil {
			values["applicationId"] = property.New(*a.ApplicationID)
		}
		if a.ComposeID != nil {
			values["composeId"] = property.New(*a.ComposeID)
		}
		if a.ServerID != nil {
			values["serverId"] = property.New(*a.ServerID)
		}
		pm := property.NewMap(values)
		got, err := (Schedule{}).Check(t.Context(), infer.CheckRequest{NewInputs: pm})
		require.NoError(t, err)
		require.Equal(t, tc.valid, len(got.Failures) == 0, tc.typ)
	}
}

func TestScheduleCheckDefersComputedTargets(t *testing.T) {
	pm := property.NewMap(map[string]property.Value{"name": property.New("n"), "cronExpression": property.New("* * * * *"), "command": property.New("echo"), "scheduleType": property.New("application"), "applicationId": property.New(property.Computed)})
	got, err := (Schedule{}).Check(t.Context(), infer.CheckRequest{NewInputs: pm})
	require.NoError(t, err)
	require.Empty(t, got.Failures)
}

func TestScheduleCheckEmptyAndComputedType(t *testing.T) {
	for _, tc := range []struct {
		name     string
		typ      property.Value
		failures []string
	}{
		{"empty", property.New(""), []string{"scheduleType"}},
		{"computed-with-known-target", property.New(property.Computed), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values := map[string]property.Value{"name": property.New("n"), "cronExpression": property.New("cron"), "command": property.New("echo"), "scheduleType": tc.typ}
			if tc.name == "computed-with-known-target" {
				values["applicationId"] = property.New("placeholder-app")
			}
			got, err := (Schedule{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(values)})
			require.NoError(t, err)
			require.ElementsMatch(t, tc.failures, failureProperties(got.Failures))
		})
	}
}

func TestScheduleCheckRejectsMissingRequiredFieldsAndInvalidShell(t *testing.T) {
	inputs := property.NewMap(map[string]property.Value{"name": property.New(""), "cronExpression": property.New(""), "command": property.New(""), "scheduleType": property.New("dokploy-server"), "shellType": property.New("zsh")})
	got, err := (Schedule{}).Check(t.Context(), infer.CheckRequest{NewInputs: inputs})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"name", "cronExpression", "command", "shellType"}, failureProperties(got.Failures))
}

func TestScheduleCheckRejectsIncompatibleTargets(t *testing.T) {
	inputs := property.NewMap(map[string]property.Value{"name": property.New("n"), "cronExpression": property.New("* * * * *"), "command": property.New("echo"), "scheduleType": property.New("application"), "applicationId": property.New("a"), "serverId": property.New("s")})
	got, err := (Schedule{}).Check(t.Context(), infer.CheckRequest{NewInputs: inputs})
	require.NoError(t, err)
	require.Equal(t, []string{"serverId"}, failureProperties(got.Failures))
}

func TestScheduleCheckRequiresTypeSpecificTarget(t *testing.T) {
	inputs := property.NewMap(map[string]property.Value{"name": property.New("n"), "cronExpression": property.New("* * * * *"), "command": property.New("echo"), "scheduleType": property.New("server")})
	got, err := (Schedule{}).Check(t.Context(), infer.CheckRequest{NewInputs: inputs})
	require.NoError(t, err)
	require.Equal(t, []string{"serverId"}, failureProperties(got.Failures))
}

func TestScheduleDiffReplacementAndUpdates(t *testing.T) {
	old := ScheduleArgs{Name: "n", CronExpression: "* * * * *", Command: "echo", ScheduleType: "application", ApplicationID: ptr("a"), AppName: ptr("app")}
	next := old
	next.ApplicationID = ptr("b")
	next.Name = "new"
	d, err := (Schedule{}).Diff(t.Context(), infer.DiffRequest[ScheduleArgs, ScheduleState]{Inputs: next, State: ScheduleState{ScheduleArgs: old}})
	require.NoError(t, err)
	require.Equal(t, p.UpdateReplace, d.DetailedDiff["applicationId"].Kind)
	require.True(t, d.DeleteBeforeReplace)
	require.Equal(t, p.Update, d.DetailedDiff["name"].Kind)
	equal, err := (Schedule{}).Diff(t.Context(), infer.DiffRequest[ScheduleArgs, ScheduleState]{Inputs: old, State: ScheduleState{ScheduleArgs: old}})
	require.NoError(t, err)
	require.False(t, equal.HasChanges)
}

func TestScheduleDiffFieldMatrix(t *testing.T) {
	for _, tc := range []struct {
		field   string
		replace bool
		change  func(*ScheduleArgs)
	}{
		{"scheduleType", true, func(a *ScheduleArgs) { a.ScheduleType = "server" }},
		{"applicationId", true, func(a *ScheduleArgs) { a.ApplicationID = ptr("new-app") }},
		{"composeId", true, func(a *ScheduleArgs) { a.ComposeID = ptr("new-compose") }},
		{"serverId", true, func(a *ScheduleArgs) { a.ServerID = ptr("new-server") }},
		{"appName", true, func(a *ScheduleArgs) { a.AppName = ptr("new-app-name") }},
		{"serviceName", true, func(a *ScheduleArgs) { a.ServiceName = ptr("new-service") }},
		{"name", false, func(a *ScheduleArgs) { a.Name = "new-name" }},
		{"cronExpression", false, func(a *ScheduleArgs) { a.CronExpression = "new-cron" }},
		{"command", false, func(a *ScheduleArgs) { a.Command = "new-command" }},
		{"description", false, func(a *ScheduleArgs) { a.Description = ptr("new-description") }},
		{"shellType", false, func(a *ScheduleArgs) { a.ShellType = ptr("bash") }},
		{"script", false, func(a *ScheduleArgs) { a.Script = ptr("new-script") }},
		{"timezone", false, func(a *ScheduleArgs) { a.Timezone = ptr("new-timezone") }},
		{"organizationId", false, func(a *ScheduleArgs) { a.OrganizationID = ptr("new-org") }},
		{"enabled", false, func(a *ScheduleArgs) { a.Enabled = true }},
	} {
		t.Run(tc.field, func(t *testing.T) {
			old := scheduleTestArgs()
			next := old
			tc.change(&next)
			got, err := (Schedule{}).Diff(t.Context(), infer.DiffRequest[ScheduleArgs, ScheduleState]{Inputs: next, State: ScheduleState{ScheduleArgs: old}})
			require.NoError(t, err)
			kind := p.Update
			if tc.replace {
				kind = p.UpdateReplace
			}
			require.True(t, got.HasChanges)
			require.Equal(t, map[string]p.PropertyDiff{tc.field: {Kind: kind}}, got.DetailedDiff)
			require.Equal(t, tc.replace, got.DeleteBeforeReplace)
		})
	}
}
