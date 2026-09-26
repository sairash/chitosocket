package chitosocket

import (
	"github.com/godzie44/go-uring/reactor"

	"github.com/godzie44/go-uring/uring"
	"github.com/puzpuzpuz/xsync/v4"
)

type ChitoSocket struct {
	Hubs       []hub
	CloseRings uring.Defer
	Reactor    *reactor.NetworkReactor

	count    uint32
	ServerID int
	subs     *xsync.Map[uint, *Subscriber]

	subCloseChan chan uint
}

type hub struct {
	hub   *xsync.Map[string, *Room]
	count uint32
}

// fd is going to be the key
type Room struct {
	room *xsync.Map[uint, *Subscriber]
	name string
}

type Config struct {
	Server     int // new server must have a new server id
	NumCPU     int
	MaxShards  int
	MaxRings   int
	WorkerPool int
}

func DefaultConfig(numCPU int) Config {
	return Config{
		Server:     0,
		NumCPU:     numCPU,
		MaxShards:  numCPU * 4,
		MaxRings:   8,
		WorkerPool: 2,
	}
}
