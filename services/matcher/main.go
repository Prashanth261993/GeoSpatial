// matcher: assigns riders to nearby drivers. Greedy (online, nearest-free) by
// default; optional batched-optimal (Hungarian) mode. Double-assignment is
// prevented with an atomic Redis lock (SET NX EX) that self-heals via TTL.
package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/Prashanth261993/geospatial/internal/bus"
	"github.com/Prashanth261993/geospatial/internal/match"
	"github.com/Prashanth261993/geospatial/internal/spatial"
	"github.com/redis/go-redis/v9"
	"github.com/twmb/franz-go/pkg/kgo"
)

var (
	rdb        *redis.Client
	prod       *kgo.Client
	nearbyURL  string
	radiusM    float64 = 2500
	lockTTL            = 30 * time.Second
	httpClient         = &http.Client{Timeout: 3 * time.Second}

	totalAssigned int64
	sumPickupM    int64 // store as int meters for atomic
	activeTrips   int64
	mode          string
)

type request struct {
	ReqID string  `json:"reqId"`
	Lat   float64 `json:"lat"`
	Lng   float64 `json:"lng"`
}

type nearbyDriver struct {
	ID   string  `json:"id"`
	Lat  float64 `json:"lat"`
	Lng  float64 `json:"lng"`
	Dist float64 `json:"dist"`
}

type tripEvent struct {
	Kind       string  `json:"kind"`
	Event      string  `json:"event"` // assigned | completed
	ReqID      string  `json:"reqId"`
	Driver     string  `json:"driver"`
	DriverLat  float64 `json:"driverLat"`
	DriverLng  float64 `json:"driverLng"`
	RiderLat   float64 `json:"riderLat"`
	RiderLng   float64 `json:"riderLng"`
	PickupDist float64 `json:"pickupDist"`
	DurMs      int64   `json:"durMs"`
	Ts         int64   `json:"ts"`
}

// batched-optimal plumbing
var batchCh = make(chan request, 4096)

func main() {
	mode = env("MATCH_MODE", "greedy")
	nearbyURL = env("NEARBY_URL", "http://localhost:8100/nearby")
	rdb = redis.NewClient(&redis.Options{Addr: env("REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("redis: %v", err)
	}
	var err error
	prod, err = bus.NewProducer(bus.Brokers(env("REDPANDA_BROKERS", "localhost:19092")))
	if err != nil {
		log.Fatalf("kafka: %v", err)
	}
	defer prod.Close()
	if mode == "optimal" {
		go batchLoop()
	}

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	http.HandleFunc("/request", handleRequest)
	http.HandleFunc("/stats", handleStats)

	addr := ":" + env("PORT", "8110")
	log.Printf("matcher on %s (mode=%s radius=%.0fm)", addr, mode, radiusM)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var req request
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.ReqID == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	ctx := context.Background()

	// Emit every rider request to the requests topic (captures ALL demand,
	// including requests that won't be matched — the strongest surge signal).
	if key, err := spatial.CellOf(req.Lat, req.Lng, bus.KeyRes); err == nil {
		body, _ := json.Marshal(map[string]any{"reqId": req.ReqID, "lat": req.Lat, "lng": req.Lng, "ts": time.Now().UnixMilli()})
		bus.Produce(ctx, prod, bus.TopicRequests, key, body)
	}

	// idempotency: a repeated reqId returns the existing assignment.
	if existing, _ := rdb.HGet(ctx, "req:assigned", req.ReqID).Result(); existing != "" {
		writeJSON(w, map[string]any{"reqId": req.ReqID, "driver": existing, "idempotent": true})
		return
	}

	if mode == "optimal" {
		batchCh <- req
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if d, dist, ok := greedyAssign(ctx, req); ok {
		writeJSON(w, map[string]any{"reqId": req.ReqID, "driver": d, "pickupDist": dist})
		return
	}
	http.Error(w, "no driver available", http.StatusServiceUnavailable)
}

// greedyAssign finds nearby drivers, sorts by distance, and locks the first one
// it can claim atomically.
func greedyAssign(ctx context.Context, req request) (string, float64, bool) {
	cands := fetchNearby(req.Lat, req.Lng)
	sort.Slice(cands, func(i, j int) bool { return cands[i].Dist < cands[j].Dist })
	for _, c := range cands {
		if claim(ctx, c.ID, req.ReqID) {
			assign(ctx, req, c)
			return c.ID, c.Dist, true
		}
	}
	return "", 0, false
}

// batchLoop collects requests over a short window and solves them optimally.
func batchLoop() {
	win, _ := strconv.Atoi(env("BATCH_WINDOW_MS", "1000"))
	ticker := time.NewTicker(time.Duration(win) * time.Millisecond)
	var pending []request
	for {
		select {
		case req := <-batchCh:
			pending = append(pending, req)
		case <-ticker.C:
			if len(pending) == 0 {
				continue
			}
			solveBatch(context.Background(), pending)
			pending = nil
		}
	}
}

func solveBatch(ctx context.Context, reqs []request) {
	// Build candidate driver universe from each rider's nearby set.
	driverPos := map[string]nearbyDriver{}
	for _, r := range reqs {
		for _, d := range fetchNearby(r.Lat, r.Lng) {
			driverPos[d.ID] = d
		}
	}
	if len(driverPos) == 0 {
		return
	}
	drivers := make([]match.Point, 0, len(driverPos))
	dmeta := make([]nearbyDriver, 0, len(driverPos))
	for _, d := range driverPos {
		drivers = append(drivers, match.Point{ID: d.ID, Lat: d.Lat, Lng: d.Lng})
		dmeta = append(dmeta, d)
	}
	riders := make([]match.Point, len(reqs))
	for i, r := range reqs {
		riders[i] = match.Point{ID: r.ReqID, Lat: r.Lat, Lng: r.Lng}
	}

	cost := match.CostMatrix(riders, drivers)
	a := match.Optimal(cost, len(drivers))
	for i, j := range a.RiderToDriver {
		if j < 0 {
			continue
		}
		d := dmeta[j]
		// Lock still required: another matcher/instance may contend.
		if claim(ctx, d.ID, reqs[i].ReqID) {
			assign(ctx, reqs[i], d)
		} else {
			// fall back to greedy for this rider if its optimal pick was taken
			greedyAssign(ctx, reqs[i])
		}
	}
}

// claim atomically reserves a driver. SET NX = set only if absent; EX = TTL so a
// crashed matcher can't leak the lock forever.
func claim(ctx context.Context, driverID, reqID string) bool {
	ok, err := rdb.SetNX(ctx, "lock:driver:"+driverID, reqID, lockTTL).Result()
	return err == nil && ok
}

func assign(ctx context.Context, req request, d nearbyDriver) {
	rdb.HSet(ctx, "req:assigned", req.ReqID, d.ID)
	atomic.AddInt64(&totalAssigned, 1)
	atomic.AddInt64(&sumPickupM, int64(d.Dist))
	atomic.AddInt64(&activeTrips, 1)

	dur := int64(math.Min(12000, math.Max(3000, d.Dist/8*1000))) // ~8 m/s
	ev := tripEvent{
		Kind: "trip", Event: "assigned", ReqID: req.ReqID, Driver: d.ID,
		DriverLat: d.Lat, DriverLng: d.Lng, RiderLat: req.Lat, RiderLng: req.Lng,
		PickupDist: d.Dist, DurMs: dur, Ts: time.Now().UnixMilli(),
	}
	publish(ctx, ev)

	go func() {
		time.Sleep(time.Duration(dur) * time.Millisecond)
		ev.Event = "completed"
		ev.Ts = time.Now().UnixMilli()
		publish(ctx, ev)
		rdb.Del(ctx, "lock:driver:"+d.ID)
		rdb.HDel(ctx, "req:assigned", req.ReqID)
		atomic.AddInt64(&activeTrips, -1)
	}()
}

func publish(ctx context.Context, ev tripEvent) {
	b, _ := json.Marshal(ev)
	// Key trips by the rider's coarse H3 cell for spatial locality on the topic.
	key, err := spatial.CellOf(ev.RiderLat, ev.RiderLng, bus.KeyRes)
	if err != nil {
		key = ev.ReqID
	}
	bus.Produce(ctx, prod, bus.TopicTrips, key, b)
}

func fetchNearby(lat, lng float64) []nearbyDriver {
	url := nearbyURL + "?lat=" + ftoa(lat) + "&lng=" + ftoa(lng) + "&radius=" + ftoa(radiusM)
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var out struct {
		Drivers []nearbyDriver `json:"drivers"`
	}
	if json.Unmarshal(body, &out) != nil {
		return nil
	}
	return out.Drivers
}

func handleStats(w http.ResponseWriter, r *http.Request) {
	total := atomic.LoadInt64(&totalAssigned)
	var avg float64
	if total > 0 {
		avg = float64(atomic.LoadInt64(&sumPickupM)) / float64(total)
	}
	writeJSON(w, map[string]any{
		"mode":          mode,
		"totalAssigned": total,
		"activeTrips":   atomic.LoadInt64(&activeTrips),
		"avgPickupM":    math.Round(avg*10) / 10,
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(v)
}
func ftoa(f float64) string { return strconv.FormatFloat(f, 'f', 6, 64) }
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
