package chat

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHubBroadcastMessageRecalledReachesUserAndAllAdmins(t *testing.T) {
	hub := NewHub()
	userFrames := make(chan []byte, 1)
	adminFrames := make(chan []byte, 1)
	hub.RegisterUser(42, userFrames)
	hub.RegisterAdmin(adminFrames)
	recalledAt := time.Now().UTC()

	hub.BroadcastMessageRecalled(7, 42, &Message{
		ID: 10, ConversationID: 7, SenderType: SenderTypeAdmin,
		Content: "", Kind: MessageKindText, RecalledAt: &recalledAt,
	})

	for _, frames := range []chan []byte{userFrames, adminFrames} {
		select {
		case payload := <-frames:
			var event outboundEvent
			require.NoError(t, json.Unmarshal(payload, &event))
			require.Equal(t, "message_recalled", event.Type)
			require.NotNil(t, event.Message)
			require.Equal(t, int64(10), event.Message.ID)
			require.Empty(t, event.Message.Content)
		default:
			t.Fatal("expected recalled message frame")
		}
	}
}
