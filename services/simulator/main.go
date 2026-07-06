// simulator: spawns N drivers that drive real Seattle roads (OSRM) and POST positions.
// Falls back to straight-line motion if OSRM is unavailable.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
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
	spread    = 0.06   // ~6km box
	stepKm    = 0.0008 // straight-line fallback step
	stepM     = 80.0   // meters advanced per tick along a route
)

var ingest string
var osrm string
var matcherURL string

var (
	mu          sync.Mutex
	driverCncls []context.CancelFunc // one per running driver goroutine
	nextDriver  int
	riderRateMs int64 = 700 // atomic-ish under mu for reads in loop; updated via control
)

func main() {
	n, _ := strconv.Atoi(env("DRIVERS", "200"))
	ingest = env("INGEST_URL", "http://localhost:8080") + "/loc"
	osrm = env("OSRM_URL", "")
	matcherURL = env("MATCHER_URL", "")
	if r, err := strconv.Atoi(env("RIDER_RATE_MS", "700")); err == nil {
		riderRateMs = int64(r)
	}

	setDrivers(n)

	if matcherURL != "" {
		go riders(matcherURL + "/request")
		log.Printf("simulator: rider demand -> %s", matcherURL)
	}

	// Control API: adjust driver count and rider rate live from the UI.
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	http.HandleFunc("/control", handleControl)

	log.Printf("simulator: %d drivers -> %s (osrm=%q)", n, ingest, osrm)
	log.Fatal(http.ListenAndServe(":"+env("PORT", "8140"), nil))
}

// setDrivers scales the running driver goroutines up or down to reach target n.
func setDrivers(n int) {
	mu.Lock()
	defer mu.Unlock()
	for len(driverCncls) < n {
		ctx, cancel := context.WithCancel(context.Background())
		id := fmt.Sprintf("d%04d", nextDriver)
		nextDriver++
		driverCncls = append(driverCncls, cancel)
		go driver(ctx, id)
	}
	for len(driverCncls) > n {
		last := len(driverCncls) - 1
		driverCncls[last]() // cancel -> goroutine exits, driver goes stale in index
		driverCncls = driverCncls[:last]
	}
}

func handleControl(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "content-type")
	if r.Method == http.MethodOptions {
		return
	}
	if r.Method == http.MethodPost {
		var req struct {
			Drivers     *int `json:"drivers"`
			RiderRateMs *int `json:"riderRateMs"`
		}
		if json.NewDecoder(r.Body).Decode(&req) == nil {
			if req.Drivers != nil {
				d := *req.Drivers
				if d < 0 {
					d = 0
				}
				if d > 1000 {
					d = 1000
				}
				setDrivers(d)
			}
			if req.RiderRateMs != nil {
				rr := int64(*req.RiderRateMs)
				if rr < 50 {
					rr = 50
				}
				mu.Lock()
				riderRateMs = rr
				mu.Unlock()
			}
		}
	}
	mu.Lock()
	out := map[string]any{"drivers": len(driverCncls), "riderRateMs": riderRateMs}
	mu.Unlock()
	json.NewEncoder(w).Encode(out)
}

// riders emits trip requests at random Seattle locations at the current rider rate.
func riders(url string) {
	cli := &http.Client{Timeout: 5 * time.Second}
	seq := 0
	for {
		mu.Lock()
		rate := riderRateMs
		mu.Unlock()
		time.Sleep(time.Duration(rate) * time.Millisecond)
		seq++
		lat := centerLat + (rand.Float64()-0.5)*spread
		lng := centerLng + (rand.Float64()-0.5)*spread
		body, _ := json.Marshal(map[string]any{
			"reqId": fmt.Sprintf("r%d-%d", time.Now().Unix(), seq),
			"lat":   lat, "lng": lng,
		})
		if resp, err := cli.Post(url, "application/json", bytes.NewReader(body)); err == nil {
			resp.Body.Close()
		}
	}
}

func driver(ctx context.Context, id string) {
	lat := centerLat + (rand.Float64()-0.5)*spread
	lng := centerLng + (rand.Float64()-0.5)*spread
	cli := &http.Client{Timeout: 5 * time.Second}
	t := time.NewTicker(time.Second)
	defer t.Stop()
	var route [][2]float64
	ri := 0
	tgtLat, tgtLng := pick()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if osrm != "" {
			if route == nil || ri >= len(route) {
				if r := fetchRoute(cli, lat, lng); r != nil {
					route, ri = r, 0
				}
			}
			if route != nil && ri < len(route) {
				lng, lat = route[ri][0], route[ri][1]
				ri++
				post(cli, id, lat, lng)
				continue
			}
		}
		if abs(lat-tgtLat) < stepKm && abs(lng-tgtLng) < stepKm {
			tgtLat, tgtLng = pick()
		}
		lat += clamp(tgtLat-lat, stepKm)
		lng += clamp(tgtLng-lng, stepKm)
		post(cli, id, lat, lng)
	}
}

func post(cli *http.Client, id string, lat, lng float64) {
	b, _ := json.Marshal(event.Position{ID: id, Lat: lat, Lng: lng, Ts: time.Now().UnixMilli(), Type: event.TypeDriver})
	if resp, err := cli.Post(ingest, "application/json", bytes.NewReader(b)); err == nil {
		resp.Body.Close()
	}
}

func fetchRoute(cli *http.Client, lat, lng float64) [][2]float64 {
	dLat, dLng := pick()
	url := fmt.Sprintf("%s/route/v1/driving/%f,%f;%f,%f?overview=full&geometries=geojson", osrm, lng, lat, dLng, dLat)
	resp, err := cli.Get(url)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var out struct {
		Routes []struct {
			Geometry struct {
				Coordinates [][]float64 `json:"coordinates"`
			} `json:"geometry"`
		} `json:"routes"`
	}
	if json.Unmarshal(body, &out) != nil || len(out.Routes) == 0 {
		return nil
	}
	c := out.Routes[0].Geometry.Coordinates
	pts := make([][2]float64, 0, len(c))
	for _, p := range c {
		if len(p) == 2 {
			pts = append(pts, [2]float64{p[0], p[1]})
		}
	}
	if len(pts) < 2 {
		return nil
	}
	return resample(pts, stepM)
}

// resample walks the polyline emitting points ~step meters apart for smooth motion.
func resample(pts [][2]float64, step float64) [][2]float64 {
	out := [][2]float64{pts[0]}
	carry := 0.0
	for i := 1; i < len(pts); i++ {
		a, b := pts[i-1], pts[i]
		seg := meters(a[1], a[0], b[1], b[0])
		for carry+seg >= step && seg > 0 {
			f := (step - carry) / seg
			lng := a[0] + (b[0]-a[0])*f
			lat := a[1] + (b[1]-a[1])*f
			out = append(out, [2]float64{lng, lat})
			a = [2]float64{lng, lat}
			seg = meters(a[1], a[0], b[1], b[0])
			carry = 0
		}
		carry += seg
	}
	return out
}

func meters(lat1, lng1, lat2, lng2 float64) float64 {
	const R = 6371000
	dLat := (lat2 - lat1) * math.Pi / 180
	dLng := (lng2 - lng1) * math.Pi / 180
	la := lat1 * math.Pi / 180
	x := dLng * math.Cos(la)
	return R * math.Sqrt(dLat*dLat+x*x)
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
