package graceful

import (
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	rpprof "runtime/pprof"
	"strings"
	"sync"
	"syscall"
	"time"
)

type SocketOpt struct {
	NoDelay           bool
	Keepalive         bool
	KeepaliveInterval time.Duration
	ReadBufLen        int
	WriteBufLen       int
	ReadTimeout       time.Duration
	ReadHeaderTimeout time.Duration
	WriteTimeout      time.Duration
	MaxHeaderBytes    int
}

type Listener struct {
	*net.TCPListener
	opt *SocketOpt
}

func (ln Listener) Accept() (net.Conn, error) {
	tc, err := ln.AcceptTCP()
	if err != nil {
		return tc, err
	}

	if ln.opt == nil {
		//same as net.http.Server.ListenAndServe
		tc.SetKeepAlive(true)
		tc.SetKeepAlivePeriod(3 * time.Minute)
	} else {
		tc.SetNoDelay(ln.opt.NoDelay)
		tc.SetKeepAlive(ln.opt.Keepalive)
		if ln.opt.Keepalive && ln.opt.KeepaliveInterval > 0 {
			tc.SetKeepAlivePeriod(ln.opt.KeepaliveInterval)
		}
		if ln.opt.ReadBufLen > 0 {
			tc.SetReadBuffer(ln.opt.ReadBufLen)
		}
		if ln.opt.WriteBufLen > 0 {
			tc.SetWriteBuffer(ln.opt.WriteBufLen)
		}
	}

	return tc, nil
}

func NewListener(addr string, opt *SocketOpt) (net.Listener, error) {
	if addr == "" {
		addr = ":http"
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		listener, err = net.Listen("tcp6", addr)
	}

	if err == nil {
		// if opt != nil {
		// 	if opt.Keepalive && opt.KeepaliveInterval < time.Minute {
		// 		opt.KeepaliveInterval = minKeepaliveInterval
		// 	}
		// 	if opt.ReadBufLen <= 0 {
		// 		opt.ReadBufLen = defautRecvBufLen
		// 	}
		// 	if opt.WriteBufLen <= 0 {
		// 		opt.WriteBufLen = defautSendBufLen
		// 	}
		// }
		return Listener{listener.(*net.TCPListener), opt}, err
	}
	return nil, err
}

type HttpHandlerWrapper struct {
	sync.WaitGroup
	handler      http.Handler
	over         bool
	pprofEnabled bool
	pprofRoutes  map[string]func(w http.ResponseWriter, r *http.Request)
}

func (wrapper *HttpHandlerWrapper) EnablePProf(root string) {
	if !strings.HasSuffix(root, "/") {
		root += "/"
	}
	if wrapper.pprofRoutes == nil {
		wrapper.pprofRoutes = map[string]func(w http.ResponseWriter, r *http.Request){}
	}

	wrapper.pprofEnabled = true

	wrapper.pprofRoutes[root+"cmdline"] = pprof.Cmdline
	wrapper.pprofRoutes[root+"profile"] = pprof.Profile
	wrapper.pprofRoutes[root+"symbol"] = pprof.Symbol
	wrapper.pprofRoutes[root+"trace"] = pprof.Trace
	wrapper.pprofRoutes[root+"index"] = pprof.Index
	for _, v := range rpprof.Profiles() {
		wrapper.pprofRoutes[root+v.Name()] = pprof.Handler(v.Name()).ServeHTTP
	}

	for k, _ := range wrapper.pprofRoutes {
		logInfo("--- pprof path: %v", k)
	}
}

func (wrapper *HttpHandlerWrapper) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	wrapper.Add(1)
	defer wrapper.Done()
	defer handlePanic()

	if !wrapper.over {
		if wrapper.pprofEnabled {
			if h, ok := wrapper.pprofRoutes[r.URL.Path]; ok {
				h(w, r)
				return
			}
		}
		wrapper.handler.ServeHTTP(w, r)
	} else {
		http.Error(w, http.StatusText(404), 404)
	}
}

type HttpServer struct {
	addr      string
	timeout   time.Duration
	listener  net.Listener
	server    *http.Server
	onTimeout func()
}

func (svr *HttpServer) EnablePProf(root string) {
	wraper, _ := svr.server.Handler.(*HttpHandlerWrapper)
	wraper.EnablePProf(root)
}

func (svr *HttpServer) Serve() {
	logDebug("http server running on: %v", svr.addr)
	err := svr.server.Serve(svr.listener)
	logDebug("http server(\"%s\") exit: %v", svr.addr, err)
}

func (svr *HttpServer) ServeTLS(certFile, keyFile string) {
	logDebug("http server running on: %v", svr.addr)
	err := svr.server.ServeTLS(svr.listener, certFile, keyFile)
	logDebug("http server(\"%s\") exit: %v", svr.addr, err)
}

func (svr *HttpServer) Shutdown() error {
	err := svr.listener.Close()
	//time.Sleep(time.Second / 10)
	logDebug("http server(\"%s\") shutdown waitting...", svr.addr)
	wrapper := svr.server.Handler.(*HttpHandlerWrapper)
	wrapper.over = true
	wrapper.Done()
	timer := time.AfterFunc(svr.timeout, func() {
		logError("http server(\"%s\") shutdown timeout(%v)", svr.addr, svr.timeout)
		if svr.onTimeout != nil {
			svr.onTimeout()
		}
	})
	defer timer.Stop()
	wrapper.Wait()
	logDebug("http server(\"%s\") shutdown done.", svr.addr)
	//time.Sleep(time.Second / 10)
	return err
}

func (svr *HttpServer) SetSocketOpt(opt *SocketOpt) {
	if opt != nil {
		readTimeout := time.Second * 60
		readHeaderTimeout := time.Second * 60
		writeTimeout := time.Second * 10
		maxHeaderBytes := 1 << 28
		if opt.ReadTimeout > 0 {
			readTimeout = opt.ReadTimeout
		}
		if opt.ReadHeaderTimeout > 0 {
			readHeaderTimeout = opt.ReadHeaderTimeout
		}
		if opt.WriteTimeout > 0 {
			writeTimeout = opt.WriteTimeout
		}
		maxHeaderBytes = opt.MaxHeaderBytes

		l, ok := svr.listener.(*Listener)
		if ok {

			l.opt = opt
		}

		svr.server.ReadTimeout = readTimeout
		svr.server.ReadHeaderTimeout = readHeaderTimeout
		svr.server.WriteTimeout = writeTimeout
		svr.server.MaxHeaderBytes = maxHeaderBytes
	}
}

func NewHttpServer(addr string, handler http.Handler, to time.Duration, opt *SocketOpt, onTimeout func()) (*HttpServer, error) {
	listener, err := NewListener(addr, opt)
	if err != nil {
		logError("NewHttpServer failed: %v", err)
		return nil, err
	}

	wrapper := &HttpHandlerWrapper{
		handler: handler,
	}
	wrapper.Add(1)

	readTimeout := time.Second * 120
	readHeaderTimeout := time.Second * 60
	writeTimeout := time.Second * 120 //pprof default min timeout 30
	maxHeaderBytes := 1 << 28
	if opt != nil {
		if opt.ReadTimeout > 0 {
			readTimeout = opt.ReadTimeout
		}
		if opt.ReadHeaderTimeout > 0 {
			readHeaderTimeout = opt.ReadHeaderTimeout
		}
		if opt.WriteTimeout > 0 {
			writeTimeout = opt.WriteTimeout
		}
		maxHeaderBytes = opt.MaxHeaderBytes
	}

	svr := &HttpServer{
		addr:     addr,
		timeout:  to,
		listener: listener,
		server: &http.Server{
			Handler:           wrapper,
			ReadTimeout:       readTimeout,
			ReadHeaderTimeout: readHeaderTimeout,
			WriteTimeout:      writeTimeout,
			MaxHeaderBytes:    maxHeaderBytes,
		},
		onTimeout: onTimeout,
	}

	return svr, nil
}

func Serve(addr string, handler http.Handler, timeout time.Duration, opt *SocketOpt) {
	svr, err := NewHttpServer(addr, handler, timeout, opt, func() {
		os.Exit(0)
	})
	if err != nil {
		logFatal("graceful: Serve failed: %v", err)
	} else {
		safeGo(svr.Serve)
	}

	handleSignal(func(sig os.Signal) {
		if sig == syscall.SIGTERM || sig == syscall.SIGINT {
			svr.Shutdown()
			os.Exit(0)
		}
	})
}

func ServeTLS(addr string, handler http.Handler, timeout time.Duration, opt *SocketOpt, certFile string, keyFile string) {
	svr, err := NewHttpServer(addr, handler, timeout, opt, func() {
		os.Exit(0)
	})
	if err != nil {
		logFatal("graceful: ServeTLS failed: %v", err)
	} else {
		safeGo(func() {
			svr.ServeTLS(certFile, keyFile)
		})
	}

	handleSignal(func(sig os.Signal) {
		if sig == syscall.SIGTERM || sig == syscall.SIGINT {
			svr.Shutdown()
			os.Exit(0)
		}
	})
}
