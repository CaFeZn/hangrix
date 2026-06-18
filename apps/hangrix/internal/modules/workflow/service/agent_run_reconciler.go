package service

import (
	"context"
	"log"
	"time"

	automationdomain "github.com/hangrix/hangrix/apps/hangrix/internal/modules/automation/domain"
	platformsettings "github.com/hangrix/hangrix/apps/hangrix/internal/modules/platform_settings/domain"
	runnerdomain "github.com/hangrix/hangrix/apps/hangrix/internal/modules/runner/domain"
	workflowdomain "github.com/hangrix/hangrix/apps/hangrix/internal/modules/workflow/domain"
	"github.com/hangrix/hangrix/apps/hangrix/internal/server"
)

const (
	defaultAgentRunReconcileThreshold = 7 * time.Minute
	agentRunReconcileInterval         = 2 * time.Minute
	maxAgentRunsPerReconcileSweep     = 200
)

// AgentRunReconciler collapses stale `_agent` workflow runs/jobs that are still
// marked running in the DB long after their last observable progress. This
// happens when the runner or API goes away after the container already
// finished, so the terminal callback never lands.
//
// The reconciler is intentionally narrow:
//   - only the hidden `_agent` workflow
//   - only runs currently marked `running`
//   - only when every running job in the run has been quiet past threshold
//
// Once a stale run is detected, we cancel the run. The existing
// AgentRunObserver and RecoverySweeper then reconcile the owning session row.
type AgentRunReconciler struct {
	workflow *Service
	settings platformsettings.Store
	repos    automationdomain.RepoLister
	runner   staleRunningSessionRepo
}

type staleRunningSessionRepo interface {
	ListStaleRunningSessions(ctx context.Context, threshold time.Duration, limit int) ([]*runnerdomain.StaleRunningSession, error)
}

type AgentRunReconcilerDeps struct {
	Workflow   *Service
	Settings   platformsettings.Store
	RepoLister automationdomain.RepoLister
	Runner     runnerdomain.Repo
}

func NewAgentRunReconciler(deps *AgentRunReconcilerDeps) *AgentRunReconciler {
	return &AgentRunReconciler{
		workflow: deps.Workflow,
		settings: deps.Settings,
		repos:    deps.RepoLister,
		runner:   deps.Runner,
	}
}

func (r *AgentRunReconciler) Start(ctx context.Context) {
	r.sweepOnce(ctx)
	t := time.NewTicker(agentRunReconcileInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.sweepOnce(ctx)
		}
	}
}

func (r *AgentRunReconciler) sweepOnce(ctx context.Context) {
	if r.workflow == nil || r.repos == nil {
		return
	}
	threshold := defaultAgentRunReconcileThreshold
	if r.settings != nil {
		if d, err := r.settings.GetDuration(ctx, "lifecycle.agent_run_reconcile_threshold"); err == nil && d > 0 {
			threshold = d
		}
	}

	repos, err := r.repos.ListAll(ctx)
	if err != nil {
		log.Printf("agent_run_reconciler: list repos: %v", err)
		return
	}
	now := time.Now()
	for _, repo := range repos {
		runs, _, err := r.workflow.store.ListAgentRunsByRepo(ctx, repo.ID, string(workflowdomain.RunStatusRunning), 0, maxAgentRunsPerReconcileSweep)
		if err != nil {
			log.Printf("agent_run_reconciler: repo=%d list running _agent runs: %v", repo.ID, err)
			continue
		}
		for _, run := range runs {
			if run == nil || run.Status != workflowdomain.RunStatusRunning {
				continue
			}
			if err := r.reconcileRun(ctx, now, run, threshold); err != nil {
				log.Printf("agent_run_reconciler: run %d: %v", run.ID, err)
			}
		}
	}
}

func (r *AgentRunReconciler) reconcileRun(ctx context.Context, now time.Time, run *workflowdomain.WorkflowRun, threshold time.Duration) error {
	jobs, err := r.workflow.store.ListJobRunsByRun(ctx, run.ID)
	if err != nil || len(jobs) == 0 {
		return err
	}

	hasRunningJob := false
	latest := run.CreatedAt
	if run.StartedAt != nil && run.StartedAt.After(latest) {
		latest = *run.StartedAt
	}
	sessionID := int64(0)

	for _, job := range jobs {
		if job == nil {
			continue
		}
		if job.Status == workflowdomain.JobStatusRunning {
			hasRunningJob = true
		}
		if sessionID == 0 {
			if id, ok := extractPendingAgentSessionID(job.StepsJSON); ok {
				sessionID = id
			}
		}
		if job.StartedAt != nil && job.StartedAt.After(latest) {
			latest = *job.StartedAt
		}
		if job.FinishedAt != nil && job.FinishedAt.After(latest) {
			latest = *job.FinishedAt
		}
	}
	if !hasRunningJob {
		return nil
	}
	if sessionID != 0 && r.runner != nil {
		staleRows, err := r.runner.ListStaleRunningSessions(ctx, threshold, maxAgentRunsPerReconcileSweep)
		if err != nil {
			return err
		}
		staleForSession := false
		for _, item := range staleRows {
			if item != nil && item.Session != nil && item.Session.ID == sessionID {
				staleForSession = true
				if item.LastActivityAt != nil && item.LastActivityAt.After(latest) {
					latest = *item.LastActivityAt
				}
				break
			}
		}
		if !staleForSession {
			return nil
		}
	}
	if now.Sub(latest) < threshold {
		return nil
	}

	if err := r.workflow.CancelRun(ctx, run.ID); err != nil && err != workflowdomain.ErrInvalidStatus {
		return err
	}
	log.Printf("agent_run_reconciler: cancelled stale _agent run %d ref=%s after %s of no observable progress", run.ID, run.Ref, now.Sub(latest).Round(time.Second))
	return nil
}

var _ server.BackgroundJob = (*AgentRunReconciler)(nil)
