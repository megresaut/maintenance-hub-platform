package middleware

import (
	"context"
	"net/http"
	"strings"

	"maintenancehub/modules/orgs"
)

type contextKey string

const (
	ContextKeyUserID contextKey = "userID"
	ContextKeyOrgID  contextKey = "orgID"
	ContextKeyRole   contextKey = "userRole"
)

// RequireAuth validates the bearer token and resolves user_id, org_id, and
// role into the request context. org_id is the tenancy boundary — handlers
// must scope every query with it.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var token string
		header := r.Header.Get("Authorization")
		if header != "" && strings.HasPrefix(header, "Bearer ") {
			token = strings.TrimPrefix(header, "Bearer ")
		} else if t := r.URL.Query().Get("token"); t != "" {
			token = t
		} else {
			http.Error(w, "missing auth token", http.StatusUnauthorized)
			return
		}
		claims, err := orgs.ParseJWT(token)
		if err != nil {
			http.Error(w, "invalid or expired token", http.StatusUnauthorized)
			return
		}

		ctx := r.Context()
		ctx = context.WithValue(ctx, ContextKeyUserID, claims.UserID)
		ctx = context.WithValue(ctx, ContextKeyOrgID, claims.OrgID)
		ctx = context.WithValue(ctx, ContextKeyRole, claims.Role)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireRole checks that the user has one of the allowed roles.
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role, _ := r.Context().Value(ContextKeyRole).(string)
			for _, allowed := range roles {
				if role == allowed {
					next.ServeHTTP(w, r)
					return
				}
			}
			http.Error(w, "forbidden", http.StatusForbidden)
		})
	}
}

func GetUserID(ctx context.Context) int64 {
	if v, ok := ctx.Value(ContextKeyUserID).(int64); ok {
		return v
	}
	return 0
}

// GetOrgID extracts the org ID from context (set by RequireAuth).
func GetOrgID(ctx context.Context) int64 {
	if v, ok := ctx.Value(ContextKeyOrgID).(int64); ok {
		return v
	}
	return 0
}

func GetRole(ctx context.Context) string {
	if v, ok := ctx.Value(ContextKeyRole).(string); ok {
		return v
	}
	return ""
}
