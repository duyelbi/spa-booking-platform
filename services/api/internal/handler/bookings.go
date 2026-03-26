package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"spa-booking/services/api/internal/realtime"
)

// CreateBookingRequest is the JSON body for POST /api/v1/bookings.
type CreateBookingRequest struct {
	BranchID      uuid.UUID `json:"branch_id"`
	ServiceID     uuid.UUID `json:"service_id"`
	CustomerEmail string    `json:"customer_email"`
	CustomerName  *string   `json:"customer_name,omitempty"`
	StartsAt      time.Time `json:"starts_at"`
	Notes         *string   `json:"notes,omitempty"`
}

// BookingCreated is returned after a successful booking creation.
type BookingCreated struct {
	ID        uuid.UUID `json:"id"`
	BranchID  uuid.UUID `json:"branch_id"`
	ServiceID uuid.UUID `json:"service_id"`
	Status    string    `json:"status"`
	StartsAt  time.Time `json:"starts_at"`
	EndsAt    time.Time `json:"ends_at"`
}

// CreateBooking creates a pending booking and publishes a realtime event.
// @Summary      Create booking
// @Description  Validates branch/service, upserts customer by email, inserts booking with computed end time from service duration.
// @Tags         bookings
// @Accept       json
// @Produce      json
// @Param        body  body      CreateBookingRequest  true  "Booking payload"
// @Success      201   {object}  BookingCreated
// @Failure      400   {string}  string  "bad request"
// @Failure      404   {string}  string  "service not found for branch"
// @Failure      500   {string}  string  "internal error"
// @Router       /api/v1/bookings [post]
func createBooking(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req CreateBookingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.CustomerEmail == "" || req.BranchID == uuid.Nil || req.ServiceID == uuid.Nil || req.StartsAt.IsZero() {
			http.Error(w, "branch_id, service_id, customer_email, starts_at required", http.StatusBadRequest)
			return
		}

		ctx := r.Context()
		tx, err := d.Pool.Begin(ctx)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			return
		}
		defer func() { _ = tx.Rollback(ctx) }()

		var duration int
		err = tx.QueryRow(ctx, `
			SELECT duration_minutes FROM services
			WHERE id = $1 AND branch_id = $2 AND is_active = true`,
			req.ServiceID, req.BranchID).Scan(&duration)
		if err != nil {
			if err == pgx.ErrNoRows {
				http.Error(w, "service not found for branch", http.StatusNotFound)
				return
			}
			http.Error(w, "database error", http.StatusInternalServerError)
			return
		}

		endsAt := req.StartsAt.Add(time.Duration(duration) * time.Minute)

		var customerID uuid.UUID
		err = tx.QueryRow(ctx, `
			INSERT INTO consumer.accounts (email, full_name)
			VALUES (LOWER(TRIM($1)), $2)
			ON CONFLICT (email) DO UPDATE SET
				full_name = COALESCE(EXCLUDED.full_name, consumer.accounts.full_name),
				updated_at = now()
			RETURNING id`,
			req.CustomerEmail, req.CustomerName).Scan(&customerID)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			return
		}

		var bookingID uuid.UUID
		err = tx.QueryRow(ctx, `
			INSERT INTO bookings (branch_id, service_id, customer_id, starts_at, ends_at, status, notes)
			VALUES ($1, $2, $3, $4, $5, 'pending', $6)
			RETURNING id`,
			req.BranchID, req.ServiceID, customerID, req.StartsAt, endsAt, req.Notes).Scan(&bookingID)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			return
		}

		if err := tx.Commit(ctx); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			return
		}

		payload := BookingCreated{
			ID:        bookingID,
			BranchID:  req.BranchID,
			ServiceID: req.ServiceID,
			Status:    "pending",
			StartsAt:  req.StartsAt,
			EndsAt:    endsAt,
		}
		b, _ := json.Marshal(map[string]any{
			"type":    "booking_created",
			"booking": payload,
		})
		if err := d.RDB.Publish(ctx, realtime.ChannelBookings, b).Err(); err != nil {
			d.Log.Error("redis publish", "err", err)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(payload)
	}
}
