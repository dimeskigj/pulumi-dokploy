package dokploy

import (
	"fmt"
	"net/url"
	"reflect"
	"strings"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
)

const (
	notificationSlack      = "slack"
	notificationTelegram   = "telegram"
	notificationDiscord    = "discord"
	notificationEmail      = "email"
	notificationResend     = "resend"
	notificationGotify     = "gotify"
	notificationNtfy       = "ntfy"
	notificationMattermost = "mattermost"
	notificationCustom     = "custom"
	notificationLark       = "lark"
	notificationTeams      = "teams"
	notificationPushover   = "pushover"
	notificationHTTPS      = "https"
)

var notificationChannels = []string{notificationSlack, notificationTelegram, notificationDiscord, notificationEmail, notificationResend, notificationGotify, notificationNtfy, notificationMattermost, notificationCustom, notificationLark, notificationTeams, notificationPushover}

func notificationBlock(a NotificationArgs, kind string) any {
	switch kind {
	case notificationSlack:
		return a.Slack
	case notificationTelegram:
		return a.Telegram
	case notificationDiscord:
		return a.Discord
	case notificationEmail:
		return a.Email
	case notificationResend:
		return a.Resend
	case notificationGotify:
		return a.Gotify
	case notificationNtfy:
		return a.Ntfy
	case notificationMattermost:
		return a.Mattermost
	case notificationCustom:
		return a.Custom
	case notificationLark:
		return a.Lark
	case notificationTeams:
		return a.Teams
	case notificationPushover:
		return a.Pushover
	default:
		return nil
	}
}
func notificationSelected(a NotificationArgs, kind string) bool {
	v := notificationBlock(a, kind)
	return v != nil && !reflect.ValueOf(v).IsNil()
}
func notificationKind(a NotificationArgs) (string, error) {
	kind := ""
	for _, k := range notificationChannels {
		if notificationSelected(a, k) {
			if kind != "" {
				return "", fmt.Errorf("select exactly one notification channel")
			}
			kind = k
		}
	}
	if kind == "" {
		return "", fmt.Errorf("select exactly one notification channel")
	}
	return kind, nil
}
func notificationOptionalString(v *string) *string {
	if v != nil {
		return ptr(*v)
	}
	return ptr("")
}
func normalizeNotificationArgs(a NotificationArgs) NotificationArgs {
	if a.Events == nil {
		a.Events = &NotificationEvents{}
	} else {
		e := *a.Events
		a.Events = &e
	}
	if a.Slack != nil {
		v := *a.Slack
		v.Channel = notificationOptionalString(v.Channel)
		a.Slack = &v
	}
	if a.Telegram != nil {
		v := *a.Telegram
		v.MessageThreadID = notificationOptionalString(v.MessageThreadID)
		a.Telegram = &v
	}
	if a.Discord != nil {
		v := *a.Discord
		if v.Decoration == nil {
			v.Decoration = ptr(false)
		}
		a.Discord = &v
	}
	if a.Email != nil {
		v := *a.Email
		v.ToAddresses = append([]string(nil), v.ToAddresses...)
		a.Email = &v
	}
	if a.Resend != nil {
		v := *a.Resend
		v.ToAddresses = append([]string(nil), v.ToAddresses...)
		a.Resend = &v
	}
	if a.Gotify != nil {
		v := *a.Gotify
		if v.Priority == nil {
			v.Priority = ptr(5)
		}
		if v.Decoration == nil {
			v.Decoration = ptr(false)
		}
		a.Gotify = &v
	}
	if a.Ntfy != nil {
		v := *a.Ntfy
		v.AccessToken = notificationOptionalString(v.AccessToken)
		if v.Priority == nil {
			v.Priority = ptr(3)
		}
		a.Ntfy = &v
	}
	if a.Mattermost != nil {
		v := *a.Mattermost
		v.Channel = notificationOptionalString(v.Channel)
		v.Username = notificationOptionalString(v.Username)
		a.Mattermost = &v
	}
	if a.Custom != nil {
		v := *a.Custom
		h := map[string]string{}
		for k, value := range v.Headers {
			h[k] = value
		}
		v.Headers = h
		a.Custom = &v
	}
	if a.Lark != nil {
		v := *a.Lark
		a.Lark = &v
	}
	if a.Teams != nil {
		v := *a.Teams
		a.Teams = &v
	}
	if a.Pushover != nil {
		v := *a.Pushover
		if v.Priority == nil {
			v.Priority = ptr(0)
		}
		a.Pushover = &v
	}
	return a
}

type notificationField struct {
	name  string
	index int
}

var notificationEventFields = []notificationField{{"appDeploy", 0}, {"appBuildError", 1}, {"databaseBackup", 2}, {"volumeBackup", 3}, {"dokployBackup", 4}, {"dokployRestart", 5}, {"dockerCleanup", 6}, {"serverThreshold", 7}}

func eventValue(e NotificationEvents, index int) bool { return reflect.ValueOf(e).Field(index).Bool() }

func validateNotificationArgs(a NotificationArgs) []p.CheckFailure {
	return validateNotificationKnown(a, property.Map{})
}

func validateNotificationKnown(a NotificationArgs, raw property.Map) []p.CheckFailure {
	failures := []p.CheckFailure{}
	known := func(path string) bool {
		segments := strings.Split(path, ".")
		v := raw.Get(segments[0])
		if v.IsComputed() {
			return false
		}
		for _, s := range segments[1:] {
			if !v.IsMap() {
				return true
			}
			v = v.AsMap().Get(s)
			if v.IsComputed() {
				return false
			}
		}
		return true
	}
	add := func(path, reason string) {
		if known(path) {
			failures = append(failures, p.CheckFailure{Property: path, Reason: reason})
		}
	}
	required := func(path, value string) {
		if value == "" {
			add(path, path+" must not be empty")
		}
	}
	address := func(path, value string) {
		if !known(path) {
			return
		}
		u, err := url.Parse(value)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != notificationHTTPS) {
			add(path, path+" must be an absolute HTTP(S) URL")
		}
	}
	recipients := func(path string, values []string) {
		if !known(path) {
			return
		}
		if len(values) == 0 {
			add(path, path+" must contain recipients")
		}
		for i, v := range values {
			rawBlock := raw.Get(strings.Split(path, ".")[0])
			if rawBlock.IsMap() {
				array := rawBlock.AsMap().Get("toAddresses")
				if array.IsArray() && i < array.AsArray().Len() && array.AsArray().Get(i).IsComputed() {
					continue
				}
			}
			if v == "" {
				add(fmt.Sprintf("%s[%d]", path, i), path+" must contain nonempty recipients")
			}
		}
	}
	required("name", a.Name)
	count := 0
	unknown := false
	for _, k := range notificationChannels {
		if notificationSelected(a, k) && known(k) {
			count++
			if count > 1 {
				add(k, "select exactly one notification channel")
			}
		}
		if raw.Get(k).IsComputed() {
			unknown = true
		}
	}
	if count == 0 && !unknown {
		add("slack", "select exactly one notification channel")
	}
	if a.Slack != nil {
		address("slack.webhookUrl", a.Slack.WebhookURL)
	}
	if a.Telegram != nil {
		required("telegram.botToken", a.Telegram.BotToken)
		required("telegram.chatId", a.Telegram.ChatID)
	}
	if a.Discord != nil {
		address("discord.webhookUrl", a.Discord.WebhookURL)
	}
	if a.Email != nil {
		required("email.smtpServer", a.Email.SMTPServer)
		required("email.username", a.Email.Username)
		required("email.password", a.Email.Password)
		required("email.fromAddress", a.Email.FromAddress)
		recipients("email.toAddresses", a.Email.ToAddresses)
		if a.Email.SMTPPort < 1 || a.Email.SMTPPort > 65535 {
			add("email.smtpPort", "email.smtpPort must be from 1 through 65535")
		}
	}
	if a.Resend != nil {
		required("resend.apiKey", a.Resend.APIKey)
		required("resend.fromAddress", a.Resend.FromAddress)
		recipients("resend.toAddresses", a.Resend.ToAddresses)
	}
	if a.Gotify != nil {
		address("gotify.serverUrl", a.Gotify.ServerURL)
		required("gotify.appToken", a.Gotify.AppToken)
		if a.Gotify.Priority != nil && *a.Gotify.Priority < 1 {
			add("gotify.priority", "gotify.priority must be at least 1")
		}
	}
	if a.Ntfy != nil {
		address("ntfy.serverUrl", a.Ntfy.ServerURL)
		required("ntfy.topic", a.Ntfy.Topic)
		if a.Ntfy.Priority != nil && (*a.Ntfy.Priority < 1 || *a.Ntfy.Priority > 5) {
			add("ntfy.priority", "ntfy.priority must be from 1 through 5")
		}
	}
	if a.Mattermost != nil {
		address("mattermost.webhookUrl", a.Mattermost.WebhookURL)
	}
	if a.Custom != nil {
		address("custom.endpoint", a.Custom.Endpoint)
	}
	if a.Lark != nil {
		address("lark.webhookUrl", a.Lark.WebhookURL)
	}
	if a.Teams != nil {
		address("teams.webhookUrl", a.Teams.WebhookURL)
	}
	if a.Pushover != nil {
		required("pushover.userKey", a.Pushover.UserKey)
		required("pushover.apiToken", a.Pushover.APIToken)
		if a.Pushover.Priority != nil && (*a.Pushover.Priority < -2 || *a.Pushover.Priority > 2) {
			add("pushover.priority", "pushover.priority must be from -2 through 2")
		}
		if a.Pushover.Retry != nil && *a.Pushover.Retry < 30 {
			add("pushover.retry", "pushover.retry must be at least 30")
		}
		if a.Pushover.Expire != nil && (*a.Pushover.Expire < 1 || *a.Pushover.Expire > 10800) {
			add("pushover.expire", "pushover.expire must be from 1 through 10800")
		}
		if a.Pushover.Priority != nil && *a.Pushover.Priority == 2 && known("pushover.priority") {
			if a.Pushover.Retry == nil {
				add("pushover.retry", "pushover.retry is required for emergency priority")
			}
			if a.Pushover.Expire == nil {
				add("pushover.expire", "pushover.expire is required for emergency priority")
			}
		}
	}
	if a.Events != nil && a.Events.ServerThreshold && ((a.Gotify != nil && known("gotify")) || (a.Ntfy != nil && known("ntfy"))) && known("events.serverThreshold") {
		add("events.serverThreshold", "serverThreshold is unsupported for Gotify and Ntfy")
	}
	return failures
}
