package service

import (
	"context"
	"errors"
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
	defaultOrphanLiveSessionThreshold = 5 * time.Minute
	orphanLiveSessionSweepInterval    = 2 * time.Minute
	maxOrphanLiveSessionsPerSweep     = 50
)

// OrphanLiveSessionSweeper repairs sessions left in pending/claimed/running
// after their coordinating `_agent` workflow run disappeared. This can happen
// when earlier workflow-run creation partially succeeded or when recovery
// cleanup removed the stale run but the session row never got re-triggered.
type OrphanLiveSessionSweeper struct {
	runner     runnerdomain.Repo
	controller agentsessiondomain.Controller
	workflow   *workflowsvc.Service
	settings   platformsettings.Store
}

type OrphanLiveSessionSweeperDeps struct {
	Runner     runnerdomain.Repo
	Controller agentsessiondomain.Controller
	Workflow   *workflowsvc.Service
	Settings   platformsettings.Store
}

func NewOrphanLiveSessionSweeper(deps *OrphanLiveSessionSweeperDeps) *OrphanLiveSessionSweeper {
	return &OrphanLiveSessionSweeper{
		runner:     deps.Runner,
		controller: deps.Controller,
		workflow:   deps.Workflow,
		settings:   deps.Settings,
	}
}

func (s *OrphanLiveSessionSweeper) Start(ctx context.Context) {
	s.sweepOnce(ctx)
	t := time.NewTicker(orphanLiveSessionSweepInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.sweepOnce(ctx)
		}
	}
}

func (s *OrphanLiveSessionSweeper) sweepOnce(ctx context.Context) {
	if s.runner == nil || s.controller == nil || s.workflow == nil {
		return
	}
	threshold := defaultOrphanLiveSessionThreshold
	if s.settings != nil {
		if d, err := s.settings.GetDuration(ctx, "lifecycle.orphan_live_session_threshold"); err == nil && d > 0 {
			threshold = d
		}
	}

	live := []runnerdomain.SessionStatus{
		runnerdomain.SessionStatusPending,
		runnerdomain.SessionStatusClaimed,
		runnerdomain.SessionStatusRunning,
	}
	recovered := 0
	for _, status := range live {
		rows, err := s.runner.ListSessions(ctx, nil, &status, maxOrphanLiveSessionsPerSweep)
		if err != nil {
			log.Printf("orphan_live_session_sweeper: list %s sessions: %v", status, err)
			continue
		}
		for _, sess := range rows {
			if recovered >= maxOrphanLiveSessionsPerSweep {
				return
			}
			if sess == nil || sess.RepoID == nil || sess.IssueNumber == nil || sess.RoleKey == "" {
				continue
			}
			if !sessionOlderThan(sess, threshold) {
				continue
			}
			ok, err := s.hasLiveAgentRun(ctx, sess)
			if err != nil {
				log.Printf("orphan_live_session_sweeper: session %d check live run: %v", sess.ID, err)
				continue
			}
			if ok {
				continue
			}
			if err := s.makeSessionResumable(ctx, sess); err != nil {
				log.Printf("orphan_live_session_sweeper: prepare session %d: %v", sess.ID, err)
				continue
			}
			if err := s.controller.Recover(ctx, sess.ID, "orphan-live-session-sweeper"); err != nil &&
				!errors.Is(err, agentsessiondomain.ErrNotResumable) &&
				!errors.Is(err, runnerdomain.ErrSessionStateInvalid) {
				log.Printf("orphan_live_session_sweeper: recover session %d: %v", sess.ID, err)
				continue
			}
			recovered++
			log.Printf("orphan_live_session_sweeper: recovered orphan live session %d (repo=%d issue=%d role=%s status=%s)", sess.ID, *sess.RepoID, *sess.IssueNumber, sess.RoleKey, sess.Status)
		}
	}
}

func sessionOlderThan(sess *runnerdomain.AgentSession, threshold time.Duration) bool {
	latest := sess.CreatedAt
	if sess.ClaimedAt != nil && sess.ClaimedAt.After(latest) {
		latest = *sess.ClaimedAt
	}
	if sess.StartedAt != nil && sess.StartedAt.After(latest) {
		latest = *sess.StartedAt
	}
	return time.Since(latest) >= threshold
}

func (s *OrphanLiveSessionSweeper) hasLiveAgentRun(ctx context.Context, sess *runnerdomain.AgentSession) (bool, error) {
	ref := "issue/" + itoa32(*sess.IssueNumber)
	for _, status := range []string{string(workflowdomain.RunStatusPending), string(workflowdomain.RunStatusRunning)} {
		runs, _, err := s.workflow.ListAgentRunsByRepo(ctx, *sess.RepoID, status, 0, 200)
		if err != nil {
			return false, err
		}
		for _, run := range runs {
			if run == nil || run.Ref != ref {
				continue
			}
			jobs, err := s.workflow.ListJobRuns(ctx, run.ID)
			if err != nil {
				return false, err
			}
			for _, job := range jobs {
				if job == nil {
					continue
				}
				id, ok := extractAgentRunSessionID(job.StepsJSON)
				if ok && id == sess.ID {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

func (s *OrphanLiveSessionSweeper) makeSessionResumable(ctx context.Context, sess *runnerdomain.AgentSession) error {
	switch sess.Status {
	case runnerdomain.SessionStatusPending, runnerdomain.SessionStatusClaimed:
		if err := s.runner.MarkSessionTerminal(ctx, sess.ID, runnerdomain.SessionStatusFailed, nil, "auto-recovered orphan live session"); err != nil &&
			!errors.Is(err, runnerdomain.ErrSessionStateInvalid) {
			return err
		}
	case runnerdomain.SessionStatusRunning:
		if err := s.controller.Stop(ctx, sess.ID, "auto-recovered orphan live session"); err != nil {
			return err
		}
	}
	return nil
}

func itoa32(v int32) string {
	return fmtInt64(int64(v))
}

func fmtInt64(v int64) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + (v % 10))
		v /= 10
	}
	return string(buf[i:])
}

var _ server.BackgroundJob = (*OrphanLiveSessionSweeper)(nil)
