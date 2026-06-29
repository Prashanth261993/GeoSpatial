// ingest-gateway: receives driver location POSTs, writes Redis GEO, publishes pub/sub.
package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/Prashanth261993/geospatial/internal/event"
	"github.com/redis/go-redis/v9"
)

const (
	geoKey  = "drivers"
	channel = "positions"
)

func main() {
	rdb := redis.NewClient(&redis.Options{Addr: env("REDIS_ADDR", "localhost:6379")})
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("redis: %v", err)
	}

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
		c := context.Background()
		if err := rdb.GeoAdd(c, geoKey, &redis.GeoLocation{Name: p.ID, Latitude: p.Lat, Longitude: p.Lng}).Err(); err != nil {
			http.Error(w, "geoadd", http.StatusInternalServerError)
			return
		}
		b, _ := json.Marshal(p)
		rdb.Publish(c, channel, b)
		w.WriteHeader(http.StatusAccepted)
	})

	addr := ":" + env("PORT", "8080")
	log.Printf("ingest-gateway on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
