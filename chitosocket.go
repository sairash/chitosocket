package chitosocket

import (
	"fmt"
	"net"
	"net/http"
	"runtime"
	"syscall"

	"github.com/gobwas/ws"
	"github.com/godzie44/go-uring/reactor"
	"github.com/godzie44/go-uring/uring"
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

	cs := newChitoSocket(config.MaxShards, closeRings, netReactor)
	return cs, nil
}

func (cs *ChitoSocket) UpgradeHTTP(r *http.Request, w http.ResponseWriter) error {
	conn, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		return err
	}

	newConnFD, err := detach(conn)
	if err != nil {
		return err
	}
	fmt.Println(newConnFD)
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
