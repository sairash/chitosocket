package chitosocket

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"net/http"
	"runtime"
	"syscall"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/tidwall/gjson"
)

// initializes and starts chitoSocket server
// use 0 to use all of the available CPU Cores
func StartUp(numCPU int) (*Socket, error) {
	numCPU = min(runtime.NumCPU(), numCPU)
	if numCPU < 1 {
		numCPU = runtime.NumCPU()
	}

	config := DefaultConfig(numCPU)
	return StartUpWithConfig(config)
}

// initializes the server with custom configuration
func StartUpWithConfig(config Config) (*Socket, error) {
	epoll, err := newEpoll()
	if err != nil {
		return nil, err
	}

	var rLimit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit); err != nil {
		return nil, err
	}
	rLimit.Cur = rLimit.Max
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rLimit); err != nil {
		log.Printf("Warning: could not increase file descriptor limit: %v", err)
	}

	socket := &Socket{
		epoller:     epoll,
		hub:         newHub(config.NumShards),
		On:          make(map[string]HandlerFunc, 7),
		eventChan:   make(chan *workerEvent, config.EventChanSize),
		workerCount: config.NumWorkers,
	}

	socket.On["ping"] = func(sub *Subscriber, data []byte) {
		socket.EmitDirect(sub, "pong", data)
	}

	socket.startWorkers()
	go socket.epollLoop()

	return socket, nil
}

// creates a new sharded hub
func newHub(numOfShards int) *HubShards {
	if numOfShards < 1 {
		numOfShards = 1
	}

	hs := &HubShards{
		hubs:  make([]Hub, numOfShards),
		count: uint32(numOfShards),
	}

	for i := range hs.hubs {
		hs.hubs[i].rooms = make(map[string]*Room, 64)
	}

	return hs
}

// launches the worker goroutine pool
func (socket *Socket) startWorkers() {
	for i := 0; i < socket.workerCount; i++ {
		go socket.worker()
	}
}

// processes events from the event channel
func (socket *Socket) worker() {
	for event := range socket.eventChan {
		socket.handleEvent(event)
		putWorkerEvent(event)
	}
}

// processes a single event
func (socket *Socket) handleEvent(event *workerEvent) {
	if event.subscriber == nil {
		if event.message != nil {
			PutMessage(event.message)
		}
		return
	}

	if event.generation != event.subscriber.generation {
		if event.message != nil {
			PutMessage(event.message)
		}
		return
	}

	switch event.eventType {
	case eventMessage:
		if event.subscriber.closed.Load() {
			if event.message != nil {
				PutMessage(event.message)
			}
			return
		}
		if event.message != nil {
			if handler, ok := socket.On[event.message.Event]; ok {
				handler(event.subscriber, event.message.Data)
			}
			PutMessage(event.message)
		}

	case eventDisconnect:
		if handler, ok := socket.On["disconnect"]; ok {
			handler(event.subscriber, nil)
		}
		if event.message != nil {
			PutMessage(event.message)
		}
		socket.cleanupSubscriber(event.subscriber)
	}
}

// this is the main event loop that reads from epoll and dispatches to workers
func (socket *Socket) epollLoop() {
	for !socket.closed.Load() {
		subscribers, err := socket.epoller.Wait()
		if err != nil {
			continue
		}

		for _, sub := range subscribers {
			if sub == nil || sub.closed.Load() {
				continue
			}

			conn := sub.conn
			if conn == nil {
				continue
			}

			msg, _, err := wsutil.ReadClientData(conn)
			if err != nil {
				socket.dispatchDisconnect(sub)
				continue
			}

			parsedMsg, err := parseMessage(msg)
			if err != nil {
				continue
			}

			socket.dispatchMessage(sub, parsedMsg)
		}
	}
}

// sends a message event to the worker pool
func (socket *Socket) dispatchMessage(sub *Subscriber, msg *SubscriberMessage) {
	if sub.closed.Load() {
		PutMessage(msg)
		return
	}

	event := getWorkerEvent()
	event.subscriber = sub
	event.message = msg
	event.eventType = eventMessage
	event.generation = sub.generation

	select {
	case socket.eventChan <- event:
	default:
		socket.handleEvent(event)
		putWorkerEvent(event)
	}
}

// sends a disconnect event to the worker pool
func (socket *Socket) dispatchDisconnect(sub *Subscriber) {
	if sub.closed.Load() {
		return
	}

	event := getWorkerEvent()
	event.subscriber = sub
	event.eventType = eventDisconnect
	event.generation = sub.generation

	select {
	case socket.eventChan <- event:
	default:
		socket.handleEvent(event)
		putWorkerEvent(event)
	}
}

// parses JSON message using gjson for speed
func parseMessage(input []byte) (*SubscriberMessage, error) {
	result := gjson.ParseBytes(input)

	eventResult := result.Get("event")
	if !eventResult.Exists() || eventResult.String() == "" {
		return nil, fmt.Errorf("no event found")
	}

	msg := GetMessage()
	msg.Event = eventResult.String()

	dataResult := result.Get("data")
	if dataResult.Exists() {
		msg.Data = unsafeStringToBytes(dataResult.Raw)
	} else {
		msg.Data = nil
	}

	return msg, nil
}

// upgrades an HTTP connection to WebSocket
func (socket *Socket) UpgradeConnection(r *http.Request, w http.ResponseWriter) (net.Conn, *bufio.ReadWriter, *Subscriber, error) {
	conn, bufioRW, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		return nil, nil, nil, err
	}

	fd := getFD(conn)
	sub := NewSubscriber(randString(12), conn, fd)

	if err := socket.epoller.Add(sub); err != nil {
		conn.Close()
		return nil, nil, nil, fmt.Errorf("failed to add to epoll: %w", err)
	}

	socket.AddToRoom(sub, sub.ID)
	socket.startWriteLoop(sub)

	return conn, bufioRW, sub, nil
}

// starts the async write goroutine for a subscriber
func (socket *Socket) startWriteLoop(sub *Subscriber) {
	if sub.writeStarted.Swap(true) {
		return
	}

	conn := sub.conn
	if conn == nil {
		return
	}

	go func() {
		for {
			select {
			case msg, ok := <-sub.Outbound:
				if !ok {
					return
				}
				if sub.closed.Load() {
					return
				}
				if err := wsutil.WriteServerMessage(conn, ws.OpText, msg); err != nil {
					return
				}
			case <-sub.closeChan:
				return
			}
		}
	}()
}

// adds a subscriber to a room
func (socket *Socket) AddToRoom(sub *Subscriber, room string) {
	hub := socket.hub.getShard(room)

	hub.lock.Lock()
	r, exists := hub.rooms[room]
	if !exists {
		r = NewRoom()
		hub.rooms[room] = r
	}
	hub.lock.Unlock()

	r.lock.Lock()
	r.members[sub.fd] = sub
	r.lock.Unlock()

	sub.roomsMu.Lock()
	sub.Rooms[room] = struct{}{}
	sub.roomsMu.Unlock()
}

// removes a subscriber from a room
func (socket *Socket) RemoveFromRoom(sub *Subscriber, room string) {
	hub := socket.hub.getShard(room)

	hub.lock.RLock()
	r, exists := hub.rooms[room]
	hub.lock.RUnlock()

	if !exists {
		return
	}

	r.lock.Lock()
	delete(r.members, sub.fd)
	isEmpty := len(r.members) == 0
	r.lock.Unlock()

	sub.roomsMu.Lock()
	delete(sub.Rooms, room)
	sub.roomsMu.Unlock()

	if isEmpty {
		hub.lock.Lock()
		r.lock.RLock()
		if len(r.members) == 0 {
			delete(hub.rooms, room)
			recycleRoom(r)
		}
		r.lock.RUnlock()
		hub.lock.Unlock()
	}
}

// removes a subscriber from all rooms and epoll
func (socket *Socket) cleanupSubscriber(sub *Subscriber) {
	if sub.closed.Swap(true) {
		return
	}

	sub.closeOnce.Do(func() {
		if sub.closeChan != nil {
			close(sub.closeChan)
		}

		if sub.Outbound != nil {
			close(sub.Outbound)
		}
	})

	socket.epoller.Remove(sub)

	if sub.conn != nil {
		sub.conn.Close()
	}

	roomsSlice := getStringSlice()
	sub.roomsMu.RLock()
	for room := range sub.Rooms {
		*roomsSlice = append(*roomsSlice, room)
	}
	sub.roomsMu.RUnlock()

	for _, room := range *roomsSlice {
		socket.RemoveFromRoom(sub, room)
	}
	putStringSlice(roomsSlice)

	recycleSubscriber(sub)
}

// removes a subscriber from the server
func (socket *Socket) Remove(sub *Subscriber) error {
	socket.cleanupSubscriber(sub)
	return nil
}

// sends a message to all subscribers in the specified rooms
func (socket *Socket) Emit(event string, excludeSub *Subscriber, data []byte, rooms ...string) {
	if event == "" {
		return
	}

	buf := getByteBuffer()
	marshalMessageFast(buf, event, data)

	payload := make([]byte, len(*buf))
	copy(payload, *buf)
	putByteBuffer(buf)

	for _, room := range rooms {
		socket.emitToRoom(room, excludeSub, payload)
	}
}

// sends a message to all subscribers in a room
func (socket *Socket) emitToRoom(room string, excludeSub *Subscriber, payload []byte) {
	hub := socket.hub.getShard(room)

	hub.lock.RLock()
	r, exists := hub.rooms[room]
	hub.lock.RUnlock()

	if !exists {
		return
	}

	subscribers := getSubscriberSlice()
	r.lock.RLock()
	for _, sub := range r.members {
		*subscribers = append(*subscribers, sub)
	}
	r.lock.RUnlock()

	for _, sub := range *subscribers {
		if sub != nil && sub == excludeSub {
			continue
		}
		sub.Send(payload)
	}
	putSubscriberSlice(subscribers)
}

// sends a message directly to a subscriber (bypasses room lookup)
func (socket *Socket) EmitDirect(sub *Subscriber, event string, data []byte) {
	if event == "" || sub.IsClosed() {
		return
	}

	buf := getByteBuffer()
	marshalMessageFast(buf, event, data)

	payload := make([]byte, len(*buf))
	copy(payload, *buf)
	putByteBuffer(buf)

	sub.Send(payload)
}

// sends a message to all connected subscribers
func (socket *Socket) Broadcast(event string, data []byte) {
	if event == "" {
		return
	}

	buf := getByteBuffer()
	marshalMessageFast(buf, event, data)

	payload := make([]byte, len(*buf))
	copy(payload, *buf)
	putByteBuffer(buf)

	socket.epoller.connections.Range(func(_, value interface{}) bool {
		sub := value.(*Subscriber)
		sub.Send(payload)
		return true
	})
}

// returns all subscribers in a room
func (socket *Socket) GetRoomMembers(room string) []*Subscriber {
	hub := socket.hub.getShard(room)

	hub.lock.RLock()
	r, exists := hub.rooms[room]
	hub.lock.RUnlock()

	if !exists {
		return nil
	}

	r.lock.RLock()
	members := make([]*Subscriber, 0, len(r.members))
	for _, sub := range r.members {
		members = append(members, sub)
	}
	r.lock.RUnlock()

	return members
}

// returns the number of subscribers in a room
func (socket *Socket) GetRoomCount(room string) int {
	hub := socket.hub.getShard(room)

	hub.lock.RLock()
	r, exists := hub.rooms[room]
	hub.lock.RUnlock()

	if !exists {
		return 0
	}

	r.lock.RLock()
	count := len(r.members)
	r.lock.RUnlock()

	return count
}

// shuts down the socket server gracefully
func (socket *Socket) Close() error {
	socket.closed.Store(true)
	close(socket.eventChan)
	return socket.epoller.Close()
}
