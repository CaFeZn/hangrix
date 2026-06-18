package service

import (
	"context"
	"testing"
	"time"

	runnerdomain "github.com/hangrix/hangrix/apps/hangrix/internal/modules/runner/domain"
	workflowdomain "github.com/hangrix/hangrix/apps/hangrix/internal/modules/workflow/domain"
)

type stubAgentRunLister struct {
	runsByStatus map[string][]*workflowdomain.WorkflowRun
	jobsByRunID  map[int64][]*workflowdomain.WorkflowJobRun
}

func (s *stubAgentRunLister) ListAgentRunsByRepo(_ context.Context, _ int64, status string, _ int32, _ int32) ([]*workflowdomain.WorkflowRun, int64, error) {
	runs := s.runsByStatus[status]
	return runs, int64(len(runs)), nil
}

func (s *stubAgentRunLister) ListJobRuns(_ context.Context, workflowRunID int64) ([]*workflowdomain.WorkflowJobRun, error) {
	return s.jobsByRunID[workflowRunID], nil
}

type stubMessageLister struct {
	bySession map[int64][]*runnerdomain.Message
}

func (s *stubMessageLister) ListMessages(_ context.Context, sessionID int64) ([]*runnerdomain.Message, error) {
	return s.bySession[sessionID], nil
}

func TestHasQueuedAgentRun_FindsPendingIssueRun(t *testing.T) {
	t.Parallel()

	sweeper := &OpenIssueResumeSweeper{
		workflow: &stubAgentRunLister{
			runsByStatus: map[string][]*workflowdomain.WorkflowRun{
				string(workflowdomain.RunStatusPending): {
					{ID: 1, RepoID: 1, WorkflowName: workflowdomain.InternalAgentWorkflowName, Status: workflowdomain.RunStatusPending, Ref: "issue/21"},
				},
			},
			jobsByRunID: map[int64][]*workflowdomain.WorkflowJobRun{
				1: {{ID: 10, WorkflowRunID: 1}},
			},
		},
	}

	queued, err := sweeper.hasQueuedAgentRun(context.Background(), 1, 21)
	if err != nil {
		t.Fatalf("hasQueuedAgentRun() error = %v", err)
	}
	if !queued {
		t.Fatal("hasQueuedAgentRun() = false, want true")
	}
}

func TestHasQueuedAgentRun_IgnoresOtherIssues(t *testing.T) {
	t.Parallel()

	sweeper := &OpenIssueResumeSweeper{
		workflow: &stubAgentRunLister{
			runsByStatus: map[string][]*workflowdomain.WorkflowRun{
				string(workflowdomain.RunStatusPending): {
					{ID: 1, RepoID: 1, WorkflowName: workflowdomain.InternalAgentWorkflowName, Status: workflowdomain.RunStatusPending, Ref: "issue/22"},
				},
				string(workflowdomain.RunStatusRunning): {
					{ID: 2, RepoID: 1, WorkflowName: workflowdomain.InternalAgentWorkflowName, Status: workflowdomain.RunStatusRunning, Ref: "issue/23"},
				},
			},
			jobsByRunID: map[int64][]*workflowdomain.WorkflowJobRun{
				1: {{ID: 10, WorkflowRunID: 1}},
				2: {{ID: 20, WorkflowRunID: 2}},
			},
		},
	}

	queued, err := sweeper.hasQueuedAgentRun(context.Background(), 1, 21)
	if err != nil {
		t.Fatalf("hasQueuedAgentRun() error = %v", err)
	}
	if queued {
		t.Fatal("hasQueuedAgentRun() = true, want false")
	}
}

func TestHasQueuedAgentRun_IgnoresOrphanPendingRunWithoutJobs(t *testing.T) {
	t.Parallel()

	sweeper := &OpenIssueResumeSweeper{
		workflow: &stubAgentRunLister{
			runsByStatus: map[string][]*workflowdomain.WorkflowRun{
				string(workflowdomain.RunStatusPending): {
					{ID: 1, RepoID: 1, WorkflowName: workflowdomain.InternalAgentWorkflowName, Status: workflowdomain.RunStatusPending, Ref: "issue/21"},
				},
			},
			jobsByRunID: map[int64][]*workflowdomain.WorkflowJobRun{},
		},
	}

	queued, err := sweeper.hasQueuedAgentRun(context.Background(), 1, 21)
	if err != nil {
		t.Fatalf("hasQueuedAgentRun() error = %v", err)
	}
	if queued {
		t.Fatal("hasQueuedAgentRun() = true, want false for orphan run")
	}
}

func TestHasQueuedAgentRun_IgnoresRunningRunBackedByIdleSession(t *testing.T) {
	t.Parallel()

	sweeper := &OpenIssueResumeSweeper{
		runner: &stubRunnerRepo{
			sessions: []*runnerdomain.AgentSession{
				{ID: 64, Status: runnerdomain.SessionStatusIdle},
			},
		},
		workflow: &stubAgentRunLister{
			runsByStatus: map[string][]*workflowdomain.WorkflowRun{
				string(workflowdomain.RunStatusRunning): {
					{ID: 1, RepoID: 1, WorkflowName: workflowdomain.InternalAgentWorkflowName, Status: workflowdomain.RunStatusRunning, Ref: "issue/21"},
				},
			},
			jobsByRunID: map[int64][]*workflowdomain.WorkflowJobRun{
				1: {{
					ID:        10,
					WorkflowRunID: 1,
					StepsJSON: []byte(`[{"env":{"HANGRIX_SESSION_ID":"64"}}]`),
				}},
			},
		},
	}

	queued, err := sweeper.hasQueuedAgentRun(context.Background(), 1, 21)
	if err != nil {
		t.Fatalf("hasQueuedAgentRun() error = %v", err)
	}
	if queued {
		t.Fatal("hasQueuedAgentRun() = true, want false for run backed by idle session")
	}
}

func TestSessionHasRecentActivity_FindsRecentMessages(t *testing.T) {
	t.Parallel()

	now := time.Now()
	sweeper := &OpenIssueResumeSweeper{
		messages: &stubMessageLister{bySession: map[int64][]*runnerdomain.Message{
			64: {{ID: 1, SessionID: 64, CreatedAt: now.Add(-10 * time.Second)}},
		}},
	}

	sess := &runnerdomain.AgentSession{
		ID:        64,
		Status:    runnerdomain.SessionStatusRunning,
		CreatedAt: now.Add(-10 * time.Minute),
		StartedAt: ptrTime(now.Add(-10 * time.Minute)),
	}

	if !sweeper.sessionHasRecentActivity(context.Background(), sess, 45*time.Second) {
		t.Fatal("sessionHasRecentActivity() = false, want true")
	}
}
func ptrTime(v time.Time) *time.Time { return &v }
