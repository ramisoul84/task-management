package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/ramisoul84/task-management/internal/config"
)

type TokenService interface {
	GenerateAccessToken(userID, email string) (string, error)
	GenerateRefreshToken() (string, error)
	HashRefreshToken(token string) string
}

type tokenService struct {
	secret         []byte
	issuer         string
	accessTokenTTL time.Duration
}

type accessClaims struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`

	jwt.RegisteredClaims
}

func NewTokenService(cfg config.SecurityConfig) TokenService {
	return &tokenService{
		secret:         []byte(cfg.Secret),
		issuer:         cfg.Issuer,
		accessTokenTTL: cfg.AccessTokenTTL,
	}
}

func (t *tokenService) GenerateAccessToken(userID, email string) (string, error) {
	now := time.Now()

	claims := accessClaims{
		UserID: userID,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    t.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(t.accessTokenTTL)),
		},
	}

	token := jwt.NewWithClaims(
		jwt.SigningMethodHS256,
		claims,
	)

	return token.SignedString(t.secret)
}

func (t *tokenService) GenerateRefreshToken() (string, error) {
	b := make([]byte, 32)

	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate refresh token: %w", err)
	}

	return hex.EncodeToString(b), nil
}

func (s *tokenService) HashRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))

	return hex.EncodeToString(sum[:])
}
