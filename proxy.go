package gproxy

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"sync"
	"time"
)

var errCert = errors.New("invail cert file or hosts")

// ProxyHandler is an HTTP Proxy Handler
type ProxyHandler struct {
	Transport http.RoundTripper
	// 目前先一个证书多个域名
	TLSConfig  *tls.Config
	BufferPool *BufferPool
	// 用来处理http
	Handler http.Handler
	hosts   []string
	mu      sync.Mutex
}

// NewProxyHandler returns a new ProxyHandler
func NewProxyHandler() *ProxyHandler {
	var tp = defaultTransport()
	// ReverseProxy 已经足够用来代理普通http
	rp := &httputil.ReverseProxy{
		Transport:  tp,
		BufferPool: defaultBufferPool,
		ErrorLog:   logger,
		Director:   director,
	}
	return &ProxyHandler{
		Transport:  tp,
		Handler:    rp,
		BufferPool: defaultBufferPool,
	}
}

// 先固定是10s
var dialer = &net.Dialer{
	Timeout:   10 * time.Second,
	KeepAlive: 30 * time.Second,
	DualStack: true,
}

// http.DefaultTransport
func defaultTransport() http.RoundTripper {
	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		// @TODO DialTLS 并行Handshake加快首次连接速度 (但需要配置h2)
	}
}

func director(req *http.Request) {}

// SetCert update certificate and hosts for tls handshake
func (ph *ProxyHandler) SetCert(hosts []string, certFile, keyFile string) (err error) {
	if certFile == "" || keyFile == "" || len(hosts) == 0 {
		return errCert
	}
	certificates := make([]tls.Certificate, 1)
	certificates[0], err = tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return
	}
	ph.mu.Lock()
	ph.TLSConfig = &tls.Config{
		MinVersion:               tls.VersionTLS12,
		Certificates:             certificates,
		PreferServerCipherSuites: true,
		ClientSessionCache:       tls.NewLRUClientSessionCache(16),
		SessionTicketsDisabled:   false,
		Renegotiation:            tls.RenegotiateNever,
		// @TODO 两边协商一致
		NextProtos: []string{"http/1.1", "h2"},
	}
	ph.hosts = hosts
	ph.mu.Unlock()
	return
}

func (ph *ProxyHandler) contains(host string) bool {
	for _, n := range ph.hosts {
		if host == n {
			return true
		}
	}
	return false
}

func (ph *ProxyHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if req.Method == "CONNECT" {
		ph.connect(rw, req)
		return
	}
	logger.Printf("%s %s", req.Method, req.URL)
	ph.Handler.ServeHTTP(rw, req)
}

// https connect
func (ph *ProxyHandler) connect(rw http.ResponseWriter, req *http.Request) {
	host, port, err := net.SplitHostPort(req.URL.Host)
	if err != nil {
		rw.WriteHeader(502)
		fmt.Fprintln(rw, "502 Bad Gateway")
		return
	}
	hj, ok := rw.(http.Hijacker)
	if !ok {
		logger.Println("connect hijacking not support")
		rw.WriteHeader(502)
		fmt.Fprintln(rw, "502 Bad Gateway")
		return
	}
	conn, _, err := hj.Hijack()
	if err != nil {
		httpError(conn, err)
		return
	}

	addr := host + ":" + port
	conn.Write([]byte("HTTP/1.0 200 OK\r\n\r\n"))
	if ph.contains(host) {
		ph.tls(addr, conn)
	} else {
		ph.tunnel(addr, conn)
	}
}

// 直接转发的 tls 连接
func (ph *ProxyHandler) tunnel(addr string, conn net.Conn) {
	logger.Printf("connect tunnel %s \n", addr)
	backend, err := dialer.Dial("tcp", addr)
	if err != nil {
		httpError(conn, err)
		return
	}
	buf := ph.BufferPool.Get()
	defer func() {
		conn.Close()
		backend.Close()
		ph.BufferPool.Put(buf)
	}()

	errc := make(chan error, 1)
	spc := copier{
		user:    conn,
		backend: backend,
		buf:     buf,
	}
	go spc.copyToBackend(errc)
	go spc.copyFromBackend(errc)
	err = <-errc
	if err != nil {
		httpError(conn, err)
	}
}

// 参与握手的 tls 连接
func (ph *ProxyHandler) tls(addr string, conn net.Conn) {
	httpError(conn, errors.New("not support yet"))
}

func httpError(w io.WriteCloser, err error) {
	logger.Println("http error:", err)
	io.WriteString(w, "HTTP/1.1 502 Bad Gateway\r\n\r\n")
	w.Close()
}

type copier struct {
	user, backend io.ReadWriter
	buf           []byte
}

func (c copier) copyFromBackend(errc chan<- error) {
	_, err := io.CopyBuffer(c.user, c.backend, c.buf)
	errc <- err
}

func (c copier) copyToBackend(errc chan<- error) {
	_, err := io.CopyBuffer(c.backend, c.user, c.buf)
	errc <- err
}
