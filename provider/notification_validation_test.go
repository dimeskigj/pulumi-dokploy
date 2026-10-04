package dokploy

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/blang/semver"
	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/pulumi/pulumi-go-provider/integration"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

func TestNotificationResolvedDefaultsAndValidation(t *testing.T) {
	for _, kind := range []string{"slack", "telegram", "discord", "email", "resend", "gotify", "ntfy", "mattermost", "custom", "lark", "teams", "pushover"} {
		a := normalizeNotificationArgs(notificationTestArgs(kind))
		require.Empty(t, validateNotificationArgs(a), kind)
	}
	a := normalizeNotificationArgs(notificationTestArgs("gotify"))
	require.Equal(t, 5, *a.Gotify.Priority)
	require.False(t, a.Events.ServerThreshold)
	a.Events.ServerThreshold = true
	require.NotEmpty(t, validateNotificationArgs(a))
	for _, tc := range []struct {
		name string
		args NotificationArgs
		path string
	}{
		{"none", NotificationArgs{Name: "example"}, "slack"},
		{"two", func() NotificationArgs {
			a := notificationTestArgs("slack")
			a.Teams = notificationTestArgs("teams").Teams
			return a
		}(), "teams"},
		{"name", NotificationArgs{Slack: notificationTestArgs("slack").Slack}, "name"},
		{"required credential", func() NotificationArgs { a := notificationTestArgs("gotify"); a.Gotify.AppToken = ""; return a }(), "gotify.appToken"},
		{"required recipients", func() NotificationArgs { a := notificationTestArgs("resend"); a.Resend.ToAddresses = nil; return a }(), "resend.toAddresses"},
		{"url", func() NotificationArgs { a := notificationTestArgs("slack"); a.Slack.WebhookURL = "relative"; return a }(), "slack.webhookUrl"},
		{"recipients", func() NotificationArgs {
			a := notificationTestArgs("email")
			a.Email.ToAddresses = []string{""}
			return a
		}(), "email.toAddresses[0]"},
		{"smtp", func() NotificationArgs { a := notificationTestArgs("email"); a.Email.SMTPPort = 65536; return a }(), "email.smtpPort"},
		{"priority", func() NotificationArgs { a := notificationTestArgs("ntfy"); a.Ntfy.Priority = ptr(6); return a }(), "ntfy.priority"},
		{"gotify priority minimum", func() NotificationArgs { a := notificationTestArgs("gotify"); a.Gotify.Priority = ptr(0); return a }(), "gotify.priority"},
		{"gotify priority unsafe integer", func() NotificationArgs {
			a := notificationTestArgs("gotify")
			a.Gotify.Priority = ptr(9007199254740992)
			return a
		}(), "gotify.priority"},
		{"pushover retry minimum", func() NotificationArgs { a := notificationTestArgs("pushover"); a.Pushover.Retry = ptr(29); return a }(), "pushover.retry"},
		{"pushover expire minimum", func() NotificationArgs { a := notificationTestArgs("pushover"); a.Pushover.Expire = ptr(0); return a }(), "pushover.expire"},
		{"pushover expire maximum", func() NotificationArgs {
			a := notificationTestArgs("pushover")
			a.Pushover.Expire = ptr(10801)
			return a
		}(), "pushover.expire"},
		{"emergency", func() NotificationArgs { a := notificationTestArgs("pushover"); a.Pushover.Priority = ptr(2); return a }(), "pushover.retry"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			failures := validateNotificationArgs(normalizeNotificationArgs(tc.args))
			require.NotEmpty(t, failures)
			require.Contains(t, failurePaths(failures), tc.path)
		})
	}
}

func TestNotificationGotifyUnsafePriorityBeforeHTTP(t *testing.T) {
	a := notificationTestArgs("gotify")
	a.Gotify.Priority = ptr(9007199254740992)
	r := Notification{client: func(context.Context) *client.Client { t.Fatal("unsafe priority touched HTTP"); return nil }}
	_, err := r.Create(t.Context(), infer.CreateRequest[NotificationArgs]{Inputs: a})
	require.Error(t, err)
	state := NotificationState{NotificationArgs: notificationTestArgs("gotify"), NotificationID: "placeholder-id", ChannelID: "placeholder-channel", OrganizationID: "org", NotificationType: "gotify"}
	_, err = r.Update(t.Context(), infer.UpdateRequest[NotificationArgs, NotificationState]{ID: state.NotificationID, State: state, Inputs: a})
	require.Error(t, err)
}

// Only used for inference Check encoding; production lifecycle stays unimplemented.
type notificationCheckHarness struct{ Notification }

func (notificationCheckHarness) Create(context.Context, infer.CreateRequest[NotificationArgs]) (infer.CreateResponse[NotificationState], error) {
	panic("Check fixture must not create")
}

type notificationCheckOutcomeHarness struct {
	notificationCheckHarness
	returnError bool
}

type notificationCheckDropHarness struct{ notificationCheckHarness }

type notificationCheckDropArrayHarness struct{ notificationCheckHarness }

func (*notificationCheckDropArrayHarness) Check(context.Context, infer.CheckRequest) (infer.CheckResponse[NotificationArgs], error) {
	return infer.CheckResponse[NotificationArgs]{Inputs: NotificationArgs{Name: "example", Resend: &NotificationResendConfig{APIKey: "placeholder", FromAddress: "sender@example.com", ToAddresses: []string{}}}}, nil
}

func TestNotificationCheckGuardRestoresMissingComputedArrayElement(t *testing.T) {
	inner := notificationResource(infer.Resource(&notificationCheckDropArrayHarness{}))
	checked, err := inner.Check(t.Context(), p.CheckRequest{Urn: lifecycleURN("Notification", "guard-fixture"), Inputs: property.NewMap(map[string]property.Value{
		"name": property.New("example"), "resend": property.New(map[string]property.Value{
			"apiKey": property.New("placeholder"), "fromAddress": property.New("sender@example.com"),
			"toAddresses": property.New([]property.Value{property.New(property.Computed)}),
		}),
	})})
	require.NoError(t, err)
	require.Empty(t, checked.Failures)
	addresses := checked.Inputs.Get("resend").AsMap().Get("toAddresses").AsArray()
	require.Equal(t, 1, addresses.Len())
	require.True(t, addresses.Get(0).IsComputed())
}

func (*notificationCheckDropHarness) Check(context.Context, infer.CheckRequest) (infer.CheckResponse[NotificationArgs], error) {
	return infer.CheckResponse[NotificationArgs]{Inputs: NotificationArgs{Name: "example", Ntfy: &NotificationNtfyConfig{ServerURL: "https://example.com", Topic: "placeholder"}}}, nil
}

func TestNotificationCheckGuardRestoresMissingComputedOptionalSecret(t *testing.T) {
	inner := notificationResource(infer.Resource(&notificationCheckDropHarness{}))
	checked, err := inner.Check(t.Context(), p.CheckRequest{Urn: lifecycleURN("Notification", "guard-fixture"), Inputs: property.NewMap(map[string]property.Value{
		"name": property.New("example"), "ntfy": property.New(map[string]property.Value{
			"serverUrl": property.New("https://example.com"), "topic": property.New("placeholder"), "accessToken": property.New(property.Computed),
		}),
	})})
	require.NoError(t, err)
	require.Empty(t, checked.Failures)
	value := checked.Inputs.Get("ntfy").AsMap().Get("accessToken")
	require.True(t, value.IsComputed())
	require.True(t, value.Secret(), "schema secrecy must also apply to restored computed fields")
}

func (h *notificationCheckOutcomeHarness) Check(context.Context, infer.CheckRequest) (infer.CheckResponse[NotificationArgs], error) {
	response := infer.CheckResponse[NotificationArgs]{Inputs: NotificationArgs{Name: "checked"}, Failures: []p.CheckFailure{{Property: "name", Reason: "fixed failure"}}}
	if h.returnError {
		return response, errors.New("fixed check error")
	}
	return response, nil
}

func TestNotificationCheckGuardPropagatesFailuresAndErrors(t *testing.T) {
	for _, returnError := range []bool{false, true} {
		inner := infer.Resource(&notificationCheckOutcomeHarness{returnError: returnError})
		request := p.CheckRequest{Urn: lifecycleURN("Notification", "guard-fixture"), Inputs: property.NewMap(map[string]property.Value{"name": property.New(property.Computed)})}
		direct, directErr := inner.Check(t.Context(), request)
		guarded, guardedErr := notificationResource(inner).Check(t.Context(), request)
		require.Equal(t, directErr, guardedErr)
		require.Equal(t, direct, guarded)
	}
}

func TestNotificationCheckPreservesComputedMetadata(t *testing.T) {
	computedSecret := property.New(property.Computed).WithSecret(true)
	require.True(t, computedSecret.IsComputed(), "secret is metadata on the computed value, not a wrapper")
	require.True(t, computedSecret.Secret())
	require.True(t, property.New(property.Computed).WithSecret(true).HasComputed())
	direct, err := notificationResource(infer.Resource(&notificationCheckHarness{})).Check(t.Context(), p.CheckRequest{Urn: lifecycleURN("Notification", "metadata-fixture"), Inputs: property.NewMap(map[string]property.Value{"name": property.New("example"), "gotify": property.New(property.Computed)})})
	require.NoError(t, err)
	require.Empty(t, direct.Failures)
	require.True(t, direct.Inputs.Get("gotify").IsComputed(), "direct inference must preserve unknown block (map=%t secret=%t nestedComputed=%t)", direct.Inputs.Get("gotify").IsMap(), direct.Inputs.Get("gotify").Secret(), direct.Inputs.Get("gotify").HasComputed())

	server, err := integration.NewServer(t.Context(), Name, semver.Version{}, integration.WithProvider(infer.Provider(infer.Options{
		Resources: []infer.InferredResource{notificationResource(infer.Resource(&notificationCheckHarness{}))},
	})))
	require.NoError(t, err)
	check := func(inputs map[string]property.Value) property.Map {
		t.Helper()
		response, err := server.Check(p.CheckRequest{Urn: lifecycleURN("Notification", "metadata-fixture"), Inputs: property.NewMap(inputs)})
		require.NoError(t, err)
		require.Empty(t, response.Failures)
		return response.Inputs
	}
	assertComputed := func(v property.Value, secret bool) {
		t.Helper()
		require.True(t, v.IsComputed(), "unknown must survive inference encoding (map=%t null=%t secret=%t nestedComputed=%t)", v.IsMap(), v.IsNull(), v.Secret(), v.HasComputed())
		require.Equal(t, secret, v.Secret())
	}
	got := check(map[string]property.Value{"name": property.New("example"), "gotify": property.New(property.Computed)})
	assertComputed(got.Get("gotify"), false)
	got = check(map[string]property.Value{"name": property.New("example"), "gotify": property.New(map[string]property.Value{
		"serverUrl": property.New("https://example.com"), "appToken": computedSecret,
		"priority": property.New(property.Computed),
	}), "events": property.New(map[string]property.Value{"appDeploy": property.New(property.Computed), "serverThreshold": property.New(property.Computed).WithSecret(true)})})
	block := got.Get("gotify")
	require.True(t, block.IsMap())
	assertComputed(block.AsMap().Get("appToken"), true)
	assertComputed(block.AsMap().Get("priority"), false)
	events := got.Get("events")
	require.True(t, events.IsMap())
	assertComputed(events.AsMap().Get("appDeploy"), false)
	assertComputed(events.AsMap().Get("serverThreshold"), true)
	got = check(map[string]property.Value{"name": property.New("example"), "gotify": property.New(map[string]property.Value{
		"serverUrl": property.New("https://example.com"), "appToken": property.New("placeholder"),
	}), "ntfy": property.New(property.Computed)})
	assertComputed(got.Get("ntfy"), false)
	got = check(map[string]property.Value{"name": property.New("example"), "custom": property.New(map[string]property.Value{
		"endpoint": property.New("https://example.com/hook"),
		"headers":  property.New(map[string]property.Value{"Authorization": computedSecret, "X.Version": property.New("known")}).WithSecret(true),
	})})
	require.True(t, got.Get("custom").AsMap().Get("headers").Secret())
	assertComputed(got.Get("custom").AsMap().Get("headers").AsMap().Get("Authorization"), true)
	require.Equal(t, "known", got.Get("custom").AsMap().Get("headers").AsMap().Get("X.Version").AsString())
	got = check(map[string]property.Value{"name": property.New("example"), "resend": property.New(map[string]property.Value{
		"apiKey": property.New(property.Computed), "fromAddress": property.New("sender@example.com"),
		"toAddresses": property.New([]property.Value{property.New("recipient@example.com"), property.New(property.Computed)}),
	})})
	assertComputed(got.Get("resend").AsMap().Get("apiKey"), true) // schema-added secret
	addresses := got.Get("resend").AsMap().Get("toAddresses").AsArray()
	require.Equal(t, "recipient@example.com", addresses.Get(0).AsString())
	assertComputed(addresses.Get(1), false)
	got = check(map[string]property.Value{"name": property.New("example"), "gotify": property.New(map[string]property.Value{
		"serverUrl": property.New("https://example.com"), "appToken": property.New("placeholder"),
	})})
	require.Equal(t, 5.0, got.Get("gotify").AsMap().Get("priority").AsNumber())
	require.False(t, got.Get("events").AsMap().Get("appDeploy").AsBool())
	got = check(map[string]property.Value{"name": property.New("example"), "slack": property.New(map[string]property.Value{
		"webhookUrl": property.New(property.Computed), "channel": property.New(""),
	}), "events": property.New(property.Computed)})
	assertComputed(got.Get("slack").AsMap().Get("webhookUrl"), true)
	assertComputed(got.Get("events"), false)
	got = check(map[string]property.Value{"name": property.New("example"), "custom": property.New(map[string]property.Value{
		"endpoint": property.New("https://example.com/hook"), "headers": property.New(property.Computed),
	})})
	assertComputed(got.Get("custom").AsMap().Get("headers"), true)
	got = check(map[string]property.Value{"name": property.New("example"), "slack": property.New(map[string]property.Value{
		"webhookUrl": property.New("https://example.com/hook"),
	}), "gotify": property.New(property.Computed), "events": property.New(map[string]property.Value{"serverThreshold": property.New(true)})})
	assertComputed(got.Get("gotify"), false)
	require.True(t, got.Get("events").AsMap().Get("serverThreshold").AsBool())
}

func failurePaths(f []p.CheckFailure) []string {
	paths := make([]string, 0, len(f))
	for _, v := range f {
		paths = append(paths, v.Property)
	}
	return paths
}

func TestNotificationCheckNestedUnknowns(t *testing.T) {
	r := notificationTestResource(t, func(http.ResponseWriter, *http.Request) { t.Error("check must not call API") })
	require.NotNil(t, r.client)
	for _, inputs := range []map[string]property.Value{
		{"name": property.New("example"), "gotify": property.New(property.Computed)},
		{"name": property.New("example"), "gotify": property.New(map[string]property.Value{"serverUrl": property.New("https://example.com"), "appToken": property.New(property.Computed)}), "events": property.New(map[string]property.Value{"serverThreshold": property.New(property.Computed)})},
		{"name": property.New("example"), "gotify": property.New(map[string]property.Value{"serverUrl": property.New("https://example.com"), "appToken": property.New("placeholder")}), "ntfy": property.New(property.Computed)},
		{"name": property.New("example"), "gotify": property.New(map[string]property.Value{"serverUrl": property.New("https://example.com"), "appToken": property.New("placeholder"), "priority": property.New(property.Computed)})},
	} {
		got, err := r.Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(inputs)})
		require.NoError(t, err)
		require.Empty(t, got.Failures)
		if block := inputs["gotify"]; block.IsMap() && block.AsMap().Get("priority").IsComputed() {
			require.NotNil(t, got.Inputs.Gotify.Priority, "inference needs an encodable placeholder for computed fields")
		}
	}
	two, err := (Notification{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{"name": property.New("example"), "slack": property.New(map[string]property.Value{"webhookUrl": property.New("https://example.com")}), "teams": property.New(map[string]property.Value{"webhookUrl": property.New("https://example.com")})})})
	require.NoError(t, err)
	require.NotEmpty(t, two.Failures)
}
