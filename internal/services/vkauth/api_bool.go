package vkmauth

import (
	"bytes"
	"fmt"
)

type apiBool bool

func (b *apiBool) UnmarshalJSON(raw []byte) error {
	switch string(bytes.TrimSpace(raw)) {
	case "true", "1":
		*b = true
	case "false", "0", "null":
		*b = false
	default:
		return fmt.Errorf("неверный флаг в ответе VK")
	}
	return nil
}
