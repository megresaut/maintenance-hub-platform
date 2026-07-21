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

	"maintenancehub/comms"
	"maintenancehub/config"
	"maintenancehub/db"
	"maintenancehub/middleware"
	"maintenancehub/migrate"
	"maintenancehub/modules/activity"
	aiclient "maintenancehub/modules/ai/client"
	"maintenancehub/modules/ai/drafts"
	aiservice "maintenancehub/modules/ai/service"
	"maintenancehub/modules/calendar"
	ftohttp "maintenancehub/modules/maintenance/fto/http"
	ftorepo "maintenancehub/modules/maintenance/fto/repository"
	ftoservice "maintenancehub/modules/maintenance/fto/service"
	recurringhttp "maintenancehub/modules/maintenance/recurring/http"
	taskcategorieshttp "maintenancehub/modules/maintenance/taskcategories/http"
	taskshttp "maintenancehub/modules/maintenance/tasks/http"
	taskrepo "maintenancehub/modules/maintenance/tasks/repository"
	taskservice "maintenancehub/modules/maintenance/tasks/service"
	workorderhttp "maintenancehub/modules/maintenance/work_order/http"
	worepo "maintenancehub/modules/maintenance/work_order/repository"
	woservice "maintenancehub/modules/maintenance/work_order/service"
	"maintenancehub/modules/orgs"
	"maintenancehub/modules/properties"
	"maintenancehub/modules/schedule"
	"maintenancehub/modules/sms"
	"maintenancehub/modules/vendors"
	"maintenancehub/modules/vendors/outreach"
)

// corsMiddleware allows the two local dev origins plus any exact origins listed
// in the CORS_ORIGINS env var (comma-separated, e.g. the deployed web URL).
func corsMiddleware(configured string) func(http.Handler) http.Handler {
	allow := map[string]bool{
		"http://localhost:5175": true,
		"http://127.0.0.1:5175": true,
	}
	for _, o := range strings.Split(configured, ",") {
		if o = strings.TrimSpace(o); o != "" {
			allow[o] = true
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && allow[origin] {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
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

	// --- shared infrastructure ---
	recorder := activity.NewRecorder(pool)
	twilio := comms.NewTwilioClient(os.Getenv("TWILIO_ACCOUNT_SID"), os.Getenv("TWILIO_AUTH_TOKEN"))
	smtp := comms.NewSMTPClient(os.Getenv("SMTP_HOST"), config.Get("SMTP_PORT", "587"),
		os.Getenv("SMTP_USERNAME"), os.Getenv("SMTP_PASSWORD"))

	// --- AI classification (OpenRouter preferred when configured; else Anthropic) ---
	var aiClient *aiclient.Client
	var aiModel string
	switch {
	case os.Getenv("OPENROUTER_API_KEY") != "":
		aiModel = config.Get("OPENROUTER_MODEL", config.Get("AI_MODEL", "google/gemini-2.5-flash-lite"))
		aiClient = aiclient.NewOpenRouter(os.Getenv("OPENROUTER_API_KEY"), aiModel)
		log.Printf("AI provider: OpenRouter (model %s)", aiModel)
	case os.Getenv("ANTHROPIC_API_KEY") != "":
		aiModel = config.Get("AI_MODEL", "claude-haiku-4-5-20251001")
		aiClient = aiclient.New(os.Getenv("ANTHROPIC_API_KEY"))
		log.Printf("AI provider: Anthropic (model %s)", aiModel)
	default:
		log.Printf("WARNING: no AI key (OPENROUTER_API_KEY or ANTHROPIC_API_KEY) set — AI classification and quote parsing disabled")
	}
	contextLoader := aiservice.NewContextLoader(pool)
	var classifier *aiservice.AIClassifier
	if aiClient != nil {
		classifier = aiservice.NewAIClassifier(aiClient, contextLoader)
	}

	// --- maintenance services (also used by the drafts adapter) ---
	taskRepo := taskrepo.NewTaskRepositoryPG(pool)
	taskSvc := taskservice.NewTaskService(taskRepo, recorder)
	woSvc := woservice.NewWorkOrderService(worepo.NewWorkOrderRepositoryPG(pool), taskRepo, recorder)
	ftoSvc := ftoservice.NewFTOService(ftorepo.NewFTORepositoryPG(pool), taskRepo, recorder)

	// --- drafts review queue ---
	draftRepo := drafts.NewRepo(pool)
	creator := &ticketCreator{db: pool, taskSvc: taskSvc, woSvc: woSvc, ftoSvc: ftoSvc}
	draftSvc := drafts.NewService(draftRepo, creator)

	// --- vendors + outreach (flagship) ---
	vendorRepo := vendors.NewRepo(pool)
	outreachRepo := outreach.NewRepo(pool)
	outreachSvc := outreach.NewService(pool, outreachRepo, vendorRepo, twilio, smtp, aiClient, recorder)

	// --- SMS intake ---
	smsRepo := sms.NewRepo(pool)
	smsSvc := sms.NewService(sms.Deps{
		Repo: smsRepo, Classifier: classifier, DraftSvc: draftSvc, Twilio: twilio, DB: pool,
	})
	// Vendor replies to outreach intercept inbound SMS before intake classification.
	smsSvc.SetReplyRouter(outreachSvc)
	smsSvc.StartPoller(ctx)

	// --- calendar (Outlook) ---
	var draftCreator calendar.DraftCreator
	if classifier != nil {
		draftCreator = &calendarDraftCreator{
			classifier: classifier, draftSvc: draftSvc,
			model: aiModel,
		}
	}
	calSvc := calendar.NewService(pool, config.Get("CALENDAR_WEBHOOK_URL", ""), draftCreator)

	// --- recurring poller (materializes due recurring tasks) ---
	go recurringhttp.StartPoller(ctx, pool, recorder, 5*time.Minute)

	// --- router ---
	r := chi.NewRouter()
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(corsMiddleware(config.Get("CORS_ORIGINS", "")))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("ok"))
	})

	// PUBLIC: auth + provider webhooks (they can't send JWTs; each validates
	// its own authenticity).
	r.Mount("/api/auth", orgs.Routes(pool))
	r.Mount("/api/sms", sms.WebhookRoutes(smsSvc, twilio))
	r.Mount("/api/calendar/webhook", calendar.WebhookRoutes(calSvc))
	r.Mount("/api/outreach/email", outreach.EmailWebhookRoutes(outreachSvc, pool))

	// AUTHENTICATED API.
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth)
		r.Mount("/api/properties", properties.Routes(pool))
		r.Mount("/api/vendors", vendors.Routes(vendorRepo))
		r.Mount("/api/preferred-vendors", vendors.PreferredRoutes(vendorRepo))
		r.Mount("/api/drafts", drafts.Routes(draftSvc))
		r.Mount("/api/tasks", taskshttp.Routes(pool, recorder))
		r.Mount("/api/task-categories", taskcategorieshttp.Routes(pool))
		r.Mount("/api/work-orders", workorderhttp.Routes(pool, recorder))
		r.Mount("/api/ftos", ftohttp.Routes(pool, recorder))
		r.Mount("/api/recurring", recurringhttp.Routes(pool, recorder))
		r.Mount("/api/outreach", outreach.Routes(outreachSvc, outreachRepo))
		r.Mount("/api/activity", activity.Routes(recorder))
		r.Mount("/api/schedule", schedule.Routes(pool))
		r.Mount("/api/sms-admin", sms.Routes(smsSvc, smsRepo))
		r.Mount("/api/calendar", calendar.Routes(calSvc))
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
}
