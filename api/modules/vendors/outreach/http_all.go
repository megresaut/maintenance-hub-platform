package outreach

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"maintenancehub/httpx"
	"maintenancehub/middleware"
)

// Thread is one outreach request with its work-order context — the row shape
// for the org-wide Outreach Center.
type Thread struct {
	Request
	WorkOrderName   string     `json:"work_order_name"`
	WorkOrderStatus string     `json:"work_order_status"`
	PropertyName    string     `json:"property_name"`
	Trade           string     `json:"trade"`
	LatestReplyAt   *time.Time `json:"latest_reply_at,omitempty"`
}

// AllRoutes adds the org-wide listing (mounted under /api/outreach alongside
// the per-work-order routes).
func AllRoutes(r chi.Router, repo *Repo, db *pgxpool.Pool) {
	r.Get("/all", func(w http.ResponseWriter, req *http.Request) {
		orgID := middleware.GetOrgID(req.Context())
		rows, err := db.Query(req.Context(), `
			SELECT `+reqCols+`, v.name, v.phone, v.primary_email,
			       wo.name, wo.status, COALESCE(wo.category,''), COALESCE(p.name,'')
			FROM vendor_outreach_requests r
			JOIN vendors v ON v.id = r.vendor_id
			JOIN work_orders wo ON wo.id = r.work_order_id
			LEFT JOIN properties p ON p.id = wo.property_id
			WHERE r.org_id = $1
			ORDER BY r.created_at DESC
			LIMIT 500`, orgID)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		out := []*Thread{}
		for rows.Next() {
			var t Thread
			if err := rows.Scan(&t.ID, &t.OrgID, &t.WorkOrderID, &t.VendorID, &t.Channel,
				&t.ToAddress, &t.MessageBody, &t.ProviderRef, &t.Status, &t.Error,
				&t.SentAt, &t.CreatedAt, &t.VendorName, &t.VendorPhone, &t.VendorEmail,
				&t.WorkOrderName, &t.WorkOrderStatus, &t.Trade, &t.PropertyName); err != nil {
				httpx.Error(w, http.StatusInternalServerError, err.Error())
				return
			}
			out = append(out, &t)
		}
		if err := rows.Err(); err != nil {
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}

		for _, t := range out {
			replies, err := repo.ListReplies(req.Context(), t.ID)
			if err != nil {
				httpx.Error(w, http.StatusInternalServerError, err.Error())
				return
			}
			t.Replies = replies
			if len(replies) > 0 {
				last := replies[len(replies)-1].ReceivedAt
				t.LatestReplyAt = &last
			}
		}
		httpx.JSON(w, http.StatusOK, out)
	})
}
