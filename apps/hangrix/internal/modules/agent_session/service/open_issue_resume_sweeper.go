package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	agentsessiondomain "github.com/hangrix/hangrix/apps/hangrix/internal/modules/agent_session/domain"
	automationdomain "github.com/hangrix/hangrix/apps/hangrix/internal/modules/automation/domain"
	issuedomain "github.com/hangrix/hangrix/apps/hangrix/internal/modules/issue/domain"
	platformsettings "github.com/hangrix/hangrix/apps/hangrix/internal/modules/platform_settings/domain"
	questionnairedomain "github.com/hangrix/hangrix/apps/hangrix/internal/modules/questionnaire/domain"
	runnerdomain "github.com/hangrix/hangrix/apps/hangrix/internal/modules/runner/domain"
	workflowdomain "github.com/hangrix/hangrix/apps/hangrix/internal/modules/workflow/domain"
	workflowsvc "github.com/hangrix/hangrix/apps/hangrix/internal/modules/workflow/service"
	"github.com/hangrix/hangrix/apps/hangrix/internal/server"
)

const (
	openIssueResumeInterval         = 30 * time.Second
	defaultOpenIssueResumeThreshold = 45 * time.Second
	maxOpenIssuesPerSweep           = 200
)

type agentRunLister interface {
	ListAgentRunsByRepo(ctx context.Context, repoID int64, status string, offset, limit int32) ([]*workflowdomain.WorkflowRun, int64, error)
	ListJobRuns(ctx context.Context, workflowRunID int64) ([]*workflowdomain.WorkflowJobRun, error)
}

type sessionMessageLister interface {
	ListMessages(ctx context.Context, sessionID int64) ([]*runnerdomain.Message, error)
}

// OpenIssueResumeSweeper keeps open issues from going permanently quiet when
// every session on the issue has settled into idle. It auto-recovers the
// issue's maintainer session when:
//   - the issue is still open
//   - there is at least one session on the issue
//   - no session is pending/claimed/running
//   - there is no open questionnaire waiting on user input
//   - the freshest session activity is older than the configured threshold
type OpenIssueResumeSweeper struct {
	repoLister     automationdomain.RepoLister
	issues         issuedomain.Store
	questionnaires questionnairedomain.Service
	runner         runnerdomain.Repo
	controller     agentsessiondomain.Controller
	settings       platformsettings.Store
	workflow       agentRunLister
	messages       sessionMessageLister
}

type OpenIssueResumeSweeperDeps struct {
	RepoLister     automationdomain.RepoLister
	Issues         issuedomain.Store
	Questionnaires questionnairedomain.Service
	Runner         runnerdomain.Repo
	Controller     agentsessiondomain.Controller
	Settings       platformsettings.Store
	Workflow       *workflowsvc.Service
}

func NewOpenIssueResumeSweeper(deps *OpenIssueResumeSweeperDeps) *OpenIssueResumeSweeper {
	return &OpenIssueResumeSweeper{
		repoLister:     deps.RepoLister,
		issues:         deps.Issues,
		questionnaires: deps.Questionnaires,
		runner:         deps.Runner,
		controller:     deps.Controller,
		settings:       deps.Settings,
		workflow:       deps.Workflow,
		messages:       deps.Runner,
	}
}

func (s *OpenIssueResumeSweeper) Start(ctx context.Context) {
	s.sweepOnce(ctx)
	t := time.NewTicker(openIssueResumeInterval)
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

func (s *OpenIssueResumeSweeper) sweepOnce(ctx context.Context) {
	threshold := defaultOpenIssueResumeThreshold
	if s.settings != nil {
		if d, err := s.settings.GetDuration(ctx, "lifecycle.open_issue_resume_threshold"); err == nil && d > 0 {
			threshold = d
		}
	}

	repoRows, err := s.repoLister.ListAll(ctx)
	if err != nil {
		log.Printf("open_issue_resume_sweeper: list repos: %v", err)
		return
	}

	for _, repo := range repoRows {
		numbers, err := s.issues.ListOpenIssueNumbers(ctx, repo.ID)
		if err != nil {
			log.Printf("open_issue_resume_sweeper: repo=%d list open issues: %v", repo.ID, err)
			continue
		}
		for _, number := range numbers {
			iss, err := s.issues.GetByNumber(ctx, repo.ID, number)
			if err != nil || iss == nil {
				continue
			}
			if err := s.resumeIssueIfQuiet(ctx, repo.ID, iss, threshold); err != nil {
				log.Printf("open_issue_resume_sweeper: repo=%d issue=%d: %v", repo.ID, number, err)
			}
		}
	}
}

func (s *OpenIssueResumeSweeper) resumeIssueIfQuiet(ctx context.Context, repoID int64, iss *issuedomain.Issue, threshold time.Duration) error {
	if iss == nil || iss.State != issuedomain.StateOpen {
		return nil
	}
	qns, err := s.questionnaires.GetByIssue(ctx, iss.ID)
	if err == nil {
		for _, qn := range qns {
			if qn != nil && qn.Status == questionnairedomain.StatusOpen {
				return nil
			}
		}
	}

	rows, err := s.runner.ListSessionsByIssue(ctx, repoID, int32(iss.Number))
	if err != nil || len(rows) == 0 {
		return err
	}

	var (
		latest     time.Time
		hasLive    bool
		maintainer *runnerdomain.AgentSession
	)
	for _, row := range rows {
		if row == nil {
			continue
		}
		if row.RoleKey == "maintainer" && row.Status != runnerdomain.SessionStatusArchived {
			maintainer = row
		}
		if row.Status == runnerdomain.SessionStatusPending || row.Status == runnerdomain.SessionStatusClaimed || row.Status == runnerdomain.SessionStatusRunning {
			if s.sessionHasRecentActivity(ctx, row, threshold) {
				hasLive = true
				break
			}
		}
		latest = maxTime(latest, row.CreatedAt)
		if row.StartedAt != nil {
			latest = maxTime(latest, *row.StartedAt)
		}
		if row.EndedAt != nil {
			latest = maxTime(latest, *row.EndedAt)
		}
	}
	if hasLive || maintainer == nil {
		return nil
	}
	if queued, err := s.hasQueuedAgentRun(ctx, repoID, int32(iss.Number)); err != nil {
		return err
	} else if queued {
		return nil
	}
	if latest.IsZero() || time.Since(latest) < threshold {
		return nil
	}
	if err := s.controller.Recover(ctx, maintainer.ID, "open-issue-resume-sweeper"); err != nil && !errors.Is(err, agentsessiondomain.ErrNotResumable) {
		return err
	}
	log.Printf("open_issue_resume_sweeper: recovered maintainer session %d for quiet open issue repo=%d issue=%d", maintainer.ID, repoID, iss.Number)
	return nil
}

func (s *OpenIssueResumeSweeper) hasQueuedAgentRun(ctx context.Context, repoID int64, issueNumber int32) (bool, error) {
	if s.workflow == nil {
		return false, nil
	}
	ref := fmt.Sprintf("issue/%d", issueNumber)
	for _, status := range []string{string(workflowdomain.RunStatusPending), string(workflowdomain.RunStatusRunning)} {
		runs, _, err := s.workflow.ListAgentRunsByRepo(ctx, repoID, status, 0, 200)
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
			if len(jobs) == 0 {
				log.Printf("open_issue_resume_sweeper: ignoring orphan queued _agent run %d ref=%s (no job rows)", run.ID, run.Ref)
				continue
			}
			if !queuedJobBacksLiveSession(ctx, s.runner, jobs[0]) {
				log.Printf("open_issue_resume_sweeper: ignoring stale queued _agent run %d ref=%s (session not live)", run.ID, run.Ref)
				continue
			}
			return true, nil
		}
	}
	return false, nil
}

func queuedJobBacksLiveSession(ctx context.Context, runner runnerdomain.Repo, job *workflowdomain.WorkflowJobRun) bool {
	if runner == nil || job == nil {
		return true
	}
	sessionID, ok := extractAgentSessionID(job.StepsJSON)
	if !ok || sessionID == 0 {
		return true
	}
	sess, err := runner.GetSessionByID(ctx, sessionID)
	if err != nil || sess == nil {
		return false
	}
	return sess.Status == runnerdomain.SessionStatusPending ||
		sess.Status == runnerdomain.SessionStatusClaimed ||
		sess.Status == runnerdomain.SessionStatusRunning
}

func (s *OpenIssueResumeSweeper) sessionHasRecentActivity(ctx context.Context, sess *runnerdomain.AgentSession, threshold time.Duration) bool {
	if sess == nil {
		return false
	}
	latest := sess.CreatedAt
	if sess.ClaimedAt != nil {
		latest = maxTime(latest, *sess.ClaimedAt)
	}
	if sess.StartedAt != nil {
		latest = maxTime(latest, *sess.StartedAt)
	}
	if s.messages != nil {
		rows, err := s.messages.ListMessages(ctx, sess.ID)
		if err == nil {
			for _, msg := range rows {
				if msg != nil {
					latest = maxTime(latest, msg.CreatedAt)
				}
			}
		}
	}
	return !latest.IsZero() && time.Since(latest) < threshold
}

func maxTime(a, b time.Time) time.Time {
	if a.IsZero() {
		return b
	}
	if b.After(a) {
		return b
	}
	return a
}

var _ server.BackgroundJob = (*OpenIssueResumeSweeper)(nil)
