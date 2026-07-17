package vendors

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repo struct {
	db *pgxpool.Pool
}

func NewRepo(db *pgxpool.Pool) *Repo {
	return &Repo{db: db}
}

const vendorCols = `id, org_id, name, category, service_area_tags, primary_email, phone,
	alt_email, alt_phone, notes, website, address, created_at`

func scanVendor(row pgx.Row) (*Vendor, error) {
	var v Vendor
	err := row.Scan(&v.ID, &v.OrgID, &v.Name, &v.Category, &v.ServiceAreaTags,
		&v.PrimaryEmail, &v.Phone, &v.AltEmail, &v.AltPhone, &v.Notes,
		&v.Website, &v.Address, &v.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

type VendorInput struct {
	Name            string   `json:"name"`
	Category        string   `json:"category"`
	ServiceAreaTags []string `json:"service_area_tags"`
	PrimaryEmail    *string  `json:"primary_email"`
	Phone           *string  `json:"phone"`
	AltEmail        *string  `json:"alt_email"`
	AltPhone        *string  `json:"alt_phone"`
	Notes           *string  `json:"notes"`
	Website         *string  `json:"website"`
	Address         *string  `json:"address"`
}

func (r *Repo) Create(ctx context.Context, orgID int64, in VendorInput) (*Vendor, error) {
	if in.ServiceAreaTags == nil {
		in.ServiceAreaTags = []string{}
	}
	q := `INSERT INTO vendors (org_id, name, category, service_area_tags, primary_email,
	        phone, alt_email, alt_phone, notes, website, address)
	      VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	      RETURNING ` + vendorCols
	return scanVendor(r.db.QueryRow(ctx, q, orgID, in.Name, normalizeCategory(in.Category),
		in.ServiceAreaTags, in.PrimaryEmail, in.Phone, in.AltEmail, in.AltPhone,
		in.Notes, in.Website, in.Address))
}

func (r *Repo) Update(ctx context.Context, orgID, id int64, in VendorInput) (*Vendor, error) {
	if in.ServiceAreaTags == nil {
		in.ServiceAreaTags = []string{}
	}
	q := `UPDATE vendors SET name = $1, category = $2, service_area_tags = $3,
	        primary_email = $4, phone = $5, alt_email = $6, alt_phone = $7,
	        notes = $8, website = $9, address = $10
	      WHERE id = $11 AND org_id = $12
	      RETURNING ` + vendorCols
	v, err := scanVendor(r.db.QueryRow(ctx, q, in.Name, normalizeCategory(in.Category),
		in.ServiceAreaTags, in.PrimaryEmail, in.Phone, in.AltEmail, in.AltPhone,
		in.Notes, in.Website, in.Address, id, orgID))
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("vendor not found")
	}
	return v, err
}

func (r *Repo) Delete(ctx context.Context, orgID, id int64) error {
	ct, err := r.db.Exec(ctx, `DELETE FROM vendors WHERE id = $1 AND org_id = $2`, id, orgID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("vendor not found")
	}
	return nil
}

func (r *Repo) GetByID(ctx context.Context, orgID, id int64) (*Vendor, error) {
	v, err := scanVendor(r.db.QueryRow(ctx,
		`SELECT `+vendorCols+` FROM vendors WHERE id = $1 AND org_id = $2`, id, orgID))
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return v, err
}

func (r *Repo) List(ctx context.Context, orgID int64, category string) ([]*Vendor, error) {
	q := `SELECT ` + vendorCols + ` FROM vendors
	      WHERE org_id = $1 AND ($2 = '' OR category = $2)
	      ORDER BY name`
	rows, err := r.db.Query(ctx, q, orgID, normalizeCategory(category))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Vendor{}
	for rows.Next() {
		v, err := scanVendor(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// LookupLocal is the fallback vendor search when no preferred list exists:
// filter by trade category and (when the property has service-area tags to
// match against) overlapping service_area_tags. Simple filter per the plan —
// no fuzzy matching.
func (r *Repo) LookupLocal(ctx context.Context, orgID int64, category string, areaTags []string) ([]*Vendor, error) {
	q := `SELECT ` + vendorCols + ` FROM vendors
	      WHERE org_id = $1 AND category = $2
	        AND (cardinality($3::text[]) = 0 OR service_area_tags && $3::text[])
	      ORDER BY name`
	if areaTags == nil {
		areaTags = []string{}
	}
	rows, err := r.db.Query(ctx, q, orgID, normalizeCategory(category), areaTags)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Vendor{}
	for rows.Next() {
		v, err := scanVendor(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ListPreferred returns the preferred vendors for a property+trade, highest
// priority (lowest number) first.
func (r *Repo) ListPreferred(ctx context.Context, orgID, propertyID int64, category string) ([]*Vendor, error) {
	q := `SELECT ` + qualVendorCols("v") + `
	      FROM preferred_vendors pv
	      JOIN vendors v ON v.id = pv.vendor_id
	      WHERE pv.org_id = $1 AND pv.property_id = $2 AND pv.category = $3
	      ORDER BY pv.priority, v.name`
	rows, err := r.db.Query(ctx, q, orgID, propertyID, normalizeCategory(category))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Vendor{}
	for rows.Next() {
		v, err := scanVendor(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *Repo) ListPreferredEntries(ctx context.Context, orgID int64) ([]*PreferredVendor, error) {
	q := `SELECT pv.id, pv.org_id, pv.property_id, pv.category, pv.vendor_id, pv.priority,
	             pv.created_at, v.name, p.name
	      FROM preferred_vendors pv
	      JOIN vendors v ON v.id = pv.vendor_id
	      JOIN properties p ON p.id = pv.property_id
	      WHERE pv.org_id = $1
	      ORDER BY p.name, pv.category, pv.priority`
	rows, err := r.db.Query(ctx, q, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*PreferredVendor{}
	for rows.Next() {
		var pv PreferredVendor
		if err := rows.Scan(&pv.ID, &pv.OrgID, &pv.PropertyID, &pv.Category, &pv.VendorID,
			&pv.Priority, &pv.CreatedAt, &pv.VendorName, &pv.PropertyName); err != nil {
			return nil, err
		}
		out = append(out, &pv)
	}
	return out, rows.Err()
}

func (r *Repo) AddPreferred(ctx context.Context, orgID int64, propertyID int64, category string, vendorID int64, priority int) (*PreferredVendor, error) {
	var pv PreferredVendor
	err := r.db.QueryRow(ctx, `
		INSERT INTO preferred_vendors (org_id, property_id, category, vendor_id, priority)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (org_id, property_id, category, vendor_id)
		DO UPDATE SET priority = $5
		RETURNING id, org_id, property_id, category, vendor_id, priority, created_at`,
		orgID, propertyID, normalizeCategory(category), vendorID, priority,
	).Scan(&pv.ID, &pv.OrgID, &pv.PropertyID, &pv.Category, &pv.VendorID, &pv.Priority, &pv.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &pv, nil
}

func (r *Repo) RemovePreferred(ctx context.Context, orgID, id int64) error {
	ct, err := r.db.Exec(ctx,
		`DELETE FROM preferred_vendors WHERE id = $1 AND org_id = $2`, id, orgID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("preferred vendor entry not found")
	}
	return nil
}

func normalizeCategory(c string) string {
	return strings.ToLower(strings.TrimSpace(c))
}

func qualVendorCols(a string) string {
	cols := strings.Split(vendorCols, ",")
	for i, c := range cols {
		cols[i] = a + "." + strings.TrimSpace(c)
	}
	return strings.Join(cols, ", ")
}
