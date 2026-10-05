package dokploy

import "github.com/pulumi/pulumi/sdk/v3/go/property"

func gitLabIntegrationIDIsComputed(inputs property.Map) bool {
	source := inputs.Get("source")
	if source.IsComputed() || !source.IsMap() {
		return false
	}
	gitlab := source.AsMap().Get("gitlab")
	if gitlab.IsComputed() || !gitlab.IsMap() {
		return false
	}
	return gitlab.AsMap().Get("integrationId").IsComputed()
}
