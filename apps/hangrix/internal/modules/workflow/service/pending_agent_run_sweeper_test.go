package service

import (
	"context"
	"errors"
	"testing"

	runnerdomain "github.com/hangrix/hangrix/apps/hangrix/internal/modules/runner/domain"
)

type stubAgentRunSessionLookup struct {
	session *runnerdomain.AgentSession
	err     error
}

func (s stubAgentRunSessionLookup) GetSessionByID(context.Context, int64) (*runnerdomain.AgentSession, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.session, nil
}

func TestAgentRunSessionLookup_AllowsLiveSession(t *testing.T) {
	t.Parallel()

	sess, err := agentRunSession(context.Background(), stubAgentRunSessionLookup{
		session: &runnerdomain.AgentSession{ID: 41, Status: runnerdomain.SessionStatusRunning},
	}, 41)
	if err != nil {
		t.Fatalf("agentRunSession() error = %v", err)
	}
	if sess == nil || sess.Status != runnerdomain.SessionStatusRunning {
		t.Fatalf("agentRunSession() = %#v, want running session", sess)
	}
}

func TestAgentRunSessionLookup_TreatsMissingSessionAsMissing(t *testing.T) {
	t.Parallel()

	_, err := agentRunSession(context.Background(), stubAgentRunSessionLookup{
		err: runnerdomain.ErrSessionNotFound,
	}, 41)
	if !isMissingAgentRunSession(err) {
		t.Fatalf("isMissingAgentRunSession(%v) = false, want true", err)
	}
}

func TestAgentRunSessionLookup_PreservesOtherErrors(t *testing.T) {
	t.Parallel()

	want := errors.New("db offline")
	_, err := agentRunSession(context.Background(), stubAgentRunSessionLookup{
		err: want,
	}, 41)
	if !errors.Is(err, want) {
		t.Fatalf("agentRunSession() error = %v, want %v", err, want)
	}
}
