package orgs

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"maintenancehub/httpx"
)

// Routes mounts public auth routes (login) — org/user provisioning is
// CLI-driven per the plan (no self-serve signup).
func Routes(db *pgxpool.Pool) chi.Router {
	r := chi.NewRouter()
	r.Post("/login", loginHandler(db))
	return r
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token   string `json:"token"`
	OrgID   int64  `json:"org_id"`
	OrgName string `json:"org_name"`
	Email   string `json:"email"`
	Role    string `json:"role"`
}

func loginHandler(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req loginRequest
		if !httpx.Decode(w, r, &req) {
			return
		}

		var (
			userID       int64
			orgID        int64
			orgName      string
			passwordHash string
			role         string
		)
		err := db.QueryRow(r.Context(), `
			SELECT u.id, u.org_id, o.name, u.password_hash, u.role
			FROM org_users u
			JOIN organizations o ON o.id = u.org_id
			WHERE u.email = $1`, req.Email,
		).Scan(&userID, &orgID, &orgName, &passwordHash, &role)
		if err != nil {
			httpx.Error(w, http.StatusUnauthorized, "invalid email or password")
			return
		}
		if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)) != nil {
			httpx.Error(w, http.StatusUnauthorized, "invalid email or password")
			return
		}

		token, err := CreateJWT(userID, orgID, role)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "failed to create token")
			return
		}
		httpx.JSON(w, http.StatusOK, loginResponse{
			Token: token, OrgID: orgID, OrgName: orgName, Email: req.Email, Role: role,
		})
	}
}
