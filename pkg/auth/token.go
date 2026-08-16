package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/ramisoul84/task-management/internal/config"
)

type TokenService interface {
	GenerateAccessToken(userID, email string) (string, error)
	GenerateRefreshToken() (string, error)
	HashRefreshToken(token string) string
	ValidateAccessToken(token string) (*TokenClaims, error)
}

type TokenClaims struct {
	UserID string
	Email  string
}

type tokenService struct {
	secret         []byte
	issuer         string
	accessTokenTTL time.Duration
}

type accessClaims struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`

	jwt.RegisteredClaims
}

func NewTokenService(cfg config.SecurityConfig) TokenService {
	return &tokenService{
		secret:         []byte(cfg.Secret),
		issuer:         cfg.Issuer,
		accessTokenTTL: cfg.AccessTokenTTL,
	}
}

func (s *tokenService) GenerateAccessToken(
	userID string,
	email string,
) (string, error) {
	now := time.Now()

	claims := accessClaims{
		UserID: userID,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.accessTokenTTL)),
		},
	}

	token := jwt.NewWithClaims(
		jwt.SigningMethodHS256,
		claims,
	)

	tokenString, err := token.SignedString(s.secret)
	if err != nil {
		return "", fmt.Errorf("sign access token: %w", err)
	}

	return tokenString, nil
}

func (s *tokenService) ValidateAccessToken(
	tokenString string,
) (*TokenClaims, error) {
	claims := &accessClaims{}

	token, err := jwt.ParseWithClaims(
		tokenString,
		claims,
		func(token *jwt.Token) (any, error) {
			if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
				return nil, errors.New("unexpected signing method")
			}

			return s.secret, nil
		},
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithIssuer(s.issuer),
	)

	if err != nil {
		return nil, fmt.Errorf("validate access token: %w", err)
	}

	if !token.Valid {
		return nil, errors.New("invalid access token")
	}

	if claims.UserID == "" {
		return nil, errors.New("access token user ID is missing")
	}

	return &TokenClaims{
		UserID: claims.UserID,
		Email:  claims.Email,
	}, nil
}

func (s *tokenService) GenerateRefreshToken() (string, error) {
	const tokenSize = 32

	b := make([]byte, tokenSize)

	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate refresh token: %w", err)
	}

	return hex.EncodeToString(b), nil
}

func (s *tokenService) HashRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))

	return hex.EncodeToString(sum[:])
}
