package stream

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
)

// ffmpegOpts describes how the supervisor should invoke ffmpeg.
type ffmpegOpts struct {
	binPath     string
	channelName string
	stateDir    string // root dir; per-channel dir = stateDir/<channel>
	segDuration int    // seconds
	listSize    int    // playlist window
}

// ffmpegProc owns a running ffmpeg process: its stdin pipe (FLV in),
// stderr tail (for crash diagnostics), and lifecycle.
type ffmpegProc struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stderrBuf *ringBuffer
	done      chan struct{}
	waitErr   error
}

// startFFmpeg spawns ffmpeg with the given options. The returned ffmpegProc
// is owned by the caller — call wait() or close stdin to drive shutdown.
//
// The context is the supervisor's lifecycle. exec.CommandContext arranges
// for a Kill if the context is cancelled before the process exits.
func startFFmpeg(ctx context.Context, opts ffmpegOpts) (*ffmpegProc, error) {
	outDir := filepath.Join(opts.stateDir, opts.channelName)
	segPattern := filepath.Join(outDir, "seg_%05d.ts")
	playlist := filepath.Join(outDir, "index.m3u8")
	thumb := filepath.Join(outDir, "thumb.jpg")

	// One ffmpeg invocation, two outputs:
	//   1. HLS playlist + .ts segments (copy, no transcode)
	//   2. A 320x180 JPEG snapshot, overwritten every 10s, used as the
	//      thumbnail on the directory cards. `-update 1` tells the image
	//      muxer to atomically replace the same file instead of producing
	//      thumb_001.jpg, thumb_002.jpg, ...
	args := []string{
		"-hide_banner",
		"-loglevel", "warning",
		"-f", "flv", "-i", "pipe:0",
		// HLS output (the live stream).
		"-map", "0",
		"-c:v", "copy", "-c:a", "copy",
		"-f", "hls",
		"-hls_time", fmt.Sprint(opts.segDuration),
		"-hls_list_size", fmt.Sprint(opts.listSize),
		"-hls_flags", "delete_segments+independent_segments",
		"-hls_segment_filename", segPattern,
		playlist,
		// Thumbnail output.
		"-map", "0:v:0",
		"-vf", "fps=1/10,scale=320:180:force_original_aspect_ratio=decrease,pad=320:180:(ow-iw)/2:(oh-ih)/2:black",
		"-update", "1",
		"-y",
		"-f", "image2",
		thumb,
	}

	cmd := exec.CommandContext(ctx, opts.binPath, args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg stdin pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("ffmpeg stderr pipe: %w", err)
	}

	ring := newRingBuffer(8 * 1024)

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("ffmpeg start: %w", err)
	}

	p := &ffmpegProc{
		cmd:       cmd,
		stdin:     stdin,
		stderrBuf: ring,
		done:      make(chan struct{}),
	}

	// Drain stderr line-by-line into the ring buffer. Lines aren't preserved
	// as lines on read — the ring only needs to retain the tail of bytes for
	// post-mortem logging.
	go func() {
		scan := bufio.NewScanner(stderrPipe)
		// ffmpeg can emit long warning lines; bump max token size.
		scan.Buffer(make([]byte, 64*1024), 1024*1024)
		for scan.Scan() {
			_, _ = ring.Write(scan.Bytes())
			_, _ = ring.Write([]byte{'\n'})
		}
	}()

	go func() {
		p.waitErr = cmd.Wait()
		close(p.done)
	}()

	return p, nil
}

// stderrTail returns a copy of the most recent stderr bytes.
func (p *ffmpegProc) stderrTail() []byte {
	return p.stderrBuf.Snapshot()
}
