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

func TestPanicShieldMiddleware_Success(t *testing.T) {
	mw := &PanicShieldMiddleware{}
	assert.Equal(t, "worker.timeout", mw.Name())

	handler := mw.Wrap(func(ctx context.Context) error {
		return nil
	}, testMsgCtx())

	err := handler(context.Background())
	assert.NoError(t, err)
}

// #285: the middleware no longer owns a timeout — processMessage's outer
// context is the single deadline owner. The shield's select still honors
// ctx.Done(), so a deadline set by the caller still surfaces as an error.
func TestPanicShieldMiddleware_HonorsCallerDeadline(t *testing.T) {
	mw := &PanicShieldMiddleware{}

	handler := mw.Wrap(func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
			return nil
		}
	}, testMsgCtx())

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := handler(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
}

func TestPanicShieldMiddleware_NoDeadlineRunsToCompletion(t *testing.T) {
	mw := &PanicShieldMiddleware{}

	called := false
	handler := mw.Wrap(func(ctx context.Context) error {
		called = true
		return nil
	}, testMsgCtx())

	err := handler(context.Background())
	assert.NoError(t, err)
	assert.True(t, called)
}

// execution-1: a panic in the handler must be recovered inside the shield's
// child goroutine and surfaced as an error, not crash the worker process.
func TestPanicShieldMiddleware_RecoversPanicInChildGoroutine(t *testing.T) {
	mw := &PanicShieldMiddleware{}

	handler := mw.Wrap(func(ctx context.Context) error {
		panic("boom")
	}, testMsgCtx())

	err := handler(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "panic")
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
	mws := DefaultMiddleware()
	require.Len(t, mws, 3)
	assert.Equal(t, "worker.recover", mws[0].Name())
	assert.Equal(t, "worker.log", mws[1].Name())
	assert.Equal(t, "worker.timeout", mws[2].Name())
}

func TestResolveMiddleware(t *testing.T) {
	mws := ResolveMiddleware([]string{"worker.log", "worker.recover"})
	require.Len(t, mws, 2)
	assert.Equal(t, "worker.log", mws[0].Name())
	assert.Equal(t, "worker.recover", mws[1].Name())
}

func TestResolveMiddleware_UnknownIgnored(t *testing.T) {
	mws := ResolveMiddleware([]string{"worker.unknown"})
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
