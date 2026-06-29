package match

import (
	"fmt"
	"math/rand"
	"testing"
)

// randomScenario places r riders and d drivers in a ~6km Seattle box.
func randomScenario(rng *rand.Rand, r, d int) ([]Point, []Point) {
	const cLat, cLng, spread = 47.6062, -122.3321, 0.06
	riders := make([]Point, r)
	drivers := make([]Point, d)
	for i := range riders {
		riders[i] = Point{ID: fmt.Sprintf("r%d", i),
			Lat: cLat + (rng.Float64()-0.5)*spread, Lng: cLng + (rng.Float64()-0.5)*spread}
	}
	for j := range drivers {
		drivers[j] = Point{ID: fmt.Sprintf("d%d", j),
			Lat: cLat + (rng.Float64()-0.5)*spread, Lng: cLng + (rng.Float64()-0.5)*spread}
	}
	return riders, drivers
}

// TestGreedyVsOptimal verifies Optimal never costs more than Greedy and prints
// the measured average-pickup-distance gain across many random scenarios.
func TestGreedyVsOptimal(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	const trials = 500
	sizes := [][2]int{{10, 12}, {25, 30}, {50, 60}, {100, 120}}

	for _, s := range sizes {
		r, d := s[0], s[1]
		var sumG, sumO float64
		var matched int
		for tr := 0; tr < trials; tr++ {
			riders, drivers := randomScenario(rng, r, d)
			cost := CostMatrix(riders, drivers)
			g := Greedy(cost, d)
			o := Optimal(cost, d)
			if o.TotalCost > g.TotalCost+1e-6 {
				t.Fatalf("optimal worse than greedy: %.2f > %.2f", o.TotalCost, g.TotalCost)
			}
			sumG += g.TotalCost
			sumO += o.TotalCost
			matched += r
		}
		avgG := sumG / float64(matched)
		avgO := sumO / float64(matched)
		gain := (avgG - avgO) / avgG * 100
		fmt.Printf("riders=%-3d drivers=%-3d  avg pickup: greedy=%6.1fm  optimal=%6.1fm  gain=%5.1f%%\n",
			r, d, avgG, avgO, gain)
	}
}
