// Package spatial wraps H3 for cell indexing and proximity queries.
// Resolution is a parameter, not a constant: cars use res 9, aircraft res ~6.
package spatial

import (
	"math"

	h3 "github.com/uber/h3-go/v4"
)

// CellOf returns the H3 cell (as a 15-char hex string) containing the point.
func CellOf(lat, lng float64, res int) (string, error) {
	c, err := h3.LatLngToCell(h3.LatLng{Lat: lat, Lng: lng}, res)
	if err != nil {
		return "", err
	}
	return h3.CellToString(c), nil
}

// DiskCells returns the H3 cells covering a radius (meters) around a point:
// the center cell plus k rings, where k is chosen to cover the radius.
func DiskCells(lat, lng float64, res int, radiusM float64) ([]string, error) {
	center, err := h3.LatLngToCell(h3.LatLng{Lat: lat, Lng: lng}, res)
	if err != nil {
		return nil, err
	}
	edge, err := h3.HexagonEdgeLengthAvgM(res)
	if err != nil {
		return nil, err
	}
	k := int(math.Ceil(radiusM / edge))
	if k < 1 {
		k = 1
	}
	cells, err := h3.GridDisk(center, k)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(cells))
	for i, c := range cells {
		out[i] = h3.CellToString(c)
	}
	return out, nil
}

// DistM is the great-circle distance in meters (narrow-phase exact filter).
func DistM(aLat, aLng, bLat, bLng float64) float64 {
	return h3.GreatCircleDistanceM(h3.LatLng{Lat: aLat, Lng: aLng}, h3.LatLng{Lat: bLat, Lng: bLng})
}

// Children returns the res-`childRes` cells contained in a parent cell string.
// Used to roll a coarse surge zone down to the finer indexing cells.
func Children(parent string, childRes int) ([]string, error) {
	c := h3.CellFromString(parent)
	kids, err := c.Children(childRes)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(kids))
	for i, k := range kids {
		out[i] = h3.CellToString(k)
	}
	return out, nil
}
