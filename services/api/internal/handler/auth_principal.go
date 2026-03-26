package handler

import (
	"context"

	"github.com/google/uuid"
)

type ctxKey int

const (
	ctxAuthUserID ctxKey = iota + 1
	ctxAuthRole
	ctxAuthPrincipal
)

func withAuth(ctx context.Context, userID uuid.UUID, role, principal string) context.Context {
	ctx = context.WithValue(ctx, ctxAuthUserID, userID)
	ctx = context.WithValue(ctx, ctxAuthRole, role)
	return context.WithValue(ctx, ctxAuthPrincipal, principal)
}

func AuthUserID(ctx context.Context) (uuid.UUID, bool) {
	v := ctx.Value(ctxAuthUserID)
	if v == nil {
		return uuid.Nil, false
	}
	id, ok := v.(uuid.UUID)
	return id, ok
}

func AuthRole(ctx context.Context) (string, bool) {
	v := ctx.Value(ctxAuthRole)
	if v == nil {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func AuthPrincipal(ctx context.Context) (string, bool) {
	v := ctx.Value(ctxAuthPrincipal)
	if v == nil {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}
