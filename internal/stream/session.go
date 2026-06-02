package stream

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Nextasy01/chtiwt/internal/store/queries"
)

// session is the internal representation of a live stream pipeline:
//   RTMP publisher  →  FLV writer  →  io.Pipe  →  ffmpeg stdin  →  HLS files
//
// Every exit path (publisher disconnect, ffmpeg crash, server shutdown,
// supervisor abort) routes through a single tearDown(reason) so DB rows,
// the registry entry, and the HLS output dir all end up in a consistent
// state.
type session struct {
	channelID   int64
	channelName string
	title       string
	startedAt   time.Time

	pipeWriter *io.PipeWriter // FLV bytes go in here (used by ingest)

	cancel    context.CancelFunc
	ffmpeg    *ffmpegProc
	dbRowID   int64
	stateDir  string // root, not per-channel

	parent *Service

	once   sync.Once
	doneCh chan struct{}
}

func (s *session) toDTO() LiveChannel {
	return LiveChannel{
		ChannelName: s.channelName,
		Title:       s.title,
		StartedAt:   s.startedAt,
	}
}

// tearDown is idempotent. The first caller wins; later callers block until
// the first tearDown completes.
//
// reason should be one of: "normal", "crashed", "orphan_swept".
func (s *session) tearDown(reason string) {
	s.once.Do(func() {
		defer close(s.doneCh)

		// 1. Signal ffmpeg that no more FLV data is coming. ffmpeg flushes
		//    its pending segments and exits cleanly on stdin EOF.
		if s.pipeWriter != nil {
			_ = s.pipeWriter.Close()
		}

		// 2. Give ffmpeg a brief grace window to exit on its own.
		graceCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		if s.ffmpeg != nil {
			select {
			case <-s.ffmpeg.done:
				// clean exit
			case <-graceCtx.Done():
				// 3. Grace expired — force kill via the supervisor context.
				if s.cancel != nil {
					s.cancel()
				}
				// Block until the process is actually reaped to avoid leaking
				// zombies on Unix and dangling handles on Windows.
				<-s.ffmpeg.done
			}

			if s.ffmpeg.waitErr != nil {
				slog.Warn("ffmpeg exited non-zero",
					"channel", s.channelName,
					"reason", reason,
					"err", s.ffmpeg.waitErr,
					"stderr_tail", string(s.ffmpeg.stderrTail()),
				)
			}
		} else if s.cancel != nil {
			s.cancel()
		}

		// 4. Remove from in-memory registry.
		if s.parent != nil {
			s.parent.registry.remove(s.channelName)
		}

		// 5. Mark the DB row closed.
		if s.parent != nil && s.dbRowID != 0 {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			endReason := reason
			err := s.parent.q.EndStreamSession(ctx, queries.EndStreamSessionParams{
				ID:        s.dbRowID,
				EndedAt:   pgtype.Timestamptz{Time: time.Now(), Valid: true},
				EndReason: &endReason,
			})
			if err != nil {
				slog.Warn("end stream session row", "channel", s.channelName, "err", err)
			}
		}

		// 6. Clean up segment files. We don't keep VOD copies in MVP — the
		//    next publisher of this channel gets a fresh dir.
		if s.stateDir != "" {
			if err := os.RemoveAll(filepath.Join(s.stateDir, s.channelName)); err != nil {
				slog.Warn("remove channel state dir", "channel", s.channelName, "err", err)
			}
		}

		slog.Info("stream ended", "channel", s.channelName, "reason", reason)
	})
}
