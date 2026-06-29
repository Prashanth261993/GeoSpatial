// Package event defines the shared position-update message.
package event

type Position struct {
	ID  string  `json:"id"`
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
	Ts  int64   `json:"ts"`
}
