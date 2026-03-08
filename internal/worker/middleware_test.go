package worker

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testMsgCtx() *MessageContext {
	return &MessageContext{
		WorkerID:  "test-worker",
		MessageID: "msg-1",
		TraceID:   "trace-1",
		Topic:     "test-topic",
		Group:     "test-group",
		Logger:    slog.Default(),
	}
}

func TestLogMiddleware_Success(t *testing.T) {
	mw := &LogMiddleware{}
	assert.Equal(t, "worker.log", mw.Name())

	called := false
	handler := mw.Wrap(func(ctx context.Context) error {
		called = true
		return nil
	}, testMsgCtx())

	err := handler(context.Background())
	assert.NoError(t, err)
	assert.True(t, called)
}

func TestLogMiddleware_Error(t *testing.T) {
	mw := &LogMiddleware{}

	handler := mw.Wrap(func(ctx context.Context) error {
		return fmt.Errorf("workflow failed")
	}, testMsgCtx())

	err := handler(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "workflow failed")
}

func TestTimeoutMiddleware_Success(t *testing.T) {
	mw := &TimeoutMiddleware{Timeout: 5 * time.Second}
	assert.Equal(t, "worker.timeout", mw.Name())

	handler := mw.Wrap(func(ctx context.Context) error {
		return nil
	}, testMsgCtx())

	err := handler(context.Background())
	assert.NoError(t, err)
}

func TestTimeoutMiddleware_Timeout(t *testing.T) {
	mw := &TimeoutMiddleware{Timeout: 50 * time.Millisecond}

	handler := mw.Wrap(func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
			return nil
		}
	}, testMsgCtx())

	err := handler(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
}

func TestTimeoutMiddleware_DefaultTimeout(t *testing.T) {
	mw := &TimeoutMiddleware{} // 0 = 30s default

	handler := mw.Wrap(func(ctx context.Context) error {
		deadline, ok := ctx.Deadline()
		assert.True(t, ok)
		assert.WithinDuration(t, time.Now().Add(30*time.Second), deadline, 2*time.Second)
		return nil
	}, testMsgCtx())

	err := handler(context.Background())
	assert.NoError(t, err)
}

func TestRecoverMiddleware_NoPanic(t *testing.T) {
	mw := &RecoverMiddleware{}
	assert.Equal(t, "worker.recover", mw.Name())

	handler := mw.Wrap(func(ctx context.Context) error {
		return nil
	}, testMsgCtx())

	err := handler(context.Background())
	assert.NoError(t, err)
}

func TestRecoverMiddleware_CatchesPanic(t *testing.T) {
	mw := &RecoverMiddleware{}

	handler := mw.Wrap(func(ctx context.Context) error {
		panic("something went wrong")
	}, testMsgCtx())

	err := handler(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "panic")
	assert.Contains(t, err.Error(), "something went wrong")
}

func TestDefaultMiddleware(t *testing.T) {
	mws := DefaultMiddleware(10 * time.Second)
	require.Len(t, mws, 3)
	assert.Equal(t, "worker.recover", mws[0].Name())
	assert.Equal(t, "worker.log", mws[1].Name())
	assert.Equal(t, "worker.timeout", mws[2].Name())
}

func TestResolveMiddleware(t *testing.T) {
	mws := ResolveMiddleware([]string{"worker.log", "worker.recover"}, 5*time.Second)
	require.Len(t, mws, 2)
	assert.Equal(t, "worker.log", mws[0].Name())
	assert.Equal(t, "worker.recover", mws[1].Name())
}

func TestResolveMiddleware_UnknownIgnored(t *testing.T) {
	mws := ResolveMiddleware([]string{"worker.unknown"}, 0)
	assert.Empty(t, mws)
}

func TestMiddlewareChaining(t *testing.T) {
	var order []string

	mw1 := &orderMiddleware{name: "first", order: &order}
	mw2 := &orderMiddleware{name: "second", order: &order}

	msgCtx := testMsgCtx()

	handler := func(ctx context.Context) error {
		order = append(order, "handler")
		return nil
	}

	// Apply in reverse (like the runtime does)
	wrapped := mw2.Wrap(handler, msgCtx)
	wrapped = mw1.Wrap(wrapped, msgCtx)

	err := wrapped(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, []string{"first-before", "second-before", "handler", "second-after", "first-after"}, order)
}

type orderMiddleware struct {
	name  string
	order *[]string
}

func (m *orderMiddleware) Name() string { return m.name }

func (m *orderMiddleware) Wrap(next Handler, _ *MessageContext) Handler {
	return func(ctx context.Context) error {
		*m.order = append(*m.order, m.name+"-before")
		err := next(ctx)
		*m.order = append(*m.order, m.name+"-after")
		return err
	}
}
