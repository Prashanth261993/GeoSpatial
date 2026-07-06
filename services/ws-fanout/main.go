// ws-fanout: consumes the positions and trips topics from Redpanda and
// broadcasts every record to all connected WebSocket clients. Each instance
// uses a UNIQUE consumer group so it receives ALL partitions (fan-out, not
// work-sharing) and starts at the latest offset (live tail).
package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sync"

	"github.com/Prashanth261993/geospatial/internal/bus"
	"github.com/coder/websocket"
	"github.com/twmb/franz-go/pkg/kgo"
)

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
		default: // drop on slow client
		}
	}
	h.mu.RUnlock()
}

func main() {
	h := newHub()
	brokers := bus.Brokers(env("REDPANDA_BROKERS", "localhost:19092"))
	group := fmt.Sprintf("fanout-%d", rand.Int63())
	cl, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(bus.TopicPositions, bus.TopicTrips),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtEnd()), // live tail
	)
	if err != nil {
		log.Fatalf("kafka: %v", err)
	}
	defer cl.Close()

	go func() {
		ctx := context.Background()
		for {
			fetches := cl.PollFetches(ctx)
			fetches.EachRecord(func(rec *kgo.Record) { h.broadcast(rec.Value) })
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
	log.Printf("ws-fanout on %s (group=%s, topics=%s,%s)", addr, group, bus.TopicPositions, bus.TopicTrips)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
