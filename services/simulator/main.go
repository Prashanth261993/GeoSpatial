// simulator: spawns N drivers that wander around Seattle and POST positions to ingest.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/Prashanth261993/geospatial/internal/event"
)

const (
	centerLat = 47.6062
	centerLng = -122.3321
	spread    = 0.06 // ~6km box
	stepKm    = 0.0008
)

func main() {
	n, _ := strconv.Atoi(env("DRIVERS", "200"))
	url := env("INGEST_URL", "http://localhost:8080") + "/loc"
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go driver(fmt.Sprintf("d%04d", i), url, &wg)
	}
	log.Printf("simulator: %d drivers -> %s", n, url)
	wg.Wait()
}

func driver(id, url string, wg *sync.WaitGroup) {
	defer wg.Done()
	lat := centerLat + (rand.Float64()-0.5)*spread
	lng := centerLng + (rand.Float64()-0.5)*spread
	tgtLat, tgtLng := pick()
	cli := &http.Client{Timeout: 5 * time.Second}
	t := time.NewTicker(time.Second)
	for range t.C {
		if abs(lat-tgtLat) < stepKm && abs(lng-tgtLng) < stepKm {
			tgtLat, tgtLng = pick()
		}
		lat += clamp(tgtLat-lat, stepKm)
		lng += clamp(tgtLng-lng, stepKm)
		b, _ := json.Marshal(event.Position{ID: id, Lat: lat, Lng: lng, Ts: time.Now().UnixMilli()})
		resp, err := cli.Post(url, "application/json", bytes.NewReader(b))
		if err == nil {
			resp.Body.Close()
		}
	}
}

func pick() (float64, float64) {
	return centerLat + (rand.Float64()-0.5)*spread, centerLng + (rand.Float64()-0.5)*spread
}
func clamp(d, m float64) float64 {
	if d > m {
		return m
	}
	if d < -m {
		return -m
	}
	return d
}
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
