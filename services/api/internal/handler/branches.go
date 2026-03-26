package handler

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
)

// BranchRow is a single branch record in API responses.
type BranchRow struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	Slug     string    `json:"slug"`
	Address  *string   `json:"address,omitempty"`
	Timezone string    `json:"timezone"`
}

// BranchesResponse is the JSON envelope for GET /api/v1/branches.
type BranchesResponse struct {
	Branches []BranchRow `json:"branches"`
}

// ListBranches returns all branches.
// @Summary      List branches
// @Description  Returns every spa branch ordered by name.
// @Tags         branches
// @Produce      json
// @Success      200  {object}  BranchesResponse
// @Router       /api/v1/branches [get]
func listBranches(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		rows, err := d.Pool.Query(ctx, `
			SELECT id, name, slug, address, timezone FROM branches ORDER BY name`)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		var out []BranchRow
		for rows.Next() {
			var b BranchRow
			if err := rows.Scan(&b.ID, &b.Name, &b.Slug, &b.Address, &b.Timezone); err != nil {
				http.Error(w, "scan error", http.StatusInternalServerError)
				return
			}
			out = append(out, b)
		}
		if err := rows.Err(); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"branches": out})
	}
}

// ServiceRow is a single service record in API responses.
type ServiceRow struct {
	ID              uuid.UUID `json:"id"`
	BranchID        uuid.UUID `json:"branch_id"`
	Name            string    `json:"name"`
	Description     *string   `json:"description,omitempty"`
	DurationMinutes int       `json:"duration_minutes"`
	PriceCents      int64     `json:"price_cents"`
	IsActive        bool      `json:"is_active"`
}

// ServicesResponse is the JSON envelope for GET /api/v1/services.
type ServicesResponse struct {
	Services []ServiceRow `json:"services"`
}

// ListServices returns active services, optionally filtered by branch_id.
// @Summary      List services
// @Description  Lists active services. When branch_id is provided, only services for that branch are returned.
// @Tags         services
// @Param        branch_id  query  string  false  "Branch UUID"
// @Produce      json
// @Success      200  {object}  ServicesResponse
// @Failure      400  {string}  string  "invalid branch_id"
// @Router       /api/v1/services [get]
func listServices(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		branchID := r.URL.Query().Get("branch_id")

		scan := func(rows interface {
			Next() bool
			Scan(dest ...any) error
			Close()
			Err() error
		}) ([]ServiceRow, error) {
			defer rows.Close()
			var out []ServiceRow
			for rows.Next() {
				var s ServiceRow
				if err := rows.Scan(&s.ID, &s.BranchID, &s.Name, &s.Description, &s.DurationMinutes, &s.PriceCents, &s.IsActive); err != nil {
					return nil, err
				}
				out = append(out, s)
			}
			return out, rows.Err()
		}

		var out []ServiceRow
		var err error
		if branchID != "" {
			bid, perr := uuid.Parse(branchID)
			if perr != nil {
				http.Error(w, "invalid branch_id", http.StatusBadRequest)
				return
			}
			rows, qerr := d.Pool.Query(ctx, `
				SELECT id, branch_id, name, description, duration_minutes, price_cents, is_active
				FROM services WHERE branch_id = $1 AND is_active = true ORDER BY name`, bid)
			if qerr != nil {
				http.Error(w, "database error", http.StatusInternalServerError)
				return
			}
			out, err = scan(rows)
		} else {
			rows, qerr := d.Pool.Query(ctx, `
				SELECT id, branch_id, name, description, duration_minutes, price_cents, is_active
				FROM services WHERE is_active = true ORDER BY name`)
			if qerr != nil {
				http.Error(w, "database error", http.StatusInternalServerError)
				return
			}
			out, err = scan(rows)
		}
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"services": out})
	}
}
