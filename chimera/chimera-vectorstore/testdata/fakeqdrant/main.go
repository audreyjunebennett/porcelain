// Package main is a minimal Qdrant stand-in for chimera-vectorstore e2e tests.
package main

import (
	"context"
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
	host := strings.TrimSpace(os.Getenv("QDRANT__SERVICE__HOST"))
	if host == "" {
		host = "127.0.0.1"
	}
	port := 6333
	if v := strings.TrimSpace(os.Getenv("QDRANT__SERVICE__HTTP_PORT")); v != "" {
		p, err := strconv.Atoi(v)
		if err == nil {
			port = p
		}
	}
	storage := strings.TrimSpace(os.Getenv("QDRANT__STORAGE__STORAGE_PATH"))
	if storage == "" {
		storage = "."
	}
	_ = os.MkdirAll(storage, 0o755)
	_ = os.WriteFile(storage+"/fake-qdrant.started", []byte(time.Now().UTC().String()), 0o644)

	var ready uint32
	if envBool("FAKE_QDRANT_START_READY") {
		atomic.StoreUint32(&ready, 1)
	}
	if s := strings.TrimSpace(os.Getenv("FAKE_QDRANT_STDOUT_SECRET")); s != "" {
		fmt.Println(s)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/collections", func(w http.ResponseWriter, r *http.Request) {
		if atomic.LoadUint32(&ready) == 1 {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"result":{"collections":[]}}`))
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"degraded"}`))
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
			if envBool("FAKE_QDRANT_IGNORE_TERMINATE") {
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
