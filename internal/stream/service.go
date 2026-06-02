package stream

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	rtmp "github.com/yutopp/go-rtmp"

	"github.com/Nextasy01/chtiwt/internal/store/queries"
)

// Options bundles everything the Service needs at construction time.
type Options struct {
	Pool       *pgxpool.Pool
	RTMPAddr   string // e.g. ":1935"
	StateDir   string // e.g. "./state"
	FFmpegPath string // e.g. "ffmpeg" (resolved via $PATH)
}

// Service is the public facade for the stream subsystem. Web and other
// consumers depend on a narrow interface they declare themselves; this
// struct provides the methods they need.
type Service struct {
	pool       *pgxpool.Pool
	q          *queries.Queries
	registry   *liveRegistry
	stateDir   string
	ffmpegPath string
	rtmpAddr   string

	// HLS tuning — fixed for MVP.
	segDuration int
	listSize    int
}

func NewService(opts Options) *Service {
	return &Service{
		pool:        opts.Pool,
		q:           queries.New(opts.Pool),
		registry:    newLiveRegistry(),
		stateDir:    opts.StateDir,
		ffmpegPath:  opts.FFmpegPath,
		rtmpAddr:    opts.RTMPAddr,
		segDuration: 2,
		listSize:    6,
	}
}

// RecoverOnBoot resets all state left behind by a previous process: any
// stream_sessions still marked open are closed with end_reason='orphan_swept',
// and the on-disk segment dir is wiped so playlists never reference dead
// segments. Must be called before the RTMP listener accepts connections.
func (s *Service) RecoverOnBoot(ctx context.Context) error {
	if err := s.q.MarkOpenStreamSessionsOrphaned(ctx); err != nil {
		return fmt.Errorf("mark orphan sessions: %w", err)
	}
	if s.stateDir != "" {
		entries, err := os.ReadDir(s.stateDir)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("read state dir: %w", err)
		}
		for _, e := range entries {
			if err := os.RemoveAll(filepath.Join(s.stateDir, e.Name())); err != nil {
				slog.Warn("recover: remove state entry", "name", e.Name(), "err", err)
			}
		}
		if err := os.MkdirAll(s.stateDir, 0o755); err != nil {
			return fmt.Errorf("mkdir state: %w", err)
		}
	}
	return nil
}

// ListLive returns a snapshot of currently-live channels.
func (s *Service) ListLive() []LiveChannel {
	sessions := s.registry.snapshot()
	out := make([]LiveChannel, 0, len(sessions))
	for _, sess := range sessions {
		out = append(out, sess.toDTO())
	}
	return out
}

// GetByName returns the live channel snapshot for the given channel name.
func (s *Service) GetByName(name string) (LiveChannel, bool) {
	sess, ok := s.registry.get(name)
	if !ok {
		return LiveChannel{}, false
	}
	return sess.toDTO(), true
}

// ListenAndServeRTMP starts the RTMP server and blocks until ctx is done
// or the listener fails. Tear down individual sessions through the registry
// during shutdown so DB rows are closed cleanly.
func (s *Service) ListenAndServeRTMP(ctx context.Context) error {
	listener, err := net.Listen("tcp", s.rtmpAddr)
	if err != nil {
		return fmt.Errorf("rtmp listen: %w", err)
	}

	srv := rtmp.NewServer(&rtmp.ServerConfig{
		OnConnect: s.onRTMPConnect,
	})

	// Stop the listener when ctx is cancelled; this unblocks srv.Serve.
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	slog.Info("rtmp listening", "addr", s.rtmpAddr)
	if err := srv.Serve(listener); err != nil && !errors.Is(err, net.ErrClosed) {
		return fmt.Errorf("rtmp serve: %w", err)
	}
	return nil
}

// ShutdownLive tears down any sessions still open at shutdown.
func (s *Service) ShutdownLive() {
	for _, sess := range s.registry.snapshot() {
		sess.tearDown("orphan_swept")
	}
}

// lookupChannelByStreamKey is used by the ingest handler to authenticate
// a publisher. Returns ErrUnknownStreamKey on miss.
func (s *Service) lookupChannelByStreamKey(ctx context.Context, plainKey string) (queries.Channel, error) {
	hash := HashKey(plainKey)
	ch, err := s.q.GetChannelByStreamKeyHash(ctx, hash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return queries.Channel{}, ErrUnknownStreamKey
		}
		return queries.Channel{}, fmt.Errorf("lookup channel: %w", err)
	}
	return ch, nil
}
