package dokploy

import (
	"context"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
)

// notificationResource is a narrow compatibility boundary for inferred Check:
// infer v1.6.0 can lose computed metadata when a value is also secret.
// Lifecycle, Diff, and schema remain delegated to the inferred resource.
func notificationResource(inner infer.InferredResource) infer.InferredResource {
	return notificationCheckedResource{InferredResource: inner}
}

type notificationCheckedResource struct{ infer.InferredResource }

var notificationCheckFields = map[string][]string{
	notificationSlack:      {"webhookUrl", "channel"},
	notificationTelegram:   {"botToken", "chatId", "messageThreadId"},
	notificationDiscord:    {"webhookUrl", "decoration"},
	notificationEmail:      {"smtpServer", "smtpPort", "username", "password", "fromAddress", "toAddresses"},
	notificationResend:     {"apiKey", "fromAddress", "toAddresses"},
	notificationGotify:     {"serverUrl", "appToken", "priority", "decoration"},
	notificationNtfy:       {"serverUrl", "topic", "accessToken", "priority"},
	notificationMattermost: {"webhookUrl", "channel", "username"},
	notificationCustom:     {"endpoint", "headers"},
	notificationLark:       {"webhookUrl"},
	notificationTeams:      {"webhookUrl"},
	notificationPushover:   {"userKey", "apiToken", "priority", "retry", "expire"},
}

func (r notificationCheckedResource) Check(ctx context.Context, req p.CheckRequest) (p.CheckResponse, error) {
	response, err := r.InferredResource.Check(ctx, req)
	if err != nil || len(response.Failures) != 0 {
		return response, err
	}
	response.Inputs = notificationRestoreChecked(req.Inputs, response.Inputs)
	return response, nil
}

func notificationRestoreChecked(original, checked property.Map) property.Map {
	values := checked.AsMap()
	for key, source := range original.AsMap() {
		if value, ok := values[key]; ok {
			values[key] = notificationRestoreValue(source, value, key)
		} else if source.IsComputed() && notificationCheckKnownField("", key) {
			values[key] = source.WithSecret(source.Secret() || notificationCheckSecretField("", key))
		}
	}
	return property.NewMap(values)
}

func notificationRestoreValue(original, checked property.Value, path string) property.Value {
	secret := original.Secret() || checked.Secret()
	if original.IsComputed() {
		return property.New(property.Computed).WithSecret(secret)
	}
	if original.IsMap() && checked.IsMap() {
		values := checked.AsMap().AsMap()
		for key, source := range original.AsMap().AsMap() {
			if value, ok := values[key]; ok {
				childPath := path + "." + key
				if value.IsNull() && source.IsArray() && notificationCheckKnownField(path, key) && notificationAllComputed(source) {
					values[key] = source
				} else {
					values[key] = notificationRestoreValue(source, value, childPath)
				}
			} else if source.IsComputed() && notificationCheckKnownField(path, key) {
				// Restore typed fields lost by the inference encoder, or map entries
				// when the containing map survived. Ignore unknown input fields.
				values[key] = source.WithSecret(source.Secret() || notificationCheckSecretField(path, key))
			} else if source.IsArray() && notificationCheckKnownField(path, key) && notificationAllComputed(source) {
				values[key] = source
			}
		}
		return property.New(values).WithSecret(secret)
	}
	if original.IsArray() && checked.IsArray() {
		values := checked.AsArray().AsSlice()
		for i, source := range original.AsArray().AsSlice() {
			if i < len(values) {
				values[i] = notificationRestoreValue(source, values[i], path)
			} else if i == len(values) && source.IsComputed() {
				values = append(values, source)
			}
		}
		return property.New(values).WithSecret(secret)
	}
	return checked.WithSecret(secret)
}

func notificationAllComputed(v property.Value) bool {
	if !v.IsArray() {
		return false
	}
	items := v.AsArray().AsSlice()
	if len(items) == 0 {
		return false
	}
	for _, item := range items {
		if !item.IsComputed() {
			return false
		}
	}
	return true
}

func notificationCheckKnownField(parent, key string) bool {
	if parent == "" {
		if key == "name" || key == "events" {
			return true
		}
		for _, kind := range notificationChannels {
			if key == kind {
				return true
			}
		}
		return false
	}
	if parent == "custom.headers" {
		return true
	}
	if parent == "events" {
		for _, event := range notificationEventFields {
			if key == event.name {
				return true
			}
		}
		return false
	}
	for _, name := range notificationCheckFields[parent] {
		if key == name {
			return true
		}
	}
	return false
}

// Only used if inference omitted an originally computed property entirely.
// Normal encoded fields already carry secrecy from infer's schema tags.
func notificationCheckSecretField(parent, key string) bool {
	switch parent + "." + key {
	case "slack.webhookUrl", "discord.webhookUrl", "mattermost.webhookUrl", "lark.webhookUrl", "teams.webhookUrl",
		"telegram.botToken", "email.password", "resend.apiKey", "gotify.serverUrl", "gotify.appToken",
		"ntfy.serverUrl", "ntfy.accessToken", "custom.endpoint", "custom.headers", "pushover.userKey", "pushover.apiToken":
		return true
	}
	return false
}
