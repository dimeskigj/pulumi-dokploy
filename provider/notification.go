package dokploy

import (
	"context"
	"reflect"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

type Notification struct{ client clientFactory }

type NotificationArgs struct {
	Name       string                        `pulumi:"name"`
	Events     *NotificationEvents           `pulumi:"events,optional"`
	Slack      *NotificationSlackConfig      `pulumi:"slack,optional"`
	Telegram   *NotificationTelegramConfig   `pulumi:"telegram,optional"`
	Discord    *NotificationDiscordConfig    `pulumi:"discord,optional"`
	Email      *NotificationEmailConfig      `pulumi:"email,optional"`
	Resend     *NotificationResendConfig     `pulumi:"resend,optional"`
	Gotify     *NotificationGotifyConfig     `pulumi:"gotify,optional"`
	Ntfy       *NotificationNtfyConfig       `pulumi:"ntfy,optional"`
	Mattermost *NotificationMattermostConfig `pulumi:"mattermost,optional"`
	Custom     *NotificationCustomConfig     `pulumi:"custom,optional"`
	Lark       *NotificationLarkConfig       `pulumi:"lark,optional"`
	Teams      *NotificationTeamsConfig      `pulumi:"teams,optional"`
	Pushover   *NotificationPushoverConfig   `pulumi:"pushover,optional"`
}

type NotificationEvents struct {
	AppDeploy       bool `pulumi:"appDeploy,optional"`
	AppBuildError   bool `pulumi:"appBuildError,optional"`
	DatabaseBackup  bool `pulumi:"databaseBackup,optional"`
	VolumeBackup    bool `pulumi:"volumeBackup,optional"`
	DokployBackup   bool `pulumi:"dokployBackup,optional"`
	DokployRestart  bool `pulumi:"dokployRestart,optional"`
	DockerCleanup   bool `pulumi:"dockerCleanup,optional"`
	ServerThreshold bool `pulumi:"serverThreshold,optional"`
}

type NotificationState struct {
	NotificationArgs
	NotificationID   string `pulumi:"notificationId"`
	NotificationType string `pulumi:"notificationType"`
	ChannelID        string `pulumi:"channelId"`
	OrganizationID   string `pulumi:"organizationId"`
}

func (r *Notification) Annotate(n infer.Annotator) {
	n.SetToken("index", "Notification")
	n.Describe(r, "A Dokploy notification destination and its event subscriptions.")
}
func (a *NotificationArgs) Annotate(n infer.Annotator) {
	n.Describe(&a.Name, "Required notification name.")
	n.Describe(&a.Events, "Optional event subscriptions; every omitted event defaults to false.")
	n.Describe(&a.Slack, "Slack destination; select exactly one channel block.")
	n.Describe(&a.Telegram, "Telegram destination; select exactly one channel block.")
	n.Describe(&a.Discord, "Discord destination; select exactly one channel block.")
	n.Describe(&a.Email, "SMTP email destination; select exactly one channel block.")
	n.Describe(&a.Resend, "Resend email destination; select exactly one channel block.")
	n.Describe(&a.Gotify, "Gotify destination; select exactly one channel block.")
	n.Describe(&a.Ntfy, "Ntfy destination; select exactly one channel block.")
	n.Describe(&a.Mattermost, "Mattermost destination; select exactly one channel block.")
	n.Describe(&a.Custom, "Custom HTTP destination; select exactly one channel block.")
	n.Describe(&a.Lark, "Lark destination; select exactly one channel block.")
	n.Describe(&a.Teams, "Teams destination; select exactly one channel block.")
	n.Describe(&a.Pushover, "Pushover destination; select exactly one channel block.")
}
func (e *NotificationEvents) Annotate(n infer.Annotator) {
	n.Describe(&e.AppDeploy, "Notify on application deployment; defaults to false.")
	n.Describe(&e.AppBuildError, "Notify on application build error; defaults to false.")
	n.Describe(&e.DatabaseBackup, "Notify on database backup; defaults to false.")
	n.Describe(&e.VolumeBackup, "Notify on volume backup; defaults to false.")
	n.Describe(&e.DokployBackup, "Notify on Dokploy backup; defaults to false.")
	n.Describe(&e.DokployRestart, "Notify on Dokploy restart; defaults to false.")
	n.Describe(&e.DockerCleanup, "Notify on Docker cleanup; defaults to false.")
	n.Describe(&e.ServerThreshold, "Notify on server threshold; unsupported for Gotify and Ntfy; defaults to false.")
}
func (s *NotificationState) Annotate(n infer.Annotator) {
	n.Describe(&s.NotificationID, "Stable Dokploy notification ID.")
	n.Describe(&s.NotificationType, "Selected notification channel type.")
	n.Describe(&s.ChannelID, "Stable selected channel relation ID.")
	n.Describe(&s.OrganizationID, "Owning Dokploy organization ID.")
}

func (r Notification) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[NotificationArgs], error) {
	in, failures, err := infer.DefaultCheck[NotificationArgs](ctx, req.NewInputs)
	if err != nil || len(failures) != 0 {
		return infer.CheckResponse[NotificationArgs]{Inputs: in, Failures: failures}, err
	}
	in = normalizeNotificationArgs(in)
	if req.NewInputs.Get("events").IsComputed() {
		in.Events = nil
	}
	if v := req.NewInputs.Get("custom"); v.IsMap() && v.AsMap().Get("headers").IsComputed() && in.Custom != nil {
		in.Custom.Headers = nil
	}
	// Keep computed optional values unresolved; inference's encoder retains their
	// original property metadata when the normalized typed inputs are returned.
	for _, field := range []struct {
		block, key string
		clear      func()
	}{
		{"slack", "channel", func() {
			if in.Slack != nil {
				in.Slack.Channel = nil
			}
		}},
		{"telegram", "messageThreadId", func() {
			if in.Telegram != nil {
				in.Telegram.MessageThreadID = nil
			}
		}},
		{"discord", "decoration", func() {
			if in.Discord != nil {
				in.Discord.Decoration = nil
			}
		}},
		{"gotify", "priority", func() {
			if in.Gotify != nil {
				in.Gotify.Priority = nil
			}
		}},
		{"gotify", "decoration", func() {
			if in.Gotify != nil {
				in.Gotify.Decoration = nil
			}
		}},
		{"ntfy", "priority", func() {
			if in.Ntfy != nil {
				in.Ntfy.Priority = nil
			}
		}},
		{"ntfy", "accessToken", func() {
			if in.Ntfy != nil {
				in.Ntfy.AccessToken = nil
			}
		}},
		{"mattermost", "channel", func() {
			if in.Mattermost != nil {
				in.Mattermost.Channel = nil
			}
		}},
		{"mattermost", "username", func() {
			if in.Mattermost != nil {
				in.Mattermost.Username = nil
			}
		}},
		{"pushover", "priority", func() {
			if in.Pushover != nil {
				in.Pushover.Priority = nil
			}
		}},
	} {
		v := req.NewInputs.Get(field.block)
		if v.IsMap() && v.AsMap().Get(field.key).IsComputed() {
			field.clear()
		}
	}
	return infer.CheckResponse[NotificationArgs]{Inputs: in, Failures: append(failures, validateNotificationKnown(in, req.NewInputs)...)}, nil
}

func (r Notification) Diff(_ context.Context, req infer.DiffRequest[NotificationArgs, NotificationState]) (infer.DiffResponse, error) {
	a, b := normalizeNotificationArgs(req.Inputs), normalizeNotificationArgs(req.State.NotificationArgs)
	d := map[string]p.PropertyDiff{}
	if a.Name != b.Name {
		d["name"] = p.PropertyDiff{Kind: p.Update}
	}
	for _, f := range notificationEventFields {
		if eventValue(*a.Events, f.index) != eventValue(*b.Events, f.index) {
			d["events."+f.name] = p.PropertyDiff{Kind: p.Update}
		}
	}
	ak, _ := notificationKind(a)
	bk, _ := notificationKind(b)
	if ak != bk {
		if bk != "" {
			d[bk] = p.PropertyDiff{Kind: p.DeleteReplace}
		}
		if ak != "" {
			d[ak] = p.PropertyDiff{Kind: p.AddReplace}
		}
	} else if ak != "" {
		notificationConfigDiff(d, ak, a, b)
	}
	return infer.DiffResponse{HasChanges: len(d) > 0, DetailedDiff: d, DeleteBeforeReplace: hasReplacement(d)}, nil
}

// The selected block is compared field-by-field, never marked replace-on-change.
func notificationConfigDiff(d map[string]p.PropertyDiff, kind string, a, b NotificationArgs) {
	av, bv := reflect.ValueOf(notificationBlock(a, kind)).Elem(), reflect.ValueOf(notificationBlock(b, kind)).Elem()
	t := av.Type()
	for i := 0; i < av.NumField(); i++ {
		if reflect.DeepEqual(av.Field(i).Interface(), bv.Field(i).Interface()) {
			continue
		}
		name := t.Field(i).Tag.Get("pulumi")
		for j, c := range name {
			if c == ',' {
				name = name[:j]
				break
			}
		}
		// A containing-property diff avoids unescaped user-controlled header keys.
		d[kind+"."+name] = p.PropertyDiff{Kind: p.Update}
	}
}
