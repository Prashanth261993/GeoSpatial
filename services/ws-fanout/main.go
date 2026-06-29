// ws-fanout: subscribes to Redis pub/sub and broadcasts to all WebSocket clients.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/coder/websocket"
	"github.com/redis/go-redis/v9"
)

const channel = "positions"

type hub struct {
	mu      sync.RWMutex
	clients map[chan []byte]struct{}
}

func newHub() *hub { return &hub{clients: map[chan []byte]struct{}{}} }

func (h *hub) add() chan []byte {
	ch := make(chan []byte, 256)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *hub) remove(ch chan []byte) {
	h.mu.Lock()
	delete(h.clients, ch)
	h.mu.Unlock()
	close(ch)
}

func (h *hub) broadcast(b []byte) {
	h.mu.RLock()
	for ch := range h.clients {
		select {
		case ch <- b:
		default: // drop on slow client (backpressure preview)
		}
	}
	h.mu.RUnlock()
}

func main() {
	rdb := redis.NewClient(&redis.Options{Addr: env("REDIS_ADDR", "localhost:6379")})
	h := newHub()

	go func() {
		sub := rdb.Subscribe(context.Background(), channel)
		for msg := range sub.Channel() {
			h.broadcast([]byte(msg.Payload))
		}
	}()

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		defer c.CloseNow()
		ch := h.add()
		defer h.remove(ch)
		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case b := <-ch:
				if err := c.Write(ctx, websocket.MessageText, b); err != nil {
					return
				}
			}
		}
	})

	addr := ":" + env("PORT", "8090")
	log.Printf("ws-fanout on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
