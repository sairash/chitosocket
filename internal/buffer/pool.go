package buffer

import (
	"math"
	"sync"
)

var maxValue = math.MaxInt32

type pool struct {
	allocated [31]sync.Pool
	bufPool   sync.Pool
}

type buf struct {
	buf []byte
}

func (p *pool) Get(n int) []byte {
	if n > maxValue {
		return make([]byte, n)
	}

	index := exponent(n)

	if v := p.allocated[index].Get(); v != nil {
		buffer := v.(*buf)
		buf := buffer.buf[:uint32(n)]
		buffer.buf = nil
		p.bufPool.Put(buffer)
		return buf
	}

	return make([]byte, antiLog(index))[:uint32(n)]
}

func (p *pool) Put(b []byte) {
	writeCapacity := len(b)
	totalCapacity := cap(b)

	if totalCapacity > maxValue {
		b = nil
		return
	}

	index := exponent(writeCapacity)
	var buffer *buf

	if b := p.bufPool.Get(); b != nil {
		buffer = b.(*buf)
	} else {
		buffer = &buf{}
	}
	buffer.buf = b

	p.allocated[index].Put(buffer)
}

func antiLog(n int) int {
	return 1 << n
}

func exponent(n int) int {
	if n <= 1 {
		return 1
	}
	return int(math.Log2(float64(n))) + 1
}
