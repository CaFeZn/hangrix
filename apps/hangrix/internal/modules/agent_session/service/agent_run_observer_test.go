package service

import (
	"context"
	"testing"
	"time"

	runnerdomain "github.com/hangrix/hangrix/apps/hangrix/internal/modules/runner/domain"
	workflowdomain "github.com/hangrix/hangrix/apps/hangrix/internal/modules/workflow/domain"
)

type stubAgentRunObserverStore struct {
	runs      []*workflowdomain.WorkflowRun
	jobsByRun map[int64][]*workflowdomain.WorkflowJobRun
}

func (s *stubAgentRunObserverStore) ListJobRunsByRun(_ context.Context, workflowRunID int64) ([]*workflowdomain.WorkflowJobRun, error) {
	return s.jobsByRun[workflowRunID], nil
}

func (s *stubAgentRunObserverStore) ListAgentRunsByRepo(_ context.Context, repoID int64, status string, _ int32, _ int32) ([]*workflowdomain.WorkflowRun, int64, error) {
	var out []*workflowdomain.WorkflowRun
	for _, run := range s.runs {
		if run == nil || run.RepoID != repoID {
			continue
		}
		if status != "" && string(run.Status) != status {
			continue
		}
		out = append(out, run)
	}
	return out, int64(len(out)), nil
}

func TestAgentRunObserver_IgnoresCancelledRunWhenNewerRunExistsForSameSession(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	oldRun := &workflowdomain.WorkflowRun{
		ID:           10,
		RepoID:       1,
		WorkflowName: workflowdomain.InternalAgentWorkflowName,
		Status:       workflowdomain.RunStatusCancelled,
		CreatedAt:    now.Add(-2 * time.Minute),
	}
	newRun := &workflowdomain.WorkflowRun{
		ID:           11,
		RepoID:       1,
		WorkflowName: workflowdomain.InternalAgentWorkflowName,
		Status:       workflowdomain.RunStatusPending,
		CreatedAt:    now.Add(-1 * time.Minute),
	}
	store := &stubAgentRunObserverStore{
		runs: []*workflowdomain.WorkflowRun{newRun, oldRun},
		jobsByRun: map[int64][]*workflowdomain.WorkflowJobRun{
			10: {{
				ID:            100,
				WorkflowRunID: 10,
				Status:        workflowdomain.JobStatusCancelled,
				ErrorMessage:  "internal agent workflow cancelled",
				StepsJSON:     []byte(`[{"env":{"HANGRIX_SESSION_ID":"67"}}]`),
			}},
			11: {{
				ID:            101,
				WorkflowRunID: 11,
				Status:        workflowdomain.JobStatusPending,
				StepsJSON:     []byte(`[{"env":{"HANGRIX_SESSION_ID":"67"}}]`),
			}},
		},
	}
	runner := newStubRunnerRepo()
	runner.sessions = []*runnerdomain.AgentSession{{
		ID:     67,
		Status: runnerdomain.SessionStatusPending,
	}}
	observer := NewAgentRunObserver(&AgentRunObserverDeps{
		Runner: runner,
	})
	observer.runStore = store

	if err := observer.OnRunStatusChanged(context.Background(), workflowdomain.RunStatusRunning, oldRun); err != nil {
		t.Fatalf("OnRunStatusChanged() error = %v", err)
	}
	if runner.sessions[0].Status != runnerdomain.SessionStatusPending {
		t.Fatalf("session status = %q, want pending", runner.sessions[0].Status)
	}
	if runner.sessions[0].ErrorMessage != "" {
		t.Fatalf("session error_message = %q, want empty", runner.sessions[0].ErrorMessage)
	}
}

func TestAgentRunObserver_IgnoresSuccessRunWhenNewerRunExistsForSameSession(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	oldRun := &workflowdomain.WorkflowRun{
		ID:           20,
		RepoID:       1,
		WorkflowName: workflowdomain.InternalAgentWorkflowName,
		Status:       workflowdomain.RunStatusSuccess,
		CreatedAt:    now.Add(-2 * time.Minute),
	}
	newRun := &workflowdomain.WorkflowRun{
		ID:           21,
		RepoID:       1,
		WorkflowName: workflowdomain.InternalAgentWorkflowName,
		Status:       workflowdomain.RunStatusRunning,
		CreatedAt:    now.Add(-1 * time.Minute),
	}
	store := &stubAgentRunObserverStore{
		runs: []*workflowdomain.WorkflowRun{newRun, oldRun},
		jobsByRun: map[int64][]*workflowdomain.WorkflowJobRun{
			20: {{
				ID:            200,
				WorkflowRunID: 20,
				Status:        workflowdomain.JobStatusSuccess,
				StepsJSON:     []byte(`[{"env":{"HANGRIX_SESSION_ID":"88"}}]`),
			}},
			21: {{
				ID:            201,
				WorkflowRunID: 21,
				Status:        workflowdomain.JobStatusRunning,
				StepsJSON:     []byte(`[{"env":{"HANGRIX_SESSION_ID":"88"}}]`),
			}},
		},
	}
	runner := newStubRunnerRepo()
	runner.sessions = []*runnerdomain.AgentSession{{
		ID:     88,
		Status: runnerdomain.SessionStatusRunning,
	}}
	observer := NewAgentRunObserver(&AgentRunObserverDeps{
		Runner: runner,
	})
	observer.runStore = store

	if err := observer.OnRunStatusChanged(context.Background(), workflowdomain.RunStatusRunning, oldRun); err != nil {
		t.Fatalf("OnRunStatusChanged() error = %v", err)
	}
	if runner.sessions[0].Status != runnerdomain.SessionStatusRunning {
		t.Fatalf("session status = %q, want running", runner.sessions[0].Status)
	}
}
