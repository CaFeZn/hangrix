package handler

import (
	"context"
	"errors"
	"fmt"

	actordomain "github.com/hangrix/hangrix/apps/hangrix/internal/modules/actor/domain"
	userdomain "github.com/hangrix/hangrix/apps/hangrix/internal/modules/user/domain"
	"github.com/hangrix/hangrix/pkg/actor"
)

func currentUserActorID(ctx context.Context, resolver actordomain.Resolver, caller *userdomain.User) (int64, error) {
	if caller == nil {
		return 0, errors.New("authenticated user missing from request")
	}
	if resolver == nil {
		return 0, errors.New("actor resolver unavailable")
	}
	resolved, err := resolver.From(ctx, actor.UserRef(caller.ID, caller.Username))
	if err != nil {
		return 0, fmt.Errorf("resolve user actor: %w", err)
	}
	if resolved == nil || resolved.ActorID <= 0 {
		return 0, errors.New("resolve user actor: empty actor id")
	}
	return resolved.ActorID, nil
}
