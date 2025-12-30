package chitosocket

import (
	"net"
	"reflect"
	"sync"
	"sync/atomic"
	"unsafe"
)

// Message pool for reducing allocations during parsing
var messagePool = sync.Pool{
	New: func() interface{} {
		return &SubscriberMessage{}
	},
}

// GetMessage gets a message from the pool
func GetMessage() *SubscriberMessage {
	return messagePool.Get().(*SubscriberMessage)
}

// PutMessage returns a message to the pool after resetting it
func PutMessage(msg *SubscriberMessage) {
	msg.Event = ""
	msg.Data = nil
	messagePool.Put(msg)
}

// workerEvent pool
var workerEventPool = sync.Pool{
	New: func() interface{} {
		return &workerEvent{}
	},
}

// gets a worker event from the pool
func getWorkerEvent() *workerEvent {
	return workerEventPool.Get().(*workerEvent)
}

// returns a worker event to the pool
func putWorkerEvent(ev *workerEvent) {
	ev.subscriber = nil
	ev.message = nil
	ev.generation = 0
	workerEventPool.Put(ev)
}

// slice pool for room operations
var subscriberSlicePool = sync.Pool{
	New: func() interface{} {
		s := make([]*Subscriber, 0, 64)
		return &s
	},
}

func getSubscriberSlice() *[]*Subscriber {
	return subscriberSlicePool.Get().(*[]*Subscriber)
}

func putSubscriberSlice(s *[]*Subscriber) {
	*s = (*s)[:0]
	subscriberSlicePool.Put(s)
}

// String slice pool for room lists
var stringSlicePool = sync.Pool{
	New: func() interface{} {
		s := make([]string, 0, 16)
		return &s
	},
}

func getStringSlice() *[]string {
	return stringSlicePool.Get().(*[]string)
}

func putStringSlice(s *[]string) {
	*s = (*s)[:0]
	stringSlicePool.Put(s)
}

// Byte buffer pool for JSON marshaling
var byteBufferPool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, 0, 512)
		return &b
	},
}

func getByteBuffer() *[]byte {
	return byteBufferPool.Get().(*[]byte)
}

func putByteBuffer(b *[]byte) {
	*b = (*b)[:0]
	byteBufferPool.Put(b)
}

var idCounter atomic.Uint64

const base62Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// randString generates a fast unique ID using atomic counter
func randString(n int) string {
	id := idCounter.Add(1)

	var buf [16]byte
	for i := 0; i < 16; i++ {
		buf[i] = base62Chars[id%62]
		id /= 62
	}

	if n > 16 {
		n = 16
	}
	return string(buf[:n])
}

const (
	fnvOffset32 = 2166136261
	fnvPrime32  = 16777619
)

// computes FNV-1a hash without allocations
func fnvHash32(s string) uint32 {
	h := uint32(fnvOffset32)
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= fnvPrime32
	}
	return h
}

func getFD(conn net.Conn) int {
	tcpConn := reflect.Indirect(reflect.ValueOf(conn)).FieldByName("conn")
	fdVal := tcpConn.FieldByName("fd")
	pfdVal := reflect.Indirect(fdVal).FieldByName("pfd")
	return int(pfdVal.FieldByName("Sysfd").Int())
}

// marshalMessageFast marshals a message with minimal allocations
// Format: {"event":"<event>","data":<data>}
func marshalMessageFast(buf *[]byte, event string, data []byte) {
	*buf = append(*buf, `{"event":"`...)
	*buf = append(*buf, event...)
	*buf = append(*buf, `","data":`...)
	if len(data) > 0 {
		*buf = append(*buf, data...)
	} else {
		*buf = append(*buf, "null"...)
	}
	*buf = append(*buf, '}')
}

// converts string to []byte without allocation
func unsafeStringToBytes(s string) []byte {
	return unsafe.Slice(unsafe.StringData(s), len(s))
}

// converts []byte to string without allocation
func unsafeBytesToString(b []byte) string {
	return unsafe.String(unsafe.SliceData(b), len(b))
}

