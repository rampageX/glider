package anytls

import (
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"time"
)

var errStreamClosed = errors.New("stream closed")

type streamDeadline struct {
	mu    sync.Mutex
	timer *time.Timer
	ch    chan struct{}
	seq   uint64
}

func newStreamDeadline() *streamDeadline {
	return &streamDeadline{ch: make(chan struct{})}
}

func (d *streamDeadline) wait() <-chan struct{} {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.ch
}

func (d *streamDeadline) set(t time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.seq++
	seq := d.seq
	if d.timer != nil {
		d.timer.Stop()
		d.timer = nil
	}

	closed := false
	select {
	case <-d.ch:
		closed = true
	default:
	}

	if t.IsZero() {
		if closed {
			d.ch = make(chan struct{})
		}
		return
	}

	dur := time.Until(t)
	if dur <= 0 {
		if !closed {
			close(d.ch)
		}
		return
	}

	if closed {
		d.ch = make(chan struct{})
	}
	ch := d.ch
	d.timer = time.AfterFunc(dur, func() {
		d.mu.Lock()
		defer d.mu.Unlock()
		if d.seq != seq || d.ch != ch {
			return
		}
		select {
		case <-ch:
		default:
			close(ch)
		}
		d.timer = nil
	})
}

type stream struct {
	id uint32
	s  *session

	in       chan []byte
	readBuf  []byte
	readDone chan struct{}
	closeIn  sync.Once
	closeOut sync.Once

	readDeadline  *streamDeadline
	writeDeadline *streamDeadline

	mu     sync.Mutex
	closed bool
}

func newStream(id uint32, s *session) *stream {
	return &stream{
		id:            id,
		s:             s,
		in:            make(chan []byte, 32),
		readDone:      make(chan struct{}),
		readDeadline:  newStreamDeadline(),
		writeDeadline: newStreamDeadline(),
	}
}

func (st *stream) Read(p []byte) (int, error) {
	for len(st.readBuf) == 0 {
		select {
		case b := <-st.in:
			st.readBuf = b
		case <-st.readDone:
			return 0, io.EOF
		case <-st.readDeadline.wait():
			return 0, os.ErrDeadlineExceeded
		}
	}
	n := copy(p, st.readBuf)
	st.readBuf = st.readBuf[n:]
	return n, nil
}

func (st *stream) Write(p []byte) (int, error) {
	st.mu.Lock()
	closed := st.closed
	st.mu.Unlock()
	if closed {
		return 0, errStreamClosed
	}

	written := 0
	for len(p) > 0 {
		select {
		case <-st.writeDeadline.wait():
			if written > 0 {
				return written, os.ErrDeadlineExceeded
			}
			return 0, os.ErrDeadlineExceeded
		default:
		}

		n := min(len(p), maxFrameData)
		if err := st.s.writeFrame(frame{command: cmdPSH, streamID: st.id, data: p[:n]}); err != nil {
			return written, err
		}
		written += n
		p = p[n:]
	}
	return written, nil
}

func (st *stream) Close() error {
	st.mu.Lock()
	already := st.closed
	st.closed = true
	st.mu.Unlock()
	if !already {
		st.closeRead()
		st.closeOut.Do(func() {
			_ = st.s.writeFrame(frame{command: cmdFIN, streamID: st.id})
		})
		st.s.removeStream(st.id)
	}
	return nil
}

func (st *stream) closeRead() {
	st.closeIn.Do(func() { close(st.readDone) })
}

func (st *stream) push(data []byte) {
	cp := make([]byte, len(data))
	copy(cp, data)
	select {
	case <-st.readDone:
	case <-st.s.done:
	case st.in <- cp:
	}
}

func (st *stream) LocalAddr() net.Addr  { return st.s.conn.LocalAddr() }
func (st *stream) RemoteAddr() net.Addr { return st.s.conn.RemoteAddr() }
func (st *stream) SetDeadline(t time.Time) error {
	st.readDeadline.set(t)
	st.writeDeadline.set(t)
	return nil
}
func (st *stream) SetReadDeadline(t time.Time) error {
	st.readDeadline.set(t)
	return nil
}
func (st *stream) SetWriteDeadline(t time.Time) error {
	st.writeDeadline.set(t)
	return nil
}
