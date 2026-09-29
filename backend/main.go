package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
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
	noticeDir := envOr("NAP_NOTICE_DIR", filepath.Join(filepath.Dir(cfgPath), "notices"))
	notices, err := NewNoticeStore(noticeDir)
	if err != nil {
		log.Fatalf("open notice store: %v", err)
	}
	defer notices.Close()

	hub := NewHub()
	ob := NewOneBot()
	pipe := NewPipeline(store, ob, hub)
	pipe.SetNoticeStore(notices)
	defer pipe.Shutdown()
	defer ob.Shutdown()

	// Wire OneBot events into the pipeline.
	ob.onEvent = pipe.Ingest
	// Track connection transitions for the live status.
	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go watchConnection(rootCtx, ob, hub)

	cfg := store.Get()
	ob.Reconfigure(cfg.OneBot)

	api := NewAPI(store, ob, hub, pipe)
	api.SetNoticeStore(notices)
	mux := api.Routes()

	srv := &http.Server{
		Addr:         addr,
		Handler:      logMiddleware(mux),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // SSE needs an unbounded write deadline
		IdleTimeout:  120 * time.Second,
	}
	log.Printf("[nap] backend listening on %s", addr)
	serverErr := make(chan error, 1)
	go func() { serverErr <- srv.ListenAndServe() }()
	select {
	case err := <-serverErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("server: %v", err)
		}
	case <-rootCtx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("server shutdown: %v", err)
			_ = srv.Close()
		}
		cancel()
		pipe.Shutdown()
		ob.Shutdown()
		<-serverErr
	}
}

// watchConnection logs and broadcasts OneBot connect/disconnect transitions.
func watchConnection(ctx context.Context, ob *OneBot, hub *Hub) {
	last := false
	greeted := false
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := ob.Connected()
			if now != last {
				last = now
				if now {
					hub.Log("info", 0, "", "已连接到 NapCat OneBot")
					if !greeted {
						if li, err := ob.GetLoginInfoContext(ctx); err == nil {
							hub.Log("info", 0, "", "当前账号："+li.Nickname)
							greeted = true
						}
					}
				} else {
					greeted = false
					hub.Log("error", 0, "", "与 NapCat 的连接断开，正在重连…")
				}
				hub.Broadcast("status", map[string]any{"onebotConnected": now, "selfId": ob.SelfID()})
			}
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

func (l *logRW) Unwrap() http.ResponseWriter { return l.ResponseWriter }

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
