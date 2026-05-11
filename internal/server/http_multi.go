package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
)

// StartHTTPListeners binds handler on each TCP address and serves concurrently.
// IPv6 loopback ([::1]) failures are skipped when another listener succeeds.
// Returns the first successful listen address, shutdown for all servers, a channel
// closed after all Serve goroutines exit (call shutdown to unblock), and an error
// if no listener started.
func StartHTTPListeners(handler http.Handler, addrs []string, log *slog.Logger) (net.Addr, func(context.Context) error, <-chan struct{}, error) {
	var servers []*http.Server
	var listeners []net.Listener
	var primary net.Addr
	for _, addr := range addrs {
		ln, lerr := net.Listen("tcp", addr)
		if lerr != nil {
			if IsIPv6LoopbackAddr(addr) {
				if log != nil {
					log.Warn("listen skipped", "msg", "gateway.listen.skipped", "addr", addr, "err", lerr)
				}
				continue
			}
			for _, l := range listeners {
				_ = l.Close()
			}
			return nil, nil, nil, lerr
		}
		listeners = append(listeners, ln)
		if primary == nil {
			primary = ln.Addr()
		}
		srv := &http.Server{Handler: handler}
		servers = append(servers, srv)
	}
	if len(servers) == 0 {
		return nil, nil, nil, fmt.Errorf("no tcp listeners started")
	}

	var wg sync.WaitGroup
	wg.Add(len(servers))
	for i, srv := range servers {
		ln := listeners[i]
		go func(s *http.Server, l net.Listener) {
			defer wg.Done()
			if err := s.Serve(l); err != nil && err != http.ErrServerClosed && log != nil {
				log.Debug("http serve exit", "msg", "gateway.http.server_error", "err", err)
			}
		}(srv, ln)
	}
	stopped := make(chan struct{})
	go func() {
		wg.Wait()
		close(stopped)
	}()

	shutdown := func(ctx context.Context) error {
		var last error
		for _, s := range servers {
			if e := s.Shutdown(ctx); e != nil {
				last = e
			}
		}
		return last
	}
	return primary, shutdown, stopped, nil
}
