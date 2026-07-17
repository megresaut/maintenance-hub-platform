package vendors

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"maintenancehub/httpx"
	"maintenancehub/middleware"
)

func Routes(repo *Repo) chi.Router {
	r := chi.NewRouter()

	r.Get("/", func(w http.ResponseWriter, req *http.Request) {
		orgID := middleware.GetOrgID(req.Context())
		list, err := repo.List(req.Context(), orgID, req.URL.Query().Get("category"))
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.JSON(w, http.StatusOK, list)
	})

	r.Post("/", func(w http.ResponseWriter, req *http.Request) {
		orgID := middleware.GetOrgID(req.Context())
		var in VendorInput
		if !httpx.Decode(w, req, &in) {
			return
		}
		if strings.TrimSpace(in.Name) == "" {
			httpx.Error(w, http.StatusBadRequest, "name is required")
			return
		}
		v, err := repo.Create(req.Context(), orgID, in)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.JSON(w, http.StatusCreated, v)
	})

	r.Get("/{id}", func(w http.ResponseWriter, req *http.Request) {
		orgID := middleware.GetOrgID(req.Context())
		id, _ := strconv.ParseInt(chi.URLParam(req, "id"), 10, 64)
		v, err := repo.GetByID(req.Context(), orgID, id)
		if err != nil || v == nil {
			httpx.Error(w, http.StatusNotFound, "vendor not found")
			return
		}
		httpx.JSON(w, http.StatusOK, v)
	})

	r.Put("/{id}", func(w http.ResponseWriter, req *http.Request) {
		orgID := middleware.GetOrgID(req.Context())
		id, _ := strconv.ParseInt(chi.URLParam(req, "id"), 10, 64)
		var in VendorInput
		if !httpx.Decode(w, req, &in) {
			return
		}
		v, err := repo.Update(req.Context(), orgID, id, in)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.JSON(w, http.StatusOK, v)
	})

	r.Delete("/{id}", func(w http.ResponseWriter, req *http.Request) {
		orgID := middleware.GetOrgID(req.Context())
		id, _ := strconv.ParseInt(chi.URLParam(req, "id"), 10, 64)
		if err := repo.Delete(req.Context(), orgID, id); err != nil {
			httpx.Error(w, http.StatusNotFound, err.Error())
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]bool{"ok": true})
	})

	// GET /lookup?category=plumbing&area_tags=stamford,greenwich — the
	// local-directory fallback search used by outreach shortlisting.
	r.Get("/lookup", func(w http.ResponseWriter, req *http.Request) {
		orgID := middleware.GetOrgID(req.Context())
		category := req.URL.Query().Get("category")
		var tags []string
		if t := req.URL.Query().Get("area_tags"); t != "" {
			tags = strings.Split(t, ",")
		}
		list, err := repo.LookupLocal(req.Context(), orgID, category, tags)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.JSON(w, http.StatusOK, list)
	})

	return r
}

func PreferredRoutes(repo *Repo) chi.Router {
	r := chi.NewRouter()

	r.Get("/", func(w http.ResponseWriter, req *http.Request) {
		orgID := middleware.GetOrgID(req.Context())
		list, err := repo.ListPreferredEntries(req.Context(), orgID)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.JSON(w, http.StatusOK, list)
	})

	r.Post("/", func(w http.ResponseWriter, req *http.Request) {
		orgID := middleware.GetOrgID(req.Context())
		var in struct {
			PropertyID int64  `json:"property_id"`
			Category   string `json:"category"`
			VendorID   int64  `json:"vendor_id"`
			Priority   int    `json:"priority"`
		}
		if !httpx.Decode(w, req, &in) {
			return
		}
		if in.PropertyID == 0 || in.VendorID == 0 || in.Category == "" {
			httpx.Error(w, http.StatusBadRequest, "property_id, vendor_id, and category are required")
			return
		}
		if in.Priority == 0 {
			in.Priority = 1
		}
		pv, err := repo.AddPreferred(req.Context(), orgID, in.PropertyID, in.Category, in.VendorID, in.Priority)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.JSON(w, http.StatusCreated, pv)
	})

	r.Delete("/{id}", func(w http.ResponseWriter, req *http.Request) {
		orgID := middleware.GetOrgID(req.Context())
		id, _ := strconv.ParseInt(chi.URLParam(req, "id"), 10, 64)
		if err := repo.RemovePreferred(req.Context(), orgID, id); err != nil {
			httpx.Error(w, http.StatusNotFound, err.Error())
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]bool{"ok": true})
	})

	return r
}
