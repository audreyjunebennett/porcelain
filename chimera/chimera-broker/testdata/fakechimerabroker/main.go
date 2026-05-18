// Package main is a minimal chimera-broker-http stand-in for chimera-broker e2e tests.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

func envBool(k string) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(k)))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func main() {
	var appDir, host, logLevel, logStyle string
	var port int
	flag.StringVar(&appDir, "app-dir", "", "")
	flag.StringVar(&host, "host", "127.0.0.1", "")
	flag.IntVar(&port, "port", 8080, "")
	flag.StringVar(&logLevel, "log-level", "info", "")
	flag.StringVar(&logStyle, "log-style", "json", "")
	flag.Parse()
	_ = logLevel
	_ = logStyle
	_ = os.MkdirAll(appDir, 0o755)
	_ = os.WriteFile(appDir+"/fake-chimera-broker.started", []byte(time.Now().UTC().String()), 0o644)

	var ready uint32
	if envBool("FAKE_CHIMERA_BROKER_START_READY") {
		atomic.StoreUint32(&ready, 1)
	}
	if s := strings.TrimSpace(os.Getenv("FAKE_CHIMERA_BROKER_STDOUT_SECRET")); s != "" {
		fmt.Println(s)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/models", func(w http.ResponseWriter, r *http.Request) {
		if atomic.LoadUint32(&ready) == 1 {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"ok":false}`))
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte("# HELP req_total requests\n# TYPE req_total counter\nreq_total{code=\"200\"} 1\nchimera_wrapper_up 42\n"))
	})
	mux.HandleFunc("/admin/ready", func(w http.ResponseWriter, r *http.Request) {
		val := r.URL.Query().Get("value")
		b := val == "1" || strings.EqualFold(val, "true")
		if b {
			atomic.StoreUint32(&ready, 1)
		} else {
			atomic.StoreUint32(&ready, 0)
		}
		_, _ = w.Write([]byte(strconv.FormatBool(atomic.LoadUint32(&ready) == 1)))
	})
	mux.HandleFunc("/admin/crash", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		go func() {
			time.Sleep(50 * time.Millisecond)
			os.Exit(9)
		}()
	})

	srv := &http.Server{
		Addr:    net.JoinHostPort(host, strconv.Itoa(port)),
		Handler: mux,
	}

	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		for range sigCh {
			if envBool("FAKE_CHIMERA_BROKER_IGNORE_TERMINATE") {
				continue
			}
			_ = srv.Shutdown(context.Background())
			return
		}
	}()
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
