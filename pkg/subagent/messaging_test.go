package subagent

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// T061: TestLocalMessageBus_SendRecv - send a message and receive it on subscriber channel
func TestLocalMessageBus_SendRecv(t *testing.T) {
	bus := NewLocalMessageBus()
	defer bus.Close()

	ch := bus.Subscribe("agent-b")

	msg := AgentMessage{
		Type:    "message",
		Content: "hello from agent-a",
	}

	err := bus.Send(context.Background(), "agent-a", "agent-b", msg)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	select {
	case received := <-ch:
		if received.Content != "hello from agent-a" {
			t.Errorf("Content = %q, want %q", received.Content, "hello from agent-a")
		}
		if received.From != "agent-a" {
			t.Errorf("From = %q, want %q", received.From, "agent-a")
		}
		if received.To != "agent-b" {
			t.Errorf("To = %q, want %q", received.To, "agent-b")
		}
		if received.ID == "" {
			t.Error("ID should be auto-generated when empty")
		}
		if received.Timestamp.IsZero() {
			t.Error("Timestamp should be auto-set when zero")
		}
	case <-time.After(time.Second):
		t.Fatal("Timed out waiting for message")
	}
}

// T062: TestLocalMessageBus_Broadcast - broadcast reaches all subscribers except sender
func TestLocalMessageBus_Broadcast(t *testing.T) {
	bus := NewLocalMessageBus()
	defer bus.Close()

	chSender := bus.Subscribe("sender")
	chA := bus.Subscribe("agent-a")
	chB := bus.Subscribe("agent-b")
	chC := bus.Subscribe("agent-c")

	msg := AgentMessage{
		Type:    "message",
		Content: "broadcast message",
	}

	err := bus.Broadcast("sender", msg)
	if err != nil {
		t.Fatalf("Broadcast() error = %v", err)
	}

	// All non-sender subscribers should receive the message
	for _, tc := range []struct {
		name string
		ch   <-chan AgentMessage
	}{
		{"agent-a", chA},
		{"agent-b", chB},
		{"agent-c", chC},
	} {
		select {
		case received := <-tc.ch:
			if received.Content != "broadcast message" {
				t.Errorf("%s: Content = %q, want %q", tc.name, received.Content, "broadcast message")
			}
			if received.From != "sender" {
				t.Errorf("%s: From = %q, want %q", tc.name, received.From, "sender")
			}
			if received.To != tc.name {
				t.Errorf("%s: To = %q, want %q", tc.name, received.To, tc.name)
			}
		case <-time.After(time.Second):
			t.Fatalf("%s: Timed out waiting for broadcast", tc.name)
		}
	}

	// Sender should NOT receive the broadcast
	select {
	case msg := <-chSender:
		t.Errorf("Sender should not receive broadcast, got: %+v", msg)
	case <-time.After(50 * time.Millisecond):
		// Expected: no message for sender
	}
}

// T063: TestLocalMessageBus_Subscribe - subscribing creates a channel, second call returns same channel
func TestLocalMessageBus_Subscribe(t *testing.T) {
	bus := NewLocalMessageBus()
	defer bus.Close()

	ch1 := bus.Subscribe("agent-x")
	if ch1 == nil {
		t.Fatal("Subscribe() returned nil channel")
	}

	ch2 := bus.Subscribe("agent-x")
	if ch2 == nil {
		t.Fatal("Subscribe() second call returned nil channel")
	}

	// Both calls should return the same channel (pointer equality via interface comparison)
	// Send on the underlying and verify both see it
	err := bus.Send(context.Background(), "test", "agent-x", AgentMessage{Content: "test"})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	// ch1 and ch2 should be the same channel, so draining one drains the other
	select {
	case <-ch1:
		// good
	case <-time.After(time.Second):
		t.Fatal("Timed out on ch1")
	}

	// ch2 should have nothing left because it is the same channel
	select {
	case <-ch2:
		t.Error("ch2 should have no messages since ch1 already drained the shared channel")
	case <-time.After(50 * time.Millisecond):
		// Expected: same channel, already drained
	}
}

// T064: TestLocalMessageBus_Unsubscribed_SendFails - sending to non-subscribed agent returns error
func TestLocalMessageBus_Unsubscribed_SendFails(t *testing.T) {
	bus := NewLocalMessageBus()
	defer bus.Close()

	err := bus.Send(context.Background(), "agent-a", "nonexistent", AgentMessage{
		Content: "will fail",
	})
	if err == nil {
		t.Fatal("Send() to non-subscribed agent should return error")
	}
}

// T065: TestLocalMessageBus_Close - close shuts all channels
func TestLocalMessageBus_Close(t *testing.T) {
	bus := NewLocalMessageBus()

	ch := bus.Subscribe("agent-a")

	err := bus.Close()
	if err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Channel should be closed - reading should return zero value immediately
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("Channel should be closed after bus.Close()")
		}
	case <-time.After(time.Second):
		t.Fatal("Timed out; channel should be closed and readable")
	}

	// Send after close should fail
	err = bus.Send(context.Background(), "a", "agent-a", AgentMessage{Content: "post-close"})
	if err == nil {
		t.Error("Send() after Close() should return error")
	}

	// Broadcast after close should fail
	err = bus.Broadcast("a", AgentMessage{Content: "post-close"})
	if err == nil {
		t.Error("Broadcast() after Close() should return error")
	}
}

// T066: TestLocalMessageBus_CloseIdempotent - closing twice doesn't panic
func TestLocalMessageBus_CloseIdempotent(t *testing.T) {
	bus := NewLocalMessageBus()
	bus.Subscribe("agent-a")

	err1 := bus.Close()
	if err1 != nil {
		t.Fatalf("First Close() error = %v", err1)
	}

	// Second close should not panic and should return nil
	err2 := bus.Close()
	if err2 != nil {
		t.Errorf("Second Close() error = %v, want nil", err2)
	}
}

// T067: TestLocalMessageBus_MultipleSubscribers - multiple agents can subscribe and receive independently
func TestLocalMessageBus_MultipleSubscribers(t *testing.T) {
	bus := NewLocalMessageBus()
	defer bus.Close()

	chAlpha := bus.Subscribe("alpha")
	chBeta := bus.Subscribe("beta")
	chGamma := bus.Subscribe("gamma")

	// Send targeted messages to each
	for _, target := range []string{"alpha", "beta", "gamma"} {
		err := bus.Send(context.Background(), "dispatcher", target, AgentMessage{
			Content: fmt.Sprintf("msg-for-%s", target),
		})
		if err != nil {
			t.Fatalf("Send to %s error = %v", target, err)
		}
	}

	// Each should receive only their own message
	for _, tc := range []struct {
		name    string
		ch      <-chan AgentMessage
		content string
	}{
		{"alpha", chAlpha, "msg-for-alpha"},
		{"beta", chBeta, "msg-for-beta"},
		{"gamma", chGamma, "msg-for-gamma"},
	} {
		select {
		case received := <-tc.ch:
			if received.Content != tc.content {
				t.Errorf("%s: Content = %q, want %q", tc.name, received.Content, tc.content)
			}
		case <-time.After(time.Second):
			t.Fatalf("%s: Timed out waiting for message", tc.name)
		}
	}
}

// T068: TestLocalMessageBus_MessageOrdering - messages arrive in order
func TestLocalMessageBus_MessageOrdering(t *testing.T) {
	bus := NewLocalMessageBus()
	defer bus.Close()

	ch := bus.Subscribe("receiver")

	messageCount := 50
	for i := 0; i < messageCount; i++ {
		err := bus.Send(context.Background(), "sender", "receiver", AgentMessage{
			Content: fmt.Sprintf("msg-%03d", i),
		})
		if err != nil {
			t.Fatalf("Send() msg-%03d error = %v", i, err)
		}
	}

	for i := 0; i < messageCount; i++ {
		expected := fmt.Sprintf("msg-%03d", i)
		select {
		case received := <-ch:
			if received.Content != expected {
				t.Fatalf("Message order broken: got %q at position %d, want %q", received.Content, i, expected)
			}
		case <-time.After(time.Second):
			t.Fatalf("Timed out at message %d", i)
		}
	}
}

// T069: TestLocalMessageBus_ConcurrentSafety - concurrent send/recv doesn't race (use -race flag)
func TestLocalMessageBus_ConcurrentSafety(t *testing.T) {
	bus := NewLocalMessageBus()
	defer bus.Close()

	agentCount := 10
	msgsPerAgent := 20

	// Subscribe all agents
	channels := make(map[string]<-chan AgentMessage)
	for i := 0; i < agentCount; i++ {
		id := fmt.Sprintf("agent-%d", i)
		channels[id] = bus.Subscribe(id)
	}

	var wg sync.WaitGroup

	// Concurrent sends from all agents to agent-0
	for i := 1; i < agentCount; i++ {
		wg.Add(1)
		go func(senderIdx int) {
			defer wg.Done()
			from := fmt.Sprintf("agent-%d", senderIdx)
			for j := 0; j < msgsPerAgent; j++ {
				_ = bus.Send(context.Background(), from, "agent-0", AgentMessage{
					Content: fmt.Sprintf("from-%d-msg-%d", senderIdx, j),
				})
			}
		}(i)
	}

	// Concurrent broadcasts
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_ = bus.Broadcast(fmt.Sprintf("agent-%d", idx), AgentMessage{
				Content: fmt.Sprintf("broadcast-%d", idx),
			})
		}(i)
	}

	// Concurrent subscribe calls (idempotent)
	for i := 0; i < agentCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			bus.Subscribe(fmt.Sprintf("agent-%d", idx))
		}(i)
	}

	wg.Wait()

	// Drain agent-0's channel to verify no deadlock
	ch := channels["agent-0"]
	drained := 0
	for {
		select {
		case <-ch:
			drained++
		case <-time.After(100 * time.Millisecond):
			goto done
		}
	}
done:
	if drained == 0 {
		t.Error("Expected agent-0 to receive at least some messages")
	}
}

// T070: TestLocalMessageBus_FullChannel - when channel is full, send doesn't block (drops message)
func TestLocalMessageBus_FullChannel(t *testing.T) {
	bus := NewLocalMessageBus()
	defer bus.Close()

	bus.Subscribe("target")

	// Fill the channel (buffer size is localInboxBufferSize = 100)
	for i := 0; i < localInboxBufferSize; i++ {
		err := bus.Send(context.Background(), "sender", "target", AgentMessage{
			Content: fmt.Sprintf("fill-%d", i),
		})
		if err != nil {
			t.Fatalf("Send() fill-%d error = %v", i, err)
		}
	}

	// This send should NOT block - it should drop the message
	done := make(chan struct{})
	go func() {
		err := bus.Send(context.Background(), "sender", "target", AgentMessage{
			Content: "overflow",
		})
		if err != nil {
			t.Errorf("Send() on full channel should not error, got %v", err)
		}
		close(done)
	}()

	select {
	case <-done:
		// Non-blocking send succeeded (message dropped silently)
	case <-time.After(2 * time.Second):
		t.Fatal("Send() blocked on full channel - should drop message instead")
	}
}
