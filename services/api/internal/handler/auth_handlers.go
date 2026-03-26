package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/oauth2"

	"spa-booking/services/api/internal/auth"
)

// AuthRegisterRequest is the body for POST /api/v1/auth/register.
type AuthRegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	FullName string `json:"full_name,omitempty"`
}

// AuthLoginRequest is the body for POST /api/v1/auth/login. Staff is matched before consumer when email exists in both.
type AuthLoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// AuthUserBrief is embedded in token responses.
type AuthUserBrief struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	Principal string `json:"principal"`
}

// AuthTokenResponse is returned on successful register/login (consumer or staff).
type AuthTokenResponse struct {
	AccessToken  string        `json:"access_token"`
	RefreshToken string        `json:"refresh_token"`
	TokenType    string        `json:"token_type"`
	ExpiresIn    int           `json:"expires_in"`
	User         AuthUserBrief `json:"user"`
}

type authLogin2FAReq struct {
	TempToken string `json:"temp_token"`
	Code      string `json:"code"`
}

type authRefreshReq struct {
	RefreshToken string `json:"refresh_token"`
}

type authLogoutReq struct {
	RefreshToken string `json:"refresh_token"`
}

type auth2FAEnableReq struct {
	Code string `json:"code"`
}

type auth2FADisableReq struct {
	Password string `json:"password"`
	Code     string `json:"code"`
}

func writeAuthJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func readJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	defer func() { _, _ = io.Copy(io.Discard, r.Body) }()
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeAuthJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return false
	}
	return true
}

func (d Deps) issueSession(ctx context.Context, userID uuid.UUID, role, principal string) (access, refresh string, expiresIn int, err error) {
	access, err = auth.MintAccess(d.JWTSecret, userID, role, principal, d.AccessTTL)
	if err != nil {
		return "", "", 0, err
	}
	raw, hash, err := auth.NewRefreshToken()
	if err != nil {
		return "", "", 0, err
	}
	exp := time.Now().Add(d.RefreshTTL)
	if principal == auth.PrincipalStaff {
		_, err = d.Pool.Exec(ctx, `
			INSERT INTO staff.refresh_tokens (account_id, token_hash, expires_at)
			VALUES ($1, $2, $3)`, userID, hash, exp)
	} else {
		_, err = d.Pool.Exec(ctx, `
			INSERT INTO consumer.refresh_tokens (account_id, token_hash, expires_at)
			VALUES ($1, $2, $3)`, userID, hash, exp)
	}
	if err != nil {
		return "", "", 0, err
	}
	return access, raw, int(d.AccessTTL.Seconds()), nil
}

// authRegister creates a consumer account (sign up).
// @Summary      Sign up
// @Description  Consumer registration: email + password (min 8). Returns access_token, refresh_token, and user.principal=consumer.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      AuthRegisterRequest  true  "Email, password, optional full_name"
// @Success      201   {object}  AuthTokenResponse
// @Failure      400   {string}  string  "bad request"
// @Failure      409   {string}  string  "email conflict"
// @Router       /api/v1/auth/register [post]
func (d Deps) authRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req AuthRegisterRequest
	if !readJSON(w, r, &req) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email == "" || len(req.Password) < 8 {
		writeAuthJSON(w, http.StatusBadRequest, map[string]string{"error": "email and password (min 8 chars) required"})
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeAuthJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not hash password"})
		return
	}
	var fn *string
	if strings.TrimSpace(req.FullName) != "" {
		s := strings.TrimSpace(req.FullName)
		fn = &s
	}
	ctx := r.Context()
	var staffTaken bool
	if err := d.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM staff.accounts WHERE email = LOWER(TRIM($1)))`, email).Scan(&staffTaken); err != nil {
		d.Log.Error("auth register staff check", "err", err)
		writeAuthJSON(w, http.StatusInternalServerError, map[string]string{"error": "registration failed"})
		return
	}
	if staffTaken {
		writeAuthJSON(w, http.StatusConflict, map[string]string{"error": "email reserved for staff account"})
		return
	}
	var id uuid.UUID
	err = d.Pool.QueryRow(ctx, `
		INSERT INTO consumer.accounts (email, full_name, password_hash, email_verified)
		VALUES (LOWER(TRIM($1)), $2, $3, false)
		RETURNING id`, email, fn, hash).Scan(&id)
	if err != nil {
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
			writeAuthJSON(w, http.StatusConflict, map[string]string{"error": "email already registered"})
			return
		}
		d.Log.Error("auth register", "err", err)
		writeAuthJSON(w, http.StatusInternalServerError, map[string]string{"error": "registration failed"})
		return
	}
	access, refresh, expIn, err := d.issueSession(ctx, id, "user", auth.PrincipalConsumer)
	if err != nil {
		d.Log.Error("auth register session", "err", err)
		writeAuthJSON(w, http.StatusInternalServerError, map[string]string{"error": "session issue failed"})
		return
	}
	writeAuthJSON(w, http.StatusCreated, map[string]any{
		"access_token": access, "refresh_token": refresh, "token_type": "Bearer",
		"expires_in": expIn, "user": map[string]any{
			"id": id.String(), "email": email, "role": "user", "principal": auth.PrincipalConsumer,
		},
	})
}

// authLogin signs in staff (tried first) or consumer. OAuth-only accounts get 401 "use oauth".
// @Summary      Sign in
// @Description  Email + password. Staff account is resolved before consumer. If 2FA enabled, returns two_factor_required and temp_token instead of tokens.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      AuthLoginRequest  true  "Email and password"
// @Success      200   {object}  AuthTokenResponse
// @Failure      400   {string}  string  "bad request"
// @Failure      401   {string}  string  "invalid credentials"
// @Router       /api/v1/auth/login [post]
func (d Deps) authLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req AuthLoginRequest
	if !readJSON(w, r, &req) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	ctx := r.Context()
	var id uuid.UUID
	var hash string
	var totpEnabled bool
	var role, principal string

	err := d.Pool.QueryRow(ctx, `
		SELECT a.id, COALESCE(a.password_hash,''), a.totp_enabled, COALESCE(p.role, 'employee')
		FROM staff.accounts a
		LEFT JOIN staff.profiles p ON p.account_id = a.id
		WHERE a.email = LOWER(TRIM($1))`, email).Scan(&id, &hash, &totpEnabled, &role)
	switch {
	case err == nil:
		principal = auth.PrincipalStaff
	case errors.Is(err, pgx.ErrNoRows):
		err = d.Pool.QueryRow(ctx, `
			SELECT id, COALESCE(password_hash,''), totp_enabled
			FROM consumer.accounts WHERE email = LOWER(TRIM($1))`, email).Scan(&id, &hash, &totpEnabled)
		if errors.Is(err, pgx.ErrNoRows) {
			writeAuthJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
			return
		}
		if err != nil {
			d.Log.Error("auth login", "err", err)
			writeAuthJSON(w, http.StatusInternalServerError, map[string]string{"error": "login failed"})
			return
		}
		role = "user"
		principal = auth.PrincipalConsumer
	default:
		d.Log.Error("auth login", "err", err)
		writeAuthJSON(w, http.StatusInternalServerError, map[string]string{"error": "login failed"})
		return
	}
	if hash == "" {
		writeAuthJSON(w, http.StatusUnauthorized, map[string]string{"error": "use oauth for this account"})
		return
	}
	if err := auth.CheckPassword(hash, req.Password); err != nil {
		writeAuthJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}
	if totpEnabled {
		temp, err := auth.Mint2FAPending(d.JWTSecret, id, principal, d.Temp2FATTL)
		if err != nil {
			writeAuthJSON(w, http.StatusInternalServerError, map[string]string{"error": "2fa token failed"})
			return
		}
		writeAuthJSON(w, http.StatusOK, map[string]any{
			"two_factor_required": true,
			"temp_token":          temp,
			"expires_in":          int(d.Temp2FATTL.Seconds()),
		})
		return
	}
	access, refresh, expIn, err := d.issueSession(ctx, id, role, principal)
	if err != nil {
		d.Log.Error("auth login session", "err", err)
		writeAuthJSON(w, http.StatusInternalServerError, map[string]string{"error": "session issue failed"})
		return
	}
	writeAuthJSON(w, http.StatusOK, map[string]any{
		"access_token": access, "refresh_token": refresh, "token_type": "Bearer",
		"expires_in": expIn, "user": map[string]any{
			"id": id.String(), "email": email, "role": role, "principal": principal,
		},
	})
}

func (d Deps) authLogin2FA(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req authLogin2FAReq
	if !readJSON(w, r, &req) {
		return
	}
	uid, principal, err := auth.Parse2FAPending(d.JWTSecret, req.TempToken)
	if err != nil {
		writeAuthJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired temp token"})
		return
	}
	ctx := r.Context()
	var secret string
	var totpEnabled bool
	var email, role string
	if principal == auth.PrincipalStaff {
		err = d.Pool.QueryRow(ctx, `
			SELECT COALESCE(a.totp_secret,''), a.totp_enabled, a.email, COALESCE(p.role, 'employee')
			FROM staff.accounts a
			LEFT JOIN staff.profiles p ON p.account_id = a.id
			WHERE a.id = $1`, uid).Scan(&secret, &totpEnabled, &email, &role)
	} else {
		err = d.Pool.QueryRow(ctx, `
			SELECT COALESCE(totp_secret,''), totp_enabled, email FROM consumer.accounts WHERE id = $1`, uid).Scan(&secret, &totpEnabled, &email)
		role = "user"
	}
	if err != nil {
		writeAuthJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid user"})
		return
	}
	if !totpEnabled || secret == "" {
		writeAuthJSON(w, http.StatusBadRequest, map[string]string{"error": "2fa not enabled"})
		return
	}
	if !auth.ValidateTOTP(secret, strings.TrimSpace(req.Code)) {
		writeAuthJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid otp code"})
		return
	}
	access, refresh, expIn, err := d.issueSession(ctx, uid, role, principal)
	if err != nil {
		writeAuthJSON(w, http.StatusInternalServerError, map[string]string{"error": "session issue failed"})
		return
	}
	writeAuthJSON(w, http.StatusOK, map[string]any{
		"access_token": access, "refresh_token": refresh, "token_type": "Bearer",
		"expires_in": expIn, "user": map[string]any{
			"id": uid.String(), "email": email, "role": role, "principal": principal,
		},
	})
}

func (d Deps) authRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req authRefreshReq
	if !readJSON(w, r, &req) {
		return
	}
	raw := strings.TrimSpace(req.RefreshToken)
	if raw == "" {
		writeAuthJSON(w, http.StatusBadRequest, map[string]string{"error": "refresh_token required"})
		return
	}
	h := auth.HashRefreshToken(raw)
	ctx := r.Context()
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		writeAuthJSON(w, http.StatusInternalServerError, map[string]string{"error": "db error"})
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var userID uuid.UUID
	var role, principal string
	err = tx.QueryRow(ctx, `
		SELECT a.id FROM consumer.refresh_tokens rt
		JOIN consumer.accounts a ON a.id = rt.account_id
		WHERE rt.token_hash = $1 AND rt.revoked_at IS NULL AND rt.expires_at > now()`, h).Scan(&userID)
	if err == nil {
		role = "user"
		principal = auth.PrincipalConsumer
	} else if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `
			SELECT a.id, COALESCE(p.role, 'employee') FROM staff.refresh_tokens rt
			JOIN staff.accounts a ON a.id = rt.account_id
			LEFT JOIN staff.profiles p ON p.account_id = a.id
			WHERE rt.token_hash = $1 AND rt.revoked_at IS NULL AND rt.expires_at > now()`, h).Scan(&userID, &role)
		if err != nil {
			writeAuthJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid refresh token"})
			return
		}
		principal = auth.PrincipalStaff
	} else {
		writeAuthJSON(w, http.StatusInternalServerError, map[string]string{"error": "db error"})
		return
	}
	if _, err := tx.Exec(ctx, `UPDATE consumer.refresh_tokens SET revoked_at = now() WHERE token_hash = $1`, h); err != nil {
		writeAuthJSON(w, http.StatusInternalServerError, map[string]string{"error": "db error"})
		return
	}
	if _, err := tx.Exec(ctx, `UPDATE staff.refresh_tokens SET revoked_at = now() WHERE token_hash = $1`, h); err != nil {
		writeAuthJSON(w, http.StatusInternalServerError, map[string]string{"error": "db error"})
		return
	}
	access, err := auth.MintAccess(d.JWTSecret, userID, role, principal, d.AccessTTL)
	if err != nil {
		writeAuthJSON(w, http.StatusInternalServerError, map[string]string{"error": "token issue failed"})
		return
	}
	nraw, nhash, err := auth.NewRefreshToken()
	if err != nil {
		writeAuthJSON(w, http.StatusInternalServerError, map[string]string{"error": "token issue failed"})
		return
	}
	exp := time.Now().Add(d.RefreshTTL)
	if principal == auth.PrincipalStaff {
		_, err = tx.Exec(ctx, `
			INSERT INTO staff.refresh_tokens (account_id, token_hash, expires_at) VALUES ($1, $2, $3)`,
			userID, nhash, exp)
	} else {
		_, err = tx.Exec(ctx, `
			INSERT INTO consumer.refresh_tokens (account_id, token_hash, expires_at) VALUES ($1, $2, $3)`,
			userID, nhash, exp)
	}
	if err != nil {
		writeAuthJSON(w, http.StatusInternalServerError, map[string]string{"error": "db error"})
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeAuthJSON(w, http.StatusInternalServerError, map[string]string{"error": "db error"})
		return
	}
	writeAuthJSON(w, http.StatusOK, map[string]any{
		"access_token": access, "refresh_token": nraw, "token_type": "Bearer",
		"expires_in": int(d.AccessTTL.Seconds()),
	})
}

func (d Deps) authLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req authLogoutReq
	if !readJSON(w, r, &req) {
		return
	}
	raw := strings.TrimSpace(req.RefreshToken)
	if raw == "" {
		writeAuthJSON(w, http.StatusBadRequest, map[string]string{"error": "refresh_token required"})
		return
	}
	h := auth.HashRefreshToken(raw)
	ctx := r.Context()
	if _, err := d.Pool.Exec(ctx, `
		UPDATE consumer.refresh_tokens SET revoked_at = now()
		WHERE token_hash = $1 AND revoked_at IS NULL`, h); err != nil {
		writeAuthJSON(w, http.StatusInternalServerError, map[string]string{"error": "db error"})
		return
	}
	_, err := d.Pool.Exec(ctx, `
		UPDATE staff.refresh_tokens SET revoked_at = now()
		WHERE token_hash = $1 AND revoked_at IS NULL`, h)
	if err != nil {
		writeAuthJSON(w, http.StatusInternalServerError, map[string]string{"error": "db error"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d Deps) authStaffPing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	role, _ := AuthRole(r.Context())
	writeAuthJSON(w, http.StatusOK, map[string]string{"scope": "staff", "role": role})
}

func (d Deps) authAdminPing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeAuthJSON(w, http.StatusOK, map[string]string{"scope": "admin"})
}

func (d Deps) authMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	uid, ok := AuthUserID(r.Context())
	if !ok {
		writeAuthJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	principal, _ := AuthPrincipal(r.Context())
	ctx := r.Context()
	var email, role string
	var full *string
	var ev, t2 bool
	var created time.Time
	var err error
	if principal == auth.PrincipalStaff {
		err = d.Pool.QueryRow(ctx, `
			SELECT a.email, COALESCE(p.role, 'employee'), a.full_name, a.email_verified, a.totp_enabled, a.created_at
			FROM staff.accounts a
			LEFT JOIN staff.profiles p ON p.account_id = a.id
			WHERE a.id = $1`, uid).Scan(&email, &role, &full, &ev, &t2, &created)
	} else {
		err = d.Pool.QueryRow(ctx, `
			SELECT email, full_name, email_verified, totp_enabled, created_at
			FROM consumer.accounts WHERE id = $1`, uid).Scan(&email, &full, &ev, &t2, &created)
		role = "user"
	}
	if err != nil {
		writeAuthJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}
	writeAuthJSON(w, http.StatusOK, map[string]any{
		"id": uid.String(), "email": email, "role": role, "principal": principal, "full_name": full,
		"email_verified": ev, "two_factor_enabled": t2, "created_at": created.UTC().Format(time.RFC3339),
	})
}

func (d Deps) auth2FASetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	uid, ok := AuthUserID(r.Context())
	if !ok {
		writeAuthJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	principal, _ := AuthPrincipal(r.Context())
	ctx := r.Context()
	var email string
	if principal == auth.PrincipalStaff {
		if err := d.Pool.QueryRow(ctx, `SELECT email FROM staff.accounts WHERE id = $1`, uid).Scan(&email); err != nil {
			writeAuthJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
			return
		}
	} else {
		if err := d.Pool.QueryRow(ctx, `SELECT email FROM consumer.accounts WHERE id = $1`, uid).Scan(&email); err != nil {
			writeAuthJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
			return
		}
	}
	secret, uri, err := auth.GenerateTOTPSecret(d.TOTPIssuer, email)
	if err != nil {
		writeAuthJSON(w, http.StatusInternalServerError, map[string]string{"error": "totp generate failed"})
		return
	}
	if principal == auth.PrincipalStaff {
		_, err = d.Pool.Exec(ctx, `
			UPDATE staff.accounts SET totp_secret = $1, totp_enabled = false WHERE id = $2`, secret, uid)
	} else {
		_, err = d.Pool.Exec(ctx, `
			UPDATE consumer.accounts SET totp_secret = $1, totp_enabled = false WHERE id = $2`, secret, uid)
	}
	if err != nil {
		writeAuthJSON(w, http.StatusInternalServerError, map[string]string{"error": "db error"})
		return
	}
	writeAuthJSON(w, http.StatusOK, map[string]any{
		"secret": secret, "otpauth_url": uri,
		"message": "Scan with an authenticator app, then POST /auth/2fa/enable with a valid code.",
	})
}

func (d Deps) auth2FAEnable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	uid, ok := AuthUserID(r.Context())
	if !ok {
		writeAuthJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	principal, _ := AuthPrincipal(r.Context())
	var req auth2FAEnableReq
	if !readJSON(w, r, &req) {
		return
	}
	ctx := r.Context()
	var secret string
	var err error
	if principal == auth.PrincipalStaff {
		err = d.Pool.QueryRow(ctx, `SELECT COALESCE(totp_secret,'') FROM staff.accounts WHERE id = $1`, uid).Scan(&secret)
	} else {
		err = d.Pool.QueryRow(ctx, `SELECT COALESCE(totp_secret,'') FROM consumer.accounts WHERE id = $1`, uid).Scan(&secret)
	}
	if err != nil || secret == "" {
		writeAuthJSON(w, http.StatusBadRequest, map[string]string{"error": "run /auth/2fa/setup first"})
		return
	}
	if !auth.ValidateTOTP(secret, strings.TrimSpace(req.Code)) {
		writeAuthJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid otp code"})
		return
	}
	if principal == auth.PrincipalStaff {
		_, err = d.Pool.Exec(ctx, `UPDATE staff.accounts SET totp_enabled = true WHERE id = $1`, uid)
	} else {
		_, err = d.Pool.Exec(ctx, `UPDATE consumer.accounts SET totp_enabled = true WHERE id = $1`, uid)
	}
	if err != nil {
		writeAuthJSON(w, http.StatusInternalServerError, map[string]string{"error": "db error"})
		return
	}
	writeAuthJSON(w, http.StatusOK, map[string]string{"message": "two-factor authentication enabled"})
}

func (d Deps) auth2FADisable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	uid, ok := AuthUserID(r.Context())
	if !ok {
		writeAuthJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	principal, _ := AuthPrincipal(r.Context())
	var req auth2FADisableReq
	if !readJSON(w, r, &req) {
		return
	}
	ctx := r.Context()
	var hash, secret string
	var totpEnabled bool
	var err error
	if principal == auth.PrincipalStaff {
		err = d.Pool.QueryRow(ctx, `
			SELECT COALESCE(password_hash,''), COALESCE(totp_secret,''), totp_enabled FROM staff.accounts WHERE id = $1`,
			uid).Scan(&hash, &secret, &totpEnabled)
	} else {
		err = d.Pool.QueryRow(ctx, `
			SELECT COALESCE(password_hash,''), COALESCE(totp_secret,''), totp_enabled FROM consumer.accounts WHERE id = $1`,
			uid).Scan(&hash, &secret, &totpEnabled)
	}
	if err != nil {
		writeAuthJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}
	if hash == "" {
		writeAuthJSON(w, http.StatusBadRequest, map[string]string{"error": "password not set for this account"})
		return
	}
	if err := auth.CheckPassword(hash, req.Password); err != nil {
		writeAuthJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid password"})
		return
	}
	if !totpEnabled || secret == "" {
		writeAuthJSON(w, http.StatusBadRequest, map[string]string{"error": "2fa not enabled"})
		return
	}
	if !auth.ValidateTOTP(secret, strings.TrimSpace(req.Code)) {
		writeAuthJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid otp code"})
		return
	}
	if principal == auth.PrincipalStaff {
		_, err = d.Pool.Exec(ctx, `
			UPDATE staff.accounts SET totp_secret = NULL, totp_enabled = false WHERE id = $1`, uid)
	} else {
		_, err = d.Pool.Exec(ctx, `
			UPDATE consumer.accounts SET totp_secret = NULL, totp_enabled = false WHERE id = $1`, uid)
	}
	if err != nil {
		writeAuthJSON(w, http.StatusInternalServerError, map[string]string{"error": "db error"})
		return
	}
	writeAuthJSON(w, http.StatusOK, map[string]string{"message": "two-factor authentication disabled"})
}

func (d Deps) oauthGoogleURL(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if d.GoogleOAuth == nil {
		writeAuthJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "google oauth not configured"})
		return
	}
	state, err := auth.MintOAuthStateJWT(d.JWTSecret, d.OAuthStateTTL)
	if err != nil {
		writeAuthJSON(w, http.StatusInternalServerError, map[string]string{"error": "state token failed"})
		return
	}
	url := d.GoogleOAuth.AuthCodeURL(state, oauth2.AccessTypeOffline)
	writeAuthJSON(w, http.StatusOK, map[string]string{"authorization_url": url})
}

type googleUserInfo struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	VerifiedEmail bool   `json:"verified_email"`
	Name          string `json:"name"`
}

func (d Deps) oauthGoogleCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if d.GoogleOAuth == nil {
		writeAuthJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "google oauth not configured"})
		return
	}
	q := r.URL.Query()
	code, state := q.Get("code"), q.Get("state")
	if code == "" || state == "" {
		writeAuthJSON(w, http.StatusBadRequest, map[string]string{"error": "missing code or state"})
		return
	}
	if err := auth.ParseOAuthStateJWT(d.JWTSecret, state); err != nil {
		writeAuthJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid state"})
		return
	}
	ctx := r.Context()
	tok, err := d.GoogleOAuth.Exchange(ctx, code)
	if err != nil {
		d.Log.Error("oauth exchange", "err", err)
		writeAuthJSON(w, http.StatusBadRequest, map[string]string{"error": "oauth exchange failed"})
		return
	}
	client := d.GoogleOAuth.Client(ctx, tok)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		writeAuthJSON(w, http.StatusBadGateway, map[string]string{"error": "google userinfo failed"})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		writeAuthJSON(w, http.StatusBadGateway, map[string]string{"error": "google userinfo failed"})
		return
	}
	var gu googleUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&gu); err != nil {
		writeAuthJSON(w, http.StatusBadGateway, map[string]string{"error": "google profile parse failed"})
		return
	}
	email := strings.ToLower(strings.TrimSpace(gu.Email))
	if email == "" || gu.ID == "" {
		writeAuthJSON(w, http.StatusBadRequest, map[string]string{"error": "google account has no email"})
		return
	}

	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		writeAuthJSON(w, http.StatusInternalServerError, map[string]string{"error": "db error"})
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var userID uuid.UUID
	err = tx.QueryRow(ctx, `
		SELECT account_id FROM consumer.oauth_accounts WHERE provider = 'google' AND provider_subject = $1`, gu.ID).Scan(&userID)
	if err == nil {
		// existing link
	} else if errors.Is(err, pgx.ErrNoRows) {
		var staffEmail bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM staff.accounts WHERE email = $1)`, email).Scan(&staffEmail); err != nil {
			writeAuthJSON(w, http.StatusInternalServerError, map[string]string{"error": "db error"})
			return
		}
		if staffEmail {
			writeAuthJSON(w, http.StatusConflict, map[string]string{"error": "use staff login for this email"})
			return
		}
		err = tx.QueryRow(ctx, `SELECT id FROM consumer.accounts WHERE email = $1`, email).Scan(&userID)
		if err == nil {
			if _, err := tx.Exec(ctx, `
				INSERT INTO consumer.oauth_accounts (account_id, provider, provider_subject, email_at_link)
				VALUES ($1, 'google', $2, $3)`,
				userID, gu.ID, email); err != nil {
				writeAuthJSON(w, http.StatusInternalServerError, map[string]string{"error": "link oauth failed"})
				return
			}
		} else if errors.Is(err, pgx.ErrNoRows) {
			var fn *string
			if strings.TrimSpace(gu.Name) != "" {
				n := strings.TrimSpace(gu.Name)
				fn = &n
			}
			err = tx.QueryRow(ctx, `
				INSERT INTO consumer.accounts (email, full_name, email_verified, password_hash)
				VALUES ($1, $2, $3, NULL)
				RETURNING id`, email, fn, gu.VerifiedEmail).Scan(&userID)
			if err != nil {
				d.Log.Error("oauth create user", "err", err)
				writeAuthJSON(w, http.StatusInternalServerError, map[string]string{"error": "user create failed"})
				return
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO consumer.oauth_accounts (account_id, provider, provider_subject, email_at_link)
				VALUES ($1, 'google', $2, $3)`, userID, gu.ID, email); err != nil {
				writeAuthJSON(w, http.StatusInternalServerError, map[string]string{"error": "oauth link failed"})
				return
			}
		} else {
			writeAuthJSON(w, http.StatusInternalServerError, map[string]string{"error": "db error"})
			return
		}
	} else {
		writeAuthJSON(w, http.StatusInternalServerError, map[string]string{"error": "db error"})
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeAuthJSON(w, http.StatusInternalServerError, map[string]string{"error": "db error"})
		return
	}

	access, refresh, expIn, err := d.issueSession(ctx, userID, "user", auth.PrincipalConsumer)
	if err != nil {
		writeAuthJSON(w, http.StatusInternalServerError, map[string]string{"error": "session issue failed"})
		return
	}
	writeAuthJSON(w, http.StatusOK, map[string]any{
		"access_token": access, "refresh_token": refresh, "token_type": "Bearer",
		"expires_in": expIn, "user": map[string]any{
			"id": userID.String(), "email": email, "role": "user", "principal": auth.PrincipalConsumer,
		},
	})
}

func mountAuthRoutes(r chi.Router, d Deps, jwtSecret []byte) {
	r.Route("/auth", func(ar chi.Router) {
		ar.Post("/register", d.authRegister)
		ar.Post("/login", d.authLogin)
		ar.Post("/login/2fa", d.authLogin2FA)
		ar.Post("/refresh", d.authRefresh)
		ar.Post("/logout", d.authLogout)
		ar.Get("/oauth/google/url", d.oauthGoogleURL)
		ar.Get("/oauth/google/callback", d.oauthGoogleCallback)
		ar.Group(func(pr chi.Router) {
			pr.Use(bearerAuth(jwtSecret))
			pr.Get("/me", d.authMe)
			pr.Post("/2fa/setup", d.auth2FASetup)
			pr.Post("/2fa/enable", d.auth2FAEnable)
			pr.Post("/2fa/disable", d.auth2FADisable)
			pr.With(requireRoles(map[string]struct{}{"employee": {}, "manager": {}, "admin": {}})).Get("/staff/ping", d.authStaffPing)
			pr.With(requireRoles(map[string]struct{}{"admin": {}})).Get("/admin/ping", d.authAdminPing)
		})
	})
}
