package client

import (
	"context"
	"errors"
	"net"
	"strings"
	"time"

	"connectrpc.com/connect"
)

const (
	criticalRPCMaxAttempts = 20
	criticalRPCMaxBackoff  = 5 * time.Second
)

func retryCriticalRPC(ctx context.Context, op func(context.Context) error) error {
	var lastErr error
	for attempt := 0; attempt < criticalRPCMaxAttempts; attempt++ {
		if err := op(ctx); err != nil {
			if !isRetryableRunnerRPCError(err) || ctx.Err() != nil {
				return err
			}
			lastErr = err
			delay := time.Duration(1<<minInt(attempt, 5)) * 250 * time.Millisecond
			if delay > criticalRPCMaxBackoff {
				delay = criticalRPCMaxBackoff
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
			continue
		}
		return nil
	}
	return lastErr
}

func detachedRetryContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	base := context.Background()
	if parent != nil {
		base = context.WithoutCancel(parent)
	}
	if timeout > 0 {
		return context.WithTimeout(base, timeout)
	}
	return context.WithCancel(base)
}

func isRetryableRunnerRPCError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	code := connect.CodeOf(err)
	switch code {
	case connect.CodeUnavailable,
		connect.CodeInternal,
		connect.CodeUnknown,
		connect.CodeResourceExhausted,
		connect.CodeDeadlineExceeded,
		connect.CodeAborted:
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "recovery mode") ||
		strings.Contains(msg, "not yet accepting connections") ||
		strings.Contains(msg, "sqlstate 57p03") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "unexpected eof") ||
		strings.Contains(msg, "timeout")
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
