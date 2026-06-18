package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	runnerdomain "github.com/hangrix/hangrix/apps/hangrix/internal/modules/runner/domain"
	workflowdomain "github.com/hangrix/hangrix/apps/hangrix/internal/modules/workflow/domain"
)

const maxObservedAgentRunsPerRepo = 500

type agentRunObserverStore interface {
	ListJobRunsByRun(ctx context.Context, workflowRunID int64) ([]*workflowdomain.WorkflowJobRun, error)
	ListAgentRunsByRepo(ctx context.Context, repoID int64, status string, offset, limit int32) ([]*workflowdomain.WorkflowRun, int64, error)
}

// AgentRunObserver reconciles hidden `_agent` workflow runs back onto the
// agent_sessions row that spawned them. This closes two failure modes:
//  1. image_build / container_start dies before the agent can post /idle,
//     leaving the session stuck in fake `running`.
//  2. a successful agent process exits without the final idle mark landing,
//     again leaving the row stale.
//
// We recover the owning session id from the workflow job's StepsJSON, where
// CreateAgentRun injects HANGRIX_SESSION_ID into every step env block.
type AgentRunObserver struct {
	workflows workflowdomain.Store
	runStore  agentRunObserverStore
	runner    runnerdomain.Repo
}

type AgentRunObserverDeps struct {
	Workflows workflowdomain.Store
	Runner    runnerdomain.Repo
}

func NewAgentRunObserver(deps *AgentRunObserverDeps) *AgentRunObserver {
	return &AgentRunObserver{
		workflows: deps.Workflows,
		runner:    deps.Runner,
	}
}

func (o *AgentRunObserver) OnRunStatusChanged(ctx context.Context, oldStatus workflowdomain.RunStatus, run *workflowdomain.WorkflowRun) error {
	if run == nil || run.WorkflowName != workflowdomain.InternalAgentWorkflowName {
		return nil
	}
	if oldStatus == run.Status {
		return nil
	}
	if !(run.Status == workflowdomain.RunStatusSuccess || run.Status == workflowdomain.RunStatusFailed || run.Status == workflowdomain.RunStatusCancelled) {
		return nil
	}

	store := o.observerStore()
	if store == nil {
		return nil
	}

	jobs, err := store.ListJobRunsByRun(ctx, run.ID)
	if err != nil || len(jobs) == 0 {
		return err
	}

	sessionID, ok := extractAgentSessionID(jobs[0].StepsJSON)
	if !ok {
		return nil
	}
	if stale, err := o.hasNewerRunForSession(ctx, store, run, sessionID); err != nil {
		return err
	} else if stale {
		return nil
	}

	sess, err := o.runner.GetSessionByID(ctx, sessionID)
	if err != nil {
		return err
	}

	switch run.Status {
	case workflowdomain.RunStatusSuccess:
		if sess.Status == runnerdomain.SessionStatusClaimed || sess.Status == runnerdomain.SessionStatusRunning {
			if err := o.runner.MarkSessionIdle(ctx, sessionID, nil); err != nil && err != runnerdomain.ErrSessionStateInvalid {
				return err
			}
		}
	case workflowdomain.RunStatusFailed, workflowdomain.RunStatusCancelled:
		if sess.Status == runnerdomain.SessionStatusPending || sess.Status == runnerdomain.SessionStatusClaimed || sess.Status == runnerdomain.SessionStatusRunning {
			status := runnerdomain.SessionStatusFailed
			if run.Status == workflowdomain.RunStatusCancelled {
				status = runnerdomain.SessionStatusCancelled
			}
			exitCode, msg := firstTerminalJobFailure(jobs)
			if msg == "" {
				msg = fmt.Sprintf("internal agent workflow %s", run.Status)
			}
			if err := o.runner.MarkSessionTerminal(ctx, sessionID, status, exitCode, msg); err != nil && err != runnerdomain.ErrSessionStateInvalid {
				return err
			}
		}
	}
	return nil
}

func (o *AgentRunObserver) observerStore() agentRunObserverStore {
	if o == nil {
		return nil
	}
	if o.runStore != nil {
		return o.runStore
	}
	return o.workflows
}

func (o *AgentRunObserver) hasNewerRunForSession(ctx context.Context, store agentRunObserverStore, run *workflowdomain.WorkflowRun, sessionID int64) (bool, error) {
	if store == nil || run == nil || run.RepoID == 0 || sessionID == 0 {
		return false, nil
	}
	runs, _, err := store.ListAgentRunsByRepo(ctx, run.RepoID, "", 0, maxObservedAgentRunsPerRepo)
	if err != nil {
		return false, err
	}
	for _, candidate := range runs {
		if candidate == nil || candidate.ID == run.ID {
			continue
		}
		if !candidate.CreatedAt.After(run.CreatedAt) && !(candidate.CreatedAt.Equal(run.CreatedAt) && candidate.ID > run.ID) {
			continue
		}
		jobs, err := store.ListJobRunsByRun(ctx, candidate.ID)
		if err != nil {
			return false, err
		}
		if len(jobs) == 0 {
			continue
		}
		candidateSessionID, ok := extractAgentSessionID(jobs[0].StepsJSON)
		if ok && candidateSessionID == sessionID {
			return true, nil
		}
	}
	return false, nil
}

type workflowStepEnv struct {
	Env map[string]string `json:"env"`
}

func extractAgentSessionID(raw []byte) (int64, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var steps []workflowStepEnv
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

func firstTerminalJobFailure(jobs []*workflowdomain.WorkflowJobRun) (*int32, string) {
	for _, job := range jobs {
		if job.Status == workflowdomain.JobStatusFailed || job.Status == workflowdomain.JobStatusCancelled {
			return job.ExitCode, job.ErrorMessage
		}
	}
	return nil, ""
}

var _ workflowdomain.RunStatusObserver = (*AgentRunObserver)(nil)
