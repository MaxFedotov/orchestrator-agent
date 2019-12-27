package config

import (
	"time"
)

// Duration wrapper for time.Duration in TOML
type Duration struct {
	time.Duration
}

// UnmarshalText from TOML
func (d *Duration) UnmarshalText(text []byte) error {
	var err error
	d.Duration, err = time.ParseDuration(string(text))
	return err
}

// Value return time.Duration value
func (d *Duration) Value() time.Duration {
	if d == nil {
		var d time.Duration
		return d
	}
	return d.Duration
}
