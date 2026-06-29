// simulator: spawns N drivers that drive real Seattle roads (OSRM) and POST positions.
// Falls back to straight-line motion if OSRM is unavailable.
package main

import (
	"bytes"
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

func main() {
	n, _ := strconv.Atoi(env("DRIVERS", "200"))
	ingest = env("INGEST_URL", "http://localhost:8080") + "/loc"
	osrm = env("OSRM_URL", "")
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go driver(fmt.Sprintf("d%04d", i), &wg)
	}
	log.Printf("simulator: %d drivers -> %s (osrm=%q)", n, ingest, osrm)
	wg.Wait()
}

func driver(id string, wg *sync.WaitGroup) {
	defer wg.Done()
	lat := centerLat + (rand.Float64()-0.5)*spread
	lng := centerLng + (rand.Float64()-0.5)*spread
	cli := &http.Client{Timeout: 5 * time.Second}
	t := time.NewTicker(time.Second)
	var route [][2]float64
	ri := 0
	tgtLat, tgtLng := pick()
	for range t.C {
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
	b, _ := json.Marshal(event.Position{ID: id, Lat: lat, Lng: lng, Ts: time.Now().UnixMilli()})
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
