package client

import (
	"context"
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
)

func TestRetryCriticalRPC_RetriesTransientConnectErrors(t *testing.T) {
	t.Parallel()

	attempts := 0
	err := retryCriticalRPC(context.Background(), func(context.Context) error {
		attempts++
		if attempts < 3 {
			return connect.NewError(connect.CodeInternal, errors.New("database system is in recovery mode (SQLSTATE 57P03)"))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("retryCriticalRPC() error = %v, want nil", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}

func TestRetryCriticalRPC_DoesNotRetryInvalidArgument(t *testing.T) {
	t.Parallel()

	attempts := 0
	wantErr := connect.NewError(connect.CodeInvalidArgument, errors.New("bad request"))
	err := retryCriticalRPC(context.Background(), func(context.Context) error {
		attempts++
		return wantErr
	})
	if err == nil {
		t.Fatal("retryCriticalRPC() error = nil, want non-nil")
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("code = %v, want %v", connect.CodeOf(err), connect.CodeInvalidArgument)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

func TestDetachedRetryContext_IgnoresParentCancellation(t *testing.T) {
	t.Parallel()

	parent, cancelParent := context.WithCancel(context.Background())
	cancelParent()

	ctx, cancel := detachedRetryContext(parent, 50*time.Millisecond)
	defer cancel()

	select {
	case <-ctx.Done():
		t.Fatal("detached retry context should outlive parent cancellation")
	default:
	}
}
