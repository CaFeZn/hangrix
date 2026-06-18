package service

import (
	"context"
	"testing"
	"time"

	"github.com/hangrix/hangrix/apps/hangrix/internal/modules/llm_provider/domain"
)

type stubProviderRepo struct {
	providers map[int64]*domain.Provider
}

func (s *stubProviderRepo) CreateProvider(context.Context, *domain.Provider) (*domain.Provider, error) {
	return nil, nil
}
func (s *stubProviderRepo) GetProviderByName(context.Context, string) (*domain.Provider, error) {
	return nil, nil
}
func (s *stubProviderRepo) GetProviderByID(_ context.Context, id int64) (*domain.Provider, error) {
	return s.providers[id], nil
}
func (s *stubProviderRepo) ListProviders(context.Context) ([]*domain.Provider, error) {
	return nil, nil
}
func (s *stubProviderRepo) UpdateProvider(context.Context, *domain.Provider) (*domain.Provider, error) {
	return nil, nil
}
func (s *stubProviderRepo) SetProviderDisabled(context.Context, int64, bool) (*domain.Provider, error) {
	return nil, nil
}
func (s *stubProviderRepo) DeleteProvider(context.Context, int64) error            { return nil }
func (s *stubProviderRepo) RecordUsage(context.Context, *domain.UsageRecord) error { return nil }

type stubGroupRepo struct {
	group   *domain.ModelGroup
	members []*domain.GroupMember
}

func (s *stubGroupRepo) CreateGroup(context.Context, *domain.ModelGroup) (*domain.ModelGroup, error) {
	return nil, nil
}
func (s *stubGroupRepo) GetGroupByName(context.Context, string) (*domain.ModelGroup, error) {
	return s.group, nil
}
func (s *stubGroupRepo) GetGroupByID(context.Context, int64) (*domain.ModelGroup, error) {
	return s.group, nil
}
func (s *stubGroupRepo) ListGroups(context.Context) ([]*domain.ModelGroup, error) { return nil, nil }
func (s *stubGroupRepo) UpdateGroup(context.Context, *domain.ModelGroup) (*domain.ModelGroup, error) {
	return nil, nil
}
func (s *stubGroupRepo) DeleteGroup(context.Context, int64) error { return nil }
func (s *stubGroupRepo) ReplaceMembers(context.Context, int64, []*domain.GroupMember) error {
	return nil
}
func (s *stubGroupRepo) ListMembersByGroupID(context.Context, int64) ([]*domain.GroupMember, error) {
	return s.members, nil
}
func (s *stubGroupRepo) ListMembersByProviderID(context.Context, int64) ([]*domain.GroupMember, error) {
	return nil, nil
}
func (s *stubGroupRepo) GetMemberByID(context.Context, int64) (*domain.GroupMember, error) {
	return nil, nil
}
func (s *stubGroupRepo) UpdateMemberHealth(context.Context, int64, domain.HealthPatch) error {
	return nil
}
func (s *stubGroupRepo) CountGroupsByName(context.Context, string) (int64, error) { return 0, nil }

type stubModelRepo struct{ domain.ModelRepo }

func TestResolveModel_UsesSoftFallbackWhenAllMembersAutoDisabled(t *testing.T) {
	t.Parallel()

	now := time.Now()
	later := now.Add(10 * time.Minute)
	soon := now.Add(2 * time.Minute)
	router := NewGroupRouter(&GroupRouterDeps{
		Repo: &stubProviderRepo{providers: map[int64]*domain.Provider{
			2: {ID: 2, Name: "codex", Type: domain.ProviderTypeOpenAICompat, BaseURL: "http://sub2api"},
		}},
		GroupRepo: &stubGroupRepo{
			group: &domain.ModelGroup{ID: 1, Name: "reviewer"},
			members: []*domain.GroupMember{
				{ID: 10, GroupID: 1, ProviderID: 2, Model: "gpt-5.5", Priority: 0, AutoDisabledUntil: &later},
				{ID: 11, GroupID: 1, ProviderID: 2, Model: "gpt-5.4", Priority: 1, AutoDisabledUntil: &soon},
			},
		},
		ModelRepo: stubModelRepo{},
	})

	res, err := router.ResolveModel(context.Background(), "reviewer")
	if err != nil {
		t.Fatalf("ResolveModel() error = %v", err)
	}
	if len(res.Candidates) != 1 {
		t.Fatalf("ResolveModel() candidates = %d, want 1", len(res.Candidates))
	}
	if got := res.Candidates[0].MemberID; got != 11 {
		t.Fatalf("soft fallback member = %d, want 11", got)
	}
	if got := res.Candidates[0].Model; got != "gpt-5.4" {
		t.Fatalf("soft fallback model = %q, want gpt-5.4", got)
	}
}
