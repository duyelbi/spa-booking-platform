package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const (
	TokenAccess     = "access"
	TokenTwoFAPend  = "2fa_pending"
	TokenOAuthState = "oauth_state"
)

var ErrInvalidToken = errors.New("invalid token")

func MintHS256(secret []byte, claims jwt.MapClaims) (string, error) {
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString(secret)
}

func ParseHS256(secret []byte, tokenStr string) (jwt.MapClaims, error) {
	tok, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return secret, nil
	})
	if err != nil || !tok.Valid {
		return nil, ErrInvalidToken
	}
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

func claimString(c jwt.MapClaims, key string) (string, bool) {
	v, ok := c[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// PrincipalConsumer / PrincipalStaff identify which identity store the subject belongs to (same DB, separate tables).
const (
	PrincipalConsumer = "consumer"
	PrincipalStaff    = "staff"
)

func MintAccess(secret []byte, userID uuid.UUID, role, principal string, ttl time.Duration) (string, error) {
	now := time.Now()
	return MintHS256(secret, jwt.MapClaims{
		"sub":        userID.String(),
		"role":       role,
		"principal":  principal,
		"typ":        TokenAccess,
		"iat":        now.Unix(),
		"exp":        now.Add(ttl).Unix(),
	})
}

func Mint2FAPending(secret []byte, userID uuid.UUID, principal string, ttl time.Duration) (string, error) {
	now := time.Now()
	return MintHS256(secret, jwt.MapClaims{
		"sub":       userID.String(),
		"typ":       TokenTwoFAPend,
		"principal": principal,
		"iat":       now.Unix(),
		"exp":       now.Add(ttl).Unix(),
	})
}

// MintOAuthStateJWT returns a compact JWT to pass as the OAuth2 `state` query parameter.
func MintOAuthStateJWT(secret []byte, ttl time.Duration) (string, error) {
	now := time.Now()
	return MintHS256(secret, jwt.MapClaims{
		"typ": TokenOAuthState,
		"jti": uuid.NewString(),
		"iat": now.Unix(),
		"exp": now.Add(ttl).Unix(),
	})
}

// ParseOAuthStateJWT validates the `state` value returned by the provider.
func ParseOAuthStateJWT(secret []byte, state string) error {
	c, err := ParseHS256(secret, state)
	if err != nil {
		return err
	}
	if typ, _ := claimString(c, "typ"); typ != TokenOAuthState {
		return ErrInvalidToken
	}
	if _, ok := claimString(c, "jti"); !ok {
		return ErrInvalidToken
	}
	if exp, ok := c["exp"].(float64); ok {
		if time.Now().Unix() > int64(exp) {
			return ErrInvalidToken
		}
	}
	return nil
}

func ParseAccess(secret []byte, tokenStr string) (userID uuid.UUID, role, principal string, err error) {
	c, err := ParseHS256(secret, tokenStr)
	if err != nil {
		return uuid.Nil, "", "", err
	}
	if typ, _ := claimString(c, "typ"); typ != TokenAccess {
		return uuid.Nil, "", "", ErrInvalidToken
	}
	sub, ok := claimString(c, "sub")
	if !ok {
		return uuid.Nil, "", "", ErrInvalidToken
	}
	uid, err := uuid.Parse(sub)
	if err != nil {
		return uuid.Nil, "", "", ErrInvalidToken
	}
	role, ok = claimString(c, "role")
	if !ok || role == "" {
		return uuid.Nil, "", "", ErrInvalidToken
	}
	principal, ok = claimString(c, "principal")
	if !ok || principal == "" {
		return uuid.Nil, "", "", ErrInvalidToken
	}
	if exp, ok := c["exp"].(float64); ok {
		if time.Now().Unix() > int64(exp) {
			return uuid.Nil, "", "", ErrInvalidToken
		}
	}
	return uid, role, principal, nil
}

func Parse2FAPending(secret []byte, tokenStr string) (uuid.UUID, string, error) {
	c, err := ParseHS256(secret, tokenStr)
	if err != nil {
		return uuid.Nil, "", err
	}
	if typ, _ := claimString(c, "typ"); typ != TokenTwoFAPend {
		return uuid.Nil, "", ErrInvalidToken
	}
	sub, ok := claimString(c, "sub")
	if !ok {
		return uuid.Nil, "", ErrInvalidToken
	}
	uid, err := uuid.Parse(sub)
	if err != nil {
		return uuid.Nil, "", ErrInvalidToken
	}
	principal, ok := claimString(c, "principal")
	if !ok || principal == "" {
		return uuid.Nil, "", ErrInvalidToken
	}
	if exp, ok := c["exp"].(float64); ok {
		if time.Now().Unix() > int64(exp) {
			return uuid.Nil, "", ErrInvalidToken
		}
	}
	return uid, principal, nil
}
