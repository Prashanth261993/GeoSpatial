// Package match implements rider<->driver assignment: a simple online Greedy
// (nearest-free) and a batch-optimal Hungarian assignment that minimizes total
// pickup cost. Cost is pickup distance in meters.
package match

import "math"

// Point is a rider or driver location.
type Point struct {
	ID  string
	Lat float64
	Lng float64
}

// Assignment maps a rider index to a driver index (-1 = unassigned), plus the cost.
type Assignment struct {
	RiderToDriver []int
	TotalCost     float64
}

// CostMatrix[i][j] = pickup distance (m) from driver j to rider i.
func CostMatrix(riders, drivers []Point) [][]float64 {
	m := make([][]float64, len(riders))
	for i, r := range riders {
		m[i] = make([]float64, len(drivers))
		for j, d := range drivers {
			m[i][j] = haversineM(r.Lat, r.Lng, d.Lat, d.Lng)
		}
	}
	return m
}

// Greedy assigns each rider (in order) the nearest still-free driver.
// Online-friendly and O(R*D), but globally suboptimal.
func Greedy(cost [][]float64, nDrivers int) Assignment {
	a := Assignment{RiderToDriver: make([]int, len(cost))}
	used := make([]bool, nDrivers)
	for i := range a.RiderToDriver {
		a.RiderToDriver[i] = -1
	}
	for i := range cost {
		best, bestC := -1, math.MaxFloat64
		for j := 0; j < nDrivers; j++ {
			if !used[j] && cost[i][j] < bestC {
				best, bestC = j, cost[i][j]
			}
		}
		if best >= 0 {
			a.RiderToDriver[i] = best
			used[best] = true
			a.TotalCost += bestC
		}
	}
	return a
}

// Optimal solves the assignment problem (minimize total cost) with the
// Hungarian algorithm over a padded square matrix.
func Optimal(cost [][]float64, nDrivers int) Assignment {
	nR := len(cost)
	n := nR
	if nDrivers > n {
		n = nDrivers
	}
	const big = 1e9
	c := make([][]float64, n)
	for i := 0; i < n; i++ {
		c[i] = make([]float64, n)
		for j := 0; j < n; j++ {
			if i < nR && j < nDrivers {
				c[i][j] = cost[i][j]
			} else {
				c[i][j] = big // padding: phantom riders/drivers
			}
		}
	}
	rowToCol := hungarian(c)

	a := Assignment{RiderToDriver: make([]int, nR)}
	for i := range a.RiderToDriver {
		a.RiderToDriver[i] = -1
	}
	for i := 0; i < nR; i++ {
		j := rowToCol[i]
		if j >= 0 && j < nDrivers && cost[i][j] < big {
			a.RiderToDriver[i] = j
			a.TotalCost += cost[i][j]
		}
	}
	return a
}

// hungarian returns rowToCol assignment for a square cost matrix (O(n^3)),
// using the Jonker-style potential method.
func hungarian(a [][]float64) []int {
	n := len(a)
	const inf = math.MaxFloat64
	u := make([]float64, n+1)
	v := make([]float64, n+1)
	p := make([]int, n+1) // p[j] = row assigned to column j
	way := make([]int, n+1)
	for i := 1; i <= n; i++ {
		p[0] = i
		j0 := 0
		minv := make([]float64, n+1)
		used := make([]bool, n+1)
		for j := 0; j <= n; j++ {
			minv[j] = inf
		}
		for {
			used[j0] = true
			i0 := p[j0]
			delta := inf
			j1 := -1
			for j := 1; j <= n; j++ {
				if used[j] {
					continue
				}
				cur := a[i0-1][j-1] - u[i0] - v[j]
				if cur < minv[j] {
					minv[j] = cur
					way[j] = j0
				}
				if minv[j] < delta {
					delta = minv[j]
					j1 = j
				}
			}
			for j := 0; j <= n; j++ {
				if used[j] {
					u[p[j]] += delta
					v[j] -= delta
				} else {
					minv[j] -= delta
				}
			}
			j0 = j1
			if p[j0] == 0 {
				break
			}
		}
		for {
			j1 := way[j0]
			p[j0] = p[j1]
			j0 = j1
			if j0 == 0 {
				break
			}
		}
	}
	rowToCol := make([]int, n)
	for i := range rowToCol {
		rowToCol[i] = -1
	}
	for j := 1; j <= n; j++ {
		if p[j] > 0 {
			rowToCol[p[j]-1] = j - 1
		}
	}
	return rowToCol
}

func haversineM(lat1, lng1, lat2, lng2 float64) float64 {
	const R = 6371000
	rad := math.Pi / 180
	dLat := (lat2 - lat1) * rad
	dLng := (lng2 - lng1) * rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return R * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
