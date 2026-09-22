package main

import (
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	// Bind to loopback by default: the Node frontend (with its password gate) is the
	// only intended public surface. Override with NAP_ADDR only behind another gate.
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

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
