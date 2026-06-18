package service

import (
	"testing"

	runnerdomain "github.com/hangrix/hangrix/apps/hangrix/internal/modules/runner/domain"
)

func TestRetryableTerminalSession(t *testing.T) {
	cases := []struct {
		name string
		msg  string
		want bool
	}{
		{name: "openai 502", msg: `llm call failed: [group worker#priority=0 attempt] upstream 502: {"error":{"message":"Upstream service temporarily unavailable"}}`, want: true},
		{name: "gateway timeout", msg: "LLM reasoning timeout after 3 attempt(s), threshold=120s", want: true},
		{name: "transport stream open", msg: "connect_transport: open stream: connection refused", want: true},
		{name: "clone killed", msg: `clone repo: clone http://hangrix-zip:8080/git/CaFeZn/hangrix-main-docker.git: git clone --no-checkout ... : signal: killed (Cloning into '/tmp/repo'...)`, want: true},
		{name: "checkout killed", msg: `clone repo: checkout cf88f34cab57c26f5f3b2aa8c50f9a4fec55c1b0: git checkout cf88f34cab57c26f5f3b2aa8c50f9a4fec55c1b0: signal: killed ()`, want: true},
		{name: "context canceled", msg: `context canceled`, want: true},
		{name: "invalid api key", msg: `llm call failed: [group fast#priority=0 attempt] upstream 401: {"error":{"message":"Authentication Fails, Your api key is invalid"}}`, want: false},
		{name: "bad request", msg: `llm call failed: upstream 400: malformed request`, want: false},
		{name: "host yaml", msg: `rewake resolve snapshot failed: host repo .hangrix/agents.yml is invalid: invalid reviewers config`, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := retryableTerminalSession(&runnerdomain.AgentSession{ErrorMessage: tc.msg})
			if got != tc.want {
				t.Fatalf("retryableTerminalSession(%q) = %v, want %v", tc.msg, got, tc.want)
			}
		})
	}
}
