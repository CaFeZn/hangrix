package runtime

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/go-chi/chi/v5"

	agentv1 "github.com/hangrix/hangrix/gen/go/hangrix/agent/v1"
	"github.com/hangrix/hangrix/gen/go/hangrix/agent/v1/agentv1connect"
)

// TestConnectTransport_RechecksKeepAliveOnTimeout closes the race behind
// "sleep schedules but the wake exits immediately afterward".
//
// Timeline:
//  1. StreamFrame enters its select with keepAlive=false and a timeout that
//     is about to fire (simulating a long-running turn near idleGrace).
//  2. Before the timeout actually fires, the loop marks the turn active.
//  3. The timeout fires anyway. StreamFrame must re-check the CURRENT
//     keepAlive predicate and continue waiting, rather than EOF using the
//     stale false captured at select-entry time.
//  4. A frame arrives shortly after and must be returned normally.
func TestConnectTransport_RechecksKeepAliveOnTimeout(t *testing.T) {
	t.Parallel()

	tr := &connectTransport{
		stream:      new(connect.ServerStreamForClient[agentv1.StreamInputsResponse]),
		frameQ:      make(chan *agentv1.StreamInputsResponse, 1),
		streamErr:   make(chan error, 1),
		lastFrameAt: time.Now().Add(-idleGrace + 50*time.Millisecond),
	}

	loop := &Loop{}
	tr.SetKeepAlive(loop.holdWakeOpen)

	want := &agentv1.StreamInputsResponse{
		Body: &agentv1.StreamInputsResponse_Event{
			Event: &agentv1.EventFrame{Event: "issue.comment.mentioned"},
		},
	}

	go func() {
		time.Sleep(20 * time.Millisecond)
		loop.turnActive.Store(true)

		time.Sleep(40 * time.Millisecond)
		tr.frameQ <- want
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	got, err := tr.StreamFrame(ctx)
	if err != nil {
		t.Fatalf("StreamFrame() error = %v, want nil", err)
	}
	if got == nil {
		t.Fatal("StreamFrame() returned nil frame")
	}
	event := got.GetEvent()
	if event == nil || event.GetEvent() != "issue.comment.mentioned" {
		t.Fatalf("StreamFrame() event = %#v, want issue.comment.mentioned", got.GetBody())
	}
}

func TestRetryCriticalTransportCall_RetriesTransientConnectErrors(t *testing.T) {
	t.Parallel()

	attempts := 0
	err := retryCriticalTransportCall(context.Background(), func(context.Context) error {
		attempts++
		if attempts < 3 {
			return connect.NewError(connect.CodeInternal, errors.New("database system is in recovery mode (SQLSTATE 57P03)"))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("retryCriticalTransportCall() error = %v, want nil", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}

func TestRetryCriticalTransportCall_DoesNotRetryInvalidArgument(t *testing.T) {
	t.Parallel()

	attempts := 0
	wantErr := connect.NewError(connect.CodeInvalidArgument, errors.New("bad request"))
	err := retryCriticalTransportCall(context.Background(), func(context.Context) error {
		attempts++
		return wantErr
	})
	if err == nil {
		t.Fatal("retryCriticalTransportCall() error = nil, want non-nil")
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("code = %v, want %v", connect.CodeOf(err), connect.CodeInvalidArgument)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

type flakyStreamHandler struct {
	agentv1connect.UnimplementedAgentServiceHandler
	streamCalls int
}

func (h *flakyStreamHandler) FetchHistory(context.Context, *connect.Request[agentv1.FetchHistoryRequest]) (*connect.Response[agentv1.FetchHistoryResponse], error) {
	return connect.NewResponse(&agentv1.FetchHistoryResponse{}), nil
}

func (h *flakyStreamHandler) AppendMessage(context.Context, *connect.Request[agentv1.AppendMessageRequest]) (*connect.Response[agentv1.AppendMessageResponse], error) {
	return connect.NewResponse(&agentv1.AppendMessageResponse{}), nil
}

func (h *flakyStreamHandler) MarkIdle(context.Context, *connect.Request[agentv1.MarkIdleRequest]) (*connect.Response[agentv1.MarkIdleResponse], error) {
	return connect.NewResponse(&agentv1.MarkIdleResponse{}), nil
}

func (h *flakyStreamHandler) StreamInputs(_ context.Context, _ *connect.Request[agentv1.StreamInputsRequest], stream *connect.ServerStream[agentv1.StreamInputsResponse]) error {
	h.streamCalls++
	if h.streamCalls == 1 {
		return connect.NewError(connect.CodeUnavailable, errors.New("temporary outage"))
	}
	if err := stream.Send(&agentv1.StreamInputsResponse{
		Body: &agentv1.StreamInputsResponse_Event{
			Event: &agentv1.EventFrame{Event: "issue.comment"},
		},
	}); err != nil {
		return err
	}
	time.Sleep(200 * time.Millisecond)
	return nil
}

func TestConnectTransport_ReconnectsRetryableStreamFailure(t *testing.T) {
	t.Parallel()

	handler := &flakyStreamHandler{}
	r := chi.NewRouter()
	path, h := agentv1connect.NewAgentServiceHandler(handler)
	r.Mount(path, h)
	srv := httptest.NewServer(r)
	defer srv.Close()

	tr, err := newConnectTransport(srv.URL, "42", "tok")
	if err != nil {
		t.Fatalf("newConnectTransport() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	got, err := tr.StreamFrame(ctx)
	if err != nil {
		t.Fatalf("StreamFrame() error = %v, want nil", err)
	}
	if got == nil || got.GetEvent() == nil || got.GetEvent().GetEvent() != "issue.comment" {
		t.Fatalf("StreamFrame() = %#v, want issue.comment event", got)
	}
	if handler.streamCalls < 2 {
		t.Fatalf("streamCalls = %d, want at least 2", handler.streamCalls)
	}
}
