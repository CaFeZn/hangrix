package service

import (
	"testing"
	"time"
)

const tol = 2 * time.Second

func TestNextBackoff(t *testing.T) {
	cap7d := 7 * 24 * time.Hour

	tests := []struct {
		step    int32
		wantMin time.Duration
	}{
		{0, 60 * time.Second},
		{1, 120 * time.Second},
		{2, 240 * time.Second},
		{3, 480 * time.Second},
		{4, 960 * time.Second},
		{5, 1920 * time.Second},
		{6, 3840 * time.Second},
		{7, 7680 * time.Second},
		{8, 15360 * time.Second},
		{9, 30720 * time.Second},
		{10, 61440 * time.Second},
		{15, cap7d},
		{20, cap7d},
		{100, cap7d},
	}

	for _, tt := range tests {
		newStep, until := NextBackoff(tt.step)
		d := time.Until(until)

		if newStep != tt.step+1 {
			t.Errorf("NextBackoff(%d) step = %d, want %d", tt.step, newStep, tt.step+1)
		}
		if d < tt.wantMin-tol || d > tt.wantMin+tol {
			t.Errorf("NextBackoff(%d) duration = %v, want ~%v (±%v)", tt.step, d, tt.wantMin, tol)
		}
	}
}

func TestNextBackoffStepIncrements(t *testing.T) {
	for step := int32(0); step < 50; step++ {
		newStep, _ := NextBackoff(step)
		if newStep != step+1 {
			t.Errorf("NextBackoff(%d) step = %d, want %d", step, newStep, step+1)
		}
	}
}

func TestNextBackoffCappedAtSevenDays(t *testing.T) {
	cap7d := 7 * 24 * time.Hour
	for step := int32(15); step < 100; step++ {
		_, until := NextBackoff(step)
		d := time.Until(until)
		if d < cap7d-tol {
			t.Errorf("NextBackoff(%d) duration = %v, want >= 7d", step, d)
		}
		if d > cap7d+tol {
			t.Errorf("NextBackoff(%d) duration = %v, want <= 7d+tol", step, d)
		}
	}
}

func TestIsRetryableFailure(t *testing.T) {
	tests := []struct {
		code     int
		expected bool
	}{
		{401, true}, {403, true}, {408, true}, {425, true}, {429, true},
		{500, true}, {502, true}, {503, true}, {504, true}, {529, true},
		{599, true},
		{400, false}, {404, false}, {422, false},
		{0, false}, {200, false},
	}

	for _, tt := range tests {
		got := isRetryableFailure(tt.code)
		if got != tt.expected {
			t.Errorf("isRetryableFailure(%d) = %v, want %v", tt.code, got, tt.expected)
		}
	}
}

func TestNextBackoffForFailure_UsesHardCooldownForInvalidDirectCredential(t *testing.T) {
	newStep, until := NextBackoffForFailure(
		0,
		"https://api.deepseek.com/v1",
		401,
		`upstream 401: {"error":{"message":"Authentication Fails, Your api key is invalid","type":"authentication_error","code":"invalid_request_error"}}`,
	)

	if newStep < hardCredentialBackoffMinStep {
		t.Fatalf("newStep = %d, want >= %d", newStep, hardCredentialBackoffMinStep)
	}

	d := time.Until(until)
	cap7d := 7 * 24 * time.Hour
	if d < cap7d-tol {
		t.Fatalf("duration = %v, want >= 7d", d)
	}
	if d > cap7d+tol {
		t.Fatalf("duration = %v, want <= 7d+tol", d)
	}
}

func TestNextBackoffForFailure_KeepsNormalCooldownForLocalProxy401(t *testing.T) {
	newStep, until := NextBackoffForFailure(
		0,
		"http://host.docker.internal:18080",
		401,
		`upstream 401: {"error":{"message":"Authentication Fails, Your api key is invalid","type":"authentication_error","code":"invalid_request_error"}}`,
	)

	if newStep != 1 {
		t.Fatalf("newStep = %d, want 1", newStep)
	}

	d := time.Until(until)
	want := 60 * time.Second
	if d < want-tol || d > want+tol {
		t.Fatalf("duration = %v, want ~%v", d, want)
	}
}
