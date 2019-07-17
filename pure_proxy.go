package gproxy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

const maxHeaderBytes = 1 << 20

var (
	errNeedMore     = errors.New("need more data: cannot find trailing lf")
	errLargeHeaders = errors.New("Request Header Fields Too Large")
)

var (
	headerHost    = []byte("Host")
	methodConnect = []byte("CONNECT")
	pathRoot      = []byte("/")
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

// Close immediately closes all connections
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
	// p.mu.Lock()
	// defer p.mu.Unlock()
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
	server                   *PureProxy
	rwc                      net.Conn
	host, method, requestURI []byte
	isTLS                    bool
}

func (c *conn) serve(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	buf := defaultBufferPool.Get()
	defer func() {
		c.close()
		c.server.trackConn(c, false)
		defaultBufferPool.Put(buf)
		cancel()
	}()
	cache, err := c.handleHost(ctx)
	if err != nil {
		httpError(c.rwc, err)
		return
	}
	logger.Printf("%s %s %s", c.method, c.host, c.requestURI)
	backend, err := dialer.Dial("tcp", string(c.host))
	if err != nil {
		httpError(c.rwc, err)
		return
	}
	if c.isTLS {
		readToend(c.rwc)
		c.rwc.Write([]byte("HTTP/1.0 200 OK\r\n\r\n"))
	} else {
		backend.Write(cache)
	}
	cache = nil
	err = tunnel(c.rwc, backend, buf)
	if err != nil {
		httpError(c.rwc, err)
	}
}

func (c *conn) close() {
	c.rwc.Close()
}

// @TODO 好像意义不大了, 先暂时这样吧
func readToend(c net.Conn) error {
	br := newBufioReader(c)
	defer putBufioReader(br)
	for {
		line, err := br.ReadSlice('\n')
		if err != nil {
			return err
		}
		fmt.Printf("line:%s %d\n", line, len(line))
		if len(line) == 2 {
			return nil
		}
	}
}

// Host: example.com / Method
// 读取 Header 的 Host 和 Method 字段
func (c *conn) handleHost(ctx context.Context) ([]byte, error) {
	if d := c.server.ReadTimeout; d != 0 {
		deadline := time.Now().Add(d)
		c.rwc.SetReadDeadline(deadline)
	}
	// http1.1
	return c.readHostmetaH1()
}

// 需要读取 Method 和 Host, 并且返回已经读取到的 bytes
func (c *conn) readHostmetaH1() ([]byte, error) {
	// 一般情况下 Host 就是第二个header
	arean := make([]byte, 20, 60)
	// 已经读取到的bytes
	cache := arean[20:]
	buf := arean[0:20]
	var next, total int
	var line []byte
	for {
		if c.host != nil {
			return cache[:total], nil
		}
		// header 太大却啥也读不到,直接响应错误
		if total > maxHeaderBytes {
			return nil, errLargeHeaders
		}
		n, err := c.rwc.Read(buf)
		if n == 0 {
			return cache[:total], err
		}
		cache = append(cache, buf[0:n]...)
		total += n
		if line, next = nextLine(cache[next:]); next == 0 {
			continue
		}
		// 读取请求第一行信息,GET / HTTP/1.1
		if c.method == nil {
			method, requestURI, err := readFirstLine(line)
			if err != nil {
				if err == errNeedMore {
					continue
				}
			}
			c.method = method
			c.isTLS = bytes.Equal(method, methodConnect)
			if c.isTLS {
				c.host = requestURI
				c.requestURI = pathRoot
			} else {
				c.requestURI = requestURI
			}
		} else {
			// @TODO 读完headers的处理
			if host := tryReadHost(line); host != nil {
				c.host = host
			}
			return cache[:total], nil
		}
	}
}

func readFirstLine(b []byte) (method, requestURI []byte, err error) {
	n := bytes.IndexByte(b, ' ')
	if n < 0 {
		err = fmt.Errorf("cannot find whitespace in the first line of request")
		return
	}
	// parse method
	n = bytes.IndexByte(b, ' ')
	if n <= 0 {
		err = fmt.Errorf("cannot find http request method")
		return
	}
	method = b[:n]
	b = b[n+1:]
	// parse requestURI
	n = bytes.LastIndexByte(b, ' ')
	requestURI = b[:n]
	return
}

func tryReadHost(buf []byte) []byte {
	n := bytes.Index(buf, headerHost)
	if n < 0 {
		return nil
	}
	return skipDelimiter(buf, n+len(headerHost))
}

func nextLine(b []byte) ([]byte, int) {
	nNext := bytes.IndexByte(b, '\n')
	if nNext < 0 {
		return nil, 0
	}
	n := nNext
	if n > 0 && b[n-1] == '\r' {
		n--
	}
	return b[:n], nNext + 1
}

func skipDelimiter(buf []byte, n int) []byte {
	if n >= len(buf) {
		return nil
	}
	if buf[n] != ':' {
		return nil
	}
	n++
	if buf[n] != ' ' {
		return nil
	}
	n++
	return buf[n:]
}

func caseInsensitiveCompare(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if a[i]|0x20 != b[i]|0x20 {
			return false
		}
	}
	return true
}
