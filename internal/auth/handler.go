package auth

import (
	"net/http"
	"strings"
	"time"

	"github.com/cyxc1124/osspilot-tenant-api/internal/httpx"
)

type Handler struct {
	store  *Store
	secret string
	ttl    time.Duration
}

func NewHandler(store *Store, secret string, ttl time.Duration) *Handler {
	return &Handler{store: store, secret: secret, ttl: ttl}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/login", h.login)
	mux.HandleFunc("POST /api/logout", h.RequireUser(h.logout))
	mux.HandleFunc("GET /api/me", h.RequireUser(h.me))
	mux.HandleFunc("POST /api/password/change", h.RequireUser(h.changePassword))
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Portal   string `json:"portal"`
}

type loginResponse struct {
	AccessToken        string `json:"access_token"`
	TokenType          string `json:"token_type"`
	ExpiresIn          int    `json:"expires_in"`
	MustChangePassword bool   `json:"must_change_password"`
}

type meResponse struct {
	ID                 int64   `json:"id"`
	Username           string  `json:"username"`
	DisplayName        *string `json:"display_name"`
	Email              *string `json:"email"`
	Phone              *string `json:"phone"`
	Role               *string `json:"role"`
	MustChangePassword bool    `json:"must_change_password"`
}

type passwordChangeRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database is not configured")
		return
	}
	var req loginRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Username == "" || req.Password == "" {
		httpx.Error(w, http.StatusBadRequest, "username and password are required")
		return
	}
	if req.Portal != "" && req.Portal != PortalTenant {
		httpx.Error(w, http.StatusBadRequest, "portal must be tenant")
		return
	}

	user, err := h.store.GetByUsername(r.Context(), req.Username)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	if user == nil || !CheckPassword(req.Password, user.PasswordHash) {
		httpx.Error(w, http.StatusUnauthorized, "Invalid username or password")
		return
	}
	if user.Status != "active" {
		httpx.Error(w, http.StatusForbidden, "Account is disabled")
		return
	}
	if user.Role == "" {
		httpx.Error(w, http.StatusForbidden, "No tenant portal access")
		return
	}

	token, err := IssueToken(h.secret, h.ttl, user.ID, user.Role)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "token error")
		return
	}
	_ = h.store.TouchLogin(r.Context(), user.ID, time.Now())
	httpx.JSON(w, http.StatusOK, loginResponse{
		AccessToken:        token,
		TokenType:          "bearer",
		ExpiresIn:          int(h.ttl.Seconds()),
		MustChangePassword: user.MustChangePassword,
	})
}

func (h *Handler) logout(w http.ResponseWriter, _ *http.Request, _ *User) {
	httpx.JSON(w, http.StatusOK, map[string]string{"message": "logged out"})
}

func (h *Handler) me(w http.ResponseWriter, _ *http.Request, user *User) {
	role := user.Role
	httpx.JSON(w, http.StatusOK, meResponse{
		ID:                 user.ID,
		Username:           user.Username,
		DisplayName:        user.DisplayName,
		Email:              user.Email,
		Phone:              user.Phone,
		Role:               &role,
		MustChangePassword: user.MustChangePassword,
	})
}

func (h *Handler) changePassword(w http.ResponseWriter, r *http.Request, user *User) {
	var req passwordChangeRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.OldPassword == "" || req.NewPassword == "" {
		httpx.Error(w, http.StatusBadRequest, "old_password and new_password are required")
		return
	}
	if len(req.NewPassword) < 8 || len(req.NewPassword) > 128 {
		httpx.Error(w, http.StatusBadRequest, "new_password must be 8-128 characters")
		return
	}
	if req.OldPassword == req.NewPassword {
		httpx.Error(w, http.StatusBadRequest, "new_password must differ from old_password")
		return
	}
	if !CheckPassword(req.OldPassword, user.PasswordHash) {
		httpx.Error(w, http.StatusBadRequest, "Current password is incorrect")
		return
	}
	hash, err := HashPassword(req.NewPassword)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "hash error")
		return
	}
	if err := h.store.UpdatePassword(r.Context(), user.ID, hash, time.Now()); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type UserHandler func(http.ResponseWriter, *http.Request, *User)

func (h *Handler) RequireUser(next UserHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.store == nil {
			httpx.Error(w, http.StatusServiceUnavailable, "database is not configured")
			return
		}
		raw := bearerToken(r.Header.Get("Authorization"))
		if raw == "" {
			httpx.Error(w, http.StatusUnauthorized, "missing token")
			return
		}
		claims, err := ParseToken(h.secret, raw)
		if err != nil {
			httpx.Error(w, http.StatusUnauthorized, "invalid token")
			return
		}
		user, err := h.store.GetByID(r.Context(), claims.UserID)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "database error")
			return
		}
		if user == nil || user.Status != "active" {
			httpx.Error(w, http.StatusUnauthorized, "invalid token")
			return
		}
		if user.MustChangePassword && r.URL.Path != "/api/password/change" && r.URL.Path != "/api/me" && r.URL.Path != "/api/logout" {
			httpx.Error(w, http.StatusForbidden, "password change required")
			return
		}
		next(w, r, user)
	}
}

func bearerToken(header string) string {
	const p = "Bearer "
	if !strings.HasPrefix(header, p) {
		return ""
	}
	return strings.TrimSpace(header[len(p):])
}
