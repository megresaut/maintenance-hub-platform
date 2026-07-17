package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	workorder "maintenancehub/modules/maintenance/work_order"
)

type WorkOrderRepositoryPG struct {
	db *pgxpool.Pool
}

func NewWorkOrderRepositoryPG(db *pgxpool.Pool) *WorkOrderRepositoryPG {
	return &WorkOrderRepositoryPG{db: db}
}

const woColumns = `
	wo.id, wo.org_id, wo.task_id, wo.property_id, wo.vendor_id,
	wo.name, wo.work_description, wo.vendor_notes, wo.internal_notes,
	wo.status, wo.priority, wo.due_date, wo.entry_preference,
	wo.quote_amount_cents, wo.actual_cost,
	wo.event_start_at, wo.event_end_at, wo.dispatched_at,
	wo.completed_at, wo.completed_by, wo.created_by,
	wo.created_at, wo.updated_at,
	p.name AS property_name,
	t.name AS task_name`

const woJoins = `
	LEFT JOIN properties p ON p.id = wo.property_id AND p.org_id = wo.org_id
	LEFT JOIN tasks t ON t.id = wo.task_id AND t.org_id = wo.org_id`

func scanWorkOrder(row pgx.Row) (*workorder.WorkOrder, error) {
	var wo workorder.WorkOrder
	err := row.Scan(
		&wo.ID, &wo.OrgID, &wo.TaskID, &wo.PropertyID, &wo.VendorID,
		&wo.Name, &wo.WorkDescription, &wo.VendorNotes, &wo.InternalNotes,
		&wo.Status, &wo.Priority, &wo.DueDate, &wo.EntryPreference,
		&wo.QuoteAmountCents, &wo.ActualCost,
		&wo.EventStartAt, &wo.EventEndAt, &wo.DispatchedAt,
		&wo.CompletedAt, &wo.CompletedBy, &wo.CreatedBy,
		&wo.CreatedAt, &wo.UpdatedAt,
		&wo.PropertyName,
		&wo.TaskName,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &wo, nil
}

// ===== CREATE =====

func (r *WorkOrderRepositoryPG) Create(ctx context.Context, orgID int64, dto workorder.CreateWorkOrderDTO) (*workorder.WorkOrder, error) {
	row := r.db.QueryRow(ctx, `
		WITH new AS (
			INSERT INTO work_orders (
				org_id, task_id, property_id, vendor_id,
				name, work_description, status, priority,
				due_date, event_start_at, event_end_at,
				entry_preference, quote_amount_cents, created_by
			)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
			RETURNING *
		)
		SELECT `+woColumns+`
		FROM new wo`+woJoins,
		orgID, dto.TaskID, dto.PropertyID, dto.VendorID,
		dto.Name, dto.WorkDescription, dto.Status, dto.Priority,
		dto.DueDate, dto.EventStartAt, dto.EventEndAt,
		dto.EntryPreference, dto.QuoteAmountCents, dto.RequestedBy,
	)
	return scanWorkOrder(row)
}

// ===== GET ONE =====

func (r *WorkOrderRepositoryPG) GetByID(ctx context.Context, orgID, id int64) (*workorder.WorkOrder, error) {
	row := r.db.QueryRow(ctx, `
		SELECT `+woColumns+`
		FROM work_orders wo`+woJoins+`
		WHERE wo.id = $1 AND wo.org_id = $2`, id, orgID)
	wo, err := scanWorkOrder(row)
	if err != nil {
		return nil, err
	}
	atts, err := r.GetWorkOrderAttachments(ctx, orgID, id)
	if err == nil {
		wo.Attachments = atts
	}
	return wo, nil
}

// ===== UPDATE =====

func (r *WorkOrderRepositoryPG) Update(ctx context.Context, orgID, id int64, dto workorder.UpdateWorkOrderDTO) (*workorder.WorkOrder, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE work_orders SET
			vendor_id          = COALESCE($1, vendor_id),
			name               = COALESCE($2, name),
			work_description   = COALESCE($3, work_description),
			vendor_notes       = COALESCE($4, vendor_notes),
			internal_notes     = COALESCE($5, internal_notes),
			priority           = COALESCE($6, priority),
			status             = COALESCE($7, status),
			due_date           = COALESCE($8, due_date),
			event_start_at     = COALESCE($9, event_start_at),
			event_end_at       = COALESCE($10, event_end_at),
			entry_preference   = COALESCE($11, entry_preference),
			quote_amount_cents = COALESCE($12, quote_amount_cents),
			actual_cost        = COALESCE($13, actual_cost),
			completed_by       = COALESCE($14, completed_by),
			dispatched_at      = CASE WHEN $7::text = 'dispatched' AND dispatched_at IS NULL
			                          THEN now() ELSE dispatched_at END,
			completed_at       = CASE WHEN $7::text = 'completed' AND completed_at IS NULL
			                          THEN now() ELSE completed_at END,
			updated_at         = now()
		WHERE id = $15 AND org_id = $16`,
		dto.VendorID, dto.Name, dto.WorkDescription, dto.VendorNotes,
		dto.InternalNotes, dto.Priority, dto.Status, dto.DueDate,
		dto.EventStartAt, dto.EventEndAt, dto.EntryPreference,
		dto.QuoteAmountCents, dto.ActualCost, dto.CompletedBy, id, orgID)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return r.GetByID(ctx, orgID, id)
}

// ===== DELETE =====

func (r *WorkOrderRepositoryPG) Delete(ctx context.Context, orgID, id int64) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM work_orders WHERE id = $1 AND org_id = $2`, id, orgID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ===== LIST =====

func (r *WorkOrderRepositoryPG) List(ctx context.Context, orgID int64, taskID *int64, propertyID *int64, vendorID *int64, status *string, limit int) ([]*workorder.WorkOrder, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.db.Query(ctx, `
		SELECT `+woColumns+`
		FROM work_orders wo`+woJoins+`
		WHERE wo.org_id = $1
		  AND ($2::bigint IS NULL OR wo.task_id = $2)
		  AND ($3::bigint IS NULL OR wo.property_id = $3)
		  AND ($4::bigint IS NULL OR wo.vendor_id = $4)
		  AND ($5::text IS NULL OR wo.status = $5)
		ORDER BY wo.created_at DESC
		LIMIT $6`,
		orgID, taskID, propertyID, vendorID, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []*workorder.WorkOrder{}
	for rows.Next() {
		wo, err := scanWorkOrder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, wo)
	}
	return out, rows.Err()
}

// ===== COMPLETE =====

func (r *WorkOrderRepositoryPG) MarkCompleted(ctx context.Context, orgID, id int64, completedBy int64, actualCost *float64) (*workorder.WorkOrder, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE work_orders SET
			status       = 'completed',
			completed_at = now(),
			completed_by = $1,
			actual_cost  = COALESCE($2, actual_cost),
			updated_at   = now()
		WHERE id = $3 AND org_id = $4`,
		completedBy, actualCost, id, orgID)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return r.GetByID(ctx, orgID, id)
}

// ===== ATTACHMENTS (metadata only) =====

func scanAttachment(row pgx.Row) (*workorder.Attachment, error) {
	var a workorder.Attachment
	err := row.Scan(&a.ID, &a.WorkOrderID, &a.FileName, &a.FileType, &a.FileSize,
		&a.StoragePath, &a.UploadedByType, &a.UploadedByID, &a.Label, &a.Description, &a.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

const attColumns = `id, work_order_id, file_name, file_type, file_size,
	storage_path, uploaded_by_type, uploaded_by_id, label, description, created_at`

func (r *WorkOrderRepositoryPG) InsertAttachment(ctx context.Context, orgID int64, a workorder.Attachment) (*workorder.Attachment, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO work_order_attachments (
			org_id, work_order_id, file_name, file_type, file_size,
			storage_path, uploaded_by_type, uploaded_by_id, label, description
		)
		SELECT $1, wo.id, $3, $4, $5, $6, $7, $8, $9, $10
		FROM work_orders wo WHERE wo.id = $2 AND wo.org_id = $1
		RETURNING `+attColumns,
		orgID, a.WorkOrderID, a.FileName, a.FileType, a.FileSize,
		a.StoragePath, a.UploadedByType, a.UploadedByID, a.Label, a.Description)
	return scanAttachment(row)
}

func (r *WorkOrderRepositoryPG) GetWorkOrderAttachments(ctx context.Context, orgID, workOrderID int64) ([]workorder.Attachment, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+attColumns+`
		FROM work_order_attachments
		WHERE org_id = $1 AND work_order_id = $2
		ORDER BY created_at ASC`, orgID, workOrderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []workorder.Attachment{}
	for rows.Next() {
		a, err := scanAttachment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

func (r *WorkOrderRepositoryPG) UpdateWorkOrderAttachment(ctx context.Context, orgID, attachmentID int64, req workorder.UpdateWorkOrderAttachmentRequest) (*workorder.Attachment, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE work_order_attachments SET
			file_name   = COALESCE($1, file_name),
			label       = COALESCE($2, label),
			description = COALESCE($3, description)
		WHERE id = $4 AND org_id = $5
		RETURNING `+attColumns,
		req.FileName, req.Label, req.Description, attachmentID, orgID)
	return scanAttachment(row)
}

func (r *WorkOrderRepositoryPG) DeleteWorkOrderAttachment(ctx context.Context, orgID, attachmentID int64) error {
	tag, err := r.db.Exec(ctx,
		`DELETE FROM work_order_attachments WHERE id = $1 AND org_id = $2`, attachmentID, orgID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ===== NOTES =====

func scanNote(row pgx.Row) (*workorder.WorkOrderNote, error) {
	var n workorder.WorkOrderNote
	err := row.Scan(&n.ID, &n.WorkOrderID, &n.AuthorType, &n.AuthorID, &n.Body,
		&n.IsVisibleToClient, &n.CreatedAt, &n.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &n, nil
}

const noteColumns = `id, work_order_id, author_type, author_id, body,
	is_visible_to_client, created_at, updated_at`

func (r *WorkOrderRepositoryPG) GetWorkOrderNotes(ctx context.Context, orgID, workOrderID int64) ([]workorder.WorkOrderNote, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+noteColumns+`
		FROM work_order_notes
		WHERE org_id = $1 AND work_order_id = $2
		ORDER BY created_at ASC`, orgID, workOrderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []workorder.WorkOrderNote{}
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *n)
	}
	return out, rows.Err()
}

func (r *WorkOrderRepositoryPG) InsertWorkOrderNote(ctx context.Context, orgID int64, n *workorder.WorkOrderNote) error {
	row := r.db.QueryRow(ctx, `
		INSERT INTO work_order_notes (org_id, work_order_id, author_type, author_id, body, is_visible_to_client)
		SELECT $1, wo.id, $3, $4, $5, $6
		FROM work_orders wo WHERE wo.id = $2 AND wo.org_id = $1
		RETURNING `+noteColumns,
		orgID, n.WorkOrderID, n.AuthorType, n.AuthorID, n.Body, n.IsVisibleToClient)
	scanned, err := scanNote(row)
	if err != nil {
		return err
	}
	*n = *scanned
	return nil
}

func (r *WorkOrderRepositoryPG) UpdateWorkOrderNote(ctx context.Context, orgID, noteID int64, req workorder.UpdateWorkOrderNoteRequest) (*workorder.WorkOrderNote, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE work_order_notes SET
			body = $1, is_visible_to_client = $2, updated_at = now()
		WHERE id = $3 AND org_id = $4
		RETURNING `+noteColumns,
		req.Body, req.IsVisibleToClient, noteID, orgID)
	return scanNote(row)
}

func (r *WorkOrderRepositoryPG) DeleteWorkOrderNote(ctx context.Context, orgID, noteID int64) error {
	tag, err := r.db.Exec(ctx,
		`DELETE FROM work_order_notes WHERE id = $1 AND org_id = $2`, noteID, orgID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
