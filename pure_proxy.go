package gproxy

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// PureProxy is a tcp proxy handler http requests
// 最单纯的http代理,只转发几乎不处理任何数据
type PureProxy struct {
	inShutdown  int32
	mu          sync.Mutex
	activeConn  map[*conn]struct{}
	doneChan    chan struct{}
	listener    *net.Listener
	ReadTimeout time.Duration
}

type tcpKeepAliveListener struct {
	*net.TCPListener
}

func (ln tcpKeepAliveListener) Accept() (net.Conn, error) {
	tc, err := ln.AcceptTCP()
	if err != nil {
		return nil, err
	}
	tc.SetKeepAlive(true)
	tc.SetKeepAlivePeriod(3 * time.Minute)
	return tc, nil
}

type onceCloseListener struct {
	net.Listener
	once     sync.Once
	closeErr error
}

func (oc *onceCloseListener) Close() error {
	oc.once.Do(oc.close)
	return oc.closeErr
}

func (oc *onceCloseListener) close() { oc.closeErr = oc.Listener.Close() }

type badStringError struct {
	what string
	str  string
}

func (e *badStringError) Error() string { return fmt.Sprintf("%s %q", e.what, e.str) }

// ListenAndServe listens on the TCP network address srv.Addr and then
// handle requests on incoming connections.
func (p *PureProxy) ListenAndServe(addr string) error {
	if p.shuttingDown() {
		return http.ErrServerClosed
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	p.serve(tcpKeepAliveListener{ln.(*net.TCPListener)})
	return nil
}

// Close immediately closes all active net.Listeners
func (p *PureProxy) Close() (err error) {
	atomic.StoreInt32(&p.inShutdown, 1)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closeDoneChanLocked()
	if p.listener != nil {
		err = (*p.listener).Close()
		p.listener = nil
	}
	for c := range p.activeConn {
		c.rwc.Close()
		delete(p.activeConn, c)
	}
	return
}

func (p *PureProxy) closeDoneChanLocked() {
	ch := p.getDoneChanLocked()
	select {
	case <-ch:
	default:
		close(ch)
	}
}

func (p *PureProxy) getDoneChan() <-chan struct{} {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.getDoneChanLocked()
}

func (p *PureProxy) getDoneChanLocked() chan struct{} {
	if p.doneChan == nil {
		p.doneChan = make(chan struct{})
	}
	return p.doneChan
}

func (p *PureProxy) serve(l net.Listener) error {
	l = &onceCloseListener{Listener: l}
	defer l.Close()
	p.listener = &l
	var tempDelay time.Duration
	ctx := context.Background()
	for {
		rw, e := l.Accept()
		if e != nil {
			select {
			case <-p.getDoneChan():
				return http.ErrServerClosed
			default:
			}
			if ne, ok := e.(net.Error); ok && ne.Temporary() {
				if tempDelay == 0 {
					tempDelay = 5 * time.Millisecond
				} else {
					tempDelay *= 2
				}
				if max := 1 * time.Second; tempDelay > max {
					tempDelay = max
				}
				logger.Printf("Accept error: %v; retrying in %v", e, tempDelay)
				time.Sleep(tempDelay)
				continue
			}
			return e
		}
		tempDelay = 0
		c := &conn{
			server: p,
			rwc:    rw,
		}
		p.trackConn(c, true)
		go c.serve(ctx)
	}
}

// copy from net/http/server.go
func (p *PureProxy) trackConn(c *conn, add bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.activeConn == nil {
		p.activeConn = make(map[*conn]struct{})
	}
	if add {
		p.activeConn[c] = struct{}{}
	} else {
		delete(p.activeConn, c)
	}
}

func (p *PureProxy) shuttingDown() bool {
	return atomic.LoadInt32(&p.inShutdown) != 0
}

type conn struct {
	server     *PureProxy
	rwc        net.Conn
	remoteAddr string
}

func (c *conn) serve(ctx context.Context) {
	c.remoteAddr = c.rwc.RemoteAddr().String()
	ctx, cancel := context.WithCancel(ctx)
	c.handleHost(ctx)
	cancel()
}

func (c *conn) close() {
	c.rwc.Close()
}

func (c *conn) handleHost(ctx context.Context) (host string, err error) {
	if d := c.server.ReadTimeout; d != 0 {
		deadline := time.Now().Add(d)
		c.rwc.SetReadDeadline(deadline)
	}

	// @TODO, 处理 maxHeaderBytes 1 << 20 + 4096

	br := newBufioReader(c.rwc)
	defer func() {
		c.close()
		c.server.trackConn(c, false)
		putBufioReader(br)
	}()

	// http1.1
	method, err := br.ReadBytes(byte(' '))
	fmt.Printf("%s", method)
	return
}
