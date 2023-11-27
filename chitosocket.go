package chitosocket

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"reflect"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"

	"golang.org/x/sys/unix"
)

var (
	// Heart Of Socket
	Epoller *Epoll
	// Heart of room
	Hub *HubStruct

	// Heart of Server Listening Function
	On = map[string]func(subs **Subscriber, op ws.OpCode, data map[string]interface{}){}
)

// For Random Number
const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
const (
	letterIdxBits = 6
	letterIdxMask = 1<<letterIdxBits - 1
	letterIdxMax  = 63 / letterIdxBits
)

// Socket Data structure
type socket_message struct {
	Event string                 `json:"event"`
	Data  map[string]interface{} `json:"data"`
}

// Room Hub Structure
type HubStruct struct {
	Subs map[string]*Subscription
	lock *sync.RWMutex
}

// Subscriber Linear Linked Tree
type Subscription struct {
	Subs *Subscriber
	Next *Subscription
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

// Random String Generator for socket id and inital room
func RandomStringGenerator(n int) string {
	var src = rand.NewSource(time.Now().UnixNano())
	b := make([]byte, n)
	for i, cache, remain := n-1, src.Int63(), letterIdxMax; i >= 0; {
		if remain == 0 {
			cache, remain = src.Int63(), letterIdxMax
		}
		if idx := int(cache & letterIdxMask); idx < len(letterBytes) {
			b[i] = letterBytes[idx]
			i--
		}
		cache >>= letterIdxBits
		remain--
	}

	return *(*string)(unsafe.Pointer(&b))
}

// Start a new Epoller
func MkEpoll() (*Epoll, error) {
	fd, err := unix.EpollCreate1(0)
	if err != nil {
		return nil, err
	}
	return &Epoll{
		Fd:          fd,
		lock:        &sync.RWMutex{},
		Connections: make(map[int]*Subscriber),
	}, nil
}

// Make a hub for storing room and connection
func MKHub() *HubStruct {
	return &HubStruct{
		Subs: make(map[string]*Subscription),
		lock: &sync.RWMutex{},
	}
}

// Start chitosocket server
func StartUp() {
	var rLimit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit); err != nil {
		panic(err)
	}
	rLimit.Cur = rLimit.Max
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rLimit); err != nil {
		panic(err)
	}

	Hub = MKHub()

	var err error
	Epoller, err = MkEpoll()
	if err != nil {
		panic(err)
	}

	go start()
}

// Upgrade http to socket Connection
func UpgradeConnection(r *http.Request, w http.ResponseWriter) (net.Conn, *bufio.ReadWriter, *Subscriber, error) {
	conn, bufioRW, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		return nil, nil, &Subscriber{}, err
	}
	id := RandomStringGenerator(15)
	new_subscriber := Subscriber{
		id,
		&conn,
		make(map[string]interface{}),
		[]string{},
	}
	if err := Epoller.Add(&new_subscriber); err != nil {
		log.Printf("Failed to add connection %v", err)
		conn.Close()
	}
	new_subscriber.AddToRoom(id)

	return conn, bufioRW, &new_subscriber, nil
}

// add new subscriber to hub room
func (s *Subscription) add_next(new_sub *Subscription) {
	here := s
	for here.Next != nil {
		here = here.Next
	}
	here.Next = new_sub
}

// Find, delete and return new subscription to remove the left connection from room
func (s *Subscription) remove_next(subs *Subscriber) *Subscription {

	help_subscription := s
	if help_subscription.Subs == subs {
		if help_subscription.Next != nil {
			help_subscription.Subs = help_subscription.Next.Subs
			help_subscription.Next = help_subscription.Next.Next
			return s
		} else {
			return nil
		}
	}

	for help_subscription.Next != nil {
		if help_subscription.Next.Subs == subs {
			help_subscription.Next = help_subscription.Next.Next
			return s
		}
		help_subscription = help_subscription.Next
	}
	return s
}

// Remove room from array of rooms in subsriber
func remove_element_from_array(strings_in_array []string, string_to_remove string) []string {
	deleteIndex := -1
	for i, s := range strings_in_array {
		if s == string_to_remove {
			deleteIndex = i
			break
		}
	}
	if deleteIndex != -1 {
		copy(strings_in_array[deleteIndex:], strings_in_array[deleteIndex+1:])
		strings_in_array = strings_in_array[:len(strings_in_array)-1]
	}
	return strings_in_array
}

// Add subscriber to a perticular room
func (subs *Subscriber) AddToRoom(room string) {
	Hub.lock.Lock()
	defer Hub.lock.Unlock()
	new_subscription := Subscription{subs, nil}

	if Hub.Subs[room] == nil {
		Hub.Subs[room] = &new_subscription
	} else {
		Hub.Subs[room].add_next(&new_subscription)
	}
	subs.Room = append(subs.Room, room)
}

// Remove Subscriber from a perticular room
func (subs *Subscriber) RemoveFromRoom(room string) {
	Hub.lock.Lock()
	defer Hub.lock.Unlock()
	subs.Room = remove_element_from_array(subs.Room, room)
	Hub.Subs[room].remove_next(subs)
}

// Add connection to both hub and epoller
func (e *Epoll) Add(new_subscriber *Subscriber) error {
	fd := websocketFD(new_subscriber)
	err := unix.EpollCtl(e.Fd, syscall.EPOLL_CTL_ADD, fd, &unix.EpollEvent{Events: unix.POLLIN | unix.POLLHUP, Fd: int32(fd)})
	if err != nil {
		return err
	}
	e.lock.Lock()
	defer e.lock.Unlock()

	e.Connections[fd] = new_subscriber

	return nil
}

// Remove connection from both hub and epoller
func (e *Epoll) Remove(sub **Subscriber) error {
	fd := websocketFD(*sub)
	err := unix.EpollCtl(e.Fd, syscall.EPOLL_CTL_DEL, fd, nil)
	if err != nil {
		return err
	}
	e.lock.Lock()
	defer e.lock.Unlock()

	Hub.lock.Lock()
	defer Hub.lock.Unlock()
	su := *sub

	for _, room := range su.Room {
		Hub.Subs[room] = Hub.Subs[room].remove_next(su)

		if Hub.Subs[room] == nil {
			delete(Hub.Subs, room)
		}
	}
	fmt.Println(sub)
	*sub = nil
	fmt.Println(*sub)
	delete(e.Connections, fd)

	return nil
}

// Wait For the connection to send any request
func (e *Epoll) Wait() ([]*Subscriber, error) {
	events := make([]unix.EpollEvent, 100)
	n, err := unix.EpollWait(e.Fd, events, 100)
	if err != nil {
		return nil, err
	}
	e.lock.RLock()
	defer e.lock.RUnlock()
	var connections []*Subscriber
	for i := 0; i < n; i++ {
		conn := e.Connections[int(events[i].Fd)]
		connections = append(connections, conn)
	}
	return connections, nil
}

// Find the FD of connection
func websocketFD(new_subscriber *Subscriber) int {
	tcpConn := reflect.Indirect(reflect.ValueOf(*new_subscriber.Connection)).FieldByName("conn")
	fdVal := tcpConn.FieldByName("fd")
	pfdVal := reflect.Indirect(fdVal).FieldByName("pfd")

	return int(pfdVal.FieldByName("Sysfd").Int())
}

// Loop through the linked list of subscribers and send it
func (s *Subscription) send_msg_in_room(event string, op ws.OpCode, data any, msg []byte) {
	if s != nil {
		Hub.lock.RLock()
		defer Hub.lock.RUnlock()

		if msg == nil {
			msg = []byte{}
			if event != "" {
				send_event := socket_message{event, data.(map[string]interface{})}
				send_event_string, err := json.Marshal(send_event)
				if err != nil {
					panic(err)
				}
				msg = []byte(string(send_event_string))
			} else {
				send_event_string, err := json.Marshal(data)
				if err != nil {
					panic(err)
				}
				msg = []byte(string(send_event_string))
			}

		}
		if s.Subs != nil {
			err := wsutil.WriteServerMessage(*s.Subs.Connection, op, []byte(msg))
			if err != nil {
				log.Printf("Failed to send %v", err)
			}

			if s.Next != nil {
				s.Next.send_msg_in_room(event, op, nil, []byte(msg))
			}
		}
	}
}

// Emit message to a room
func Emit(event string, room interface{}, op ws.OpCode, data any) {
	Hub.lock.RLock()
	defer Hub.lock.RUnlock()

	room_in_arr, ok := room.([]string)
	if ok {
		for _, room := range room_in_arr {
			go Hub.Subs[room].send_msg_in_room(event, op, data, nil)
		}
	} else {
		room_in_string, ok := room.(string)
		if ok {
			go Hub.Subs[room_in_string].send_msg_in_room(event, op, data, nil)
		}
	}
}

// Converting messages sent by client to interface
func unmarshal_msg(msg []byte) (string, map[string]interface{}, error) {
	Hub.lock.RLock()
	defer Hub.lock.RUnlock()

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

// For Getting And looping through all the connection to get Subscriber Message
func start() {
	for {
		subscriber, err := Epoller.Wait()
		if err != nil {
			// log.Printf("Failed to epoll wait %v", err)
			continue
		}
		for _, subs := range subscriber {
			if subs.Connection == nil {
				break
			}
			if msg, op, err := wsutil.ReadClientData(*subs.Connection); err != nil {
				if disconnect_event, ok := On["disconnect"]; ok {
					disconnect_event(&subs, op, map[string]interface{}{})
				}
				con := *subs.Connection
				if err := Epoller.Remove(&subs); err != nil {
					log.Printf("Failed to remove %v", err)
				}

				con.Close()

				fmt.Println(subs)

			} else {
				event, msg_from_client, err := unmarshal_msg(msg)
				if err != nil {
					log.Println(err)
				}
				if on_event, ok := On[event]; ok {
					on_event(&subs, op, msg_from_client)
				}
			}
		}
	}
}
