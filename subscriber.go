package chitosocket

import (
	"fmt"
	"sync/atomic"

	"github.com/godzie44/go-uring/reactor"
	"github.com/godzie44/go-uring/uring"
	"golang.org/x/sys/unix"
)

type Subscriber struct {
	fd    int
	id    string
	rooms []string

	reactor *reactor.NetworkReactor

	// need better buffer implementation
	buff []byte
	used uint64

	isClosed atomic.Bool
}

func (s *Subscriber) Capture() {
	op := uring.Recv(uintptr(s.fd), s.buff[s.used:], 0)

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
		go s.Capture()

		s.used += uint64(event.Res)

		s.PrintCurrentBuffer()
	})
}

func (s *Subscriber) PrintCurrentBuffer() {
	buff := s.buff[:s.used]

	fmt.Println(buff)
}
