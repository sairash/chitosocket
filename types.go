package chitosocket

import (
	"net"

	"github.com/godzie44/go-uring/reactor"

	"github.com/godzie44/go-uring/uring"
	"github.com/puzpuzpuz/xsync/v4"
)

type ChitoSocket struct {
	Hubs       []Hub
	CloseRings uring.Defer
	Reactor    *reactor.NetworkReactor

	count uint32
}

func newChitoSocket(maxShards int, close uring.Defer, reactor *reactor.NetworkReactor) *ChitoSocket {
	if maxShards < 1 {
		maxShards = 1
	}

	cs := &ChitoSocket{
		Hubs:       make([]Hub, maxShards),
		CloseRings: close,
		Reactor:    reactor,
		count:      uint32(maxShards),
	}

	for k := range cs.Hubs {
		cs.Hubs[k] = xsync.NewMap[string, *Room]()
	}

	return cs
}

type Hub *xsync.Map[string, *Room]

// fd is going to be the key
type Room *xsync.Map[uint, *Subscriber]

type Subscriber struct {
	fd    int
	id    string
	conn  net.Conn
	rooms []string
}

type Config struct {
	NumCPU     int
	MaxShards  int
	MaxRings   int
	WorkerPool int
}

func DefaultConfig(numCpu int) Config {
	return Config{
		NumCPU:     numCpu,
		MaxShards:  numCpu * 4,
		MaxRings:   8,
		WorkerPool: 2,
	}
}
