package main

import (
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	// Bind to loopback by default. Authentication now lives in this backend
	// (OTP + break-glass password → session cookie), but binding to 127.0.0.1
	// keeps the attack surface local unless you deliberately expose it. Set
	// NAP_PASSWORD to enable break-glass login when NapCat/masters are offline.
	addr := envOr("NAP_ADDR", "127.0.0.1:8787")
	cfgPath := envOr("NAP_CONFIG", "config.json")

	store, err := NewStore(cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	log.Printf("[nap] config loaded from %s", cfgPath)

	hub := NewHub()
	ob := NewOneBot()
	pipe := NewPipeline(store, ob, hub)

	// Wire OneBot events into the pipeline.
	ob.onEvent = pipe.Ingest
	// Track connection transitions for the live status.
	go watchConnection(ob, hub)

	cfg := store.Get()
	ob.Reconfigure(cfg.OneBot)

	api := NewAPI(store, ob, hub, pipe)
	mux := api.Routes()

	srv := &http.Server{
		Addr:         addr,
		Handler:      logMiddleware(mux),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // SSE needs an unbounded write deadline
		IdleTimeout:  120 * time.Second,
	}
	log.Printf("[nap] backend listening on %s", addr)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server: %v", err)
	}
}

// watchConnection logs and broadcasts OneBot connect/disconnect transitions.
func watchConnection(ob *OneBot, hub *Hub) {
	last := false
	greeted := false
	for {
		time.Sleep(2 * time.Second)
		now := ob.Connected()
		if now != last {
			last = now
			if now {
				hub.Log("info", 0, "", "已连接到 NapCat OneBot")
				if !greeted {
					if li, err := ob.GetLoginInfo(); err == nil {
						hub.Log("info", 0, "", "当前账号："+li.Nickname)
						greeted = true
					}
				}
			} else {
				hub.Log("error", 0, "", "与 NapCat 的连接断开，正在重连…")
			}
			hub.Broadcast("status", map[string]any{"onebotConnected": now, "selfId": ob.SelfID()})
		}
	}
}

// logRW wraps http.ResponseWriter to capture the status code for the access
// log while preserving the http.Flusher behaviour that SSE (/api/events)
// depends on. A wrapper that doesn't forward Flush would silently break the
// live event stream.
type logRW struct {
	http.ResponseWriter
	status int
}

func (l *logRW) WriteHeader(code int) {
	l.status = code
	l.ResponseWriter.WriteHeader(code)
}

func (l *logRW) Flush() {
	if f, ok := l.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// logMiddleware writes a one-line access log per request. Status defaults to 200
// for handlers that write a body without an explicit WriteHeader.
func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lw := &logRW{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(lw, r)
		log.Printf("[nap] %s %s -> %d (%s)", r.Method, r.URL.Path, lw.status, time.Since(start).Round(time.Millisecond))
	})
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
