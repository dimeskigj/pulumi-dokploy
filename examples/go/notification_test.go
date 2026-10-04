package main

import (
	"fmt"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/sdk/go/dokploy"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
)

type notificationMocks struct{ registrations int }

func (m *notificationMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	if args.TypeToken != "dokploy:index:Notification" {
		return "", nil, fmt.Errorf("unexpected resource type: %s", args.TypeToken)
	}
	m.registrations++
	outputs := args.Inputs.Copy()
	for key, value := range map[string]string{
		"notificationId": "notification-placeholder",
		"notificationType": "slack",
		"channelId": "channel-placeholder",
		"organizationId": "organization-placeholder",
	} {
		outputs[resource.PropertyKey(key)] = resource.NewStringProperty(value)
	}
	return "notification-placeholder", outputs, nil
}

func (*notificationMocks) Call(pulumi.MockCallArgs) (resource.PropertyMap, error) {
	return nil, fmt.Errorf("unexpected invoke")
}

func TestNotificationSDKExample(t *testing.T) {
	mocks := &notificationMocks{}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		// Compile a typed Custom headers map, including non-identifier keys; this
		// block is not selected alongside Slack on the same resource.
		custom := dokploy.NotificationCustomConfigArgs{
			Endpoint: pulumi.ToSecret(pulumi.String("https://example.invalid/notify")).(pulumi.StringOutput),
			Headers: pulumi.StringMap{
				"X.Trace.Id": pulumi.String("placeholder"),
				"X[Audit]":  pulumi.String("placeholder"),
			},
		}
		_ = custom
		notification, err := dokploy.NewNotification(ctx, "alerts", &dokploy.NotificationArgs{
			Name: pulumi.String("alerts"),
			Slack: dokploy.NotificationSlackConfigArgs{
				WebhookUrl: pulumi.ToSecret(pulumi.String("https://example.invalid/webhook")).(pulumi.StringOutput),
			},
			Events: dokploy.NotificationEventsArgs{
				AppDeploy:     pulumi.BoolPtr(true),
				AppBuildError: pulumi.BoolPtr(true),
			},
		})
		if err != nil {
			return err
		}
		ctx.Export("notificationId", pulumi.All(notification.ID(), notification.NotificationId).ApplyT(func(values []interface{}) (string, error) {
			id := string(values[0].(pulumi.ID))
			if id != "notification-placeholder" || values[1].(string) != id {
				return "", fmt.Errorf("mock notification ID output did not match registration")
			}
			return id, nil
		}))
		return nil
	}, pulumi.WithMocks("notification-example", "test", mocks))
	if err != nil {
		t.Fatal(err)
	}
	if mocks.registrations != 1 {
		t.Fatalf("expected one Notification registration, got %d", mocks.registrations)
	}
}
