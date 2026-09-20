package chitosocket

import (
	"fmt"
	"sync/atomic"

	"github.com/gobwas/ws"
	"github.com/godzie44/go-uring/reactor"
	"github.com/godzie44/go-uring/uring"
	"github.com/sairash/chitosocket/buffer"
	"golang.org/x/sys/unix"
)

type Subscriber struct {
	fd    int
	id    string
	rooms []string

	reactor *reactor.NetworkReactor

	buffer *buffer.Buffer

	isClosed atomic.Bool
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
			unix.Close(s.fd)
			s.isClosed.Store(true)
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
}

func (s *Subscriber) Close() error {
	s.isClosed.Store(true)
	err := unix.Close(s.fd)
	if err != nil {
		return err
	}
	return nil
}
