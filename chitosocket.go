package chitosocket2

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"reflect"
	"sync"
	"syscall"

	"github.com/gobwas/ws"
)

type Socket struct {
	Epoller *Epoll
	Hub     *HubStruct
	On      map[string]func(subs **Subscriber, op ws.OpCode, data map[string]interface{})
}

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
	Subs map[string]map[string]*Subscriber
	lock *sync.RWMutex
}

// One Conenction has one subscriber
type Subscriber struct {
	Id         string
	Connection *net.Conn
	Data       map[string]interface{}
	Room       []interface{}
}

type Epoll struct {
	Fd          int
	Connections map[string]*Subscriber
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
		Connections: make(map[string]*Subscriber),
	}, nil
}

func MKHub() *HubStruct {
	return &HubStruct{
		Subs: make(map[string]map[string]*Subscriber),
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
		fmt.Printf("Error %e \n", err)
		os.Exit(1)
	}

	go start()
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

	new_subscriber.Id = string(fd)
	if err := socket.Epoller.Add(&new_subscriber, fd); err != nil {
		log.Printf("Failed to add connection %v", err)
		conn.Close()
	}

	socket.AddToRoom(&new_subscriber, string(fd))

	return conn, bufioRW, &new_subscriber, nil
}

func (socket *Socket) AddToRoom(subs *Subscriber, room string) {
	defer socket.Hub.lock.Unlock()
	subs.Room = append(subs.Room, room)
	socket.Hub.lock.Lock()
	socket.Hub.Subs[room][subs.Id] = subs
}

func (e *Epoll) Add(new_subscriber *Subscriber, fd int) error {
	err := syscall.EpollCtl(e.Fd, syscall.EPOLL_CTL_ADD, fd, &syscall.EpollEvent{Events: syscall.EPOLLIN | syscall.EPOLLHUP, Fd: int32(fd)})
	if err != nil {
		return err
	}
	e.lock.Lock()
	defer e.lock.Unlock()

	e.Connections[string(fd)] = new_subscriber

	return nil
}

func (new_subscriber *Subscriber) websocketFD() int {
	tcpConn := reflect.Indirect(reflect.ValueOf(*new_subscriber.Connection)).FieldByName("conn")
	fdVal := tcpConn.FieldByName("fd")
	pfdVal := reflect.Indirect(fdVal).FieldByName("pfd")

	return int(pfdVal.FieldByName("Sysfd").Int())
}
