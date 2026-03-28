package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"modelprobe/backend/internal/model"
	"modelprobe/backend/internal/repository"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrAdminAuthRequired  = errors.New("admin authentication required")
	ErrInvalidCredentials = errors.New("invalid admin credentials")
	ErrAdminNotConfigured = errors.New("admin account is not configured")
)

type AdminAuthService struct {
	repo       *repository.PostgresRepository
	sessionTTL time.Duration
}

func NewAdminAuthService(repo *repository.PostgresRepository, sessionTTL time.Duration) *AdminAuthService {
	return &AdminAuthService{
		repo:       repo,
		sessionTTL: sessionTTL,
	}
}

func (s *AdminAuthService) EnsureBootstrapAdmin(ctx context.Context, username string, password string) error {
	count, err := s.repo.CountAdminUsers(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	if username == "" || password == "" {
		return nil
	}

	passwordHash, err := hashPassword(password)
	if err != nil {
		return fmt.Errorf("hash bootstrap admin password: %w", err)
	}

	if _, err := s.repo.CreateAdminUser(ctx, username, passwordHash); err != nil {
		return fmt.Errorf("create bootstrap admin user: %w", err)
	}

	return nil
}

func (s *AdminAuthService) GetSessionState(ctx context.Context, rawToken string) (model.AdminSessionResponse, error) {
	count, err := s.repo.CountAdminUsers(ctx)
	if err != nil {
		return model.AdminSessionResponse{}, err
	}

	response := model.AdminSessionResponse{
		Configured: count > 0,
	}
	if rawToken == "" || count == 0 {
		return response, nil
	}

	user, _, err := s.getUserBySessionToken(ctx, rawToken)
	if err != nil {
		if errors.Is(err, ErrAdminAuthRequired) {
			return response, nil
		}
		return model.AdminSessionResponse{}, err
	}

	profile := user.Profile()
	response.Authenticated = true
	response.User = &profile
	return response, nil
}

func (s *AdminAuthService) Login(ctx context.Context, username string, password string, userAgent string, ipAddress string) (*model.AdminUserProfile, string, time.Time, error) {
	if err := s.repo.DeleteExpiredAdminSessions(ctx); err != nil {
		return nil, "", time.Time{}, err
	}

	user, err := s.repo.GetAdminUserByUsername(ctx, strings.TrimSpace(username))
	if err != nil {
		return nil, "", time.Time{}, err
	}
	if user == nil {
		count, countErr := s.repo.CountAdminUsers(ctx)
		if countErr != nil {
			return nil, "", time.Time{}, countErr
		}
		if count == 0 {
			return nil, "", time.Time{}, ErrAdminNotConfigured
		}
		return nil, "", time.Time{}, ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(strings.TrimSpace(password))); err != nil {
		return nil, "", time.Time{}, ErrInvalidCredentials
	}

	rawToken, tokenHash, err := newSessionToken()
	if err != nil {
		return nil, "", time.Time{}, fmt.Errorf("create admin session token: %w", err)
	}

	expiresAt := time.Now().UTC().Add(s.sessionTTL)
	if _, err := s.repo.CreateAdminSession(ctx, user.ID, tokenHash, expiresAt, userAgent, ipAddress); err != nil {
		return nil, "", time.Time{}, err
	}
	if err := s.repo.TouchAdminUserLastLogin(ctx, user.ID); err != nil {
		return nil, "", time.Time{}, err
	}

	refreshedUser, err := s.repo.GetAdminUserByID(ctx, user.ID)
	if err != nil {
		return nil, "", time.Time{}, err
	}
	if refreshedUser == nil {
		return nil, "", time.Time{}, ErrAdminAuthRequired
	}

	profile := refreshedUser.Profile()
	return &profile, rawToken, expiresAt, nil
}

func (s *AdminAuthService) Logout(ctx context.Context, rawToken string) error {
	if rawToken == "" {
		return nil
	}
	return s.repo.DeleteAdminSessionByTokenHash(ctx, hashToken(rawToken))
}

func (s *AdminAuthService) RequireSession(ctx context.Context, rawToken string) (*model.AdminUserRecord, error) {
	user, _, err := s.getUserBySessionToken(ctx, rawToken)
	return user, err
}

func (s *AdminAuthService) UpdateAdminAccount(ctx context.Context, adminUserID int64, currentPassword string, username string, newPassword string) (*model.AdminUserProfile, error) {
	user, err := s.repo.GetAdminUserByID(ctx, adminUserID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrAdminAuthRequired
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(strings.TrimSpace(currentPassword))); err != nil {
		return nil, ErrInvalidCredentials
	}

	nextUsername := strings.TrimSpace(username)
	if nextUsername == "" {
		nextUsername = user.Username
	}

	updatePassword := strings.TrimSpace(newPassword) != ""
	nextPasswordHash := user.PasswordHash
	if updatePassword {
		nextHash, err := hashPassword(strings.TrimSpace(newPassword))
		if err != nil {
			return nil, fmt.Errorf("hash new password: %w", err)
		}
		nextPasswordHash = nextHash
	}

	updated, err := s.repo.UpdateAdminUserCredentials(ctx, adminUserID, nextUsername, nextPasswordHash, updatePassword)
	if err != nil {
		return nil, err
	}

	profile := updated.Profile()
	return &profile, nil
}

func (s *AdminAuthService) getUserBySessionToken(ctx context.Context, rawToken string) (*model.AdminUserRecord, *model.AdminSessionRecord, error) {
	if rawToken == "" {
		return nil, nil, ErrAdminAuthRequired
	}

	if err := s.repo.DeleteExpiredAdminSessions(ctx); err != nil {
		return nil, nil, err
	}

	session, err := s.repo.GetAdminSessionByTokenHash(ctx, hashToken(rawToken))
	if err != nil {
		return nil, nil, err
	}
	if session == nil {
		return nil, nil, ErrAdminAuthRequired
	}

	expiresAt, err := time.Parse(time.RFC3339Nano, session.ExpiresAt)
	if err != nil {
		return nil, nil, fmt.Errorf("parse admin session expiry: %w", err)
	}
	if !expiresAt.After(time.Now().UTC()) {
		_ = s.repo.DeleteAdminSessionByTokenHash(ctx, session.TokenHash)
		return nil, nil, ErrAdminAuthRequired
	}

	user, err := s.repo.GetAdminUserByID(ctx, session.AdminUserID)
	if err != nil {
		return nil, nil, err
	}
	if user == nil {
		_ = s.repo.DeleteAdminSessionByTokenHash(ctx, session.TokenHash)
		return nil, nil, ErrAdminAuthRequired
	}

	if err := s.repo.TouchAdminSession(ctx, session.TokenHash); err != nil {
		return nil, nil, err
	}

	return user, session, nil
}

func hashPassword(password string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

func newSessionToken() (string, string, error) {
	seed := make([]byte, 32)
	if _, err := rand.Read(seed); err != nil {
		return "", "", err
	}

	rawToken := base64.RawURLEncoding.EncodeToString(seed)
	return rawToken, hashToken(rawToken), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
