package runtime

import (
	"context"
	"errors"
	"net"
	"strings"
	"time"

	"connectrpc.com/connect"
)

const (
	transportCriticalMaxAttempts = 20
	transportCriticalMaxBackoff  = 5 * time.Second
)

func retryCriticalTransportCall(ctx context.Context, op func(context.Context) error) error {
	var lastErr error
	for attempt := 0; attempt < transportCriticalMaxAttempts; attempt++ {
		if err := op(ctx); err != nil {
			if !isRetryableTransportError(err) || ctx.Err() != nil {
				return err
			}
			lastErr = err
			delay := time.Duration(1<<minInt(attempt, 5)) * 250 * time.Millisecond
			if delay > transportCriticalMaxBackoff {
				delay = transportCriticalMaxBackoff
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

func isRetryableTransportError(err error) bool {
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
	switch connect.CodeOf(err) {
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
