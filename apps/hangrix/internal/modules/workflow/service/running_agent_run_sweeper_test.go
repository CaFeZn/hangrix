package service

import (
	"testing"
	"time"

	runnerdomain "github.com/hangrix/hangrix/apps/hangrix/internal/modules/runner/domain"
	workflowdomain "github.com/hangrix/hangrix/apps/hangrix/internal/modules/workflow/domain"
)

func TestIsLiveAgentSessionStatus(t *testing.T) {
	t.Parallel()

	live := []runnerdomain.SessionStatus{
		runnerdomain.SessionStatusPending,
		runnerdomain.SessionStatusClaimed,
		runnerdomain.SessionStatusRunning,
	}
	for _, status := range live {
		if !isLiveAgentSessionStatus(status) {
			t.Fatalf("status %q should be treated as live", status)
		}
	}

	notLive := []runnerdomain.SessionStatus{
		runnerdomain.SessionStatusIdle,
		runnerdomain.SessionStatusSucceeded,
		runnerdomain.SessionStatusFailed,
		runnerdomain.SessionStatusCancelled,
		runnerdomain.SessionStatusArchived,
	}
	for _, status := range notLive {
		if isLiveAgentSessionStatus(status) {
			t.Fatalf("status %q should not be treated as live", status)
		}
	}
}

func TestIsStalePendingSessionRun(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	started := now.Add(-30 * time.Minute)
	recent := now.Add(-1 * time.Minute)
	containerID := "agent-container"
	cutoff := now.Add(-10 * time.Minute)

	tests := []struct {
		name string
		sess *runnerdomain.AgentSession
		job  *workflowdomain.WorkflowJobRun
		want bool
	}{
		{
			name: "pending running job without container past grace",
			sess: &runnerdomain.AgentSession{Status: runnerdomain.SessionStatusPending},
			job:  &workflowdomain.WorkflowJobRun{Status: workflowdomain.JobStatusRunning, StartedAt: &started},
			want: true,
		},
		{
			name: "pending running job with container is real progress",
			sess: &runnerdomain.AgentSession{Status: runnerdomain.SessionStatusPending},
			job:  &workflowdomain.WorkflowJobRun{Status: workflowdomain.JobStatusRunning, StartedAt: &started, ContainerID: &containerID},
			want: false,
		},
		{
			name: "recent pending run stays protected",
			sess: &runnerdomain.AgentSession{Status: runnerdomain.SessionStatusPending},
			job:  &workflowdomain.WorkflowJobRun{Status: workflowdomain.JobStatusRunning, StartedAt: &recent},
			want: false,
		},
		{
			name: "running session is handled by recovery path",
			sess: &runnerdomain.AgentSession{Status: runnerdomain.SessionStatusRunning},
			job:  &workflowdomain.WorkflowJobRun{Status: workflowdomain.JobStatusRunning, StartedAt: &started},
			want: false,
		},
		{
			name: "pending job is not a stalled running claim",
			sess: &runnerdomain.AgentSession{Status: runnerdomain.SessionStatusPending},
			job:  &workflowdomain.WorkflowJobRun{Status: workflowdomain.JobStatusPending, StartedAt: &started},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := isStalePendingSessionRun(tc.sess, tc.job, cutoff); got != tc.want {
				t.Fatalf("isStalePendingSessionRun() = %v, want %v", got, tc.want)
			}
		})
	}
}
