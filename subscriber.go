package chitosocket

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/gobwas/ws"
	"github.com/godzie44/go-uring/reactor"
	"github.com/godzie44/go-uring/uring"
	"github.com/sairash/chitosocket/buffer"
	"golang.org/x/sys/unix"
)

type Subscriber struct {
	fd    uint
	id    string
	rooms []string
	mu    sync.RWMutex

	reactor *reactor.NetworkReactor

	buffer *buffer.Buffer

	isClosed atomic.Bool

	CloseChan chan uint
}

func (s *Subscriber) Capture() {
	s.buffer.AlwaysAvailableForWrite()
	op := uring.Recv(uintptr(s.fd), s.buffer.Buf[s.buffer.W:], 0)

	s.reactor.Queue(op, func(event uring.CQEvent) {
		if s.isClosed.Load() {
			return
		}

		err := event.Error()
		if err != nil || event.Res == 0 {
			s.Close()
			return
		}

		s.buffer.W += int(event.Res)

		s.PrintCurrentBuffer()
		go s.Capture()
	})
}

func (s *Subscriber) PrintCurrentBuffer() {
	readCheckpoint := s.buffer.R

	h, err := ws.ReadHeader(s.buffer)
	s.buffer.R = readCheckpoint
	if err != nil {
		fmt.Println("header: ", err, h)
		return
	}

	required := int(h.Length) + ws.HeaderSize(h)
	s.buffer.ReadyWrite(required)

	if s.buffer.Len() < required {
		return
	}

	f, err := ws.ReadFrame(s.buffer)
	if err != nil {
		fmt.Println("frame:", err, f.Header)
		return
	}
	fmt.Println("fin: ", f.Header.Fin, f.Header.OpCode, f.Header.Rsv, f.Header.Length)

	s.buffer.R = required

	unMaskBuffer(f.Payload, f.Header.Mask)
	fmt.Println(string(f.Payload))
	s.Write(f.Payload)
}

func (s *Subscriber) Write(p []byte) error {
	buf, err := ws.CompileFrame(ws.NewTextFrame(p))
	if err != nil {
		return err
	}

	op := uring.Send(uintptr(s.fd), buf, 0)

	fmt.Println(buf)

	s.reactor.Queue(op, func(event uring.CQEvent) {
		if err := event.Error(); err == nil {
			return
		}

		if event.Res > 0 {
			return
		}
		// if the buffer is not sent need to send it again
	})
	return nil
}

func (s *Subscriber) Close() error {
	s.isClosed.Store(true)

	err := unix.Close(int(s.fd))
	s.CloseChan <- s.fd

	if err != nil {
		return err
	}
	return nil
}
