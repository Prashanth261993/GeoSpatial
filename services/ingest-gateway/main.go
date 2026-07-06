// ingest-gateway: receives driver location POSTs and produces them to the
// Redpanda "positions" topic, keyed by a coarse H3 cell so an area's events
// land on the same partition (spatial locality). No Redis dependency: the
// indexer owns the spatial index downstream.
package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/Prashanth261993/geospatial/internal/bus"
	"github.com/Prashanth261993/geospatial/internal/event"
	"github.com/Prashanth261993/geospatial/internal/spatial"
)

func main() {
	brokers := bus.Brokers(env("REDPANDA_BROKERS", "localhost:19092"))
	prod, err := bus.NewProducer(brokers)
	if err != nil {
		log.Fatalf("kafka: %v", err)
	}
	defer prod.Close()

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })

	http.HandleFunc("/loc", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		var p event.Position
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		if p.ID == "" || p.Lat < -90 || p.Lat > 90 || p.Lng < -180 || p.Lng > 180 {
			http.Error(w, "invalid fields", http.StatusBadRequest)
			return
		}
		if p.Ts == 0 {
			p.Ts = time.Now().UnixMilli()
		}
		// Partition key = coarse H3 cell -> spatial locality across consumers.
		key, err := spatial.CellOf(p.Lat, p.Lng, bus.KeyRes)
		if err != nil {
			http.Error(w, "bad point", http.StatusBadRequest)
			return
		}
		b, _ := json.Marshal(p)
		bus.Produce(context.Background(), prod, bus.TopicPositions, key, b)
		w.WriteHeader(http.StatusAccepted)
	})

	addr := ":" + env("PORT", "8080")
	log.Printf("ingest-gateway on %s -> topic %s (key res=%d)", addr, bus.TopicPositions, bus.KeyRes)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
