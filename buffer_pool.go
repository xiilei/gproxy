package gproxy

import (
	"bufio"
	"io"
	"sync"
)

// BufferPool a buffer pool to
// get byte slices for use by io.CopyBuffer
type BufferPool struct {
	pool sync.Pool // 目前只是包了sync.Pool
}

var defaultBufferPool = NewBufferPool()
var (
	bufioReaderPool sync.Pool
)

// NewBufferPool creates a buffer pool
func NewBufferPool() *BufferPool {
	return &BufferPool{
		pool: sync.Pool{
			New: func() interface{} {
				return make([]byte, 32*1024)
			},
		},
	}
}

// Get returns a byte slice
func (bp *BufferPool) Get() []byte {
	return bp.pool.Get().([]byte)
}

// Put add a byte slice to pool
func (bp *BufferPool) Put(buf []byte) {
	bp.pool.Put(buf)
}

func newBufioReader(r io.Reader) *bufio.Reader {
	if v := bufioReaderPool.Get(); v != nil {
		br := v.(*bufio.Reader)
		br.Reset(r)
		return br
	}
	return bufio.NewReader(r)
}

func putBufioReader(br *bufio.Reader) {
	br.Reset(nil)
	bufioReaderPool.Put(br)
}
