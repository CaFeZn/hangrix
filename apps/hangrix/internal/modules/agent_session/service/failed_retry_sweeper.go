package service

import (
	"context"
	"errors"
	"log"
	"regexp"
	"strings"
	"time"

	agentsessiondomain "github.com/hangrix/hangrix/apps/hangrix/internal/modules/agent_session/domain"
	platformsettings "github.com/hangrix/hangrix/apps/hangrix/internal/modules/platform_settings/domain"
	runnerdomain "github.com/hangrix/hangrix/apps/hangrix/internal/modules/runner/domain"
	"github.com/hangrix/hangrix/apps/hangrix/internal/server"
)

const (
	defaultFailedRetryThreshold = 45 * time.Second
	failedRetrySweepInterval    = 30 * time.Second
	maxRecoveredFailedPerSweep  = 20
)

var retryableFailureRe = regexp.MustCompile(`(?i)(upstream (502|503|504|529)|temporarily unavailable|deadline exceeded|reasoning timeout|llm call failed after retries|connect_transport: open stream|connect_transport: stream err|clone repo: .*signal: killed|git checkout .*signal: killed|context canceled)`)

// FailedRetrySweeper auto-recovers transiently failed/cancelled issue
// sessions. It is intentionally conservative: only transient upstream / timeout
// failures are retried. Config/auth/schema problems remain terminal and are
// expected to be surfaced to the user.
type FailedRetrySweeper struct {
	runner     runnerdomain.Repo
	controller agentsessiondomain.Controller
	settings   platformsettings.Store
}

type FailedRetrySweeperDeps struct {
	Runner     runnerdomain.Repo
	Controller agentsessiondomain.Controller
	Settings   platformsettings.Store
}

func NewFailedRetrySweeper(deps *FailedRetrySweeperDeps) *FailedRetrySweeper {
	return &FailedRetrySweeper{
		runner:     deps.Runner,
		controller: deps.Controller,
		settings:   deps.Settings,
	}
}

func (r *FailedRetrySweeper) Start(ctx context.Context) {
	r.sweepOnce(ctx)
	t := time.NewTicker(failedRetrySweepInterval)
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

func (r *FailedRetrySweeper) sweepOnce(ctx context.Context) {
	threshold := defaultFailedRetryThreshold
	if r.settings != nil {
		if d, err := r.settings.GetDuration(ctx, "lifecycle.failed_retry_threshold"); err == nil && d > 0 {
			threshold = d
		}
	}

	stale, err := r.runner.ListStaleTerminalSessions(ctx, threshold, maxRecoveredFailedPerSweep)
	if err != nil {
		log.Printf("failed_retry_sweeper: list stale terminal sessions: %v", err)
		return
	}
	for _, item := range stale {
		if item == nil || item.Session == nil {
			continue
		}
		if !retryableTerminalSession(item.Session) {
			continue
		}
		if err := r.controller.Recover(ctx, item.Session.ID, "failed-retry-sweeper"); err != nil && !errors.Is(err, agentsessiondomain.ErrNotResumable) {
			log.Printf("failed_retry_sweeper: recover session %d: %v", item.Session.ID, err)
			continue
		}
		log.Printf("failed_retry_sweeper: recovered retryable session %d (repo=%d issue=%d role=%s status=%s)", item.Session.ID, derefInt64(item.Session.RepoID), derefInt32(item.Session.IssueNumber), item.Session.RoleKey, item.Session.Status)
	}
}

func retryableTerminalSession(sess *runnerdomain.AgentSession) bool {
	if sess == nil {
		return false
	}
	msg := strings.ToLower(sess.ErrorMessage)
	if msg == "" {
		return false
	}
	return retryableFailureRe.MatchString(msg)
}

var _ server.BackgroundJob = (*FailedRetrySweeper)(nil)
