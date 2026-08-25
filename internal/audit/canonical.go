package audit

import (
	"bytes"
	"encoding/json"
	"fmt"
)

func CanonicalJSON(value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return nil, fmt.Errorf("规范化 JSON: %w", err)
	}
	return compact.Bytes(), nil
}
