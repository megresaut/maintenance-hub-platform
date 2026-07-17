package aiservice

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"maintenancehub/modules/ai"
)

const cacheTTL = 5 * time.Minute

// ContextLoader loads per-org reference data (vendors, properties, field
// team) for AI prompts. Ported from ra-avm; queries rewritten against this
// repo's schema and cached per org instead of globally.
type ContextLoader struct {
	db *pgxpool.Pool

	mu     sync.RWMutex
	cached map[int64]*cachedContext
}

type cachedContext struct {
	data     *ai.ContextData
	cachedAt time.Time
}

func NewContextLoader(db *pgxpool.Pool) *ContextLoader {
	return &ContextLoader{db: db, cached: map[int64]*cachedContext{}}
}

// Load returns the cached context data for one org.
func (cl *ContextLoader) Load(ctx context.Context, orgID int64) (*ai.ContextData, error) {
	cl.mu.RLock()
	if c, ok := cl.cached[orgID]; ok && time.Since(c.cachedAt) < cacheTTL {
		cl.mu.RUnlock()
		return c.data, nil
	}
	cl.mu.RUnlock()

	cl.mu.Lock()
	defer cl.mu.Unlock()
	if c, ok := cl.cached[orgID]; ok && time.Since(c.cachedAt) < cacheTTL {
		return c.data, nil
	}

	data := &ai.ContextData{}

	rows, err := cl.db.Query(ctx,
		`SELECT id, name, COALESCE(category, '') FROM vendors WHERE org_id = $1 ORDER BY name`, orgID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var v ai.VendorInfo
		if err := rows.Scan(&v.ID, &v.Name, &v.Category); err != nil {
			rows.Close()
			return nil, err
		}
		data.Vendors = append(data.Vendors, v)
	}
	rows.Close()

	rows, err = cl.db.Query(ctx,
		`SELECT id, name, address FROM properties WHERE org_id = $1 ORDER BY name`, orgID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var p ai.PropertyInfo
		if err := rows.Scan(&p.ID, &p.Name, &p.Address); err != nil {
			rows.Close()
			return nil, err
		}
		data.Properties = append(data.Properties, p)
	}
	rows.Close()

	rows, err = cl.db.Query(ctx,
		`SELECT id, name, email FROM org_users WHERE org_id = $1 ORDER BY name`, orgID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var u ai.UserInfo
		if err := rows.Scan(&u.ID, &u.Name, &u.Email); err != nil {
			rows.Close()
			return nil, err
		}
		data.Users = append(data.Users, u)
	}
	rows.Close()

	cl.cached[orgID] = &cachedContext{data: data, cachedAt: time.Now()}
	return data, nil
}

// Invalidate drops the cache for an org (e.g. after vendor/property edits).
func (cl *ContextLoader) Invalidate(orgID int64) {
	cl.mu.Lock()
	delete(cl.cached, orgID)
	cl.mu.Unlock()
}

// FilterToProperties narrows properties to the given IDs (sender identity known).
func (cl *ContextLoader) FilterToProperties(full *ai.ContextData, propertyIDs []int64) *ai.ContextData {
	idSet := make(map[int64]bool, len(propertyIDs))
	for _, id := range propertyIDs {
		idSet[id] = true
	}
	filtered := &ai.ContextData{
		Vendors: full.Vendors,
		Users:   full.Users,
	}
	for _, p := range full.Properties {
		if idSet[p.ID] {
			filtered.Properties = append(filtered.Properties, p)
		}
	}
	return filtered
}

// FilterForEvent returns a subset of context data relevant to the event text,
// keeping prompts small when the vendor directory is large.
func (cl *ContextLoader) FilterForEvent(full *ai.ContextData, eventText string) *ai.ContextData {
	filtered := &ai.ContextData{
		Properties: full.Properties,
		Users:      full.Users,
	}

	lower := strings.ToLower(eventText)
	words := extractWords(lower)

	for _, v := range full.Vendors {
		vendorLower := strings.ToLower(v.Name)
		if strings.Contains(lower, vendorLower) || strings.Contains(vendorLower, lower) {
			filtered.Vendors = append(filtered.Vendors, v)
			continue
		}
		vendorWords := extractWords(vendorLower)
		if wordOverlap(words, vendorWords) >= 1 {
			filtered.Vendors = append(filtered.Vendors, v)
		}
	}

	// Small directories fit in the prompt whole — include everything so the
	// model can do its own matching.
	if len(filtered.Vendors) == 0 && len(full.Vendors) <= 75 {
		filtered.Vendors = full.Vendors
	}
	if len(filtered.Vendors) > 75 {
		filtered.Vendors = filtered.Vendors[:75]
	}

	return filtered
}

func extractWords(text string) []string {
	stops := map[string]bool{
		"the": true, "and": true, "for": true, "with": true, "from": true,
		"this": true, "that": true, "are": true, "was": true, "will": true,
		"has": true, "have": true, "had": true, "not": true, "but": true,
		"all": true, "can": true, "her": true, "his": true, "how": true,
		"its": true, "may": true, "new": true, "now": true, "old": true,
		"our": true, "out": true, "own": true, "say": true, "she": true,
		"too": true, "use": true, "meet": true, "meeting": true,
	}
	replacer := strings.NewReplacer("-", " ", "_", " ", ",", " ", ".", " ", "/", " ", "(", " ", ")", " ")
	text = replacer.Replace(text)

	var words []string
	for _, w := range strings.Fields(text) {
		w = strings.TrimSpace(w)
		if len(w) >= 3 && !stops[w] {
			words = append(words, w)
		}
	}
	return words
}

func wordOverlap(a, b []string) int {
	bSet := make(map[string]bool, len(b))
	for _, w := range b {
		bSet[w] = true
	}
	count := 0
	for _, w := range a {
		if bSet[w] {
			count++
		}
	}
	return count
}
