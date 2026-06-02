package stream

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	flv "github.com/yutopp/go-flv"
	flvtag "github.com/yutopp/go-flv/tag"
	rtmp "github.com/yutopp/go-rtmp"
	rtmpmsg "github.com/yutopp/go-rtmp/message"

	"github.com/Nextasy01/chtiwt/internal/store/queries"
)

// onRTMPConnect is the per-TCP-connection factory go-rtmp calls when a
// client opens the socket. We return a fresh Handler that owns the FLV
// remux + ffmpeg pipeline for that publisher.
func (s *Service) onRTMPConnect(conn net.Conn) (io.ReadWriteCloser, *rtmp.ConnConfig) {
	h := &rtmpHandler{svc: s, remote: conn.RemoteAddr().String()}
	return conn, &rtmp.ConnConfig{
		Handler: h,
		ControlState: rtmp.StreamControlStateConfig{
			DefaultBandwidthWindowSize: 6 * 1024 * 1024 / 8,
		},
	}
}

// rtmpHandler implements rtmp.Handler. One handler per publisher connection.
type rtmpHandler struct {
	rtmp.DefaultHandler

	svc    *Service
	remote string

	// Populated on OnPublish if auth succeeds.
	sess   *session
	flvEnc *flv.Encoder
	pipeR  *io.PipeReader
}

func (h *rtmpHandler) OnServe(_ *rtmp.Conn) {}

func (h *rtmpHandler) OnConnect(_ uint32, _ *rtmpmsg.NetConnectionConnect) error {
	return nil
}

func (h *rtmpHandler) OnCreateStream(_ uint32, _ *rtmpmsg.NetConnectionCreateStream) error {
	return nil
}

// OnPublish is where authentication happens. The PublishingName field is
// the stream key the client appended to the RTMP URL.
func (h *rtmpHandler) OnPublish(_ *rtmp.StreamContext, _ uint32, cmd *rtmpmsg.NetStreamPublish) error {
	streamKey := cmd.PublishingName
	if streamKey == "" {
		return fmt.Errorf("publish: empty stream key")
	}

	ctx, cancelLookup := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelLookup()

	channel, err := h.svc.lookupChannelByStreamKey(ctx, streamKey)
	if err != nil {
		slog.Info("rtmp publish rejected", "remote", h.remote, "reason", err.Error())
		return err
	}

	// Start a fresh session.
	startedAt := time.Now()
	row, err := h.svc.q.CreateStreamSession(context.Background(), queries.CreateStreamSessionParams{
		ChannelID: channel.ID,
		StartedAt: pgtype.Timestamptz{Time: startedAt, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("create stream_session row: %w", err)
	}

	// Per-channel output dir for HLS.
	outDir := filepath.Join(h.svc.stateDir, channel.Name)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("mkdir hls out: %w", err)
	}

	// Set up the FLV→ffmpeg pipe. The order matters: spawn ffmpeg and start
	// the drain goroutine BEFORE constructing the flv.Encoder. NewEncoder
	// writes the 9-byte FLV header synchronously, and io.Pipe has no buffer,
	// so without an active reader on pipeR the call would block forever.
	pipeR, pipeW := io.Pipe()

	supCtx, cancel := context.WithCancel(context.Background())

	proc, err := startFFmpeg(supCtx, ffmpegOpts{
		binPath:     h.svc.ffmpegPath,
		channelName: channel.Name,
		stateDir:    h.svc.stateDir,
		segDuration: h.svc.segDuration,
		listSize:    h.svc.listSize,
	})
	if err != nil {
		cancel()
		_ = pipeW.Close()
		_ = pipeR.Close()
		return fmt.Errorf("start ffmpeg: %w", err)
	}

	// Bridge: FLV bytes from pipeR → ffmpeg's stdin.
	go func() {
		if _, err := io.Copy(proc.stdin, pipeR); err != nil && !errors.Is(err, io.ErrClosedPipe) {
			slog.Debug("flv→ffmpeg copy ended", "channel", channel.Name, "err", err)
		}
		_ = proc.stdin.Close()
	}()

	enc, err := flv.NewEncoder(pipeW, flv.FlagsAudio|flv.FlagsVideo)
	if err != nil {
		cancel()
		_ = pipeW.Close()
		_ = pipeR.Close()
		return fmt.Errorf("flv encoder: %w", err)
	}

	sess := &session{
		channelID:   channel.ID,
		channelName: channel.Name,
		title:       channel.Title,
		startedAt:   startedAt,
		pipeWriter:  pipeW,
		cancel:      cancel,
		ffmpeg:      proc,
		dbRowID:     row.ID,
		stateDir:    h.svc.stateDir,
		parent:      h.svc,
		doneCh:      make(chan struct{}),
	}

	if !h.svc.registry.add(sess) {
		// Lost the race against another publisher on the same channel.
		cancel()
		_ = pipeW.Close()
		_ = pipeR.Close()
		// Mark the row we just created as orphan_swept so it isn't open.
		endReason := "orphan_swept"
		_ = h.svc.q.EndStreamSession(context.Background(), queries.EndStreamSessionParams{
			ID:        row.ID,
			EndedAt:   pgtype.Timestamptz{Time: time.Now(), Valid: true},
			EndReason: &endReason,
		})
		return ErrAlreadyLive
	}

	// Watchdog: if ffmpeg exits on its own (crash, OBS disconnect after
	// stdin EOF, etc.), make sure the session is torn down with the right
	// reason.
	go func() {
		<-proc.done
		reason := "normal"
		if proc.waitErr != nil {
			reason = "crashed"
		}
		sess.tearDown(reason)
	}()

	h.sess = sess
	h.flvEnc = enc
	h.pipeR = pipeR

	slog.Info("stream started", "channel", channel.Name, "remote", h.remote)
	return nil
}

// OnSetDataFrame forwards the @setDataFrame metadata (resolution, framerate,
// codec params) into the FLV stream so ffmpeg has it before the first frame.
func (h *rtmpHandler) OnSetDataFrame(timestamp uint32, data *rtmpmsg.NetStreamSetDataFrame) error {
	if h.flvEnc == nil {
		return nil
	}
	var script flvtag.ScriptData
	if err := flvtag.DecodeScriptData(bytes.NewReader(data.Payload), &script); err != nil {
		// Some encoders send odd ScriptData — log and continue.
		slog.Debug("decode setDataFrame", "err", err)
		return nil
	}
	return h.flvEnc.Encode(&flvtag.FlvTag{
		TagType:   flvtag.TagTypeScriptData,
		Timestamp: timestamp,
		Data:      &script,
	})
}

func (h *rtmpHandler) OnAudio(timestamp uint32, payload io.Reader) error {
	if h.flvEnc == nil {
		return nil
	}
	var audio flvtag.AudioData
	if err := flvtag.DecodeAudioData(payload, &audio); err != nil {
		return err
	}
	return h.flvEnc.Encode(&flvtag.FlvTag{
		TagType:   flvtag.TagTypeAudio,
		Timestamp: timestamp,
		Data:      &audio,
	})
}

func (h *rtmpHandler) OnVideo(timestamp uint32, payload io.Reader) error {
	if h.flvEnc == nil {
		return nil
	}
	var video flvtag.VideoData
	if err := flvtag.DecodeVideoData(payload, &video); err != nil {
		return err
	}
	return h.flvEnc.Encode(&flvtag.FlvTag{
		TagType:   flvtag.TagTypeVideo,
		Timestamp: timestamp,
		Data:      &video,
	})
}

// OnClose runs when the RTMP connection drops (clean or otherwise). We
// route it through tearDown like every other exit path. The watchdog
// goroutine launched in OnPublish may also fire — sync.Once makes that safe.
func (h *rtmpHandler) OnClose() {
	if h.sess != nil {
		h.sess.tearDown("normal")
	}
}
