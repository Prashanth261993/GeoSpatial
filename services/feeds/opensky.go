package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Prashanth261993/geospatial/internal/bus"
	"github.com/Prashanth261993/geospatial/internal/event"
	"github.com/Prashanth261993/geospatial/internal/spatial"
)

// Pacific Northwest bounding box (around Seattle / SeaTac).
const (
	laMin, loMin = 46.5, -123.6
	laMax, loMax = 48.6, -121.0
	pollEvery    = 12 * time.Second
)

// openSkyResp is the minimal shape we read from the foreign API. Each state is a
// heterogeneous array; we reference documented indices and translate into our
// clean internal Position (anti-corruption layer — the array shape never leaks).
type openSkyResp struct {
	States [][]any `json:"states"`
}

func pollOpenSky(ctx context.Context, m *manager, f *feed) {
	cli := &http.Client{Timeout: 10 * time.Second}
	base := env("OPENSKY_URL", "https://opensky-network.org/api/states/all")
	user, pass := env("OPENSKY_USER", ""), env("OPENSKY_PASS", "")

	// poll immediately, then on an interval
	poll := func() {
		url := fmt.Sprintf("%s?lamin=%f&lomin=%f&lamax=%f&lomax=%f", base, laMin, loMin, laMax, loMax)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if user != "" {
			req.SetBasicAuth(user, pass)
		}
		resp, err := cli.Do(req)
		if err != nil {
			setErr(m, f, "request: "+err.Error())
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			setErr(m, f, "rate limited (429) — backing off")
			return
		}
		if resp.StatusCode != http.StatusOK {
			setErr(m, f, fmt.Sprintf("status %d", resp.StatusCode))
			return
		}
		body, _ := io.ReadAll(resp.Body)
		var out openSkyResp
		if json.Unmarshal(body, &out) != nil {
			setErr(m, f, "decode failed")
			return
		}
		n := emit(ctx, m, out.States)
		m.mu.Lock()
		f.lastCount = n
		f.lastPoll = time.Now()
		f.lastErr = ""
		m.mu.Unlock()
		log.Printf("opensky: %d aircraft", n)
	}

	poll()
	t := time.NewTicker(pollEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			poll()
		}
	}
}

// emit normalizes OpenSky state vectors into Position events and produces them.
// Documented indices: [0]=icao24, [1]=callsign, [5]=lng, [6]=lat, [8]=on_ground,
// [10]=true_track (heading). The foreign array shape never escapes this function.
func emit(ctx context.Context, m *manager, states [][]any) int {
	n := 0
	for _, s := range states {
		if len(s) < 11 {
			continue
		}
		lng, ok1 := s[5].(float64)
		lat, ok2 := s[6].(float64)
		if !ok1 || !ok2 {
			continue // no position fix
		}
		onGround, _ := s[8].(bool)
		if onGround {
			continue // only airborne aircraft
		}
		id, _ := s[0].(string)
		if id == "" {
			continue
		}
		hdg, _ := s[10].(float64)
		callsign := ""
		if c, ok := s[1].(string); ok {
			callsign = strings.TrimSpace(c)
		}
		p := event.Position{
			ID: "ac-" + id, Lat: lat, Lng: lng, Ts: time.Now().UnixMilli(),
			Type: event.TypeAircraft, Hdg: hdg,
		}
		_ = callsign
		key, err := spatial.CellOf(lat, lng, bus.KeyRes)
		if err != nil {
			continue
		}
		b, _ := json.Marshal(p)
		bus.Produce(ctx, m.prod, bus.TopicPositions, key, b)
		n++
	}
	return n
}

func setErr(m *manager, f *feed, msg string) {
	m.mu.Lock()
	f.lastErr = msg
	f.lastPoll = time.Now()
	m.mu.Unlock()
	log.Printf("opensky: %s", msg)
}
