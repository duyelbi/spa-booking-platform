package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	httpSwagger "github.com/swaggo/http-swagger"
	"golang.org/x/oauth2"

	"spa-booking/services/api/internal/realtime"
)

type Deps struct {
	Log  *slog.Logger
	Pool *pgxpool.Pool
	RDB  *redis.Client
	Hub  *realtime.Hub

	JWTSecret     []byte
	AccessTTL     time.Duration
	RefreshTTL    time.Duration
	Temp2FATTL    time.Duration
	OAuthStateTTL time.Duration
	TOTPIssuer    string
	GoogleOAuth   *oauth2.Config
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // tighten in production (allowed origins from env)
	},
}

// HealthResponse is the JSON body for GET /health.
type HealthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
}

// health reports API availability.
// @Summary      Health check
// @Description  Liveness probe for load balancers and monitoring.
// @Tags         system
// @Produce      json
// @Success      200  {object}  HealthResponse
// @Router       /health [get]
func health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(HealthResponse{Status: "ok", Service: "spa-booking-api"})
}

func NewRouter(d Deps) http.Handler {
	if d.Log == nil {
		d.Log = slog.Default()
	}
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:3000", "http://localhost:3001", "http://localhost:3002", "http://127.0.0.1:3000"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	r.Get("/health", health)

	r.Get("/swagger/*", httpSwagger.WrapHandler)

	r.Route("/api/v1", func(r chi.Router) {
		mountAuthRoutes(r, d, d.JWTSecret)
		r.Get("/branches", listBranches(d))
		r.Get("/services", listServices(d))
		r.Post("/bookings", createBooking(d))
	})

	r.Get("/ws/live", func(w http.ResponseWriter, req *http.Request) {
		conn, err := upgrader.Upgrade(w, req, nil)
		if err != nil {
			d.Log.Error("ws upgrade", "err", err)
			return
		}
		realtime.ServeWS(d.Hub, conn)
	})

	return r
}
