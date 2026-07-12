package auth

import (
	"context"

	"github.com/mc-panel/control-plane/internal/store"
)

type userCtxKey struct{}

func WithUser(ctx context.Context, u *store.User) context.Context {
	return context.WithValue(ctx, userCtxKey{}, u)
}

func UserFrom(ctx context.Context) *store.User {
	u, _ := ctx.Value(userCtxKey{}).(*store.User)
	return u
}
