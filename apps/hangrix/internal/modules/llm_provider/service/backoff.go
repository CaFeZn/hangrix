// Package service holds the business-logic layer for the llm_provider module.
// This file contains the pure-function exponential backoff calculator used
// by the group router's state machine.
package service

import (
	"strings"
	"time"
)

const (
	// backoffBase is the starting backoff duration (1 minute).
	backoffBase = 60 * time.Second
	// backoffCap is the maximum backoff duration (7 days).
	backoffCap = 7 * 24 * time.Hour
	// hardCredentialBackoffMinStep jumps obviously broken direct-provider
	// credentials straight to the capped cooldown so they do not keep getting
	// retried every few minutes.
	hardCredentialBackoffMinStep int32 = 20
)

// NextBackoff computes the next exponential backoff step and the wall-clock
// deadline until which the member should remain auto-disabled.
//
// The formula is: duration = min(cap, base << step), where base is 60 seconds.
// Step 0 → 1 min, step 1 → 2 min, step 2 → 4 min, … cap 7 days.
//
// The returned newStep is always step+1; callers store it regardless of whether
// the cap was hit, so subsequent failures keep the member at the cap.
func NextBackoff(step int32) (newStep int32, until time.Time) {
	newStep = step + 1
	d := backoffBase
	if step > 0 {
		// Guard against overflow: if the shift would exceed the cap or overflow int64,
		// just use the cap directly. backoffBase<<19 ≈ 3.6 days; backoffBase<<20 ≈ 7.3d.
		// Shift past 30 overflows int64; cap before it ever gets there.
		if step < 20 && backoffBase<<step <= backoffCap {
			d = backoffBase << step
		} else {
			d = backoffCap
		}
	}
	return newStep, time.Now().UTC().Add(d)
}

// NextBackoffForFailure computes the cooldown for a concrete failed dispatch.
// For obviously invalid direct-provider credentials we jump straight to the
// capped cooldown. Internal relay/proxy providers are excluded because their
// 401s can still recover via their own internal account failover.
func NextBackoffForFailure(step int32, providerBaseURL string, statusCode int, message string) (newStep int32, until time.Time) {
	if shouldUseHardCredentialBackoff(providerBaseURL, statusCode, message) {
		newStep = step + 1
		if newStep < hardCredentialBackoffMinStep {
			newStep = hardCredentialBackoffMinStep
		}
		return newStep, time.Now().UTC().Add(backoffCap)
	}
	return NextBackoff(step)
}

func shouldUseHardCredentialBackoff(providerBaseURL string, statusCode int, message string) bool {
	return isHardCredentialFailure(statusCode, message) && !isLocalProxyBaseURL(providerBaseURL)
}

func isHardCredentialFailure(statusCode int, message string) bool {
	lower := strings.ToLower(strings.TrimSpace(message))
	if lower == "" {
		return false
	}
	if statusCode != 401 {
		return false
	}
	if strings.Contains(lower, "authentication fails") {
		return true
	}
	if strings.Contains(lower, "invalid api key") {
		return true
	}
	if strings.Contains(lower, "api key") && strings.Contains(lower, "invalid") {
		return true
	}
	return strings.Contains(lower, "authentication_error") &&
		strings.Contains(lower, "invalid_request_error")
}

func isLocalProxyBaseURL(baseURL string) bool {
	lower := strings.ToLower(strings.TrimSpace(baseURL))
	if lower == "" {
		return false
	}
	return strings.Contains(lower, "localhost") ||
		strings.Contains(lower, "127.0.0.1") ||
		strings.Contains(lower, "::1") ||
		strings.Contains(lower, "host.docker.internal")
}

// isRetryableFailure classifies an HTTP status code for group failover.
// 5xx, 429 (rate-limit), specific transport-ish 4xx codes (408/425), and
// provider-level auth/access failures (401/403) trigger failover and backoff
// increment. We intentionally treat 401/403 as retryable at the group-member
// level: for model groups they usually mean a broken upstream credential or a
// suspended upstream account, so keeping the member available just causes the
// router to hammer the same dead target forever.
//
// Note: 429 is currently treated identically to 5xx for backoff purposes.
// A future improvement could use a shorter initial backoff for rate-limit
// responses, which typically clear within 1 minute.
func isRetryableFailure(statusCode int) bool {
	switch statusCode {
	case 401, 403, 408, 425, 429, 500, 502, 503, 504, 529:
		return true
	default:
		return statusCode >= 500
	}
}
