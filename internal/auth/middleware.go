package auth

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/axmipic/axmipic/internal/store"
)

// Authenticator resolves Bearer credentials (API tokens or session JWTs) into a
// Principal. It does not reject anonymous requests; use RequireAuth to enforce.
type Authenticator struct {
	repo   *store.Repository
	issuer *SessionIssuer
}

// NewAuthenticator creates an Authenticator.
func NewAuthenticator(repo *store.Repository, issuer *SessionIssuer) *Authenticator {
	return &Authenticator{repo: repo, issuer: issuer}
}

// Authenticate is optional authentication: a valid credential is attached to
// the request context, an invalid one is rejected, and no credential passes
// through as a guest.
func (a *Authenticator) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		credential := bearerToken(r)
		if credential == "" {
			next.ServeHTTP(w, r)
			return
		}
		principal, err := a.resolve(r, credential)
		if err != nil {
			writeAuthError(w, http.StatusUnauthorized, "invalid or expired credential")
			return
		}
		next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), principal)))
	})
}

func (a *Authenticator) resolve(r *http.Request, credential string) (*Principal, error) {
	ctx := r.Context()
	if strings.HasPrefix(credential, TokenPrefix) {
		token, err := a.repo.GetTokenByHash(ctx, HashToken(credential))
		if err != nil {
			return nil, ErrUnauthenticated
		}
		if token.ExpiresAt != nil && time.Now().After(*token.ExpiresAt) {
			return nil, ErrUnauthenticated
		}
		user, err := a.repo.GetUserByID(ctx, token.UserID)
		if err != nil {
			return nil, ErrUnauthenticated
		}
		if err := a.repo.TouchToken(ctx, token.ID); err != nil {
			// A failed last-used update must not block an otherwise valid request.
			_ = err
		}
		return &Principal{
			UserID:   user.ID,
			Username: user.Username,
			Role:     Role(user.Role),
			TokenID:  token.ID,
		}, nil
	}

	userID, role, err := a.issuer.Parse(credential)
	if err != nil {
		return nil, err
	}
	user, err := a.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, ErrUnauthenticated
	}
	return &Principal{UserID: user.ID, Username: user.Username, Role: Role(role)}, nil
}

// RequireAuth rejects requests without an authenticated, non-guest principal.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := FromContext(r.Context())
		if !ok || principal.IsGuest() {
			writeAuthError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func bearerToken(r *http.Request) string {
	const prefix = "Bearer "
	header := r.Header.Get("Authorization")
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}

// writeAuthError emits an API error envelope. The auth package writes it
// directly to avoid depending on the api package.
func writeAuthError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"code":    status,
		"message": message,
		"data":    nil,
	})
}
