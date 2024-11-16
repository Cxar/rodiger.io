package types

import (
	"encoding/json"
	"time"
)

// Duration is a time.Duration that supports JSON marshaling/unmarshaling
type Duration time.Duration

func (d *Duration) UnmarshalJSON(b []byte) error {
	var v string
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}

	parsed, err := time.ParseDuration(v)
	if err != nil {
		return err
	}

	*d = Duration(parsed)
	return nil
}
