package dokploy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/blang/semver"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/integration"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

func TestNotificationInferencePreviewAndSecrets(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("preview must not call API") }))
	defer s.Close()
	server, err := integration.NewServer(t.Context(), Name, semver.Version{}, integration.WithProvider(Provider()))
	require.NoError(t, err)
	require.NoError(t, server.Configure(p.ConfigureRequest{Args: property.NewMap(map[string]property.Value{"endpoint": property.New(s.URL), "apiKey": property.New("placeholder")})}))
	urn := lifecycleURN("Notification", "preview")
	block := func(webhook property.Value) property.Value {
		return property.New(map[string]property.Value{"webhookUrl": webhook})
	}
	base := func() map[string]property.Value { return map[string]property.Value{"name": property.New("example")} }
	check := func(inputs map[string]property.Value) property.Map {
		t.Helper()
		resp, err := server.Check(p.CheckRequest{Urn: urn, Inputs: property.NewMap(inputs)})
		require.NoError(t, err)
		require.Empty(t, resp.Failures)
		return resp.Inputs
	}
	unknown := property.New(property.Computed)
	for _, inputs := range []map[string]property.Value{
		{"name": property.New("example"), "slack": unknown},
		{"name": property.New("example"), "slack": block(unknown)},
		{"name": property.New("example"), "slack": block(property.New("https://example.com/hook")), "events": unknown},
		{"name": property.New("example"), "slack": block(property.New("https://example.com/hook")), "events": property.New(map[string]property.Value{"appDeploy": unknown})},
		{"name": property.New("example"), "slack": block(property.New("https://example.com/hook")), "teams": unknown},
	} {
		got := check(inputs)
		for name, value := range inputs {
			if value.IsComputed() {
				require.True(t, got.Get(name).IsComputed(), name)
			}
			if value.IsMap() {
				for child, v := range value.AsMap().AsMap() {
					if v.IsComputed() {
						require.True(t, got.Get(name).AsMap().Get(child).IsComputed(), name+"."+child)
					}
				}
			}
		}
		created, err := server.Create(p.CreateRequest{Urn: urn, Properties: got, DryRun: true})
		require.NoError(t, err)
		for _, field := range []string{"notificationId", "notificationType", "channelId", "organizationId"} {
			require.True(t, created.Properties.Get(field).IsComputed(), field)
		}
	}
	inputs := base()
	inputs["slack"] = block(property.New("https://example.com/hook"))
	checked := check(inputs)
	require.True(t, checked.Get("slack").AsMap().Get("webhookUrl").Secret())
	created, err := server.Create(p.CreateRequest{Urn: urn, Properties: checked, DryRun: true})
	require.NoError(t, err)
	require.True(t, created.Properties.Get("slack").AsMap().Get("webhookUrl").Secret())
	for _, field := range []string{"notificationId", "notificationType", "channelId", "organizationId"} {
		require.False(t, created.Properties.Get(field).Secret(), field)
	}
	for _, sample := range []struct {
		channel, defaultField string
		value                 property.Value
		want                  float64
	}{
		{"gotify", "priority", property.New(map[string]property.Value{"serverUrl": property.New("https://example.com"), "appToken": property.New("placeholder")}), 5},
		{"ntfy", "priority", property.New(map[string]property.Value{"serverUrl": property.New("https://example.com"), "topic": property.New("placeholder")}), 3},
		{"pushover", "priority", property.New(map[string]property.Value{"userKey": property.New("placeholder"), "apiToken": property.New("placeholder")}), 0},
	} {
		got := check(map[string]property.Value{"name": property.New("example"), sample.channel: sample.value})
		require.Equal(t, sample.want, got.Get(sample.channel).AsMap().Get(sample.defaultField).AsNumber(), sample.channel)
		require.False(t, got.Get("events").AsMap().Get("serverThreshold").AsBool())
	}
	for _, sample := range []struct {
		channel, field string
		block          map[string]property.Value
		secret         bool
	}{
		{"slack", "channel", map[string]property.Value{"webhookUrl": property.New("https://example.com/hook"), "channel": unknown}, false},
		{"discord", "decoration", map[string]property.Value{"webhookUrl": property.New("https://example.com/hook"), "decoration": unknown}, false},
		{"gotify", "priority", map[string]property.Value{"serverUrl": property.New("https://example.com"), "appToken": property.New("placeholder"), "priority": unknown}, false},
		{"ntfy", "accessToken", map[string]property.Value{"serverUrl": property.New("https://example.com"), "topic": property.New("placeholder"), "accessToken": unknown}, true},
		{"custom", "headers", map[string]property.Value{"endpoint": property.New("https://example.com/hook"), "headers": unknown}, true},
	} {
		got := check(map[string]property.Value{"name": property.New("example"), sample.channel: property.New(sample.block)})
		value := got.Get(sample.channel).AsMap().Get(sample.field)
		require.True(t, value.IsComputed(), sample.channel+"."+sample.field)
		require.Equal(t, sample.secret, value.Secret(), sample.channel+"."+sample.field)
	}
	custom := check(map[string]property.Value{"name": property.New("example"), "custom": property.New(map[string]property.Value{
		"endpoint": property.New("https://example.com/hook"),
		"headers":  property.New(map[string]property.Value{"Authorization": property.New("placeholder")}),
	})})
	require.True(t, custom.Get("custom").AsMap().Get("endpoint").Secret())
	require.True(t, custom.Get("custom").AsMap().Get("headers").Secret())
	omittedHeaders := check(map[string]property.Value{"name": property.New("example"), "custom": property.New(map[string]property.Value{
		"endpoint": property.New("https://example.com/hook"),
	})})
	require.True(t, omittedHeaders.Get("custom").AsMap().Get("headers").IsMap())
	require.Empty(t, omittedHeaders.Get("custom").AsMap().Get("headers").AsMap().AsMap())
	require.True(t, omittedHeaders.Get("custom").AsMap().Get("headers").Secret())
	customPreview, err := server.Create(p.CreateRequest{Urn: urn, Properties: custom, DryRun: true})
	require.NoError(t, err)
	require.True(t, customPreview.Properties.Get("custom").AsMap().Get("headers").Secret())
	updated := base()
	updated["slack"] = block(property.New("https://example.com/new"))
	prior := property.NewMap(map[string]property.Value{"name": property.New("example"), "slack": inputs["slack"], "notificationId": property.New("opaque"), "notificationType": property.New("slack"), "channelId": property.New("channel"), "organizationId": property.New("org")})
	diff, err := server.Diff(p.DiffRequest{Urn: urn, ID: "opaque", Inputs: property.NewMap(updated), State: prior})
	require.NoError(t, err)
	require.Equal(t, p.Update, diff.DetailedDiff["slack.webhookUrl"].Kind)
	preview, err := server.Update(p.UpdateRequest{Urn: urn, ID: "opaque", Inputs: check(updated), OldInputs: checked, State: prior, DryRun: true})
	require.NoError(t, err)
	for _, field := range []string{"notificationId", "notificationType", "channelId", "organizationId"} {
		require.Equal(t, prior.Get(field), preview.Properties.Get(field), field)
	}
	require.True(t, preview.Properties.Get("slack").AsMap().Get("webhookUrl").Secret())
	switched := base()
	switched["teams"] = block(property.New("https://example.com/hook"))
	diff, err = server.Diff(p.DiffRequest{Urn: urn, ID: "opaque", Inputs: property.NewMap(switched), State: prior})
	require.NoError(t, err)
	require.Equal(t, p.AddReplace, diff.DetailedDiff["teams"].Kind)
}
