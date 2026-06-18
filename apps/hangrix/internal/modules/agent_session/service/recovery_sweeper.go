package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	agentsessiondomain "github.com/hangrix/hangrix/apps/hangrix/internal/modules/agent_session/domain"
	platformsettings "github.com/hangrix/hangrix/apps/hangrix/internal/modules/platform_settings/domain"
	runnerdomain "github.com/hangrix/hangrix/apps/hangrix/internal/modules/runner/domain"
	workflowdomain "github.com/hangrix/hangrix/apps/hangrix/internal/modules/workflow/domain"
	workflowsvc "github.com/hangrix/hangrix/apps/hangrix/internal/modules/workflow/service"
	"github.com/hangrix/hangrix/apps/hangrix/internal/server"
)

const (
	defaultStaleRunningRecoveryThreshold = 5 * time.Minute
	recoverySweepInterval                = 5 * time.Minute
	maxRecoveredPerSweep                 = 20
)

// RecoverySweeper repairs zombie agent sessions / _agent workflow runs that
// stayed `running` after the underlying container already died or the runner /
// DB lost the terminal callback. It cancels the stale workflow run (so the run
// status converges), then resumes the same session row so the runner picks it
// up again.
type RecoverySweeper struct {
	runner     runnerdomain.Repo
	controller agentsessiondomain.Controller
	workflow   *workflowsvc.Service
	settings   platformsettings.Store
}

type RecoverySweeperDeps struct {
	Runner     runnerdomain.Repo
	Controller agentsessiondomain.Controller
	Workflow   *workflowsvc.Service
	Settings   platformsettings.Store
}

func NewRecoverySweeper(deps *RecoverySweeperDeps) *RecoverySweeper {
	return &RecoverySweeper{
		runner:     deps.Runner,
		controller: deps.Controller,
		workflow:   deps.Workflow,
		settings:   deps.Settings,
	}
}

func (r *RecoverySweeper) Start(ctx context.Context) {
	r.sweepOnce(ctx)
	t := time.NewTicker(recoverySweepInterval)
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

func (r *RecoverySweeper) sweepOnce(ctx context.Context) {
	threshold := defaultStaleRunningRecoveryThreshold
	if r.settings != nil {
		if d, err := r.settings.GetDuration(ctx, "lifecycle.stale_running_recovery_threshold"); err == nil && d > 0 {
			threshold = d
		}
	}

	stale, err := r.runner.ListStaleRunningSessions(ctx, threshold, maxRecoveredPerSweep)
	if err != nil {
		log.Printf("recovery_sweeper: list stale running sessions: %v", err)
		return
	}
	for _, item := range stale {
		if item == nil || item.Session == nil {
			continue
		}
		if err := r.recoverSession(ctx, item); err != nil {
			log.Printf("recovery_sweeper: recover session %d: %v", item.Session.ID, err)
		}
	}
}

func (r *RecoverySweeper) recoverSession(ctx context.Context, stale *runnerdomain.StaleRunningSession) error {
	sess := stale.Session
	if sess == nil {
		return nil
	}

	if run, err := r.findRunningAgentRunForSession(ctx, sess); err == nil && run != nil {
		if err := r.workflow.CancelRun(ctx, run.ID); err != nil && !errors.Is(err, workflowdomain.ErrInvalidStatus) && !errors.Is(err, workflowdomain.ErrRunNotFound) {
			return err
		}
	} else if err != nil {
		return err
	}

	// If the workflow run was already gone or couldn't be mapped, forcibly
	// terminalize the session row before rewaking it.
	if curr, err := r.runner.GetSessionByID(ctx, sess.ID); err == nil && curr.Status == runnerdomain.SessionStatusRunning {
		if err := r.controller.Stop(ctx, sess.ID, "auto-recovered stale running session"); err != nil && !errors.Is(err, agentsessiondomain.ErrNotResumable) {
			return err
		}
	}

	if err := r.controller.Recover(ctx, sess.ID, "recovery-sweeper"); err != nil && !errors.Is(err, agentsessiondomain.ErrNotResumable) {
		return err
	}
	log.Printf("recovery_sweeper: resumed stale session %d (repo=%d issue=%d role=%s)", sess.ID, derefInt64(sess.RepoID), derefInt32(sess.IssueNumber), sess.RoleKey)
	return nil
}

func (r *RecoverySweeper) findRunningAgentRunForSession(ctx context.Context, sess *runnerdomain.AgentSession) (*workflowdomain.WorkflowRun, error) {
	if sess == nil || sess.RepoID == nil || sess.IssueNumber == nil || r.workflow == nil {
		return nil, nil
	}
	runs, _, err := r.workflow.ListAgentRunsByRepo(ctx, *sess.RepoID, string(workflowdomain.RunStatusRunning), 0, 200)
	if err != nil {
		return nil, err
	}
	wantRef := fmt.Sprintf("issue/%d", *sess.IssueNumber)
	for _, run := range runs {
		if run == nil || run.Ref != wantRef || run.Status != workflowdomain.RunStatusRunning {
			continue
		}
		jobs, err := r.workflow.ListJobRuns(ctx, run.ID)
		if err != nil {
			return nil, err
		}
		for _, job := range jobs {
			if job == nil || job.Status != workflowdomain.JobStatusRunning {
				continue
			}
			id, ok := extractAgentSessionID(job.StepsJSON)
			if ok && id == sess.ID {
				return run, nil
			}
		}
	}
	return nil, nil
}

func derefInt64(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}

func derefInt32(v *int32) int32 {
	if v == nil {
		return 0
	}
	return *v
}

var _ server.BackgroundJob = (*RecoverySweeper)(nil)
