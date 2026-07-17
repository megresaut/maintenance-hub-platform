package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"maintenancehub/config"
	"maintenancehub/db"
	"maintenancehub/middleware"
	"maintenancehub/migrate"
	"maintenancehub/modules/activity"
	"maintenancehub/modules/orgs"
	"maintenancehub/modules/properties"
)

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		allowed := strings.HasPrefix(origin, "http://localhost:5175") ||
			strings.HasPrefix(origin, "http://127.0.0.1:5175")
		if allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	config.LoadDotEnv(".env", "../.env")
	orgs.InitJWT()

	ctx := context.Background()
	pool, err := db.Connect(ctx, config.MustGet("DATABASE_URL"))
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()

	migrationsDir := config.Get("MIGRATIONS_DIR", "../migrations")
	if err := migrate.Run(ctx, pool, migrationsDir); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	recorder := activity.NewRecorder(pool)
	_ = recorder // wired into modules as they land

	r := chi.NewRouter()
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(withCORS)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("ok"))
	})

	// Public: auth + inbound webhooks (mounted by modules below).
	r.Mount("/api/auth", orgs.Routes(pool))

	// Authenticated API.
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth)
		r.Mount("/api/properties", properties.Routes(pool))
	})

	port := config.Get("PORT", "8091")
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("maintenance-hub api listening on :%s", port)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
	os.Exit(0)
}
