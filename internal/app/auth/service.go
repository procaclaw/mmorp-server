package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/argon2"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrEmailInUse         = errors.New("email already in use")
)

type Service struct {
	db        *pgxpool.Pool
	jwtSecret []byte
	jwtTTL    time.Duration
}

type AuthResult struct {
	UserID uuid.UUID `json:"user_id"`
	Token  string    `json:"token"`
}

func NewService(db *pgxpool.Pool, jwtSecret string, jwtTTL time.Duration) *Service {
	return &Service{db: db, jwtSecret: []byte(jwtSecret), jwtTTL: jwtTTL}
}

func (s *Service) Register(ctx context.Context, email, password string) (AuthResult, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" || len(password) < 8 {
		return AuthResult{}, ErrInvalidCredentials
	}
	hash, err := hashPassword(password)
	if err != nil {
		return AuthResult{}, fmt.Errorf("hash password: %w", err)
	}
	id := uuid.New()
	_, err = s.db.Exec(ctx, `
INSERT INTO users (id, email, password_hash)
VALUES ($1, $2, $3)
`, id, email, hash)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return AuthResult{}, ErrEmailInUse
		}
		return AuthResult{}, fmt.Errorf("insert user: %w", err)
	}
	token, err := s.issueToken(id, email)
	if err != nil {
		return AuthResult{}, err
	}
	return AuthResult{UserID: id, Token: token}, nil
}

func (s *Service) Login(ctx context.Context, email, password string) (AuthResult, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	var id uuid.UUID
	var hash string
	err := s.db.QueryRow(ctx, `SELECT id, password_hash FROM users WHERE email = $1`, email).Scan(&id, &hash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AuthResult{}, ErrInvalidCredentials
		}
		return AuthResult{}, fmt.Errorf("query user: %w", err)
	}
	ok, err := verifyPassword(hash, password)
	if err != nil || !ok {
		return AuthResult{}, ErrInvalidCredentials
	}
	token, err := s.issueToken(id, email)
	if err != nil {
		return AuthResult{}, err
	}
	return AuthResult{UserID: id, Token: token}, nil
}

func (s *Service) ParseToken(tokenString string) (uuid.UUID, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return s.jwtSecret, nil
	})
	if err != nil || !token.Valid {
		return uuid.Nil, ErrInvalidCredentials
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return uuid.Nil, ErrInvalidCredentials
	}
	sub, ok := claims["sub"].(string)
	if !ok {
		return uuid.Nil, ErrInvalidCredentials
	}
	uid, err := uuid.Parse(sub)
	if err != nil {
		return uuid.Nil, ErrInvalidCredentials
	}
	return uid, nil
}

func (s *Service) issueToken(userID uuid.UUID, email string) (string, error) {
	now := time.Now().UTC()
	claims := jwt.MapClaims{
		"sub":   userID.String(),
		"email": email,
		"iat":   now.Unix(),
		"exp":   now.Add(s.jwtTTL).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return signed, nil
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	const memory = 64 * 1024
	const iterations = 3
	const parallelism = 2
	const keyLength = 32
	hash := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, keyLength)
	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", memory, iterations, parallelism, b64Salt, b64Hash), nil
}

func verifyPassword(encodedHash, password string) (bool, error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 {
		return false, fmt.Errorf("invalid hash format")
	}
	var memory uint32
	var iterations uint32
	var parallelism uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil {
		return false, fmt.Errorf("parse hash params: %w", err)
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, err
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, err
	}
	computed := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, uint32(len(hash)))
	if len(computed) != len(hash) {
		return false, nil
	}
	var diff byte
	for i := range hash {
		diff |= hash[i] ^ computed[i]
	}
	return diff == 0, nil
}
