package rbac

import (
	"context"
	"net/http"

	"github.com/cyxc1124/osspilot-tenant-api/internal/auth"
	"github.com/cyxc1124/osspilot-tenant-api/internal/httpx"
)

type Checker struct {
	store *Store
}

func NewChecker(store *Store) *Checker {
	if store == nil {
		return nil
	}
	return &Checker{store: store}
}

func (c *Checker) Forbidden(w http.ResponseWriter, r *http.Request, user *auth.User, bucketName, prefix, action string) bool {
	if c == nil || auth.IsAdmin(user) {
		return false
	}
	ok, err := c.Allow(r.Context(), user, bucketName, prefix, action)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return true
	}
	if !ok {
		httpx.Error(w, http.StatusForbidden, "Permission denied")
		return true
	}
	return false
}

func (c *Checker) Allow(ctx context.Context, user *auth.User, bucketName, prefix, action string) (bool, error) {
	if c == nil || auth.IsAdmin(user) {
		return true, nil
	}
	accountID := auth.AccountID(user)
	gids, err := c.store.GroupIDs(ctx, accountID, user.ID)
	if err != nil {
		return false, err
	}
	rules, err := c.store.LoadRules(ctx, accountID, user.ID, user.Role)
	if err != nil {
		return false, err
	}
	return Allowed(user.ID, user.Role, gids, bucketName, prefix, action, rules), nil
}
