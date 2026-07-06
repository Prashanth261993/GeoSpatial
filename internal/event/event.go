// Package event defines the shared position-update message.
package event

// Position is a location update for any moving entity. Type distinguishes
// drivers (matchable) from real-feed entities like aircraft and buses. Hdg is
// the heading in degrees (0-360), used to rotate directional icons.
type Position struct {
	ID   string  `json:"id"`
	Lat  float64 `json:"lat"`
	Lng  float64 `json:"lng"`
	Ts   int64   `json:"ts"`
	Type string  `json:"type,omitempty"` // "driver" (default) | "aircraft" | "bus"
	Hdg  float64 `json:"hdg,omitempty"`
}

// Entity types.
const (
	TypeDriver   = "driver"
	TypeAircraft = "aircraft"
	TypeBus      = "bus"
)

// IsDriver reports whether the position is a matchable driver (empty type
// defaults to driver for backward compatibility).
func (p Position) IsDriver() bool { return p.Type == "" || p.Type == TypeDriver }
