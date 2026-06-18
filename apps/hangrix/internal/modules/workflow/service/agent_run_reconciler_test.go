package service

import (
	"context"
	"testing"
	"time"

	runnerdomain "github.com/hangrix/hangrix/apps/hangrix/internal/modules/runner/domain"
	workflowdomain "github.com/hangrix/hangrix/apps/hangrix/internal/modules/workflow/domain"
)

type stubAgentRunReconcilerStore struct {
	workflowdomain.Store
	runsByStatus map[string][]*workflowdomain.WorkflowRun
	jobsByRunID  map[int64][]*workflowdomain.WorkflowJobRun
	cancelled    []int64
}

func (s *stubAgentRunReconcilerStore) ListAgentRunsByRepo(_ context.Context, _ int64, status string, _ int32, _ int32) ([]*workflowdomain.WorkflowRun, int64, error) {
	runs := s.runsByStatus[status]
	return runs, int64(len(runs)), nil
}

func (s *stubAgentRunReconcilerStore) ListJobRunsByRun(_ context.Context, workflowRunID int64) ([]*workflowdomain.WorkflowJobRun, error) {
	return s.jobsByRunID[workflowRunID], nil
}

func (s *stubAgentRunReconcilerStore) GetRun(_ context.Context, id int64) (*workflowdomain.WorkflowRun, error) {
	for _, runs := range s.runsByStatus {
		for _, run := range runs {
			if run != nil && run.ID == id {
				return run, nil
			}
		}
	}
	return &workflowdomain.WorkflowRun{ID: id, Status: workflowdomain.RunStatusRunning}, nil
}

func (s *stubAgentRunReconcilerStore) CancelRunningJobs(_ context.Context, _ int64) error { return nil }
func (s *stubAgentRunReconcilerStore) SkipRemainingJobs(_ context.Context, _ int64, _ int32) error {
	return nil
}
func (s *stubAgentRunReconcilerStore) MarkRunTerminal(_ context.Context, id int64, _ workflowdomain.RunStatus) error {
	s.cancelled = append(s.cancelled, id)
	return nil
}

type stubStaleRunnerRepo struct {
	stale []*runnerdomain.StaleRunningSession
}

func (s stubStaleRunnerRepo) ListStaleRunningSessions(context.Context, time.Duration, int) ([]*runnerdomain.StaleRunningSession, error) {
	return s.stale, nil
}

func ptrTime(v time.Time) *time.Time { return &v }

func TestAgentRunReconciler_DoesNotCancelWhenSessionIsNotStale(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	run := &workflowdomain.WorkflowRun{
		ID:        100,
		RepoID:    1,
		Ref:       "issue/21",
		Status:    workflowdomain.RunStatusRunning,
		CreatedAt: now.Add(-20 * time.Minute),
		StartedAt: ptrTime(now.Add(-10 * time.Minute)),
	}
	store := &stubAgentRunReconcilerStore{
		runsByStatus: map[string][]*workflowdomain.WorkflowRun{
			string(workflowdomain.RunStatusRunning): {run},
		},
		jobsByRunID: map[int64][]*workflowdomain.WorkflowJobRun{
			100: {{
				ID:            200,
				WorkflowRunID: 100,
				Status:        workflowdomain.JobStatusRunning,
				StartedAt:     ptrTime(now.Add(-10 * time.Minute)),
				StepsJSON:     []byte(`[{"env":{"HANGRIX_SESSION_ID":"69"}}]`),
			}},
		},
	}
	svc := &Service{store: store}
	reconciler := &AgentRunReconciler{
		workflow: svc,
		repos:    nil,
		runner:   stubStaleRunnerRepo{stale: nil},
	}

	if err := reconciler.reconcileRun(context.Background(), now, run, 7*time.Minute); err != nil {
		t.Fatalf("reconcileRun() error = %v", err)
	}
	if len(store.cancelled) != 0 {
		t.Fatalf("cancelled runs = %v, want none", store.cancelled)
	}
}
