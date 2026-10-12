package dokploy

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

type liveNotificationFixtureOps struct {
	create    func(context.Context, NotificationArgs) (infer.CreateResponse[NotificationState], error)
	read      func(context.Context, string, NotificationState) (infer.ReadResponse[NotificationArgs, NotificationState], error)
	remove    func(context.Context, string, NotificationState) error
	register  func(string, NotificationState) func()
	exercise  func(context.Context, string, NotificationState) error
	uncertain func()
}

func liveNotificationArgs(kind, name string, updated bool) NotificationArgs {
	a := NotificationArgs{Name: name, Events: &NotificationEvents{}}
	suffix := "initial"
	if updated {
		suffix = "updated"
	}
	address := "https://notification.example.invalid/" + kind + "/" + suffix
	token := "placeholder-" + suffix
	from, to := "sender-"+suffix+"@example.invalid", []string{"recipient-" + suffix + "@example.invalid"}
	switch kind {
	case notificationSlack:
		a.Slack = &NotificationSlackConfig{WebhookURL: address}
		if updated {
			a.Slack.Channel = ptr("acceptance-updated")
		}
	case notificationTelegram:
		a.Telegram = &NotificationTelegramConfig{BotToken: token, ChatID: "-1000000000000"}
		if updated {
			a.Telegram.ChatID, a.Telegram.MessageThreadID = "-1000000000001", ptr("1")
		}
	case notificationDiscord:
		a.Discord = &NotificationDiscordConfig{WebhookURL: address}
		if updated {
			a.Discord.Decoration = ptr(true)
		}
	case notificationEmail:
		a.Email = &NotificationEmailConfig{SMTPServer: "smtp-" + suffix + ".example.invalid", SMTPPort: 2525, Username: token, Password: token, FromAddress: from, ToAddresses: to}
		if updated {
			a.Email.SMTPPort = 2526
		}
	case notificationResend:
		a.Resend = &NotificationResendConfig{APIKey: token, FromAddress: from, ToAddresses: to}
	case notificationGotify:
		a.Gotify = &NotificationGotifyConfig{ServerURL: address, AppToken: token}
		if updated {
			a.Gotify.Priority, a.Gotify.Decoration = ptr(7), ptr(true)
		}
	case notificationNtfy:
		a.Ntfy = &NotificationNtfyConfig{ServerURL: address, Topic: "acceptance-" + suffix}
		if updated {
			a.Ntfy.AccessToken, a.Ntfy.Priority = ptr(token), ptr(4)
		}
	case notificationMattermost:
		a.Mattermost = &NotificationMattermostConfig{WebhookURL: address}
		if updated {
			a.Mattermost.Channel, a.Mattermost.Username = ptr("acceptance-updated"), ptr(token)
		}
	case notificationCustom:
		a.Custom = &NotificationCustomConfig{Endpoint: address}
		if updated {
			a.Custom.Headers = map[string]string{"X-Acceptance-Placeholder": token}
		}
	case notificationLark:
		a.Lark = &NotificationLarkConfig{WebhookURL: address}
	case notificationTeams:
		a.Teams = &NotificationTeamsConfig{WebhookURL: address}
	case notificationPushover:
		a.Pushover = &NotificationPushoverConfig{UserKey: token, APIToken: token}
		if updated {
			a.Pushover.Priority, a.Pushover.Retry, a.Pushover.Expire = ptr(2), ptr(30), ptr(120)
		}
	}
	return a
}

// Serial, independent records: no external integrations, test/send calls, or
// event subscriptions. The Tier 1 caller owns the live opt-in gate.
func runLiveNotificationCases(t *testing.T, ctx context.Context, api *client.Client) {
	t.Helper()
	for _, kind := range notificationChannels {
		t.Run(kind, func(t *testing.T) {
			if heavyLiveTierStopped() {
				t.Skip("live acceptance stopped after a cleanup or server-health failure")
			}
			args := liveNotificationArgs(kind, liveRunName("notification-"+kind), false)
			r := Notification{client: fixedClient(api)}
			ops := liveNotificationFixtureOps{
				create: func(c context.Context, a NotificationArgs) (infer.CreateResponse[NotificationState], error) {
					return r.Create(c, infer.CreateRequest[NotificationArgs]{Inputs: a})
				},
				read: func(c context.Context, id string, state NotificationState) (infer.ReadResponse[NotificationArgs, NotificationState], error) {
					return r.Read(c, infer.ReadRequest[NotificationArgs, NotificationState]{ID: id, State: state})
				},
				remove: func(c context.Context, id string, state NotificationState) error {
					_, err := r.Delete(c, infer.DeleteRequest[NotificationState]{ID: id, State: state})
					return err
				},
				uncertain: func() {
					recordStructuralCleanup("notification", errors.New("create ownership uncertain"))
				},
				exercise: func(c context.Context, id string, state NotificationState) error {
					return exerciseNotificationLive(t, c, r, id, state, args, kind)
				},
			}
			ops.register = func(id string, state NotificationState) func() {
				return registerLiveCleanup(t, "notification", id, func(c context.Context) error {
					return ops.remove(c, id, state)
				}, func(c context.Context) (string, error) {
					gone, err := ops.read(c, id, state)
					return gone.ID, err
				})
			}
			requireLiveLifecycleNoError(t, "Notification", "disabled lifecycle", runNotificationLiveFixture(t, ctx, args, ops))
		})
		// t.Run completes its fallback cleanup before allowing another channel.
		if heavyLiveTierStopped() {
			return
		}
	}
}

func exerciseNotificationLive(t *testing.T, ctx context.Context, r Notification, id string, state NotificationState, initial NotificationArgs, kind string) error {
	t.Helper()
	noOp, err := r.Diff(ctx, infer.DiffRequest[NotificationArgs, NotificationState]{ID: id, Inputs: initial, State: state})
	if err != nil {
		return err
	}
	requireLiveEqual(t, "notification initial no-op", false, noOp.HasChanges)
	requireLiveEqual(t, "notification initial no-op details", 0, len(noOp.DetailedDiff))
	updated := liveNotificationArgs(kind, initial.Name+"-updated", true)
	diff, err := r.Diff(ctx, infer.DiffRequest[NotificationArgs, NotificationState]{ID: id, Inputs: updated, State: state})
	if err != nil {
		return err
	}
	requireLiveEqual(t, "notification update changes", true, diff.HasChanges)
	requireLiveEqual(t, "notification update replacement", false, diff.DeleteBeforeReplace)
	requireLiveDiffKind(t, diff.DetailedDiff, "name", p.Update)
	block := reflect.TypeOf(notificationBlock(updated, kind)).Elem()
	for i := 0; i < block.NumField(); i++ {
		field := strings.Split(block.Field(i).Tag.Get("pulumi"), ",")[0]
		requireLiveDiffKind(t, diff.DetailedDiff, kind+"."+field, p.Update)
	}
	changed, err := r.Update(ctx, infer.UpdateRequest[NotificationArgs, NotificationState]{ID: id, Inputs: updated, State: state})
	if err != nil {
		return err
	}
	requireLiveEqual(t, "notification update identity", notificationIdentityOf(state, id), notificationIdentityOf(changed.Output, changed.Output.NotificationID))
	requireLiveEqual(t, "notification update settings", normalizeNotificationArgs(updated), changed.Output.NotificationArgs)
	read, err := r.Read(ctx, infer.ReadRequest[NotificationArgs, NotificationState]{ID: id, State: changed.Output})
	if err != nil {
		return err
	}
	requireLiveEqual(t, "notification updated read ID", id, read.ID)
	requireLiveEqual(t, "notification updated read state", changed.Output, read.State)
	requireLiveEqual(t, "notification updated read inputs", normalizeNotificationArgs(updated), read.Inputs)
	imported, err := r.Read(ctx, infer.ReadRequest[NotificationArgs, NotificationState]{ID: id})
	if err != nil {
		return err
	}
	requireLiveEqual(t, "notification import ID", id, imported.ID)
	requireLiveEqual(t, "notification import identity", notificationIdentityOf(state, id), notificationIdentityOf(imported.State, imported.State.NotificationID))
	requireLiveEqual(t, "notification import settings", normalizeNotificationArgs(updated), imported.Inputs)
	requireLiveEqual(t, "notification import state settings", imported.Inputs, imported.State.NotificationArgs)
	noOp, err = r.Diff(ctx, infer.DiffRequest[NotificationArgs, NotificationState]{ID: id, Inputs: updated, State: imported.State})
	if err != nil {
		return err
	}
	requireLiveEqual(t, "notification updated no-op", false, noOp.HasChanges)
	requireLiveEqual(t, "notification updated no-op details", 0, len(noOp.DetailedDiff))
	replacementKind := notificationSlack
	if kind == notificationSlack {
		replacementKind = notificationTeams
	}
	replacement := liveNotificationArgs(replacementKind, updated.Name, false)
	diff, err = r.Diff(ctx, infer.DiffRequest[NotificationArgs, NotificationState]{ID: id, Inputs: replacement, State: imported.State})
	if err != nil {
		return err
	}
	requireLiveEqual(t, "notification channel replacement changes", true, diff.HasChanges)
	requireLiveEqual(t, "notification channel replacement fields", 2, len(diff.DetailedDiff))
	requireLiveDiffKind(t, diff.DetailedDiff, kind, p.DeleteReplace)
	requireLiveDiffKind(t, diff.DetailedDiff, replacementKind, p.AddReplace)
	// Replacement is a diff assertion only; never mutate another destination.
	return nil
}

func runNotificationLiveFixture(t *testing.T, ctx context.Context, args NotificationArgs, ops liveNotificationFixtureOps) error {
	t.Helper()
	args.Events = &NotificationEvents{}
	created, err := ops.create(ctx, args)
	if created.ID == "" {
		// A 4xx from discovery can follow a successful mutation. The provider's
		// fixed uncertainty marker takes precedence over the HTTP class; do not
		// mistake a failed follow-up GET for a rejected create POST.
		definitelyRejected := notificationDefiniteRejection(err) && !strings.Contains(err.Error(), "creation may have occurred")
		if !definitelyRejected {
			ops.uncertain()
		}
		return errors.New("Notification create identity unavailable")
	}
	// Register the acknowledged ID AND full state before inspecting partial errors.
	// Cleanup keeps this verified identity, never a substituted read/update output.
	release := ops.register(created.ID, created.Output)
	if err != nil {
		return errors.New("Notification acknowledged create could not be confirmed")
	}
	kind, _ := notificationKind(args)
	if created.Output.NotificationID != created.ID || created.Output.ChannelID == "" || created.Output.OrganizationID == "" || created.Output.NotificationType != kind {
		return errors.New("Notification acknowledged create identity incomplete")
	}
	read, err := ops.read(ctx, created.ID, created.Output)
	if err != nil || read.ID != created.ID || notificationIdentityOf(read.State, read.ID) != notificationIdentityOf(created.Output, created.ID) ||
		!reflect.DeepEqual(read.Inputs, normalizeNotificationArgs(args)) || !reflect.DeepEqual(read.State.NotificationArgs, read.Inputs) {
		return errors.New("Notification disabled stateful read could not be confirmed")
	}
	if ops.exercise != nil {
		if err := ops.exercise(ctx, created.ID, read.State); err != nil {
			return errors.New("Notification disabled lifecycle failed")
		}
	}
	cleanupCtx, cancel := cleanupContext()
	defer cancel()
	if err := deleteAndVerifyLiveOwned(cleanupCtx, func() error {
		return ops.remove(cleanupCtx, created.ID, created.Output)
	}, func() (string, error) {
		gone, err := ops.read(cleanupCtx, created.ID, created.Output)
		return gone.ID, err
	}, release); err != nil {
		return errors.New("Notification deletion or absence verification failed")
	}
	return nil
}
