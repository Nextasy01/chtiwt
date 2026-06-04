package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Nextasy01/chtiwt/internal/store/queries"
	"github.com/Nextasy01/chtiwt/internal/stream"
)

type Service struct {
	pool       *pgxpool.Pool
	q          *queries.Queries
	sessionTTL time.Duration
}

func NewService(pool *pgxpool.Pool, sessionTTL time.Duration) *Service {
	return &Service{
		pool:       pool,
		q:          queries.New(pool),
		sessionTTL: sessionTTL,
	}
}

// Signup creates a user, an associated channel, and an initial stream key.
// Returns the user plus the plain stream key — caller must show the plain
// key to the streamer exactly once; the DB only retains the hash.
func (s *Service) Signup(ctx context.Context, username, email, password string) (queries.User, string, error) {
	if err := validateUsername(username); err != nil {
		return queries.User{}, "", err
	}
	if err := validateEmail(email); err != nil {
		return queries.User{}, "", err
	}
	if err := validatePassword(password); err != nil {
		return queries.User{}, "", err
	}

	passwordHash, err := hashPassword(password)
	if err != nil {
		return queries.User{}, "", fmt.Errorf("hash password: %w", err)
	}

	plainKey, keyHash, err := stream.GenerateKey()
	if err != nil {
		return queries.User{}, "", fmt.Errorf("generate stream key: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return queries.User{}, "", fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)

	user, err := qtx.CreateUser(ctx, queries.CreateUserParams{
		Username:     username,
		Email:        email,
		PasswordHash: passwordHash,
	})
	if err != nil {
		return queries.User{}, "", mapUniqueViolation(err)
	}

	_, err = qtx.CreateChannel(ctx, queries.CreateChannelParams{
		UserID:        user.ID,
		Name:          username,
		StreamKeyHash: keyHash,
	})
	if err != nil {
		return queries.User{}, "", fmt.Errorf("create channel: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return queries.User{}, "", fmt.Errorf("commit: %w", err)
	}
	return user, plainKey, nil
}

func (s *Service) Login(ctx context.Context, username, password string) (queries.User, error) {
	user, err := s.q.GetUserByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return queries.User{}, ErrBadCredentials
		}
		return queries.User{}, fmt.Errorf("lookup user: %w", err)
	}
	if !verifyPassword(user.PasswordHash, password) {
		return queries.User{}, ErrBadCredentials
	}
	return user, nil
}

func (s *Service) CreateSession(ctx context.Context, userID int64) (string, error) {
	token, err := newSessionToken()
	if err != nil {
		return "", err
	}
	expiresAt := pgtype.Timestamptz{Time: time.Now().Add(s.sessionTTL), Valid: true}
	if _, err := s.q.CreateSession(ctx, queries.CreateSessionParams{
		ID:        token,
		UserID:    userID,
		ExpiresAt: expiresAt,
	}); err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}
	return token, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.q.DeleteSession(ctx, token)
}

func (s *Service) SessionUser(ctx context.Context, token string) (queries.User, error) {
	row, err := s.q.GetSessionWithUser(ctx, token)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return queries.User{}, ErrNoSession
		}
		return queries.User{}, fmt.Errorf("lookup session: %w", err)
	}
	return queries.User{
		ID:           row.UserID,
		Username:     row.UserUsername,
		Email:        row.UserEmail,
		PasswordHash: row.UserPasswordHash,
		CreatedAt:    row.UserCreatedAt,
	}, nil
}

func (s *Service) ChannelForUser(ctx context.Context, userID int64) (queries.Channel, error) {
	c, err := s.q.GetChannelByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return queries.Channel{}, ErrNoChannel
		}
		return queries.Channel{}, fmt.Errorf("lookup channel: %w", err)
	}
	return c, nil
}

func (s *Service) ChannelByName(ctx context.Context, name string) (queries.Channel, error) {
	c, err := s.q.GetChannelByName(ctx, name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return queries.Channel{}, ErrNoChannel
		}
		return queries.Channel{}, fmt.Errorf("lookup channel: %w", err)
	}
	return c, nil
}

func (s *Service) RegenerateStreamKey(ctx context.Context, channelID int64) (string, error) {
	plain, hash, err := stream.GenerateKey()
	if err != nil {
		return "", fmt.Errorf("generate key: %w", err)
	}
	if err := s.q.UpdateChannelStreamKey(ctx, queries.UpdateChannelStreamKeyParams{
		ID:            channelID,
		StreamKeyHash: hash,
	}); err != nil {
		return "", fmt.Errorf("update key: %w", err)
	}
	return plain, nil
}

func (s *Service) UpdateChannelTitle(ctx context.Context, channelID int64, title string) error {
	if len(title) > 200 {
		title = title[:200]
	}
	return s.q.UpdateChannelTitle(ctx, queries.UpdateChannelTitleParams{
		ID:    channelID,
		Title: title,
	})
}

// SweepExpiredSessions deletes expired session rows. Safe to call on boot
// or periodically from a ticker.
func (s *Service) SweepExpiredSessions(ctx context.Context) error {
	return s.q.DeleteExpiredSessions(ctx)
}

// mapUniqueViolation turns pg unique-constraint errors into typed auth errors.
func mapUniqueViolation(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		switch pgErr.ConstraintName {
		case "users_username_key":
			return ErrUsernameTaken
		case "users_email_key":
			return ErrEmailTaken
		}
	}
	return fmt.Errorf("create user: %w", err)
}
