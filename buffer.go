package chitosocket

import "io"

const minFreeSpace = 4096

type buffer struct {
	buf  []byte
	used uint64
	read uint64
}

func newBuffer(length int, previousBytes []byte) *buffer {
	previousLen := len(previousBytes)
	b := &buffer{
		buf:  make([]byte, length),
		used: 0,
	}

	if previousLen > 0 {
		copy(b.buf, previousBytes)
	}

	return b
}

func (buf *buffer) Len() int {
	return len(buf.buf)
}

func (buf *buffer) Cap() int {
	return cap(buf.buf)
}

func (buf *buffer) Grow(n int) {
	if n <= 0 {
		return
	}

	required := buf.Len() + n

	if required < buf.Cap() {
		return
	}

	newCap := buf.Cap() * 2
	if newCap == 0 {
		newCap = 64
	}

	if newCap < required {
		newCap = required
	}

	newBuf := make([]byte, newCap)
	copy(newBuf, buf.buf)

	buf.buf = newBuf
}

func (buf *buffer) Read(p []byte) (int, error) {
	if buf.read >= buf.used {
		return 0, io.EOF
	}

	unread := buf.buf[buf.read:buf.used]
	n := copy(p, unread)

	buf.read += uint64(n)

	return n, nil
}

func (buf *buffer) AlwaysEnoughSpace() {
	currentCap := uint64(buf.Len())
	freeSpace := currentCap - buf.used

	if freeSpace > minFreeSpace {
		return
	}

	if currentCap == 0 {
		currentCap = 8192
	}

	buf.Grow(int(currentCap))
}

// shrink will delete the prefix and make the total length
func (buf *buffer) Shrink(till, total int) {
	newBuf := make([]byte, total)
	copy(newBuf, buf.buf[till:])
	buf.buf = newBuf
	buf.used = 0
}
