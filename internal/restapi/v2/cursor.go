package v2

import (
	"encoding/base64"
	"encoding/json"
	"errors"
)

const opaqueCursorVersion = 1

var errInvalidOpaqueCursor = errors.New("invalid opaque cursor")

type opaqueCursorEnvelope struct {
	Version int             `json:"v"`
	Kind    string          `json:"kind"`
	Value   json.RawMessage `json:"value"`
}

func encodeOpaqueCursor(kind string, value any) string {
	payload, _ := json.Marshal(value)
	envelope, _ := json.Marshal(opaqueCursorEnvelope{Version: opaqueCursorVersion, Kind: kind, Value: payload})
	return base64.RawURLEncoding.EncodeToString(envelope)
}

func decodeOpaqueCursor(kind, raw string, target any) error {
	payload, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return errInvalidOpaqueCursor
	}
	var envelope opaqueCursorEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil || envelope.Version != opaqueCursorVersion || envelope.Kind != kind || len(envelope.Value) == 0 {
		return errInvalidOpaqueCursor
	}
	if err := json.Unmarshal(envelope.Value, target); err != nil {
		return errInvalidOpaqueCursor
	}
	return nil
}
