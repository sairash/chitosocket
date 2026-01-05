package chitosocket

import (
	"net"
	"sync"
	"sync/atomic"
)

// Socket is the main server struct with event handlers
type Socket struct {
	epoller     *Epoll
	hub         *HubShards
	On          map[string]HandlerFunc
	eventChan   chan *workerEvent
	workerCount int
	closed      atomic.Bool
}

// HandlerFunc is the callback type for event handlers
type HandlerFunc func(subs *Subscriber, data []byte)

// workerEvent represents an event dispatched to worker goroutines
type workerEvent struct {
	subscriber *Subscriber
	message    *SubscriberMessage
	eventType  eventType
	generation uint64 // Generation of subscriber when event was created
}

type eventType uint8

const (
	eventMessage eventType = iota
	eventDisconnect
)

// SubscriberMessage represents the JSON message format
type SubscriberMessage struct {
	Event string
	Data  []byte
}

// Subscriber represents a connected WebSocket client
type Subscriber struct {
	ID       string
	Outbound chan []byte
	Metadata sync.Map
	Rooms    sync.Map // O(1) room membership lookup

	fd           int
	conn         net.Conn
	writeStarted atomic.Bool   // Ensures write loop starts only once
	closed       atomic.Bool   // Tracks if subscriber has been cleaned up
	closeChan    chan struct{} // Signals write loop to stop
	closeOnce    sync.Once     // Ensures channels are only closed once
	generation   uint64        // Incremented on each reuse to detect stale events
}

// Global generation counter
var globalGeneration atomic.Uint64

// Subscriber pool to reduce allocations
var subscriberPool = sync.Pool{
	New: func() interface{} {
		return &Subscriber{
			Rooms:     sync.Map{},
			closeChan: make(chan struct{}),
		}
	},
}

// NewSubscriber creates a new subscriber with initialized fields
func NewSubscriber(id string, conn net.Conn, fd int) *Subscriber {
	sub := subscriberPool.Get().(*Subscriber)
	sub.ID = id
	sub.conn = conn
	sub.fd = fd
	sub.Outbound = make(chan []byte, 64)
	sub.closeChan = make(chan struct{}) // Always create fresh channels
	sub.closeOnce = sync.Once{}         // Reset sync.Once for new subscriber
	sub.generation = globalGeneration.Add(1)
	sub.writeStarted.Store(false)
	sub.closed.Store(false)
	return sub
}

// recycleSubscriber returns a subscriber to the pool
func recycleSubscriber(sub *Subscriber) {
	sub.ID = ""
	sub.conn = nil
	sub.fd = 0
	sub.Outbound = nil
	sub.Metadata = sync.Map{}
	sub.Rooms.Range(func(key, _ interface{}) bool {
		sub.Rooms.Delete(key)
		return true
	})
	subscriberPool.Put(sub)
}

// Send safely sends a message to the subscriber's outbound channel
// Returns false if the subscriber is closed or the channel is full
func (s *Subscriber) Send(payload []byte) bool {
	if s.closed.Load() {
		return false
	}
	select {
	case s.Outbound <- payload:
		return true
	default:
		// Channel full, drop message (backpressure)
		return false
	}
}

// IsClosed returns whether the subscriber has been cleaned up
func (s *Subscriber) IsClosed() bool {
	return s.closed.Load()
}

// Room represents a group of subscribers
type Room struct {
	members map[int]*Subscriber // O(1) add/remove by fd
	lock    sync.RWMutex
}

// Room pool
var roomPool = sync.Pool{
	New: func() interface{} {
		return &Room{
			members: make(map[int]*Subscriber, 8),
		}
	},
}

// NewRoom creates a new room with initialized map
func NewRoom() *Room {
	r := roomPool.Get().(*Room)
	// Clear existing members
	for k := range r.members {
		delete(r.members, k)
	}
	return r
}

// recycleRoom returns a room to the pool
func recycleRoom(r *Room) {
	for k := range r.members {
		delete(r.members, k)
	}
	roomPool.Put(r)
}

// Hub represents a shard containing multiple rooms
type Hub struct {
	rooms map[string]*Room
	lock  sync.RWMutex // Properly initialized mutex
}

// loadOrStore loads a room from the hub if it exists, otherwise creates a new room
func (h *Hub) loadOrStore(room string) *Room {
	h.lock.RLock()
	r, exists := h.rooms[room]
	h.lock.RUnlock()

	if exists {
		return r
	}

	// Create room outside the lock
	newR := NewRoom()

	h.lock.Lock()
	// Check again after acquiring write lock
	if r, exists = h.rooms[room]; exists {
		h.lock.Unlock()
		return r
	}

	h.rooms[room] = newR
	h.lock.Unlock()

	return newR
}

// HubShards distributes rooms across multiple shards to reduce contention
type HubShards struct {
	hubs  []Hub
	count uint32
}

// getShard returns the hub shard for a given room key (no allocation)
func (hs *HubShards) getShard(key string) *Hub {
	return &hs.hubs[fnvHash32(key)%hs.count]
}

// Config holds server configuration
type Config struct {
	NumWorkers    int
	NumShards     int
	EventChanSize int
}

// DefaultConfig returns sensible defaults
func DefaultConfig(numCPU int) Config {
	return Config{
		NumWorkers:    numCPU * 2,
		NumShards:     numCPU * 4,
		EventChanSize: 8192,
	}
}
