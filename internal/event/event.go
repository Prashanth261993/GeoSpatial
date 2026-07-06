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

	// Aircraft-only enrichment (from OpenSky), used for the hover tooltip.
	Callsign string  `json:"callsign,omitempty"`
	Country  string  `json:"country,omitempty"`
	AltM     float64 `json:"altM,omitempty"`  // geometric altitude, meters
	VelMps   float64 `json:"velMps,omitempty"` // ground speed, m/s
	VRateMps float64 `json:"vRateMps,omitempty"` // vertical rate, m/s
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
