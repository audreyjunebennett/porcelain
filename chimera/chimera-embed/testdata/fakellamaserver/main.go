// Package main is a minimal llama-server stand-in for chimera-embed e2e tests.
package main

import (
	"context"
	"encoding/json"
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
	host := "127.0.0.1"
	port := 8090
	for i := 1; i < len(os.Args)-1; i++ {
		switch os.Args[i] {
		case "--host":
			host = strings.TrimSpace(os.Args[i+1])
		case "--port":
			if p, err := strconv.Atoi(strings.TrimSpace(os.Args[i+1])); err == nil {
				port = p
			}
		}
	}
	if v := strings.TrimSpace(os.Getenv("LLAMA_HOST")); v != "" {
		host = v
	}
	if v := strings.TrimSpace(os.Getenv("LLAMA_PORT")); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			port = p
		}
	}
	dim := 768
	if v := strings.TrimSpace(os.Getenv("FAKE_EMBED_DIM")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			dim = n
		}
	}

	var ready uint32
	if envBool("FAKE_LLAMA_START_READY") {
		atomic.StoreUint32(&ready, 1)
	}
	if s := strings.TrimSpace(os.Getenv("FAKE_LLAMA_STDOUT_SECRET")); s != "" {
		fmt.Println(s)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if atomic.LoadUint32(&ready) == 1 {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	mux.HandleFunc("/v1/embeddings", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if atomic.LoadUint32(&ready) != 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		var req struct {
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if len(req.Input) == 0 {
			req.Input = []string{""}
		}
		vec := make([]float32, dim)
		for i := range vec {
			vec[i] = 0.01 * float32(i+1)
		}
		data := make([]map[string]any, len(req.Input))
		for i := range req.Input {
			data[i] = map[string]any{"index": i, "embedding": vec}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": data})
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte("# HELP req_total requests\nreq_total{code=\"200\"} 1\n"))
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
			if envBool("FAKE_LLAMA_IGNORE_TERMINATE") {
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
