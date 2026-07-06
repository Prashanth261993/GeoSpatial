// feeds: adapters that pull real-world moving entities from public APIs and
// normalize them into our internal Position events (anti-corruption layer),
// producing to the same positions topic so the whole pipeline handles real data
// unchanged. Pollers are OFF by default and controlled on-demand via /feeds so
// we only consume external rate-limit budget while someone is watching.
package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/Prashanth261993/geospatial/internal/bus"
	"github.com/twmb/franz-go/pkg/kgo"
)

type feed struct {
	name    string
	enabled bool
	cancel  context.CancelFunc
	// live stats
	lastCount int
	lastPoll  time.Time
	lastErr   string
}

type manager struct {
	mu    sync.Mutex
	prod  *kgo.Client
	feeds map[string]*feed
}

func main() {
	prod, err := bus.NewProducer(bus.Brokers(env("REDPANDA_BROKERS", "localhost:19092")))
	if err != nil {
		log.Fatalf("kafka: %v", err)
	}
	defer prod.Close()

	m := &manager{prod: prod, feeds: map[string]*feed{
		"opensky": {name: "opensky"},
	}}

	// Optional: start enabled feeds from env (default off).
	if env("OPENSKY_ENABLED", "false") == "true" {
		m.enable("opensky")
	}

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	http.HandleFunc("/feeds", m.handleFeeds)

	addr := ":" + env("PORT", "8130")
	log.Printf("feeds on %s (pollers off by default)", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

// handleFeeds: GET returns status; POST {feed, enabled} starts/stops a poller.
func (m *manager) handleFeeds(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "content-type")
	if r.Method == http.MethodOptions {
		return
	}
	if r.Method == http.MethodPost {
		var req struct {
			Feed    string `json:"feed"`
			Enabled bool   `json:"enabled"`
		}
		if json.NewDecoder(r.Body).Decode(&req) != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		if req.Enabled {
			m.enable(req.Feed)
		} else {
			m.disable(req.Feed)
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]any{}
	for name, f := range m.feeds {
		out[name] = map[string]any{
			"enabled": f.enabled, "lastCount": f.lastCount,
			"lastPoll": f.lastPoll.UnixMilli(), "lastErr": f.lastErr,
		}
	}
	json.NewEncoder(w).Encode(out)
}

func (m *manager) enable(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f := m.feeds[name]
	if f == nil || f.enabled {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	f.enabled = true
	f.cancel = cancel
	switch name {
	case "opensky":
		go pollOpenSky(ctx, m, f)
	}
	log.Printf("feed %s enabled", name)
}

func (m *manager) disable(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f := m.feeds[name]
	if f == nil || !f.enabled {
		return
	}
	f.enabled = false
	if f.cancel != nil {
		f.cancel()
	}
	log.Printf("feed %s disabled", name)
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
