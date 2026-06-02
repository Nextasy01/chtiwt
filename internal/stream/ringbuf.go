package stream

import "sync"

// ringBuffer is a fixed-capacity byte buffer used to capture the tail of
// ffmpeg's stderr. When full, new writes overwrite the oldest bytes.
type ringBuffer struct {
	mu   sync.Mutex
	buf  []byte
	size int
	full bool
	head int // next write position
}

func newRingBuffer(capacity int) *ringBuffer {
	return &ringBuffer{buf: make([]byte, capacity), size: capacity}
}

func (r *ringBuffer) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := len(p)
	if n >= r.size {
		// Only the last r.size bytes matter.
		copy(r.buf, p[n-r.size:])
		r.head = 0
		r.full = true
		return n, nil
	}
	if r.head+n <= r.size {
		copy(r.buf[r.head:], p)
	} else {
		k := r.size - r.head
		copy(r.buf[r.head:], p[:k])
		copy(r.buf, p[k:])
		r.full = true
	}
	r.head += n
	if r.head >= r.size {
		r.head -= r.size
		r.full = true
	}
	return n, nil
}

// Snapshot returns a copy of the current contents in chronological order.
func (r *ringBuffer) Snapshot() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.full {
		out := make([]byte, r.head)
		copy(out, r.buf[:r.head])
		return out
	}
	out := make([]byte, r.size)
	copy(out, r.buf[r.head:])
	copy(out[r.size-r.head:], r.buf[:r.head])
	return out
}
