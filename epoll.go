package chitosocket

import (
	"sync"
	"syscall"
)

const maxEpollEvents = 128

// Epoll manages epoll-based connection monitoring
type Epoll struct {
	fd          int
	connections sync.Map // map[int]*Subscriber
}

// pools EpollEvent slices to reduce allocations
var epollEventPool = sync.Pool{
	New: func() interface{} {
		events := make([]syscall.EpollEvent, maxEpollEvents)
		return &events
	},
}

// gets an event slice from the pool
func getEpollEvents() *[]syscall.EpollEvent {
	return epollEventPool.Get().(*[]syscall.EpollEvent)
}

// returns an event slice to the pool
func putEpollEvents(events *[]syscall.EpollEvent) {
	epollEventPool.Put(events)
}

// creates a new epoll instance
func newEpoll() (*Epoll, error) {
	fd, err := syscall.EpollCreate1(syscall.EPOLL_CLOEXEC)
	if err != nil {
		return nil, err
	}
	return &Epoll{
		fd: fd,
	}, nil
}

// registers a subscriber's connection with epoll
func (e *Epoll) Add(sub *Subscriber) error {
	err := syscall.EpollCtl(e.fd, syscall.EPOLL_CTL_ADD, sub.fd, &syscall.EpollEvent{
		Events: syscall.EPOLLIN | syscall.EPOLLHUP | syscall.EPOLLERR | syscall.EPOLLRDHUP,
		Fd:     int32(sub.fd),
	})
	if err != nil {
		return err
	}
	e.connections.Store(sub.fd, sub)
	return nil
}

// unregisters a subscriber's connection from epoll
func (e *Epoll) Remove(sub *Subscriber) error {
	err := syscall.EpollCtl(e.fd, syscall.EPOLL_CTL_DEL, sub.fd, nil)
	if err != nil {
		return err
	}
	e.connections.Delete(sub.fd)
	return nil
}

// retrieves a subscriber by file descriptor
func (e *Epoll) Get(fd int) (*Subscriber, bool) {
	val, ok := e.connections.Load(fd)
	if !ok {
		return nil, false
	}
	return val.(*Subscriber), true
}

// waits for epoll events and returns ready subscribers
func (e *Epoll) Wait() ([]*Subscriber, error) {
	events := getEpollEvents()
	defer putEpollEvents(events)

	n, err := syscall.EpollWait(e.fd, *events, -1)
	if err != nil {
		// just retry
		if err == syscall.EINTR {
			return nil, nil
		}
		return nil, err
	}

	if n == 0 {
		return nil, nil
	}

	subscribers := make([]*Subscriber, 0, n)
	for i := 0; i < n; i++ {
		fd := int((*events)[i].Fd)
		if sub, ok := e.Get(fd); ok {
			subscribers = append(subscribers, sub)
		}
	}
	return subscribers, nil
}

// closes the epoll file descriptor
func (e *Epoll) Close() error {
	return syscall.Close(e.fd)
}

