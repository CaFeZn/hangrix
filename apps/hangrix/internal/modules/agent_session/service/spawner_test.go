package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hangrix/hangrix/apps/hangrix/internal/agentsconfig"
	"github.com/hangrix/hangrix/apps/hangrix/internal/config"
	"github.com/hangrix/hangrix/apps/hangrix/internal/modules/agent_session/domain"
	platformsettings "github.com/hangrix/hangrix/apps/hangrix/internal/modules/platform_settings/domain"
	repodomain "github.com/hangrix/hangrix/apps/hangrix/internal/modules/repo/domain"
	runnerdomain "github.com/hangrix/hangrix/apps/hangrix/internal/modules/runner/domain"
	workflowdomain "github.com/hangrix/hangrix/apps/hangrix/internal/modules/workflow/domain"
)

// testEncryptionKey is a base64-encoded 32-byte key — same shape the
// runner / llm modules expect at startup. Generated once; deterministic
// so the spawner test cases don't drift between runs.
const testEncryptionKey = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="

// hostFixture bundles a team-only `.hangrix/agents.yml` body with the
// per-role `.hangrix/agents/<role>.md` files. Roles no longer live in
// agents.yml — each is a Markdown file (YAML front matter + prompt body)
// under .hangrix/agents/ that LoadHostConfig assembles. newTestSpawner
// registers both the team yaml and every role file under the `main:` ref
// so the stub blob's ReadBlob + ListBlobs serve them like git would.
type hostFixture struct {
	yaml  string            // team-only agents.yml body
	roles map[string]string // role key → full `.md` file body (front matter + prompt)
}

// agentMD assembles a `.hangrix/agents/<role>.md` body from a front-matter
// block (trigger/permission/tools/scope, no fences) and a short prompt.
func agentMD(frontMatter, prompt string) string {
	return "---\n" + frontMatter + "\n---\n" + prompt + "\n"
}

// teamYAML is the shared team-only agents.yml: version, container (with a
// NODE_ENV env var some tests assert flows through), team llm, and an
// `all: ["*"]` tool rule every fixture role references.
const teamYAML = `version: 1
container:
  image: ghcr.io/acme/dev:1.2.3
  env:
    NODE_ENV: development
llm:
  model: claude-sonnet-4-6
tools:
  all: ["*"]
`

// hostSingleRole: one `backend` role on issue.opened, permission write.
func hostSingleRole() *hostFixture {
	return &hostFixture{
		yaml: teamYAML,
		roles: map[string]string{
			"backend": agentMD("triggers:\n  issue.opened: {}\npermission: write\ntools: [all]", "hi"),
		},
	}
}

// hostMultiRole exercises trigger filtering: dispatcher subscribes to
// issue.opened, reviewer to commit.pushed. A spawn fired with
// issue.opened should ONLY create the dispatcher session.
func hostMultiRole() *hostFixture {
	return &hostFixture{
		yaml: teamYAML,
		roles: map[string]string{
			"dispatcher": agentMD("triggers:\n  issue.opened: {}\npermission: write\ntools: [all]", "hi"),
			"reviewer":   agentMD("triggers:\n  commit.pushed: {}\npermission: write\ntools: [all]", "hi"),
		},
	}
}

// hostMentions exercises the M7b mention path: two roles each subscribe
// to issue.comment with mentioned_only=true. A spawn fired with
// TriggerIssueComment + RoleKey="backend" should wake backend only —
// frontend stays cold even though it also subscribes to the trigger.
func hostMentions() *hostFixture {
	cmtFM := "triggers:\n  issue.comment:\n    mentioned_only: true\npermission: write\ntools: [all]"
	return &hostFixture{
		yaml: teamYAML,
		roles: map[string]string{
			"backend":  agentMD(cmtFM, "hi"),
			"frontend": agentMD(cmtFM, "hi"),
		},
	}
}

// newTestSpawner wires the unit-test stubs onto a Spawner. The host repo
// + agent repo rows + their on-disk shas are passed in so tests can
// pre-seed and assert on the spawn output.
type testHarness struct {
	spawner  *Spawner
	runner   *stubRunnerRepo
	workflow *stubWorkflow
	repos    *stubRepoStore
	git      *stubGit
	blob     *stubBlob
}

type stubPlatformSettings struct {
	boolValues map[string]bool
	intValues  map[string]int
}

func (s *stubPlatformSettings) Get(_ context.Context, _ string) (string, bool, error) { return "", false, nil }
func (s *stubPlatformSettings) GetDuration(_ context.Context, _ string) (time.Duration, error) {
	return 0, nil
}
func (s *stubPlatformSettings) GetBool(_ context.Context, key string) (bool, error) {
	if s == nil || s.boolValues == nil {
		return false, nil
	}
	return s.boolValues[key], nil
}
func (s *stubPlatformSettings) GetInt(_ context.Context, key string) (int, error) {
	if s == nil || s.intValues == nil {
		return 0, nil
	}
	return s.intValues[key], nil
}
func (s *stubPlatformSettings) Set(context.Context, string, string, string) error { return nil }
func (s *stubPlatformSettings) List(context.Context) ([]platformsettings.Setting, error) {
	return nil, nil
}

func newTestSpawner(t *testing.T, fixture *hostFixture, lockBody []byte) *testHarness {
	t.Helper()
	cfg := &config.Config{
		LLM:    config.LLMConfig{EncryptionKey: testEncryptionKey},
		Server: config.ServerConfig{URL: "http://localhost:8080"},
	}
	repos := newStubRepoStore()
	// Host repo (kind=standard is fine — kind is only enforced on the
	// agent repo).
	hostRepo := &repodomain.Repo{
		ID:            1,
		OwnerKind:     repodomain.OwnerKindUser,
		OwnerID:       100,
		OwnerName:     "alice",
		Name:          "myproject",
		DefaultBranch: "main",
	}
	repos.add(hostRepo)
	// Agent repos for the host yaml's roles.
	repos.add(&repodomain.Repo{
		ID:            10,
		OwnerKind:     repodomain.OwnerKindUser,
		OwnerID:       200,
		OwnerName:     "acme",
		Name:          "coder",
		DefaultBranch: "main",
	})
	repos.add(&repodomain.Repo{
		ID:            11,
		OwnerKind:     repodomain.OwnerKindUser,
		OwnerID:       200,
		OwnerName:     "acme",
		Name:          "dispatcher",
		DefaultBranch: "main",
	})
	repos.add(&repodomain.Repo{
		ID:            12,
		OwnerKind:     repodomain.OwnerKindUser,
		OwnerID:       200,
		OwnerName:     "acme",
		Name:          "reviewer",
		DefaultBranch: "main",
	})

	resolver := newStubResolver()
	resolver.addUser("alice", 100)
	resolver.addUser("acme", 200)

	storage := stubPathResolver{}

	git := newStubGit()
	// Host repo base-branch sha + each agent repo's ref→sha resolution.
	hostRepoSHA := "repoSHA00000000000000000000000000000000"
	git.add("/fake/alice/myproject.git", "main", hostRepoSHA)
	git.add("/fake/acme/coder.git", "v1.0.0", "coderSHA0000000000000000000000000000000")
	git.add("/fake/acme/dispatcher.git", "v1.0.0", "dispatcherSHA000000000000000000000000")
	git.add("/fake/acme/reviewer.git", "v1.0.0", "reviewerSHA00000000000000000000000000")

	// Register the team-only agents.yml plus one `.hangrix/agents/<role>.md`
	// per role, all under the host's `main:` ref so the stub blob's
	// ReadBlob + ListBlobs serve them exactly the way GitBlobReader does
	// off a real bare repo at the default-branch tip.
	files := map[string][]byte{}
	if fixture != nil {
		files["main:"+agentsconfig.HostConfigPath] = []byte(fixture.yaml)
		files[hostRepoSHA+":"+agentsconfig.HostConfigPath] = []byte(fixture.yaml)
		for key, body := range fixture.roles {
			files["main:"+agentsconfig.AgentsDir+"/"+key+".md"] = []byte(body)
			files[hostRepoSHA+":"+agentsconfig.AgentsDir+"/"+key+".md"] = []byte(body)
		}
	}
	if lockBody != nil {
		files["main:.hangrix/agents.lock"] = lockBody
		files[hostRepoSHA+":.hangrix/agents.lock"] = lockBody
	}
	blob := newStubBlob(files)

	runner := newStubRunnerRepo()
	workflow := newStubWorkflow()

	s := NewSpawner(&SpawnerDeps{
		Repos:    repos,
		Resolver: resolver,
		Storage:  storage,
		Git:      git,
		Blob:     blob,
		Runner:   runner,
		Workflow: workflow,
		Config:   cfg,
	})
	return &testHarness{spawner: s, runner: runner, workflow: workflow, repos: repos, git: git, blob: blob}
}

// TestEncryptionKey is exposed so an out-of-package test can use it.
// (Not exported on purpose — keep it test-local for now.)
func TestEncryptionKeyShape(t *testing.T) {
	raw, err := base64.StdEncoding.DecodeString(testEncryptionKey)
	if err != nil || len(raw) != 32 {
		t.Fatalf("encryption key invalid: %v len=%d", err, len(raw))
	}
}

// TestOnTriggerHappyPath fires issue.opened on a host yaml that has a
// matching role. We assert: exactly one session row created, snapshot
// fields populated, cause frame seeded, GIT_AUTHOR_NAME env matches
// the role key. The history frame is no longer seeded onto /inputs —
// the runner fetches it from GET /sessions/{id}/history at agent boot.
func TestOnTriggerHappyPath(t *testing.T) {
	h := newTestSpawner(t, hostSingleRole(), nil)
	got, err := h.spawner.OnTrigger(context.Background(), domain.TriggerInput{
		Trigger:     agentsconfig.TriggerIssueOpened,
		CauseKind:   domain.CauseKindIssueOpened,
		RepoID:      1,
		IssueNumber: 42,
		ActorID:     7,
	})
	if err != nil {
		t.Fatalf("OnTrigger err: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d spawned sessions, want 1", len(got))
	}
	if got[0].RoleKey != "backend" {
		t.Fatalf("role_key = %q, want backend", got[0].RoleKey)
	}

	if len(h.runner.sessions) != 1 {
		t.Fatalf("stub stored %d sessions, want 1", len(h.runner.sessions))
	}
	s := h.runner.sessions[0]
	if s.RepoSHA != "repoSHA00000000000000000000000000000000" {
		t.Fatalf("repo_sha = %q", s.RepoSHA)
	}
	if s.RoleKey != "backend" {
		t.Fatalf("role_key column = %q", s.RoleKey)
	}
	if s.CauseKind != string(domain.CauseKindIssueOpened) {
		t.Fatalf("cause_kind = %q", s.CauseKind)
	}
	if s.AgentImage != "ghcr.io/acme/dev:1.2.3" {
		t.Fatalf("agent_image = %q", s.AgentImage)
	}
	if s.Model != "claude-sonnet-4-6" {
		t.Fatalf("model = %q", s.Model)
	}
	if s.BaseBranch != "main" {
		t.Fatalf("base_branch = %q", s.BaseBranch)
	}
	if s.WorkingBranch != "issue/42" {
		t.Fatalf("working_branch = %q", s.WorkingBranch)
	}

	// Snapshot must round-trip as JSON and contain the role's repo
	// permission level + the host image. We don't pin the full snapshot
	// shape — only the keys an audit consumer would rely on.
	var snap map[string]any
	if err := json.Unmarshal(s.RoleConfig, &snap); err != nil {
		t.Fatalf("role_config not JSON: %v", err)
	}
	if got := snap["permission"]; got == nil {
		t.Fatalf("snapshot missing `permission`")
	}
	if got := snap["model"]; got != "claude-sonnet-4-6" {
		t.Fatalf("snapshot model = %v", got)
	}

	// Env: role-key identity is the spawner's job (the runner injects
	// HANGRIX_SESSION_TOKEN, but the role-key git identity comes from
	// here per docs/agent-config.md §"Identity 与 Audit").
	var env map[string]string
	if err := json.Unmarshal(s.Env, &env); err != nil {
		t.Fatalf("env not JSON: %v", err)
	}
	if env["GIT_AUTHOR_NAME"] != "backend" {
		t.Fatalf("GIT_AUTHOR_NAME = %q", env["GIT_AUTHOR_NAME"])
	}
	if env["GIT_AUTHOR_EMAIL"] != "backend@agents.localhost" {
		t.Fatalf("GIT_AUTHOR_EMAIL = %q", env["GIT_AUTHOR_EMAIL"])
	}
	if env["HANGRIX_ROLE_KEY"] != "backend" {
		t.Fatalf("HANGRIX_ROLE_KEY = %q", env["HANGRIX_ROLE_KEY"])
	}
	// Audit pin is injected so the in-container agent can include it
	// in its own logs / tool-call payloads without an extra
	// platform-MCP roundtrip.
	if env["HANGRIX_REPO_SHA"] != "repoSHA00000000000000000000000000000000" {
		t.Fatalf("HANGRIX_REPO_SHA = %q", env["HANGRIX_REPO_SHA"])
	}
	if env["HANGRIX_CAUSE_KIND"] != string(domain.CauseKindIssueOpened) {
		t.Fatalf("HANGRIX_CAUSE_KIND = %q", env["HANGRIX_CAUSE_KIND"])
	}
	// Host yaml's container.env keys flow through.
	if env["NODE_ENV"] != "development" {
		t.Fatalf("NODE_ENV = %q", env["NODE_ENV"])
	}

	// Inputs queue: cause frame only. History is served separately via
	// GET /sessions/{id}/history at runner boot, not enqueued here.
	if len(h.runner.inputs) != 1 {
		t.Fatalf("inputs queued = %d, want 1", len(h.runner.inputs))
	}
	if !strings.Contains(string(h.runner.inputs[0].Payload), `"event":"issue.opened"`) {
		t.Fatalf("first input is not issue.opened event: %s", string(h.runner.inputs[0].Payload))
	}

	// Message log: one event message persisted for the cause.
	if len(h.runner.messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(h.runner.messages))
	}
	if h.runner.messages[0].EventName != "issue.opened" {
		t.Fatalf("message event = %q", h.runner.messages[0].EventName)
	}
}

func TestOnTriggerFirepowerPrefersGPT55ForFastRole(t *testing.T) {
	fixture := &hostFixture{
		yaml: teamYAML,
		roles: map[string]string{
			"fast-worker": agentMD("triggers:\n  issue.opened: {}\npermission: write\ntools: [all]", "hi"),
		},
	}
	h := newTestSpawner(t, fixture, nil)
	h.spawner.settings = &stubPlatformSettings{boolValues: map[string]bool{
		platformsettings.SettingFirepowerEnabled: true,
	}}

	got, err := h.spawner.OnTrigger(context.Background(), domain.TriggerInput{
		Trigger:     agentsconfig.TriggerIssueOpened,
		CauseKind:   domain.CauseKindIssueOpened,
		RepoID:      1,
		IssueNumber: 42,
	})
	if err != nil {
		t.Fatalf("OnTrigger err: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d spawned sessions, want 1", len(got))
	}
	if h.runner.sessions[0].Model != platformsettings.FirepowerPreferredModel {
		t.Fatalf("model = %q, want %q", h.runner.sessions[0].Model, platformsettings.FirepowerPreferredModel)
	}
	calls := h.workflow.Calls()
	if len(calls) != 1 {
		t.Fatalf("workflow calls = %d, want 1", len(calls))
	}
	if calls[0].LLMModel != platformsettings.FirepowerPreferredModel {
		t.Fatalf("workflow LLMModel = %q, want %q", calls[0].LLMModel, platformsettings.FirepowerPreferredModel)
	}
}

// TestOnTriggerFiltersByTrigger asserts the matched-role filter: a host
// yaml with dispatcher (issue.opened) + reviewer (commit.pushed) fired
// with issue.opened produces exactly one row, dispatcher only.
func TestOnTriggerFiltersByTrigger(t *testing.T) {
	h := newTestSpawner(t, hostMultiRole(), nil)
	got, err := h.spawner.OnTrigger(context.Background(), domain.TriggerInput{
		Trigger:     agentsconfig.TriggerIssueOpened,
		CauseKind:   domain.CauseKindIssueOpened,
		RepoID:      1,
		IssueNumber: 7,
		ActorID:     1,
	})
	if err != nil {
		t.Fatalf("OnTrigger err: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d sessions, want 1 (reviewer should be filtered out)", len(got))
	}
	if got[0].RoleKey != "dispatcher" {
		t.Fatalf("role = %q, want dispatcher", got[0].RoleKey)
	}
}

// TestOnTriggerIdempotent reruns the same trigger after one successful
// spawn and confirms the second call returns zero new sessions. The
// in-memory stub keeps the original row.
func TestOnTriggerIdempotent(t *testing.T) {
	h := newTestSpawner(t, hostSingleRole(), nil)
	in := domain.TriggerInput{
		Trigger:     agentsconfig.TriggerIssueOpened,
		CauseKind:   domain.CauseKindIssueOpened,
		RepoID:      1,
		IssueNumber: 42,
		ActorID:     1,
	}
	first, _ := h.spawner.OnTrigger(context.Background(), in)
	if len(first) != 1 {
		t.Fatalf("first OnTrigger = %d, want 1", len(first))
	}
	second, _ := h.spawner.OnTrigger(context.Background(), in)
	if len(second) != 0 {
		t.Fatalf("second OnTrigger = %d, want 0 (idempotent)", len(second))
	}
	if len(h.runner.sessions) != 1 {
		t.Fatalf("stub stored %d rows after rerun, want 1", len(h.runner.sessions))
	}
}

// TestOnTriggerMissingHostYAMLNoOp asserts that a host with no
// `.hangrix/agents.yml` produces zero sessions and no error — the
// common case for non-agent repos.
func TestOnTriggerMissingHostYAMLUsesFallback(t *testing.T) {
	h := newTestSpawner(t, hostSingleRole(), nil)
	// Drop every host-config blob, simulating a repo with no committed
	// `.hangrix`. The platform fallback should still provide roles.
	h.blob.files = map[string][]byte{}

	got, err := h.spawner.OnTrigger(context.Background(), domain.TriggerInput{
		Trigger:     agentsconfig.TriggerIssueOpened,
		CauseKind:   domain.CauseKindIssueOpened,
		RepoID:      1,
		IssueNumber: 99,
		ActorID:     1,
	})
	if err != nil {
		t.Fatalf("missing host yaml should use fallback config, got err: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d sessions, want 1", len(got))
	}
	if got[0].RoleKey != "maintainer" {
		t.Fatalf("role = %q, want maintainer", got[0].RoleKey)
	}
}

// TestArchiverFlipsActiveSessions covers the issue.closed / .merged
// path: every non-archived row on the (repo, issue) flips to archived.
func TestArchiverFlipsActiveSessions(t *testing.T) {
	h := newTestSpawner(t, hostSingleRole(), nil)
	// Spawn one session first.
	_, _ = h.spawner.OnTrigger(context.Background(), domain.TriggerInput{
		Trigger:     agentsconfig.TriggerIssueOpened,
		CauseKind:   domain.CauseKindIssueOpened,
		RepoID:      1,
		IssueNumber: 42,
		ActorID:     1,
	})

	arch := NewArchiver(&ArchiverDeps{Runner: h.runner})
	n, err := arch.OnIssueClosed(context.Background(), 1, 42)
	if err != nil {
		t.Fatalf("Archiver err: %v", err)
	}
	if n != 1 {
		t.Fatalf("archived = %d, want 1", n)
	}
	if h.runner.sessions[0].Status != runnerdomain.SessionStatusArchived {
		t.Fatalf("status = %q, want archived", h.runner.sessions[0].Status)
	}
	// Idempotent on rerun.
	n2, _ := arch.OnIssueClosed(context.Background(), 1, 42)
	if n2 != 0 {
		t.Fatalf("second archive = %d, want 0", n2)
	}
}

// TestOnTriggerArchivedRoleSpawnsReplacement covers the post-archive
// re-trigger path: once a role's session on an issue is archived
// (issue.closed/merged or operator-driven Delete on a containered row),
// the next matching trigger spawns a fresh row instead of being
// silenced. The archived row is left in place for audit; the new row
// becomes the canonical session for the (issue, role).
func TestOnTriggerArchivedRoleSpawnsReplacement(t *testing.T) {
	h := newTestSpawner(t, hostMentions(), nil)
	in := domain.TriggerInput{
		Trigger:     agentsconfig.TriggerIssueComment,
		Comment:     &domain.CommentContext{Mentions: []string{"backend"}},
		CauseKind:   domain.CauseKindCommentMentioned,
		CauseID:     "100",
		RepoID:      1,
		IssueNumber: 7,
		ActorID:     1,
		RoleKey:     "backend",
	}
	first, err := h.spawner.OnTrigger(context.Background(), in)
	if err != nil {
		t.Fatalf("first OnTrigger err: %v", err)
	}
	if len(first) != 1 || first[0].Action != domain.SpawnActionSpawned {
		t.Fatalf("first call = %+v, want one spawned", first)
	}

	// Flip the row to archived — same terminal state Delete (on a
	// containered row) and OnIssueClosed produce.
	if _, err := NewArchiver(&ArchiverDeps{Runner: h.runner}).OnIssueClosed(context.Background(), 1, 7); err != nil {
		t.Fatalf("archive err: %v", err)
	}
	if h.runner.sessions[0].Status != runnerdomain.SessionStatusArchived {
		t.Fatalf("precondition: first row not archived (status=%q)", h.runner.sessions[0].Status)
	}

	// Same role, fresh mention — a new comment id so the (cause)
	// idempotency gate doesn't fire.
	in.CauseID = "101"
	second, err := h.spawner.OnTrigger(context.Background(), in)
	if err != nil {
		t.Fatalf("second OnTrigger err: %v", err)
	}
	if len(second) != 1 {
		t.Fatalf("second call = %d sessions, want 1 (archive should not silence the role)", len(second))
	}
	if second[0].Action != domain.SpawnActionSpawned {
		t.Fatalf("second action = %q, want spawned (a fresh row, not rewake of the archived predecessor)", second[0].Action)
	}
	if len(h.runner.sessions) != 2 {
		t.Fatalf("stored rows = %d, want 2 (archived + replacement)", len(h.runner.sessions))
	}
	if h.runner.sessions[0].Status != runnerdomain.SessionStatusArchived {
		t.Fatalf("archived row mutated: status=%q", h.runner.sessions[0].Status)
	}
	// Spawner inlines the pending→running transition (the runner no
	// longer claims agent sessions; agent runs ride workflow_jobs).
	if h.runner.sessions[1].Status != runnerdomain.SessionStatusPending {
		t.Fatalf("replacement row status = %q, want pending", h.runner.sessions[1].Status)
	}
	if h.runner.sessions[1].RoleKey != "backend" {
		t.Fatalf("replacement role_key = %q, want backend", h.runner.sessions[1].RoleKey)
	}
	if h.runner.sessions[1].CauseID != "101" {
		t.Fatalf("replacement cause_id = %q, want 101", h.runner.sessions[1].CauseID)
	}
}

// TestOnTriggerRoleKeyScopesToOneRole verifies M7b's per-mention
// scoping: a comment-mentioned trigger with RoleKey="backend" wakes
// backend only, even though frontend also subscribes to the same
// trigger.
func TestOnTriggerRoleKeyScopesToOneRole(t *testing.T) {
	h := newTestSpawner(t, hostMentions(), nil)
	got, err := h.spawner.OnTrigger(context.Background(), domain.TriggerInput{
		Trigger:     agentsconfig.TriggerIssueComment,
		Comment:     &domain.CommentContext{Mentions: []string{"backend"}},
		CauseKind:   domain.CauseKindCommentMentioned,
		CauseID:     "42",
		RepoID:      1,
		IssueNumber: 7,
		ActorID:     1,
		RoleKey:     "backend",
	})
	if err != nil {
		t.Fatalf("OnTrigger err: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d sessions, want 1 (frontend should be filtered out)", len(got))
	}
	if got[0].RoleKey != "backend" {
		t.Fatalf("role = %q, want backend", got[0].RoleKey)
	}
	if got[0].Action != domain.SpawnActionSpawned {
		t.Fatalf("action = %q, want spawned", got[0].Action)
	}
	if h.runner.sessions[0].CauseID != "42" {
		t.Fatalf("cause_id stored = %q, want 42", h.runner.sessions[0].CauseID)
	}
}

// TestOnTriggerEnqueueOntoLiveSession covers the M7b "live session
// reuse" path: when the first mention spawned a session, a second
// mention with a different CauseID appends an event frame to the
// existing row's inputs queue instead of creating a duplicate row.
func TestOnTriggerEnqueueOntoLiveSession(t *testing.T) {
	h := newTestSpawner(t, hostMentions(), nil)
	first, err := h.spawner.OnTrigger(context.Background(), domain.TriggerInput{
		Trigger:     agentsconfig.TriggerIssueComment,
		Comment:     &domain.CommentContext{Mentions: []string{"backend"}},
		CauseKind:   domain.CauseKindCommentMentioned,
		CauseID:     "100",
		RepoID:      1,
		IssueNumber: 7,
		ActorID:     1,
		RoleKey:     "backend",
	})
	if err != nil {
		t.Fatalf("first OnTrigger err: %v", err)
	}
	if len(first) != 1 || first[0].Action != domain.SpawnActionSpawned {
		t.Fatalf("first call = %+v, want one spawned", first)
	}

	// Second mention with a new comment id. The existing session row
	// is still pending (M6c never transitioned it terminal here).
	second, err := h.spawner.OnTrigger(context.Background(), domain.TriggerInput{
		Trigger:     agentsconfig.TriggerIssueComment,
		Comment:     &domain.CommentContext{Mentions: []string{"backend"}},
		CauseKind:   domain.CauseKindCommentMentioned,
		CauseID:     "101",
		RepoID:      1,
		IssueNumber: 7,
		ActorID:     2,
		RoleKey:     "backend",
	})
	if err != nil {
		t.Fatalf("second OnTrigger err: %v", err)
	}
	if len(second) != 1 || second[0].Action != domain.SpawnActionEnqueued {
		t.Fatalf("second call = %+v, want one enqueued", second)
	}
	if len(h.runner.sessions) != 1 {
		t.Fatalf("stub stored %d rows, want 1 (no duplicate spawn)", len(h.runner.sessions))
	}
	// Inputs queue should now have cause-1 + cause-2. History lives on
	// GET /sessions/{id}/history, not on this queue.
	if len(h.runner.inputs) != 2 {
		t.Fatalf("inputs = %d, want 2 (2 cause events)", len(h.runner.inputs))
	}
	if !strings.Contains(string(h.runner.inputs[1].Payload), `"cause_id":"101"`) {
		t.Fatalf("second input is not cause_id=101: %s", string(h.runner.inputs[1].Payload))
	}
}

// TestOnTriggerEnqueueWhenSessionStatusLagsButRunStillRunning covers the
// race where the session row looks non-live (idle) but a running `_agent`
// workflow run still exists for the same session. We should enqueue onto the
// existing session instead of creating a second run and relying on the
// duplicate-run sweeper to clean it up later.
func TestOnTriggerEnqueueWhenSessionStatusLagsButRunStillRunning(t *testing.T) {
	h := newTestSpawner(t, hostMentions(), nil)
	first, err := h.spawner.OnTrigger(context.Background(), domain.TriggerInput{
		Trigger:     agentsconfig.TriggerIssueComment,
		Comment:     &domain.CommentContext{Mentions: []string{"backend"}},
		CauseKind:   domain.CauseKindCommentMentioned,
		CauseID:     "200",
		RepoID:      1,
		IssueNumber: 7,
		ActorID:     1,
		RoleKey:     "backend",
	})
	if err != nil {
		t.Fatalf("first OnTrigger err: %v", err)
	}
	if len(first) != 1 || first[0].Action != domain.SpawnActionSpawned {
		t.Fatalf("first call = %+v, want one spawned", first)
	}

	// Simulate a lagging session row while the workflow run is still running.
	h.runner.sessions[0].Status = runnerdomain.SessionStatusIdle

	second, err := h.spawner.OnTrigger(context.Background(), domain.TriggerInput{
		Trigger:     agentsconfig.TriggerIssueComment,
		Comment:     &domain.CommentContext{Mentions: []string{"backend"}},
		CauseKind:   domain.CauseKindCommentMentioned,
		CauseID:     "201",
		RepoID:      1,
		IssueNumber: 7,
		ActorID:     2,
		RoleKey:     "backend",
	})
	if err != nil {
		t.Fatalf("second OnTrigger err: %v", err)
	}
	if len(second) != 1 || second[0].Action != domain.SpawnActionEnqueued {
		t.Fatalf("second call = %+v, want one enqueued", second)
	}
	if len(h.runner.sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(h.runner.sessions))
	}
	if len(h.workflow.Calls()) != 1 {
		t.Fatalf("workflow.CreateAgentRun calls = %d, want 1", len(h.workflow.Calls()))
	}
	if len(h.runner.inputs) != 2 {
		t.Fatalf("inputs = %d, want 2", len(h.runner.inputs))
	}
	if !strings.Contains(string(h.runner.inputs[1].Payload), `"cause_id":"201"`) {
		t.Fatalf("second input is not cause_id=201: %s", string(h.runner.inputs[1].Payload))
	}
}

// TestOnTriggerEnqueueWhenSessionStatusLagsButRunStillPending covers the
// sister race where the session row looks non-live (idle) while a pending
// `_agent` workflow run already exists for that session. We should enqueue
// onto the existing session instead of creating yet another pending run.
func TestOnTriggerEnqueueWhenSessionStatusLagsButRunStillPending(t *testing.T) {
	h := newTestSpawner(t, hostMentions(), nil)
	first, err := h.spawner.OnTrigger(context.Background(), domain.TriggerInput{
		Trigger:     agentsconfig.TriggerIssueComment,
		Comment:     &domain.CommentContext{Mentions: []string{"backend"}},
		CauseKind:   domain.CauseKindCommentMentioned,
		CauseID:     "210",
		RepoID:      1,
		IssueNumber: 7,
		ActorID:     1,
		RoleKey:     "backend",
	})
	if err != nil {
		t.Fatalf("first OnTrigger err: %v", err)
	}
	if len(first) != 1 || first[0].Action != domain.SpawnActionSpawned {
		t.Fatalf("first call = %+v, want one spawned", first)
	}

	// Simulate a lagging session row while the workflow run is still only pending.
	h.runner.sessions[0].Status = runnerdomain.SessionStatusIdle
	h.workflow.runs[0].Status = workflowdomain.RunStatusPending
	h.workflow.jobs[h.workflow.runs[0].ID][0].Status = workflowdomain.JobStatusPending

	second, err := h.spawner.OnTrigger(context.Background(), domain.TriggerInput{
		Trigger:     agentsconfig.TriggerIssueComment,
		Comment:     &domain.CommentContext{Mentions: []string{"backend"}},
		CauseKind:   domain.CauseKindCommentMentioned,
		CauseID:     "211",
		RepoID:      1,
		IssueNumber: 7,
		ActorID:     2,
		RoleKey:     "backend",
	})
	if err != nil {
		t.Fatalf("second OnTrigger err: %v", err)
	}
	if len(second) != 1 || second[0].Action != domain.SpawnActionEnqueued {
		t.Fatalf("second call = %+v, want one enqueued", second)
	}
	if len(h.runner.sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(h.runner.sessions))
	}
	if len(h.workflow.Calls()) != 1 {
		t.Fatalf("workflow.CreateAgentRun calls = %d, want 1", len(h.workflow.Calls()))
	}
	if len(h.runner.inputs) != 2 {
		t.Fatalf("inputs = %d, want 2", len(h.runner.inputs))
	}
	if !strings.Contains(string(h.runner.inputs[1].Payload), `"cause_id":"211"`) {
		t.Fatalf("second input is not cause_id=211: %s", string(h.runner.inputs[1].Payload))
	}
}

// TestOnTriggerRewakeRotatesIdleToken asserts that a re-trigger against
// an idle row rotates the session token. This severs any late traffic
// from the previous container generation so a stale MarkIdle or tool
// callback cannot mutate the newly rewoken session.
func TestOnTriggerRewakeRotatesIdleToken(t *testing.T) {
	h := newTestSpawner(t, hostSingleRole(), nil)
	first, err := h.spawner.OnTrigger(context.Background(), domain.TriggerInput{
		Trigger:     agentsconfig.TriggerIssueOpened,
		CauseKind:   domain.CauseKindIssueOpened,
		CauseID:     "1",
		RepoID:      1,
		IssueNumber: 7,
		ActorID:     1,
	})
	if err != nil {
		t.Fatalf("first OnTrigger err: %v", err)
	}
	if len(first) != 1 || first[0].Action != domain.SpawnActionSpawned {
		t.Fatalf("first call = %+v, want one spawned", first)
	}
	original := h.runner.sessions[0]
	wantPrefix, wantHash, wantSealed := original.SessionTokenPrefix, original.SessionTokenHash, original.SessionTokenSealed

	// Simulate the runner reporting clean exit (MarkSessionIdle would
	// flip status without touching the token).
	original.Status = runnerdomain.SessionStatusIdle
	h.workflow.runs[0].Status = workflowdomain.RunStatusSuccess
	h.workflow.jobs[h.workflow.runs[0].ID][0].Status = workflowdomain.JobStatusSuccess

	second, err := h.spawner.OnTrigger(context.Background(), domain.TriggerInput{
		Trigger:     agentsconfig.TriggerIssueOpened,
		CauseKind:   domain.CauseKindIssueOpened,
		CauseID:     "2",
		RepoID:      1,
		IssueNumber: 7,
		ActorID:     1,
	})
	if err != nil {
		t.Fatalf("second OnTrigger err: %v", err)
	}
	if len(second) != 1 || second[0].Action != domain.SpawnActionRewoken {
		t.Fatalf("second call = %+v, want one rewoken", second)
	}
	got := h.runner.sessions[0]
	if got.SessionTokenPrefix == wantPrefix {
		t.Errorf("rewake reused prefix %q, want a fresh token", wantPrefix)
	}
	if got.SessionTokenHash == wantHash {
		t.Errorf("rewake reused hash, want a fresh token")
	}
	if got.SessionTokenSealed == wantSealed {
		t.Errorf("rewake reused sealed token, want a fresh token")
	}
	// Each wake fires one _agent workflow run — first call spawned, second
	// rewoke. Both runs reference the same SessionID but with a fresh
	// SessionToken plaintext on the rewake.
	calls := h.workflow.Calls()
	if len(calls) != 2 {
		t.Fatalf("workflow.CreateAgentRun calls = %d, want 2", len(calls))
	}
	if calls[0].SessionID != got.ID || calls[1].SessionID != got.ID {
		t.Errorf("workflow calls SessionIDs = %d/%d, want both = %d", calls[0].SessionID, calls[1].SessionID, got.ID)
	}
	if calls[0].SessionToken == calls[1].SessionToken {
		t.Errorf("workflow calls SessionToken reused across wakes: %q", calls[0].SessionToken)
	}
}

// TestOnTriggerRewakeAfterFailed verifies that a failed session IS
// automatically rewoken on a new trigger with a different cause, and
// the rewake rotates the token to isolate the new container generation.
func TestOnTriggerRewakeAfterFailed(t *testing.T) {
	h := newTestSpawner(t, hostSingleRole(), nil)
	if _, err := h.spawner.OnTrigger(context.Background(), domain.TriggerInput{
		Trigger:     agentsconfig.TriggerIssueOpened,
		CauseKind:   domain.CauseKindIssueOpened,
		CauseID:     "1",
		RepoID:      1,
		IssueNumber: 7,
		ActorID:     1,
	}); err != nil {
		t.Fatalf("first OnTrigger err: %v", err)
	}
	original := h.runner.sessions[0]
	wantPrefix, wantHash, wantSealed := original.SessionTokenPrefix, original.SessionTokenHash, original.SessionTokenSealed

	// Simulate runner terminate with a non-zero exit code. Modern
	// MarkSessionTerminal preserves sealed.
	original.Status = runnerdomain.SessionStatusFailed
	h.workflow.runs[0].Status = workflowdomain.RunStatusFailed
	h.workflow.jobs[h.workflow.runs[0].ID][0].Status = workflowdomain.JobStatusFailed

	second, err := h.spawner.OnTrigger(context.Background(), domain.TriggerInput{
		Trigger:     agentsconfig.TriggerIssueOpened,
		CauseKind:   domain.CauseKindIssueOpened,
		CauseID:     "2",
		RepoID:      1,
		IssueNumber: 7,
		ActorID:     1,
	})
	if err != nil {
		t.Fatalf("second OnTrigger err: %v", err)
	}
	// Failed session is rewoken, not replaced.
	if len(second) != 1 || second[0].Action != domain.SpawnActionRewoken {
		t.Fatalf("second call = %+v, want one rewoken", second)
	}
	// Status is NOT updated to 'pending' by the spawner anymore — the
	// runner-relay "pending → claim → running" lifecycle is replaced by
	// workflow_job_run pickups. The session row's status only flips when
	// the agent itself transitions it via /api/agent/sessions/{id}/idle
	// or the cleanup reaper marks it terminal.
	// Only one session row — no new row created.
	if len(h.runner.sessions) != 1 {
		t.Fatalf("sessions = %d, want 1 (rewoken row, no new spawn)", len(h.runner.sessions))
	}
	if original.SessionTokenPrefix == wantPrefix {
		t.Errorf("rewake reused prefix %q, want a fresh token", wantPrefix)
	}
	if original.SessionTokenHash == wantHash {
		t.Errorf("rewake reused hash, want a fresh token")
	}
	if original.SessionTokenSealed == wantSealed {
		t.Errorf("rewake reused sealed token, want a fresh token")
	}
}

// TestOnTriggerRewakeAfterLegacyFailed verifies that a legacy failed row
// is rewoken with a freshly minted token when a new trigger arrives.
func TestOnTriggerRewakeAfterLegacyFailed(t *testing.T) {
	h := newTestSpawner(t, hostSingleRole(), nil)
	if _, err := h.spawner.OnTrigger(context.Background(), domain.TriggerInput{
		Trigger:     agentsconfig.TriggerIssueOpened,
		CauseKind:   domain.CauseKindIssueOpened,
		CauseID:     "1",
		RepoID:      1,
		IssueNumber: 7,
		ActorID:     1,
	}); err != nil {
		t.Fatalf("first OnTrigger err: %v", err)
	}
	original := h.runner.sessions[0]
	priorPrefix, priorHash := original.SessionTokenPrefix, original.SessionTokenHash

	// Simulate a row that died before the sealed-preservation change.
	original.Status = runnerdomain.SessionStatusFailed
	original.SessionTokenSealed = ""
	h.workflow.runs[0].Status = workflowdomain.RunStatusFailed
	h.workflow.jobs[h.workflow.runs[0].ID][0].Status = workflowdomain.JobStatusFailed

	if _, err := h.spawner.OnTrigger(context.Background(), domain.TriggerInput{
		Trigger:     agentsconfig.TriggerIssueOpened,
		CauseKind:   domain.CauseKindIssueOpened,
		CauseID:     "2",
		RepoID:      1,
		IssueNumber: 7,
		ActorID:     1,
	}); err != nil {
		t.Fatalf("second OnTrigger err: %v", err)
	}
	// Legacy failed session is rewoken — a fresh token is minted because
	// the sealed plaintext is missing. Spawner inlines the pending→
	// running transition (the runner no longer claims agent sessions;
	// agent runs ride workflow_jobs).
	if original.Status != runnerdomain.SessionStatusPending {
		t.Fatalf("original status = %q, want pending (rewoken)", original.Status)
	}
	// Only one session row — the legacy row is rewoken, not replaced.
	if len(h.runner.sessions) != 1 {
		t.Fatalf("sessions = %d, want 1 (rewoken legacy row)", len(h.runner.sessions))
	}
	// Because the legacy row had no sealed token, a fresh token is minted.
	if original.SessionTokenPrefix == priorPrefix && priorPrefix != "" {
		t.Errorf("legacy rewake reused empty-sealed prefix %q, want a fresh mint", priorPrefix)
	}
	if original.SessionTokenHash == priorHash && priorHash != "" {
		t.Errorf("legacy rewake reused empty-sealed hash, want a fresh mint")
	}
	if original.SessionTokenSealed == "" {
		t.Errorf("legacy rewake left sealed empty, want freshly encrypted plaintext")
	}
}

// TestOnTriggerPayloadMergedIntoCauseFrame asserts the M7b Payload
// field is layered onto the input frame's payload object — the agent
// sees comment_body etc. directly on its stdin without a tool call.
func TestOnTriggerPayloadMergedIntoCauseFrame(t *testing.T) {
	h := newTestSpawner(t, hostSingleRole(), nil)
	extra, _ := json.Marshal(map[string]any{
		"comment_id":   42,
		"comment_body": "please add /healthz",
	})
	_, err := h.spawner.OnTrigger(context.Background(), domain.TriggerInput{
		Trigger:     agentsconfig.TriggerIssueOpened,
		CauseKind:   domain.CauseKindIssueOpened,
		CauseID:     "1",
		RepoID:      1,
		IssueNumber: 9,
		ActorID:     1,
		Payload:     extra,
	})
	if err != nil {
		t.Fatalf("OnTrigger err: %v", err)
	}
	if len(h.runner.inputs) != 1 {
		t.Fatalf("inputs = %d, want 1", len(h.runner.inputs))
	}
	body := string(h.runner.inputs[0].Payload)
	if !strings.Contains(body, `"comment_body":"please add /healthz"`) {
		t.Fatalf("cause frame missing comment_body: %s", body)
	}
	if !strings.Contains(body, `"comment_id":42`) {
		t.Fatalf("cause frame missing comment_id: %s", body)
	}
}

// TestLoadHostConfigReturnsParsedRoles covers the interface the issue
// handler uses to resolve `@agent-<role-key>` mentions against the
// host yaml's role declarations.
func TestLoadHostConfigReturnsParsedRoles(t *testing.T) {
	h := newTestSpawner(t, hostMentions(), nil)
	cfg, err := h.spawner.LoadHostConfig(context.Background(), 1)
	if err != nil {
		t.Fatalf("LoadHostConfig err: %v", err)
	}
	if cfg == nil {
		t.Fatalf("LoadHostConfig returned nil")
	}
	if _, ok := cfg.Roles["backend"]; !ok {
		t.Fatalf("config missing backend role; got keys %v", roleKeyNames(cfg.Roles))
	}
}

// TestLoadHostConfigMissingReturnsNil — non-agent repo returns (nil, nil).
func TestLoadHostConfigMissingUsesFallback(t *testing.T) {
	h := newTestSpawner(t, hostSingleRole(), nil)
	h.blob.files = map[string][]byte{}
	cfg, err := h.spawner.LoadHostConfig(context.Background(), 1)
	if err != nil {
		t.Fatalf("LoadHostConfig err: %v", err)
	}
	if cfg == nil {
		t.Fatal("LoadHostConfig returned nil for missing host config")
	}
	if _, ok := cfg.Roles["maintainer"]; !ok {
		t.Fatalf("fallback config missing maintainer role; got keys %v", roleKeyNames(cfg.Roles))
	}
	if _, ok := cfg.Roles["fast-worker"]; !ok {
		t.Fatalf("fallback config missing fast-worker role; got keys %v", roleKeyNames(cfg.Roles))
	}
}

func TestLoadHostConfigUsesResolvedCommitSnapshot(t *testing.T) {
	h := newTestSpawner(t, hostSingleRole(), nil)
	hostFS := "/fake/alice/myproject.git"
	snapshotSHA := "repoSNAPSHOT0000000000000000000000000000"
	h.git.add(hostFS, "main", snapshotSHA)

	brokenMainYAML := teamYAML + `reviewers:
  rules:
    - paths: ["apps/web/**"]
      reviewers: [frontend]
  fallback: [backend]
`
	backendMD := agentMD("triggers:\n  issue.opened: {}\npermission: write\ntools: [all]", "hi")
	h.blob.files = map[string][]byte{
		"main:" + agentsconfig.HostConfigPath:                      []byte(brokenMainYAML),
		"main:" + agentsconfig.AgentsDir + "/backend.md":           []byte(backendMD),
		snapshotSHA + ":" + agentsconfig.HostConfigPath:            []byte(teamYAML),
		snapshotSHA + ":" + agentsconfig.AgentsDir + "/backend.md": []byte(backendMD),
	}

	cfg, err := h.spawner.LoadHostConfig(context.Background(), 1)
	if err != nil {
		t.Fatalf("LoadHostConfig err: %v", err)
	}
	if cfg == nil {
		t.Fatalf("LoadHostConfig returned nil")
	}
	if _, ok := cfg.Roles["backend"]; !ok {
		t.Fatalf("config missing backend role; got keys %v", roleKeyNames(cfg.Roles))
	}
}

func roleKeyNames(roles map[string]*agentsconfig.Role) []string {
	out := make([]string, 0, len(roles))
	for k := range roles {
		out = append(out, k)
	}
	return out
}

// TestAuditorReturnsSnapshotColumns confirms ListByIssue surfaces the
// frozen pins (agent_sha, repo_sha, cause_kind, etc.) in spawn order.
func TestAuditorReturnsSnapshotColumns(t *testing.T) {
	h := newTestSpawner(t, hostSingleRole(), nil)
	_, _ = h.spawner.OnTrigger(context.Background(), domain.TriggerInput{
		Trigger:     agentsconfig.TriggerIssueOpened,
		CauseKind:   domain.CauseKindIssueOpened,
		CauseID:     "issue-event-1",
		RepoID:      1,
		IssueNumber: 42,
		ActorID:     1,
	})
	audit := NewAuditor(&AuditorDeps{Runner: h.runner})
	rows, err := audit.ListByIssue(context.Background(), 1, 42)
	if err != nil {
		t.Fatalf("ListByIssue err: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("audit rows = %d, want 1", len(rows))
	}
	r := rows[0]
	if r.RoleKey != "backend" {
		t.Fatalf("role_key = %q", r.RoleKey)
	}
	if r.RepoSHA == "" {
		t.Fatalf("audit row missing repo_sha pin: %q", r.RepoSHA)
	}
	if r.CauseKind != string(domain.CauseKindIssueOpened) {
		t.Fatalf("cause_kind = %q", r.CauseKind)
	}
	if r.CauseID != "issue-event-1" {
		t.Fatalf("cause_id = %q", r.CauseID)
	}
	if len(r.RoleConfig) == 0 {
		t.Fatalf("role_config empty")
	}
}

// TestOnTriggerRewakeResumeFailureMarksFailed verifies that when a rewake
// dispatches an _agent workflow_run that the workflow service rejects,
// the session is marked failed via MarkSessionTerminal — preventing the
// row from sitting silently broken in the admin UI. Under the old
// runner-relay model the failure mode was ResumeSession returning an
// error; under the workflow-cutover model it's CreateAgentRun failing
// (e.g. migration not applied, container spec invalid).
func TestOnTriggerRewakeResumeFailureMarksFailed(t *testing.T) {
	h := newTestSpawner(t, hostSingleRole(), nil)

	// Step 1: spawn a session.
	first, err := h.spawner.OnTrigger(context.Background(), domain.TriggerInput{
		Trigger:     agentsconfig.TriggerIssueOpened,
		CauseKind:   domain.CauseKindIssueOpened,
		CauseID:     "1",
		RepoID:      1,
		IssueNumber: 7,
		ActorID:     1,
	})
	if err != nil {
		t.Fatalf("first OnTrigger err: %v", err)
	}
	if len(first) != 1 || first[0].Action != domain.SpawnActionSpawned {
		t.Fatalf("first call = %+v, want one spawned", first)
	}

	// Step 2: set the session to idle (simulates clean shutdown).
	original := h.runner.sessions[0]
	original.Status = runnerdomain.SessionStatusIdle
	h.workflow.runs[0].Status = workflowdomain.RunStatusSuccess
	h.workflow.jobs[h.workflow.runs[0].ID][0].Status = workflowdomain.JobStatusSuccess

	// Step 3: make the next CreateAgentRun (for this session) fail.
	h.workflow.failOn[original.ID] = fmt.Errorf("simulated workflow dispatch failure")

	// Step 4: trigger rewake with a different cause. OnTrigger swallows
	// per-role errors (recordSpawnError + continue), so the call itself
	// succeeds but returns zero spawned sessions.
	second, err := h.spawner.OnTrigger(context.Background(), domain.TriggerInput{
		Trigger:     agentsconfig.TriggerIssueOpened,
		CauseKind:   domain.CauseKindIssueOpened,
		CauseID:     "2",
		RepoID:      1,
		IssueNumber: 7,
		ActorID:     1,
	})
	if err != nil {
		t.Fatalf("OnTrigger returned unexpected error: %v", err)
	}
	if len(second) != 0 {
		t.Fatalf("got %d sessions, want 0 (rewake failed)", len(second))
	}

	// Step 5: session must be marked failed by MarkSessionTerminal.
	if original.Status != runnerdomain.SessionStatusFailed {
		t.Fatalf("session status = %q, want failed", original.Status)
	}
	if !strings.Contains(original.ErrorMessage, "simulated workflow dispatch failure") {
		t.Fatalf("error message = %q, want it to mention 'simulated workflow dispatch failure'", original.ErrorMessage)
	}

	// Step 6: the input from the failed rewake is in the queue.
	// This is a known residual — no DeleteInput API exists — but the
	// session is now failed so no runner will claim it.
	if len(h.runner.inputs) != 2 {
		t.Fatalf("inputs = %d, want 2 (spawn cause + orphaned rewake cause)", len(h.runner.inputs))
	}
}

// TestOnTriggerSameCauseIdempotentAcrossNonLive verifies that the
// alreadyForCause dedupe now covers all non-archived sessions, not just
// live ones. A retrigger with the same (role, cause_kind, cause_id) on
// an idle/failed/succeeded session is a no-op — no duplicate input.
func TestOnTriggerSameCauseIdempotentAcrossNonLive(t *testing.T) {
	h := newTestSpawner(t, hostSingleRole(), nil)

	// Spawn a session.
	first, err := h.spawner.OnTrigger(context.Background(), domain.TriggerInput{
		Trigger:     agentsconfig.TriggerIssueOpened,
		CauseKind:   domain.CauseKindIssueOpened,
		CauseID:     "unique-99",
		RepoID:      1,
		IssueNumber: 7,
		ActorID:     1,
	})
	if err != nil {
		t.Fatalf("first OnTrigger err: %v", err)
	}
	if len(first) != 1 || first[0].Action != domain.SpawnActionSpawned {
		t.Fatalf("first call = %+v, want one spawned", first)
	}

	// Set the session to idle.
	h.runner.sessions[0].Status = runnerdomain.SessionStatusIdle

	// Retrigger with the SAME cause. The session is idle (non-live), but
	// alreadyForCause now covers all non-archived sessions, so this must
	// be a no-op.
	second, err := h.spawner.OnTrigger(context.Background(), domain.TriggerInput{
		Trigger:     agentsconfig.TriggerIssueOpened,
		CauseKind:   domain.CauseKindIssueOpened,
		CauseID:     "unique-99",
		RepoID:      1,
		IssueNumber: 7,
		ActorID:     1,
	})
	if err != nil {
		t.Fatalf("second OnTrigger err: %v", err)
	}
	if len(second) != 0 {
		t.Fatalf("second call = %d sessions, want 0 (idempotent across non-live)", len(second))
	}

	// Inputs must still be 1 (only the original spawn cause).
	if len(h.runner.inputs) != 1 {
		t.Fatalf("inputs = %d, want 1 (no duplicate from same-cause retry)", len(h.runner.inputs))
	}

	// Also verify it works for failed status.
	h.runner.sessions[0].Status = runnerdomain.SessionStatusFailed
	third, err := h.spawner.OnTrigger(context.Background(), domain.TriggerInput{
		Trigger:     agentsconfig.TriggerIssueOpened,
		CauseKind:   domain.CauseKindIssueOpened,
		CauseID:     "unique-99",
		RepoID:      1,
		IssueNumber: 7,
		ActorID:     1,
	})
	if err != nil {
		t.Fatalf("third OnTrigger err: %v", err)
	}
	if len(third) != 0 {
		t.Fatalf("third call = %d sessions, want 0 (idempotent across failed)", len(third))
	}
}
