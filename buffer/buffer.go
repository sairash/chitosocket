package buffer

import (
	"io"
)

var Bufferpool = new(pool)

const (
	smallByte = 64
)

type Buffer struct {
	Buf   []byte
	W     int
	R     int
	small [smallByte]byte // so we have very fast small allocations
}

func NewBuffer(b []byte) *Buffer {
	buf := new(Buffer)

	if len(b) > 0 {
		buf.Buf = buf.getBuf(len(b))
		copy(buf.Buf, b)
		buf.W = len(b)
		return buf
	}

	buf.Buf = buf.getBuf(smallByte)
	return buf
}

func NewBufferWithLength(n int) *Buffer {
	buf := new(Buffer)

	if n <= 0 {
		n = smallByte
	}

	buf.Buf = buf.getBuf(n)
	return buf
}

func (b *Buffer) Grow(n int) {
	wCap := b.W
	aCap := cap(b.Buf)

	// we have the capacity just increase it and return
	if wCap+n <= aCap {
		b.Buf = b.Buf[:wCap+n]
		return
	}

	buffSize := wCap - b.R
	leastNeededCap := (buffSize + n) * 2

	if aCap >= leastNeededCap {
		// just slide it back
		b.Buf = b.Buf[b.R:]
	} else {
		newByte := b.getBuf(leastNeededCap)
		copy(newByte, b.Buf[b.R:])

		b.putBufferPool()

		b.Buf = newByte
	}

	b.R = 0
	b.W = buffSize
	b.Buf = b.Buf[:buffSize+n]
}

func (b *Buffer) getBuf(n int) []byte {
	if n <= smallByte {
		return b.small[:n]
	}
	return Bufferpool.Get(n)
}

func (b *Buffer) putBufferPool() {
	if cap(b.Buf) > smallByte {
		Bufferpool.Put(b.Buf)
	}
	b.Buf = nil
}

func (b *Buffer) Read(buf []byte) (int, error) {
	used := b.W
	if b.R >= used {
		return 0, io.EOF
	}

	unread := b.Buf[b.R:used]
	n := copy(buf, unread)
	b.R += n

	return n, nil
}

func (b *Buffer) Write(buf []byte) (int, error) {
	b.Grow(len(buf))
	n := copy(b.Buf[b.W:], buf)
	b.W += n
	return n, nil
}

func (b *Buffer) Len() int {
	return b.W - b.R
}

func (b *Buffer) ReadyWrite(n int) {
	if cap(b.Buf)-b.W >= n {
		b.Buf = b.Buf[:b.W+n]
		return
	}

	unread := b.W - b.R

	nextBuf := b.getBuf(unread + n)
	copy(nextBuf, b.Buf[b.R:b.W])

	b.putBufferPool()

	b.Buf = nextBuf
	b.R = 0
	b.W = unread
}

func (b *Buffer) AlwaysAvailableForWrite() {
	if cap(b.Buf)-b.W < smallByte {
		b.ReadyWrite(smallByte)
	}
}
