package properties

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"maintenancehub/httpx"
	"maintenancehub/middleware"
)

// Property is deliberately thin per the plan: org_id, name, address.
type Property struct {
	ID        int64     `json:"id"`
	OrgID     int64     `json:"org_id"`
	Name      string    `json:"name"`
	Address   string    `json:"address"`
	CreatedAt time.Time `json:"created_at"`
}

func Routes(db *pgxpool.Pool) chi.Router {
	r := chi.NewRouter()
	r.Get("/", list(db))
	r.Post("/", create(db))
	r.Get("/{id}", get(db))
	r.Put("/{id}", update(db))
	r.Delete("/{id}", remove(db))
	return r
}

func list(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.GetOrgID(r.Context())
		rows, err := db.Query(r.Context(), `
			SELECT id, org_id, name, address, created_at
			FROM properties WHERE org_id = $1 ORDER BY name`, orgID)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()
		out := []Property{}
		for rows.Next() {
			var p Property
			if err := rows.Scan(&p.ID, &p.OrgID, &p.Name, &p.Address, &p.CreatedAt); err != nil {
				httpx.Error(w, http.StatusInternalServerError, err.Error())
				return
			}
			out = append(out, p)
		}
		httpx.JSON(w, http.StatusOK, out)
	}
}

type propertyInput struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

func create(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.GetOrgID(r.Context())
		var in propertyInput
		if !httpx.Decode(w, r, &in) {
			return
		}
		if in.Name == "" {
			httpx.Error(w, http.StatusBadRequest, "name is required")
			return
		}
		var p Property
		err := db.QueryRow(r.Context(), `
			INSERT INTO properties (org_id, name, address)
			VALUES ($1, $2, $3)
			RETURNING id, org_id, name, address, created_at`,
			orgID, in.Name, in.Address,
		).Scan(&p.ID, &p.OrgID, &p.Name, &p.Address, &p.CreatedAt)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.JSON(w, http.StatusCreated, p)
	}
}

func get(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.GetOrgID(r.Context())
		id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		var p Property
		err := db.QueryRow(r.Context(), `
			SELECT id, org_id, name, address, created_at
			FROM properties WHERE id = $1 AND org_id = $2`, id, orgID,
		).Scan(&p.ID, &p.OrgID, &p.Name, &p.Address, &p.CreatedAt)
		if err != nil {
			httpx.Error(w, http.StatusNotFound, "property not found")
			return
		}
		httpx.JSON(w, http.StatusOK, p)
	}
}

func update(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.GetOrgID(r.Context())
		id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		var in propertyInput
		if !httpx.Decode(w, r, &in) {
			return
		}
		ct, err := db.Exec(r.Context(), `
			UPDATE properties SET name = $1, address = $2
			WHERE id = $3 AND org_id = $4`,
			in.Name, in.Address, id, orgID)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		if ct.RowsAffected() == 0 {
			httpx.Error(w, http.StatusNotFound, "property not found")
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

func remove(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.GetOrgID(r.Context())
		id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		ct, err := db.Exec(r.Context(),
			`DELETE FROM properties WHERE id = $1 AND org_id = $2`, id, orgID)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		if ct.RowsAffected() == 0 {
			httpx.Error(w, http.StatusNotFound, "property not found")
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}
