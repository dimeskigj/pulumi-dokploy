package dokploy

import (
	"context"
	"fmt"
	"math"
	"strings"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

type PortProtocol string

func (PortProtocol) Annotate(n infer.Annotator) {
	n.SetToken("index", "PortProtocol")
	n.Describe(PortProtocolTCP, "Port protocol: tcp or udp.")
}

const (
	PortProtocolTCP PortProtocol = "tcp"
	PortProtocolUDP PortProtocol = "udp"
)

func (PortProtocol) Values() []infer.EnumValue[PortProtocol] {
	return []infer.EnumValue[PortProtocol]{{Name: "Tcp", Value: PortProtocolTCP, Description: "TCP protocol (tcp)."}, {Name: "Udp", Value: PortProtocolUDP, Description: "UDP protocol (udp)."}}
}

type PortPublishMode string

func (PortPublishMode) Annotate(n infer.Annotator) {
	n.SetToken("index", "PortPublishMode")
	n.Describe(PortPublishModeIngress, "Port publish mode: ingress or host.")
}

const (
	PortPublishModeIngress PortPublishMode = "ingress"
	PortPublishModeHost    PortPublishMode = "host"
)

func (PortPublishMode) Values() []infer.EnumValue[PortPublishMode] {
	return []infer.EnumValue[PortPublishMode]{{Name: "Ingress", Value: PortPublishModeIngress, Description: "Ingress publish mode (ingress)."}, {Name: "Host", Value: PortPublishModeHost, Description: "Host publish mode (host)."}}
}

type PortArgs struct {
	ApplicationID string          `pulumi:"applicationId" provider:"replaceOnChanges"`
	PublishedPort int             `pulumi:"publishedPort"`
	TargetPort    int             `pulumi:"targetPort"`
	Protocol      PortProtocol    `pulumi:"protocol,optional" provider:"default:tcp"`
	PublishMode   PortPublishMode `pulumi:"publishMode,optional" provider:"default:ingress"`
}
type PortState struct {
	PortArgs
	PortID string `pulumi:"portId"`
}
type Port struct{ client clientFactory }

func (a *PortArgs) Annotate(n infer.Annotator) {
	n.Describe(&a.ApplicationID, "The Dokploy application ID.")
	n.Describe(&a.PublishedPort, "The externally published port (1-65535).")
	n.Describe(&a.TargetPort, "The application target port (1-65535).")
	n.Describe(&a.Protocol, "The port protocol: tcp or udp.")
	n.SetDefault(&a.Protocol, PortProtocolTCP)
	n.Describe(&a.PublishMode, "The port publish mode: ingress or host.")
	n.SetDefault(&a.PublishMode, PortPublishModeIngress)
}
func (s *PortState) Annotate(n infer.Annotator) { n.Describe(&s.PortID, "Stable Dokploy port ID.") }
func (r *Port) Annotate(n infer.Annotator) {
	n.SetToken("index", "Port")
	n.Describe(r, "A Dokploy application port mapping.")
}
func (r Port) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[PortArgs], error) {
	in, failures, err := infer.DefaultCheck[PortArgs](ctx, req.NewInputs)
	if err != nil {
		return infer.CheckResponse[PortArgs]{Inputs: in, Failures: failures}, err
	}
	add := func(k, s string) { failures = append(failures, p.CheckFailure{Property: k, Reason: s}) }
	if _, ok := req.NewInputs.GetOk("protocol"); !ok {
		in.Protocol = PortProtocolTCP
	}
	if _, ok := req.NewInputs.GetOk("publishMode"); !ok {
		in.PublishMode = PortPublishModeIngress
	}
	app := req.NewInputs.Get("applicationId")
	if !app.HasComputed() && (app.IsNull() || in.ApplicationID == "") {
		add("applicationId", "applicationId must not be empty")
	}
	for _, f := range []struct {
		k string
		v int
	}{{"publishedPort", in.PublishedPort}, {"targetPort", in.TargetPort}} {
		raw, present := req.NewInputs.GetOk(f.k)
		if !present {
			add(f.k, f.k+" must be an integer between 1 and 65535")
			continue
		}
		if raw.HasComputed() {
			continue
		}
		if !raw.IsNumber() {
			add(f.k, f.k+" must be an integer between 1 and 65535")
			continue
		}
		number := raw.AsNumber()
		if math.Trunc(number) != number || number < 1 || number > 65535 {
			add(f.k, f.k+" must be an integer between 1 and 65535")
		}
	}
	if !app.HasComputed() && strings.TrimSpace(in.ApplicationID) == "" && in.ApplicationID != "" {
		add("applicationId", "applicationId must not be empty")
	}
	for _, f := range []struct {
		k, v    string
		allowed []string
		def     string
	}{{"protocol", string(in.Protocol), []string{"tcp", "udp"}, "tcp"}, {"publishMode", string(in.PublishMode), []string{"ingress", "host"}, "ingress"}} {
		raw, present := req.NewInputs.GetOk(f.k)
		if !present {
			continue
		}
		if raw.IsString() && !raw.HasComputed() && raw.AsString() == "" {
			add(f.k, f.k+" must not be empty")
		}
		if raw.IsNull() {
			add(f.k, f.k+" must not be null")
		}
		if raw.IsComputed() {
			continue
		}
		if raw.IsNull() {
			continue
		}
		found := false
		for _, v := range f.allowed {
			if f.v == v {
				found = true
			}
		}
		if !found {
			add(f.k, fmt.Sprintf("%s must be %s or %s", f.k, f.allowed[0], f.allowed[1]))
		}
	}
	return infer.CheckResponse[PortArgs]{Inputs: in, Failures: failures}, nil
}
func validatePortArgs(a PortArgs) error {
	if a.ApplicationID == "" {
		return fmt.Errorf("applicationId must not be empty")
	}
	for k, v := range map[string]int{"publishedPort": a.PublishedPort, "targetPort": a.TargetPort} {
		if v < 1 || v > 65535 {
			return fmt.Errorf("%s must be between 1 and 65535", k)
		}
	}
	if a.Protocol != PortProtocolTCP && a.Protocol != PortProtocolUDP {
		return fmt.Errorf("protocol must be tcp or udp")
	}
	if a.PublishMode != PortPublishModeIngress && a.PublishMode != PortPublishModeHost {
		return fmt.Errorf("publishMode must be ingress or host")
	}
	return nil
}
func (r Port) Diff(_ context.Context, req infer.DiffRequest[PortArgs, PortState]) (infer.DiffResponse, error) {
	a, b := req.Inputs, req.State.PortArgs
	d := map[string]p.PropertyDiff{}
	if a.ApplicationID != b.ApplicationID {
		d["applicationId"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	for _, f := range []struct {
		k       string
		changed bool
	}{{"publishedPort", a.PublishedPort != b.PublishedPort}, {"targetPort", a.TargetPort != b.TargetPort}, {"protocol", a.Protocol != b.Protocol}, {"publishMode", a.PublishMode != b.PublishMode}} {
		if f.changed {
			d[f.k] = p.PropertyDiff{Kind: p.Update}
		}
	}
	return infer.DiffResponse{HasChanges: len(d) > 0, DetailedDiff: d}, nil
}
func (r Port) WireDependencies(f infer.FieldSelector, args *PortArgs, state *PortState) {
	f.OutputField(&state.PortID).DependsOn(f.InputField(&args.ApplicationID), f.InputField(&args.PublishedPort), f.InputField(&args.TargetPort), f.InputField(&args.Protocol), f.InputField(&args.PublishMode))
}
