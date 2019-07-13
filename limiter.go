package gproxy

import (
	"context"
	"io"

	"golang.org/x/time/rate"
)

const burst = 32 * 1024 * 1024

// RateReader a limited Reader
type RateReader struct {
	inner   io.ReadWriter
	context context.Context
	limiter *rate.Limiter
}

// NewRateReader return a new RateReader
func NewRateReader(reader io.ReadWriter, bps uint) *RateReader {
	logger.Printf("init bps %d \n", bps)
	limiter := rate.NewLimiter(rate.Limit(bps), burst)
	r := &RateReader{
		inner:   reader,
		limiter: limiter,
		context: context.TODO(),
	}
	// r.limiter.AllowN(time.Now(), 32*1024)
	return r
}

func (r *RateReader) Read(p []byte) (int, error) {
	n, err := r.inner.Read(p)
	if err != nil {
		return n, err
	}
	if err := r.limiter.WaitN(r.context, n); err != nil {
		return n, err
	}
	return n, nil
}

// 不限制 Write
func (r *RateReader) Write(p []byte) (int, error) {
	return r.inner.Write(p)
}
