package chat

import "github.com/google/wire"

// ProvideService constructs the Service and wires the Hub in as its
// Broadcaster, mirroring the SetXxx post-construction pattern used elsewhere
// in this codebase (e.g. handler.ProvideAdminSettingHandler).
func ProvideService(conversationRepo ConversationRepository, messageRepo MessageRepository, quickReplyRepo QuickReplyRepository, hub *Hub) *Service {
	svc := NewService(conversationRepo, messageRepo, quickReplyRepo)
	svc.SetBroadcaster(hub)
	return svc
}

// ProviderSet is the Wire provider set for the chat module.
var ProviderSet = wire.NewSet(
	NewHub,
	ProvideService,
)
