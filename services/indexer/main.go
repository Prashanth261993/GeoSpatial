// indexer: maintains an H3 spatial index in Redis (Set per cell) from the
// positions stream, and serves proximity queries (/nearby) with a broad-phase
// cell scan + narrow-phase exact distance filter.
package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/Prashanth261993/geospatial/internal/event"
	"github.com/Prashanth261993/geospatial/internal/spatial"
	"github.com/redis/go-redis/v9"
)

const channel = "positions"

var res int

func main() {
	res, _ = strconv.Atoi(env("H3_RES", "9"))
	rdb := redis.NewClient(&redis.Options{Addr: env("REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("redis: %v", err)
	}

	go consume(rdb)

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	http.HandleFunc("/nearby", nearby(rdb))

	addr := ":" + env("PORT", "8100")
	log.Printf("indexer on %s (h3 res=%d)", addr, res)
	log.Fatal(http.ListenAndServe(addr, nil))
}

// consume keeps the index current: on a cell transition, move the id between
// the old and new cell Sets; always refresh the latest position.
func consume(rdb *redis.Client) {
	ctx := context.Background()
	sub := rdb.Subscribe(ctx, channel)
	for msg := range sub.Channel() {
		var p event.Position
		if json.Unmarshal([]byte(msg.Payload), &p) != nil {
			continue
		}
		cell, err := spatial.CellOf(p.Lat, p.Lng, res)
		if err != nil {
			continue
		}
		prev, _ := rdb.HGet(ctx, "geo:cell", p.ID).Result()
		pipe := rdb.Pipeline()
		if prev != cell {
			if prev != "" {
				pipe.SRem(ctx, "h3:"+prev, p.ID)
			}
			pipe.SAdd(ctx, "h3:"+cell, p.ID)
			pipe.HSet(ctx, "geo:cell", p.ID, cell)
		}
		pipe.HSet(ctx, "geo:pos", p.ID, fmtPos(p.Lat, p.Lng))
		pipe.Exec(ctx)
	}
}

type driver struct {
	ID   string  `json:"id"`
	Lat  float64 `json:"lat"`
	Lng  float64 `json:"lng"`
	Dist float64 `json:"dist"`
}

func nearby(rdb *redis.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		q := r.URL.Query()
		lat, _ := strconv.ParseFloat(q.Get("lat"), 64)
		lng, _ := strconv.ParseFloat(q.Get("lng"), 64)
		radius, _ := strconv.ParseFloat(q.Get("radius"), 64)
		if radius <= 0 {
			radius = 1000
		}
		ctx := context.Background()

		// broad phase: cells covering the radius
		cells, err := spatial.DiskCells(lat, lng, res, radius)
		if err != nil {
			http.Error(w, "bad point", http.StatusBadRequest)
			return
		}
		pipe := rdb.Pipeline()
		cmds := make([]*redis.StringSliceCmd, len(cells))
		for i, c := range cells {
			cmds[i] = pipe.SMembers(ctx, "h3:"+c)
		}
		pipe.Exec(ctx)
		idset := map[string]struct{}{}
		for _, cmd := range cmds {
			for _, id := range cmd.Val() {
				idset[id] = struct{}{}
			}
		}
		ids := make([]string, 0, len(idset))
		for id := range idset {
			ids = append(ids, id)
		}

		// narrow phase: exact distance filter
		drivers := []driver{}
		if len(ids) > 0 {
			vals, _ := rdb.HMGet(ctx, "geo:pos", ids...).Result()
			for i, v := range vals {
				s, ok := v.(string)
				if !ok {
					continue
				}
				plat, plng := parsePos(s)
				if d := spatial.DistM(lat, lng, plat, plng); d <= radius {
					drivers = append(drivers, driver{ID: ids[i], Lat: plat, Lng: plng, Dist: d})
				}
			}
		}

		json.NewEncoder(w).Encode(map[string]any{
			"query":          map[string]float64{"lat": lat, "lng": lng, "radius": radius},
			"cells":          cells,
			"drivers":        drivers,
			"candidateCount": len(ids),
			"matchCount":     len(drivers),
		})
	}
}

func fmtPos(lat, lng float64) string {
	return strconv.FormatFloat(lat, 'f', 6, 64) + "," + strconv.FormatFloat(lng, 'f', 6, 64)
}
func parsePos(s string) (float64, float64) {
	parts := strings.SplitN(s, ",", 2)
	if len(parts) != 2 {
		return 0, 0
	}
	lat, _ := strconv.ParseFloat(parts[0], 64)
	lng, _ := strconv.ParseFloat(parts[1], 64)
	return lat, lng
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
