package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"reflect"
	"sync"
	"syscall"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

type Socket struct {
	Epoller *Epoll
	Hub     *HubStruct
	On      map[string]func(subs *Subscriber, op ws.OpCode, data map[string]interface{})
}

// Socket Data structure
type socket_message struct {
	Event string                 `json:"event"`
	Data  map[string]interface{} `json:"data"`
}

type HubSubStruct struct {
	Room map[string]*Subscriber
	lock *sync.RWMutex
}

// Room Hub Structure
type HubStruct struct {
	Subs map[string]*HubSubStruct
	lock *sync.RWMutex
}

// One Conenction has one subscriber
type Subscriber struct {
	Id         string
	Connection *net.Conn
	Data       map[string]interface{}
	Room       []string
}

type Epoll struct {
	Fd          int
	Connections map[int]*Subscriber
	lock        *sync.RWMutex
}

func MkEpoll() (*Epoll, error) {
	fd, err := syscall.EpollCreate1(0)
	if err != nil {
		return nil, err
	}
	return &Epoll{
		Fd:          fd,
		lock:        &sync.RWMutex{},
		Connections: make(map[int]*Subscriber),
	}, nil
}

func MKHub() *HubStruct {
	return &HubStruct{
		Subs: make(map[string]*HubSubStruct),
		lock: &sync.RWMutex{},
	}
}

func StartUp() *Socket {
	socket := Socket{}
	var rLimit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit); err != nil {
		panic(err)
	}
	rLimit.Cur = rLimit.Max
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rLimit); err != nil {
		panic(err)
	}

	socket.Hub = MKHub()

	var err error
	socket.Epoller, err = MkEpoll()
	if err != nil {
		log.Printf("Error %e \n", err)
		os.Exit(1)
	}
	socket.On = make(map[string]func(subs *Subscriber, op ws.OpCode, data map[string]interface{}))

	go socket.start()
	return &socket
}

func (socket *Socket) UpgradeConnection(r *http.Request, w http.ResponseWriter) (net.Conn, *bufio.ReadWriter, *Subscriber, error) {
	conn, bufioRW, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		return nil, nil, &Subscriber{}, err
	}
	new_subscriber := Subscriber{
		Connection: &conn,
		Data:       make(map[string]interface{}),
	}

	fd := new_subscriber.websocketFD()
	new_subscriber.Id = fmt.Sprint(fd)
	if err := socket.Epoller.Add(&new_subscriber, fd); err != nil {
		log.Printf("Failed to add connection %v", err)
		conn.Close()
	}

	socket.AddToRoom(&new_subscriber, new_subscriber.Id)

	return conn, bufioRW, &new_subscriber, nil
}

func (socket *Socket) AddToRoom(subs *Subscriber, room string) {
	if socket.Hub.Subs[room] == nil {
		socket.Hub.lock.Lock()
		socket.Hub.Subs[room] = &HubSubStruct{
			Room: make(map[string]*Subscriber),
			lock: &sync.RWMutex{},
		}
		socket.Hub.lock.Unlock()
	}
	room_s := socket.Hub.Subs[room]

	defer room_s.lock.Unlock()
	subs.Room = append(subs.Room, room)
	room_s.lock.Lock()
	room_s.Room[subs.Id] = subs
}

func (e *Epoll) Add(new_subscriber *Subscriber, fd int) error {
	err := syscall.EpollCtl(e.Fd, syscall.EPOLL_CTL_ADD, fd, &syscall.EpollEvent{Events: syscall.EPOLLIN | syscall.EPOLLHUP, Fd: int32(fd)})
	if err != nil {
		return err
	}
	e.lock.Lock()
	defer e.lock.Unlock()
	e.Connections[fd] = new_subscriber
	return nil
}

func (socket *Socket) RemoveFromRoom(subs *Subscriber, room string) {
	subs.Room = remove_element_from_array(subs.Room, room)
	room_s := socket.Hub.Subs[room]
	socket.Hub.lock.Lock()
	delete(room_s.Room, subs.Id)
	room_s.lock.Unlock()
}

func (socket *Socket) Emit(event string, data map[string]interface{}, room ...string) {
	if event == "" {
		log.Printf("Cannot have empty event")
		return
	}
	send_event := socket_message{event, data}
	msg, err := json.Marshal(send_event)
	if err != nil {
		panic(err)
	}

	for _, room := range room {
		room_s := socket.Hub.Subs[room]
		room_s.lock.RLock()
		for _, subs := range room_s.Room {
			err = wsutil.WriteServerMessage(*subs.Connection, 1, msg)
			if err != nil {
				log.Printf("Failed to send %v", err)
			}
		}
		room_s.lock.RUnlock()
	}
}

func (socket *Socket) Wait() ([]*Subscriber, error) {
	events := make([]syscall.EpollEvent, 100)
	n, err := syscall.EpollWait(socket.Epoller.Fd, events, 100)
	if err != nil {
		return nil, err
	}
	socket.Epoller.lock.RLock()
	defer socket.Epoller.lock.RUnlock()
	var connections []*Subscriber
	for i := 0; i < n; i++ {
		conn := socket.Epoller.Connections[int(events[i].Fd)]
		connections = append(connections, conn)
	}
	return connections, nil
}

func (socket *Socket) Remove(sub **Subscriber) error {
	su := *sub
	fd := su.websocketFD()
	err := syscall.EpollCtl(socket.Epoller.Fd, syscall.EPOLL_CTL_DEL, fd, nil)
	if err != nil {
		return err
	}

	for _, room := range su.Room {
		room_s := socket.Hub.Subs[room]
		if room_s == nil {
			continue
		}
		room_s.lock.Lock()
		delete(room_s.Room, su.Id)
		room_s.lock.Unlock()

		if room_s == nil {
			socket.Hub.lock.Lock()
			delete(socket.Hub.Subs, room)
			socket.Hub.lock.Unlock()
		}
	}
	*sub = nil
	socket.Epoller.lock.Lock()
	delete(socket.Epoller.Connections, fd)
	socket.Epoller.lock.Unlock()

	return nil
}

func remove_element_from_array(strings_in_array []string, string_to_remove string) []string {
	deleteIndex := -1
	for i, s := range strings_in_array {
		if s == string_to_remove {
			deleteIndex = i
			break
		}
	}
	if deleteIndex != -1 {
		strings_in_array[deleteIndex] = strings_in_array[len(strings_in_array)]
		strings_in_array = strings_in_array[:len(strings_in_array)-1]
	}
	return strings_in_array
}

func (new_subscriber *Subscriber) websocketFD() int {
	tcpConn := reflect.Indirect(reflect.ValueOf(*new_subscriber.Connection)).FieldByName("conn")
	fdVal := tcpConn.FieldByName("fd")
	pfdVal := reflect.Indirect(fdVal).FieldByName("pfd")

	return int(pfdVal.FieldByName("Sysfd").Int())
}

func unmarshal_msg(msg []byte) (string, map[string]interface{}, error) {
	msg_un_mar := socket_message{
		Event: "",
		Data:  make(map[string]interface{}),
	}
	json.Unmarshal(msg, &msg_un_mar)
	if msg_un_mar.Event == "" {
		return "", nil, fmt.Errorf("no event Found")
	}
	return msg_un_mar.Event, msg_un_mar.Data, nil
}

func (socket *Socket) start() {
	for {
		subscriber, err := socket.Wait()
		if err != nil {
			// log.Printf("Failed to epoll wait %v", err)
			continue
		}
		for _, subs := range subscriber {
			if subs.Connection == nil {
				break
			}
			if msg, op, err := wsutil.ReadClientData(*subs.Connection); err != nil {
				if disconnect_event, ok := socket.On["disconnect"]; ok {
					disconnect_event(subs, op, map[string]interface{}{})
				}
				con := *subs.Connection
				if err := socket.Remove(&subs); err != nil {
					log.Printf("Failed to remove %v", err)
				}

				con.Close()
			} else {
				event, msg_from_client, err := unmarshal_msg(msg)
				if err != nil {
					log.Println(err)
				}
				if on_event, ok := socket.On[event]; ok {
					on_event(subs, op, msg_from_client)
				}
			}
		}
	}
}
