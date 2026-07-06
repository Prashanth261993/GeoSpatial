// surge: computes a live demand/supply surge multiplier per H3 zone (res 7) over
// a sliding time window built from sub-buckets. Demand = rider requests in the
// window; supply = available drivers currently in the zone. Publishes surge to
// Redis (for the map) and persists snapshots to TimescaleDB (for history).
package main

import (
	"context"
	"encoding/json"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/Prashanth261993/geospatial/internal/bus"
	"github.com/Prashanth261993/geospatial/internal/spatial"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/twmb/franz-go/pkg/kgo"
)

const (
	slotMs     = 10_000 // each sub-bucket is 10s
	windowSize = 30     // window = 30 slots = 5 minutes
	surgeCap   = 3.0
	surgeK     = 0.6
	idxRes     = 9 // indexer's resolution; we roll res-9 cells up to res-7 zones
)

// zone holds a ring of demand counts per slot for one H3 res-7 zone.
type zone struct {
	demand [windowSize]int
}

var (
	mu     sync.Mutex
	zones  = map[string]*zone{}
	curSlot int64

	rdb  *redis.Client
	pool *pgxpool.Pool
)

func main() {
	rdb = redis.NewClient(&redis.Options{Addr: env("REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("redis: %v", err)
	}
	initTimescale()

	brokers := bus.Brokers(env("REDPANDA_BROKERS", "localhost:19092"))
	cl, err := bus.NewConsumer(brokers, "surge", bus.TopicRequests)
	if err != nil {
		log.Fatalf("kafka: %v", err)
	}
	defer cl.Close()

	curSlot = time.Now().UnixMilli() / slotMs
	go consume(cl)
	go computeLoop()

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	http.HandleFunc("/surge", handleSurge)

	addr := ":" + env("PORT", "8120")
	log.Printf("surge on %s (window=%ds, slot=%ds)", addr, windowSize*slotMs/1000, slotMs/1000)
	log.Fatal(http.ListenAndServe(addr, nil))
}

// consume tallies each rider request into the current slot of its zone.
func consume(cl *kgo.Client) {
	ctx := context.Background()
	for {
		fetches := cl.PollFetches(ctx)
		fetches.EachRecord(func(rec *kgo.Record) {
			var r struct {
				Lat float64 `json:"lat"`
				Lng float64 `json:"lng"`
			}
			if json.Unmarshal(rec.Value, &r) != nil {
				return
			}
			z, err := spatial.CellOf(r.Lat, r.Lng, bus.KeyRes)
			if err != nil {
				return
			}
			slot := int(time.Now().UnixMilli() / slotMs % windowSize)
			mu.Lock()
			zn := zones[z]
			if zn == nil {
				zn = &zone{}
				zones[z] = zn
			}
			zn.demand[slot]++
			mu.Unlock()
		})
		cl.CommitUncommittedOffsets(ctx)
	}
}

// computeLoop advances the window each slot: clears the slot about to be reused,
// computes surge per zone, writes to Redis, and persists a snapshot to Timescale.
func computeLoop() {
	tick := time.NewTicker(slotMs * time.Millisecond)
	for range tick.C {
		now := time.Now()
		nextSlot := int(now.UnixMilli()/slotMs%windowSize + 1)
		if nextSlot >= windowSize {
			nextSlot = 0
		}

		type snap struct {
			zone       string
			demand     int
			supply     int
			multiplier float64
		}
		var snaps []snap

		mu.Lock()
		for z, zn := range zones {
			zn.demand[nextSlot] = 0 // evict the slot we're about to overwrite next

			demand := 0
			for _, d := range zn.demand {
				demand += d
			}
			if demand == 0 {
				delete(zones, z)
				rdb.HDel(context.Background(), "surge", z)
				continue
			}
			supply := supplyInZone(z)
			m := multiplier(demand, supply)
			snaps = append(snaps, snap{z, demand, supply, m})
		}
		mu.Unlock()

		ctx := context.Background()
		for _, s := range snaps {
			rdb.HSet(ctx, "surge", s.zone, strconv.FormatFloat(s.multiplier, 'f', 2, 64))
			persist(ctx, now, s.zone, s.demand, s.supply, s.multiplier)
		}
	}
}

// supplyInZone counts available drivers whose res-9 index cells roll up to this
// res-7 zone. Uses the indexer's per-cell Sets in Redis.
func supplyInZone(zone string) int {
	ctx := context.Background()
	children, err := spatial.Children(zone, idxRes)
	if err != nil {
		return 0
	}
	total := 0
	pipe := rdb.Pipeline()
	cmds := make([]*redis.IntCmd, len(children))
	for i, c := range children {
		cmds[i] = pipe.SCard(ctx, "h3:"+c)
	}
	pipe.Exec(ctx)
	for _, cmd := range cmds {
		total += int(cmd.Val())
	}
	return total
}

// multiplier maps a demand/supply imbalance to a bounded surge factor.
func multiplier(demand, supply int) float64 {
	if supply <= 0 {
		supply = 1
	}
	ratio := float64(demand) / float64(supply)
	m := 1.0 + surgeK*(ratio-1)
	if m < 1 {
		m = 1
	}
	if m > surgeCap {
		m = surgeCap
	}
	return math.Round(m*100) / 100
}

func handleSurge(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	all, _ := rdb.HGetAll(context.Background(), "surge").Result()
	out := make([]map[string]any, 0, len(all))
	for cell, mult := range all {
		m, _ := strconv.ParseFloat(mult, 64)
		out = append(out, map[string]any{"cell": cell, "surge": m})
	}
	json.NewEncoder(w).Encode(map[string]any{"zones": out})
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
