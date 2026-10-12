package middleware

import "github.com/Wei-Shaw/sub2api/internal/application/service"

// Called only after CloneForRequest: preserve binding order while ensuring
// native requests cannot authorize, bill, or fail over through a chat-only group.
func filterSystemOneGroupBindings(apiKey *service.APIKey) {
	if apiKey == nil || len(apiKey.GroupBindings) == 0 {
		return
	}
	bindings := make([]service.APIKeyGroupBinding, 0, len(apiKey.GroupBindings))
	for _, binding := range apiKey.GroupBindings {
		if binding.Group != nil && (binding.Group.Platform == service.PlatformTypeSafe || binding.Group.Platform == service.PlatformComposite) {
			bindings = append(bindings, binding)
		}
	}
	apiKey.GroupBindings = bindings
	apiKey.Group, apiKey.GroupID = nil, nil
}
