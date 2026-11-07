package openai_ws_v2

import (
	"context"
	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
	"sync/atomic"
	"testing"
	"time"
)

func TestRelay_OnTurnComplete_FirstTokenMsRecordedForEveryTurn(t *testing.T) {
	t.Parallel()

	clientConn := newPassthroughTestFrameConn(nil, false)
	upstreamConn := newPassthroughTestFrameConn(nil, false)
	firstTurnAt := time.Date(2026, time.November, 4, 9, 0, 0, 0, time.UTC)
	secondTurnAt := firstTurnAt.Add(time.Minute)
	var clock atomic.Int64
	clock.Store(firstTurnAt.UnixNano())

	turns := make(chan RelayTurnResult, 2)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = Relay(
			ctx,
			clientConn,
			upstreamConn,
			[]byte(`{"type":"response.create","model":"gpt-5.3-codex","input":[]}`),
			RelayOptions{
				Now: func() time.Time { return time.Unix(0, clock.Load()).UTC() },
				OnTurnComplete: func(current RelayTurnResult) {
					turns <- current
				},
			},
		)
	}()

	// sendTurn replays one real OpenAI turn: only response.created and the
	// terminal event carry response.id, the deltas in between do not.
	sendUpstream := func(at time.Time, payload string) {
		forwarded := len(clientConn.Writes())
		clock.Store(at.UnixNano())
		upstreamConn.readCh <- passthroughTestFrame{msgType: coderws.MessageText, payload: []byte(payload)}
		// Wait until the frame is observed and forwarded before moving the clock.
		require.Eventually(t, func() bool { return len(clientConn.Writes()) == forwarded+1 }, time.Second, time.Millisecond)
	}
	sendTurn := func(responseID string, startAt time.Time) RelayTurnResult {
		sendUpstream(startAt.Add(100*time.Millisecond), `{"type":"response.created","response":{"id":"`+responseID+`"}}`)
		sendUpstream(startAt.Add(700*time.Millisecond), `{"type":"response.output_text.delta","delta":"Hi"}`)
		sendUpstream(startAt.Add(900*time.Millisecond), `{"type":"response.output_text.delta","delta":" there"}`)
		sendUpstream(startAt.Add(2*time.Second), `{"type":"response.completed","response":{"id":"`+responseID+`","usage":{"input_tokens":1,"output_tokens":2}}}`)
		select {
		case turn := <-turns:
			return turn
		case <-time.After(time.Second):
			t.Fatalf("turn %s did not complete", responseID)
			return RelayTurnResult{}
		}
	}

	require.Eventually(t, func() bool { return len(upstreamConn.Writes()) == 1 }, time.Second, time.Millisecond)
	firstTurn := sendTurn("resp_first", firstTurnAt)

	clock.Store(secondTurnAt.UnixNano())
	clientConn.readCh <- passthroughTestFrame{
		msgType: coderws.MessageText,
		payload: []byte(`{"type":"response.create","model":"gpt-5.3-codex","input":[]}`),
	}
	require.Eventually(t, func() bool { return len(upstreamConn.Writes()) == 2 }, time.Second, time.Millisecond)
	secondTurn := sendTurn("resp_second", secondTurnAt)

	for _, turn := range []RelayTurnResult{firstTurn, secondTurn} {
		require.NotNil(t, turn.FirstTokenMs, "turn %s must record its own first token", turn.RequestID)
		require.Equal(t, 600, *turn.FirstTokenMs, "turn %s first token is measured from its own start", turn.RequestID)
		require.Equal(t, 1900*time.Millisecond, turn.Duration)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("relay did not stop after cancellation")
	}
}
