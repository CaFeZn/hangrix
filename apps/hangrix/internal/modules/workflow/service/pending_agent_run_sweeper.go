package service

import (
	"context"
	"encoding/json"
	"log"
	"strconv"
	"time"

	automationdomain "github.com/hangrix/hangrix/apps/hangrix/internal/modules/automation/domain"
	platformsettings "github.com/hangrix/hangrix/apps/hangrix/internal/modules/platform_settings/domain"
	runnerdomain "github.com/hangrix/hangrix/apps/hangrix/internal/modules/runner/domain"
	workflowdomain "github.com/hangrix/hangrix/apps/hangrix/internal/modules/workflow/domain"
	"github.com/hangrix/hangrix/apps/hangrix/internal/server"
)

const (
	defaultPendingAgentRunSweepThreshold = 5 * time.Minute
	pendingAgentRunSweepInterval         = 2 * time.Minute
	maxPendingAgentRunsPerSweep          = 300
)

// PendingAgentRunSweeper cancels stale duplicate pending `_agent` runs that
// belong to a session which has already completed a newer-or-older agent run.
// This is narrowly scoped cleanup for backlog left behind by earlier recovery
// storms: the same session can accumulate multiple pending `_agent` runs, but
// only one future wake is meaningful.
type PendingAgentRunSweeper struct {
	workflow *Service
	settings platformsettings.Store
	repos    automationdomain.RepoLister
	runner   agentRunSessionLookup
}

type PendingAgentRunSweeperDeps struct {
	Workflow   *Service
	Settings   platformsettings.Store
	RepoLister automationdomain.RepoLister
	Runner     runnerdomain.Repo
}

func NewPendingAgentRunSweeper(deps *PendingAgentRunSweeperDeps) *PendingAgentRunSweeper {
	return &PendingAgentRunSweeper{
		workflow: deps.Workflow,
		settings: deps.Settings,
		repos:    deps.RepoLister,
		runner:   deps.Runner,
	}
}

func (r *PendingAgentRunSweeper) Start(ctx context.Context) {
	r.sweepOnce(ctx)
	t := time.NewTicker(pendingAgentRunSweepInterval)
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

func (r *PendingAgentRunSweeper) sweepOnce(ctx context.Context) {
	if r.workflow == nil {
		return
	}
	threshold := defaultPendingAgentRunSweepThreshold
	if r.settings != nil {
		if d, err := r.settings.GetDuration(ctx, "lifecycle.pending_agent_run_sweep_threshold"); err == nil && d > 0 {
			threshold = d
		}
	}

	repos, err := r.repos.ListAll(ctx)
	if err != nil {
		log.Printf("pending_agent_run_sweeper: list repos: %v", err)
		return
	}

	cutoff := time.Now().Add(-threshold)
	for _, repo := range repos {
		runs, _, err := r.workflow.store.ListAgentRunsByRepo(ctx, repo.ID, string(workflowdomain.RunStatusPending), 0, maxPendingAgentRunsPerSweep)
		if err != nil {
			log.Printf("pending_agent_run_sweeper: repo=%d list pending _agent runs: %v", repo.ID, err)
			continue
		}
		seenSessionTerminalAt := map[int64]time.Time{}
		for _, run := range runs {
			if run == nil || run.Status != workflowdomain.RunStatusPending {
				continue
			}
			jobs, err := r.workflow.store.ListJobRunsByRun(ctx, run.ID)
			if err != nil {
				continue
			}
			if len(jobs) == 0 {
				if err := r.workflow.CancelRun(ctx, run.ID); err != nil && err != workflowdomain.ErrInvalidStatus {
					log.Printf("pending_agent_run_sweeper: cancel orphan run %d: %v", run.ID, err)
					continue
				}
				log.Printf("pending_agent_run_sweeper: cancelled orphan pending _agent run %d ref=%s (no job rows)", run.ID, run.Ref)
				continue
			}
			sessionID, ok := extractPendingAgentSessionID(jobs[0].StepsJSON)
			if !ok || sessionID == 0 {
				continue
			}
			sess, err := agentRunSession(ctx, r.runner, sessionID)
			if err != nil {
				if !isMissingAgentRunSession(err) {
					log.Printf("pending_agent_run_sweeper: session %d lookup: %v", sessionID, err)
					continue
				}
				if err := r.workflow.CancelRun(ctx, run.ID); err != nil && err != workflowdomain.ErrInvalidStatus {
					log.Printf("pending_agent_run_sweeper: cancel orphan run %d session=%d: %v", run.ID, sessionID, err)
					continue
				}
				log.Printf("pending_agent_run_sweeper: cancelled orphan pending _agent run %d ref=%s session=%d (session missing)", run.ID, run.Ref, sessionID)
				continue
			}
			if sess != nil && !isLiveAgentSessionStatus(sess.Status) {
				if err := r.workflow.CancelRun(ctx, run.ID); err != nil && err != workflowdomain.ErrInvalidStatus {
					log.Printf("pending_agent_run_sweeper: cancel stale run %d session=%d: %v", run.ID, sessionID, err)
					continue
				}
				log.Printf("pending_agent_run_sweeper: cancelled stale pending _agent run %d ref=%s session=%d session_status=%s", run.ID, run.Ref, sessionID, sess.Status)
				continue
			}
			if run.CreatedAt.After(cutoff) {
				continue
			}
			terminalAt, cached := seenSessionTerminalAt[sessionID]
			if !cached {
				terminalAt = r.latestTerminalAgentRunTime(ctx, repo.ID, sessionID)
				seenSessionTerminalAt[sessionID] = terminalAt
			}
			if terminalAt.IsZero() || !terminalAt.After(run.CreatedAt) {
				continue
			}
			if err := r.workflow.CancelRun(ctx, run.ID); err != nil && err != workflowdomain.ErrInvalidStatus {
				log.Printf("pending_agent_run_sweeper: cancel run %d: %v", run.ID, err)
				continue
			}
			log.Printf("pending_agent_run_sweeper: cancelled stale duplicate pending _agent run %d ref=%s session=%d", run.ID, run.Ref, sessionID)
		}
	}
}

func (r *PendingAgentRunSweeper) latestTerminalAgentRunTime(ctx context.Context, repoID, sessionID int64) time.Time {
	var latest time.Time
	for _, status := range []string{
		string(workflowdomain.RunStatusSuccess),
		string(workflowdomain.RunStatusFailed),
		string(workflowdomain.RunStatusCancelled),
	} {
		runs, _, err := r.workflow.store.ListAgentRunsByRepo(ctx, repoID, status, 0, maxPendingAgentRunsPerSweep)
		if err != nil {
			continue
		}
		for _, run := range runs {
			if run == nil {
				continue
			}
			jobs, err := r.workflow.store.ListJobRunsByRun(ctx, run.ID)
			if err != nil || len(jobs) == 0 {
				continue
			}
			id, ok := extractPendingAgentSessionID(jobs[0].StepsJSON)
			if !ok || id != sessionID {
				continue
			}
			when := run.CreatedAt
			if run.FinishedAt != nil {
				when = *run.FinishedAt
			} else if run.StartedAt != nil {
				when = *run.StartedAt
			}
			if when.After(latest) {
				latest = when
			}
		}
	}
	return latest
}

type pendingWorkflowStepEnv struct {
	Env map[string]string `json:"env"`
}

func extractPendingAgentSessionID(raw []byte) (int64, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var steps []pendingWorkflowStepEnv
	if err := json.Unmarshal(raw, &steps); err != nil {
		return 0, false
	}
	for _, step := range steps {
		if step.Env == nil {
			continue
		}
		if v := step.Env["HANGRIX_SESSION_ID"]; v != "" {
			id, err := strconv.ParseInt(v, 10, 64)
			if err == nil && id > 0 {
				return id, true
			}
		}
	}
	return 0, false
}

var _ server.BackgroundJob = (*PendingAgentRunSweeper)(nil)
