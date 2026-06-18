package service

import (
	"context"
	"errors"
	"log"
	"time"

	automationdomain "github.com/hangrix/hangrix/apps/hangrix/internal/modules/automation/domain"
	platformsettings "github.com/hangrix/hangrix/apps/hangrix/internal/modules/platform_settings/domain"
	runnerdomain "github.com/hangrix/hangrix/apps/hangrix/internal/modules/runner/domain"
	workflowdomain "github.com/hangrix/hangrix/apps/hangrix/internal/modules/workflow/domain"
	"github.com/hangrix/hangrix/apps/hangrix/internal/server"
)

const (
	defaultRunningAgentRunSweepThreshold = 2 * time.Minute
	defaultPendingSessionRunGrace        = 10 * time.Minute
	runningAgentRunSweepInterval         = 2 * time.Minute
	maxRunningAgentRunsPerSweep          = 300
)

// RunningAgentRunSweeper collapses duplicate running `_agent` runs belonging
// to the same session. It keeps the newest running run for a session and
// cancels older still-running duplicates after a short grace period.
type RunningAgentRunSweeper struct {
	workflow *Service
	settings platformsettings.Store
	repos    automationdomain.RepoLister
	runner   agentRunSessionLookup
}

type RunningAgentRunSweeperDeps struct {
	Workflow   *Service
	Settings   platformsettings.Store
	RepoLister automationdomain.RepoLister
	Runner     runnerdomain.Repo
}

func NewRunningAgentRunSweeper(deps *RunningAgentRunSweeperDeps) *RunningAgentRunSweeper {
	return &RunningAgentRunSweeper{
		workflow: deps.Workflow,
		settings: deps.Settings,
		repos:    deps.RepoLister,
		runner:   deps.Runner,
	}
}

func (r *RunningAgentRunSweeper) Start(ctx context.Context) {
	r.sweepOnce(ctx)
	t := time.NewTicker(runningAgentRunSweepInterval)
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

func (r *RunningAgentRunSweeper) sweepOnce(ctx context.Context) {
	if r.workflow == nil || r.repos == nil {
		return
	}
	threshold := defaultRunningAgentRunSweepThreshold
	if r.settings != nil {
		if d, err := r.settings.GetDuration(ctx, "lifecycle.running_agent_run_sweep_threshold"); err == nil && d > 0 {
			threshold = d
		}
	}

	repos, err := r.repos.ListAll(ctx)
	if err != nil {
		log.Printf("running_agent_run_sweeper: list repos: %v", err)
		return
	}
	cutoff := time.Now().Add(-threshold)
	pendingSessionCutoff := time.Now().Add(-maxDuration(threshold, defaultPendingSessionRunGrace))
	for _, repo := range repos {
		runs, _, err := r.workflow.store.ListAgentRunsByRepo(ctx, repo.ID, string(workflowdomain.RunStatusRunning), 0, maxRunningAgentRunsPerSweep)
		if err != nil {
			log.Printf("running_agent_run_sweeper: repo=%d list running _agent runs: %v", repo.ID, err)
			continue
		}
		type runningRef struct {
			run *workflowdomain.WorkflowRun
		}
		bySession := map[int64][]runningRef{}
		for _, run := range runs {
			if run == nil || run.CreatedAt.After(cutoff) {
				continue
			}
			jobs, err := r.workflow.store.ListJobRunsByRun(ctx, run.ID)
			if err != nil || len(jobs) == 0 {
				continue
			}
			sessionID, ok := extractPendingAgentSessionID(jobs[0].StepsJSON)
			if !ok || sessionID == 0 {
				continue
			}
			sess, err := agentRunSession(ctx, r.runner, sessionID)
			if err != nil {
				if !isMissingAgentRunSession(err) {
					log.Printf("running_agent_run_sweeper: session %d lookup: %v", sessionID, err)
					continue
				}
				if err := r.workflow.CancelRun(ctx, run.ID); err != nil && err != workflowdomain.ErrInvalidStatus {
					log.Printf("running_agent_run_sweeper: cancel orphan run %d session=%d: %v", run.ID, sessionID, err)
				} else {
					log.Printf("running_agent_run_sweeper: cancelled orphan running _agent run %d ref=%s session=%d (session missing)", run.ID, run.Ref, sessionID)
				}
				continue
			}
			if sess != nil && !isLiveAgentSessionStatus(sess.Status) {
				if err := r.workflow.CancelRun(ctx, run.ID); err != nil && err != workflowdomain.ErrInvalidStatus {
					log.Printf("running_agent_run_sweeper: cancel stale run %d session=%d: %v", run.ID, sessionID, err)
				} else {
					log.Printf("running_agent_run_sweeper: cancelled stale running _agent run %d ref=%s session=%d session_status=%s", run.ID, run.Ref, sessionID, sess.Status)
				}
				continue
			}
			if sess != nil && isStalePendingSessionRun(sess, jobs[0], pendingSessionCutoff) {
				if err := r.workflow.CancelRun(ctx, run.ID); err != nil && err != workflowdomain.ErrInvalidStatus {
					log.Printf("running_agent_run_sweeper: cancel stalled pending-session run %d session=%d: %v", run.ID, sessionID, err)
				} else {
					log.Printf("running_agent_run_sweeper: cancelled stalled running _agent run %d ref=%s session=%d (session still pending; no container after grace)", run.ID, run.Ref, sessionID)
				}
				continue
			}
			bySession[sessionID] = append(bySession[sessionID], runningRef{run: run})
		}
		for sessionID, refs := range bySession {
			latestIdx := 0
			for i := 1; i < len(refs); i++ {
				if refs[i].run.CreatedAt.After(refs[latestIdx].run.CreatedAt) {
					latestIdx = i
				}
			}
			for i, ref := range refs {
				if i == latestIdx {
					continue
				}
				if err := r.workflow.CancelRun(ctx, ref.run.ID); err != nil && err != workflowdomain.ErrInvalidStatus {
					log.Printf("running_agent_run_sweeper: cancel run %d session=%d: %v", ref.run.ID, sessionID, err)
					continue
				}
				log.Printf("running_agent_run_sweeper: cancelled duplicate running _agent run %d ref=%s session=%d", ref.run.ID, ref.run.Ref, sessionID)
			}
		}
	}
}

type agentRunSessionLookup interface {
	GetSessionByID(ctx context.Context, id int64) (*runnerdomain.AgentSession, error)
}

func agentRunSession(ctx context.Context, runner agentRunSessionLookup, sessionID int64) (*runnerdomain.AgentSession, error) {
	if runner == nil || sessionID == 0 {
		return nil, nil
	}
	return runner.GetSessionByID(ctx, sessionID)
}

func isMissingAgentRunSession(err error) bool {
	return errors.Is(err, runnerdomain.ErrSessionNotFound)
}

func isLiveAgentSessionStatus(status runnerdomain.SessionStatus) bool {
	return status == runnerdomain.SessionStatusPending ||
		status == runnerdomain.SessionStatusClaimed ||
		status == runnerdomain.SessionStatusRunning
}

func isStalePendingSessionRun(sess *runnerdomain.AgentSession, job *workflowdomain.WorkflowJobRun, cutoff time.Time) bool {
	if sess == nil || job == nil {
		return false
	}
	if sess.Status != runnerdomain.SessionStatusPending {
		return false
	}
	if job.Status != workflowdomain.JobStatusRunning || job.StartedAt == nil || job.StartedAt.After(cutoff) {
		return false
	}
	return job.ContainerID == nil || *job.ContainerID == ""
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

var _ server.BackgroundJob = (*RunningAgentRunSweeper)(nil)
