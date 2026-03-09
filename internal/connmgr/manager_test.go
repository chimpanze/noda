package connmgr

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterAndSend(t *testing.T) {
	mgr := NewManager()
	var received []byte

	conn := &Conn{
		ID:       "c1",
		Channel:  "chat.room1",
		Endpoint: "ws-chat",
		SendFn: func(data []byte) error {
			received = data
			return nil
		},
	}

	mgr.Register(conn)
	assert.Equal(t, int64(1), mgr.Count())

	err := mgr.Send(context.Background(), "chat.room1", "hello")
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), received)
}

func TestUnregister_NoDelivery(t *testing.T) {
	mgr := NewManager()
	var called bool

	conn := &Conn{
		ID:      "c1",
		Channel: "ch1",
		SendFn: func(data []byte) error {
			called = true
			return nil
		},
	}

	mgr.Register(conn)
	mgr.Unregister("c1")
	assert.Equal(t, int64(0), mgr.Count())

	mgr.Send(context.Background(), "ch1", "msg")
	assert.False(t, called)
}

func TestWildcard_StarDot(t *testing.T) {
	mgr := NewManager()
	var received1, received2 []byte

	mgr.Register(&Conn{
		ID:      "c1",
		Channel: "user.123",
		SendFn:  func(data []byte) error { received1 = data; return nil },
	})
	mgr.Register(&Conn{
		ID:      "c2",
		Channel: "user.456",
		SendFn:  func(data []byte) error { received2 = data; return nil },
	})

	err := mgr.Send(context.Background(), "user.*", "broadcast")
	require.NoError(t, err)
	assert.Equal(t, []byte("broadcast"), received1)
	assert.Equal(t, []byte("broadcast"), received2)
}

func TestWildcard_Star_MatchesAll(t *testing.T) {
	mgr := NewManager()
	var count atomic.Int32

	for i := 0; i < 5; i++ {
		id := string(rune('a' + i))
		mgr.Register(&Conn{
			ID:      id,
			Channel: "ch." + id,
			SendFn:  func(data []byte) error { count.Add(1); return nil },
		})
	}

	err := mgr.Send(context.Background(), "*", "all")
	require.NoError(t, err)
	assert.Equal(t, int32(5), count.Load())
}

func TestWildcard_NoMatch(t *testing.T) {
	mgr := NewManager()
	var called bool

	mgr.Register(&Conn{
		ID:      "c1",
		Channel: "user.123",
		SendFn:  func(data []byte) error { called = true; return nil },
	})

	mgr.Send(context.Background(), "admin.*", "msg")
	assert.False(t, called)
}

func TestConcurrentRegisterUnregister(t *testing.T) {
	mgr := NewManager()
	var wg sync.WaitGroup

	// Concurrent registers
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := string(rune(i))
			mgr.Register(&Conn{
				ID:      id,
				Channel: "ch",
				SendFn:  func(data []byte) error { return nil },
			})
		}(i)
	}
	wg.Wait()

	assert.Equal(t, int64(100), mgr.Count())

	// Concurrent unregisters
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			mgr.Unregister(string(rune(i)))
		}(i)
	}
	wg.Wait()

	assert.Equal(t, int64(0), mgr.Count())
}

func TestSendSSE(t *testing.T) {
	mgr := NewManager()
	var gotEvent, gotData, gotID string

	mgr.Register(&Conn{
		ID:      "c1",
		Channel: "updates",
		SSEFn: func(event, data, id string) error {
			gotEvent = event
			gotData = data
			gotID = id
			return nil
		},
	})

	err := mgr.SendSSE(context.Background(), "updates", "message", "hello world", "1")
	require.NoError(t, err)
	assert.Equal(t, "message", gotEvent)
	assert.Equal(t, "hello world", gotData)
	assert.Equal(t, "1", gotID)
}

func TestMultipleConnectionsSameChannel(t *testing.T) {
	mgr := NewManager()
	var count atomic.Int32

	for i := 0; i < 3; i++ {
		id := string(rune('a' + i))
		mgr.Register(&Conn{
			ID:      id,
			Channel: "shared",
			SendFn:  func(data []byte) error { count.Add(1); return nil },
		})
	}

	assert.Equal(t, 3, mgr.ChannelCount("shared"))

	mgr.Send(context.Background(), "shared", "msg")
	assert.Equal(t, int32(3), count.Load())

	// Unregister one
	mgr.Unregister("a")
	assert.Equal(t, 2, mgr.ChannelCount("shared"))
}

func TestMatchWildcard(t *testing.T) {
	tests := []struct {
		pattern string
		channel string
		match   bool
	}{
		{"*", "anything", true},
		{"*", "a.b", true}, // * matches everything (global wildcard)
		{"user.*", "user.123", true},
		{"user.*", "user.abc", true},
		{"user.*", "admin.123", false},
		{"user.*", "user.a.b", false},
		{"*.updates", "user.updates", true},
		{"*.updates", "admin.updates", true},
		{"a.b.c", "a.b.c", true},
		{"a.b.c", "a.b.d", false},
		{"a.*.c", "a.b.c", true},
		{"a.*.c", "a.x.c", true},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.channel, func(t *testing.T) {
			assert.Equal(t, tt.match, matchWildcard(tt.pattern, tt.channel))
		})
	}
}

func TestChannelPattern(t *testing.T) {
	tests := []struct {
		pattern string
		params  map[string]string
		userID  string
		expect  string
	}{
		{"doc.{{ request.params.doc_id }}", map[string]string{"doc_id": "abc"}, "", "doc.abc"},
		{"tasks.{{ auth.sub }}", nil, "user123", "tasks.user123"},
		{"room.{room_id}", map[string]string{"room_id": "42"}, "", "room.42"},
		{"static", nil, "", "static"},
	}

	for _, tt := range tests {
		t.Run(tt.expect, func(t *testing.T) {
			result := resolveChannelPattern(tt.pattern, tt.params, tt.userID)
			assert.Equal(t, tt.expect, result)
		})
	}
}

func TestNoClients_NoError(t *testing.T) {
	mgr := NewManager()
	err := mgr.Send(context.Background(), "empty-channel", "msg")
	assert.NoError(t, err)
}
