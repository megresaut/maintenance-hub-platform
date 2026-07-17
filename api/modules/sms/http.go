package sms

import (
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"maintenancehub/comms"
	"maintenancehub/httpx"
	"maintenancehub/middleware"
)

// WebhookRoutes are PUBLIC (Twilio can't send a JWT) — authenticity is
// checked via the AccountSid on the form body.
func WebhookRoutes(svc *Service, twilio *comms.TwilioClient) chi.Router {
	r := chi.NewRouter()
	r.Post("/webhook", func(w http.ResponseWriter, req *http.Request) {
		if twilio.Configured() && !twilio.ValidateWebhook(req) {
			http.Error(w, "invalid signature", http.StatusForbidden)
			return
		}
		if err := req.ParseForm(); err != nil {
			http.Error(w, "invalid form data", http.StatusBadRequest)
			return
		}

		from := req.FormValue("From")
		to := req.FormValue("To")
		body := req.FormValue("Body")
		messageSid := req.FormValue("MessageSid")
		mmsSubject := req.FormValue("Subject")

		var mediaURLs []string
		numMedia, _ := strconv.Atoi(req.FormValue("NumMedia"))
		for i := 0; i < numMedia; i++ {
			if u := req.FormValue(fmt.Sprintf("MediaUrl%d", i)); u != "" {
				mediaURLs = append(mediaURLs, u)
			}
		}

		log.Printf("[sms] inbound from=%s to=%s subject=%q body=%q media=%d", from, to, mmsSubject, body, numMedia)

		if err := svc.HandleInboundMessage(req.Context(), from, to, body, messageSid, mmsSubject, mediaURLs); err != nil {
			log.Printf("[sms] handle inbound error: %v", err)
			// Still 200 so Twilio doesn't retry.
		}

		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><Response></Response>`)
	})
	return r
}

// Routes are the authenticated management endpoints.
func Routes(svc *Service, repo *Repo) chi.Router {
	r := chi.NewRouter()

	r.Get("/conversations", func(w http.ResponseWriter, req *http.Request) {
		orgID := middleware.GetOrgID(req.Context())
		convs, err := repo.ListConversations(req.Context(), orgID)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.JSON(w, http.StatusOK, convs)
	})

	r.Get("/conversations/{id}/messages", func(w http.ResponseWriter, req *http.Request) {
		orgID := middleware.GetOrgID(req.Context())
		id, _ := strconv.ParseInt(chi.URLParam(req, "id"), 10, 64)
		conv, err := repo.GetConversationByID(req.Context(), orgID, id)
		if err != nil || conv == nil {
			httpx.Error(w, http.StatusNotFound, "conversation not found")
			return
		}
		msgs, err := repo.GetRecentMessages(req.Context(), id, 200)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.JSON(w, http.StatusOK, msgs)
	})

	r.Post("/send", func(w http.ResponseWriter, req *http.Request) {
		orgID := middleware.GetOrgID(req.Context())
		var in struct {
			To   string `json:"to"`
			Body string `json:"body"`
		}
		if !httpx.Decode(w, req, &in) {
			return
		}
		if in.To == "" || in.Body == "" {
			httpx.Error(w, http.StatusBadRequest, "to and body are required")
			return
		}
		if err := svc.SendSMS(req.Context(), orgID, in.To, in.Body); err != nil {
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]bool{"ok": true})
	})

	r.Post("/groups", func(w http.ResponseWriter, req *http.Request) {
		orgID := middleware.GetOrgID(req.Context())
		var in struct {
			Name         string   `json:"name"`
			Participants []string `json:"participants"`
		}
		if !httpx.Decode(w, req, &in) {
			return
		}
		conv, err := repo.CreateGroupConversation(req.Context(), orgID, in.Name, in.Participants)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.JSON(w, http.StatusCreated, conv)
	})

	return r
}
