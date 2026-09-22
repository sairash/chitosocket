// Package chitosocket is the best socket server
package chitosocket

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"syscall"

	"github.com/gobwas/ws"
	"github.com/godzie44/go-uring/reactor"
	"github.com/godzie44/go-uring/uring"
	"github.com/puzpuzpuz/xsync/v4"
	"github.com/sairash/chitosocket/buffer"
	"golang.org/x/sys/unix"
)

// supply max cpu if you want specific cpu usages
// if 0 is given it uses all of the cpu available
func New(maxCPU int) (*ChitoSocket, error) {
	if maxCPU < 1 {
		maxCPU = runtime.NumCPU()
	} else {
		maxCPU = min(runtime.NumCPU(), maxCPU)
	}

	conf := DefaultConfig(maxCPU)
	return NewWithConfig(conf)
}

// start a server with custom config
func NewWithConfig(config Config) (*ChitoSocket, error) {
	rings, closeRings, err := uring.CreateMany(config.MaxRings, uring.MaxEntries>>3, config.WorkerPool)
	if err != nil {
		return nil, err
	}

	netReactor, err := reactor.NewNet(rings)
	if err != nil {
		return nil, err
	}

	go func() {
		netReactor.Run(context.Background())
	}()

	cs := newChitoSocket(config.MaxShards, closeRings, netReactor, config.Server)
	return cs, nil
}

func (cs *ChitoSocket) UpgradeHTTP(r *http.Request, w http.ResponseWriter) error {
	conn, rw, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		return err
	}

	newConnFD, err := detach(conn)
	if err != nil {
		return err
	}

	n := rw.Reader.Buffered()

	cache, err := rw.Peek(n)
	if err != nil {
		return err
	}

	id, err := randomSessionKey(cs.ServerID)
	if err != nil {
		return err
	}
	fmt.Println(id)

	s := Subscriber{
		fd: newConnFD,
		id: id,

		buffer:  buffer.NewBuffer(cache),
		reactor: cs.Reactor,
	}

	s.isClosed.Store(false)
	go s.Capture()

	return nil
}

func detach(conn net.Conn) (int, error) {
	sc, ok := conn.(syscall.Conn)
	if !ok {
		return -1, ConnectionNoExposeSyscallConn
	}

	raw, err := sc.SyscallConn()
	if err != nil {
		return -1, err
	}

	var (
		newFD    int
		dupError error
	)

	err = raw.Control(func(fd uintptr) {
		newFD, dupError = unix.FcntlInt(fd, unix.F_DUPFD_CLOEXEC, 0)
	})
	if err != nil {
		return -1, err
	}

	if dupError != nil {
		return -1, err
	}

	if err := conn.Close(); err != nil {
		unix.Close(newFD)
		return -1, err
	}

	return newFD, nil
}

func newChitoSocket(maxShards int, close uring.Defer, reactor *reactor.NetworkReactor, id int) *ChitoSocket {
	if maxShards < 1 {
		maxShards = 1
	}

	cs := &ChitoSocket{
		Hubs:       make([]hub, maxShards),
		CloseRings: close,
		Reactor:    reactor,
		count:      uint32(maxShards),
		ServerID:   id,
	}

	for k := range cs.Hubs {
		cs.Hubs[k] = xsync.NewMap[string, *Room]()
	}

	return cs
}
