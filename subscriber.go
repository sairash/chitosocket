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
	s.buffer.AlwaysEnoughSpace()
	op := uring.Recv(uintptr(s.fd), s.buffer.Buf[s.buffer.W:], 0)

	s.reactor.Queue(op, func(event uring.CQEvent) {
		fmt.Println("reactor getting data")
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

	f, err := ws.ReadFrame(s.buffer)
	if err != nil {
		fmt.Println(err, f.Header)
		s.buffer.R = readCheckpoint
		return
	}
	fmt.Println("fin: ", f.Header.Fin, f.Header.OpCode, f.Header.Rsv, f.Header.Length)

	s.buffer.R = int(f.Header.Length) + ws.HeaderSize(f.Header)
	// fmt.Printf("%v", f)
}

func (s *Subscriber) Close() error {
	s.isClosed.Store(true)
	err := unix.Close(s.fd)
	if err != nil {
		return err
	}
	return nil
}
