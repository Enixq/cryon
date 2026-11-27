package intelligence

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
)

// DecodeStrict rejects ambiguous objects, missing required fields and trailing
// data before typed decoding. All plan fields are required, including arrays.
func DecodeStrict(raw []byte, out any) error {
	if len(raw) > 65536 {
		return fmt.Errorf("plan too large")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	value, err := readJSON(d, 0)
	if err != nil {
		return err
	}
	if _, err = d.Token(); err != io.EOF {
		return fmt.Errorf("trailing JSON")
	}
	if err = checkShape(value, reflect.TypeOf(out).Elem()); err != nil {
		return err
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	return d.Decode(out)
}
func readJSON(d *json.Decoder, depth int) (any, error) {
	if depth > 64 {
		return nil, fmt.Errorf("JSON too deeply nested")
	}
	tok, err := d.Token()
	if err != nil {
		return nil, err
	}
	if delim, ok := tok.(json.Delim); ok {
		if delim == '{' {
			m := map[string]any{}
			for d.More() {
				k, e := d.Token()
				if e != nil {
					return nil, e
				}
				key := k.(string)
				if _, ok := m[key]; ok {
					return nil, fmt.Errorf("duplicate JSON field")
				}
				v, e := readJSON(d, depth+1)
				if e != nil {
					return nil, e
				}
				m[key] = v
			}
			_, e := d.Token()
			return m, e
		}
		if delim == '[' {
			a := []any{}
			for d.More() {
				v, e := readJSON(d, depth+1)
				if e != nil {
					return nil, e
				}
				a = append(a, v)
			}
			_, e := d.Token()
			return a, e
		}
		return nil, fmt.Errorf("unexpected delimiter")
	}
	return tok, nil
}
func checkShape(v any, t reflect.Type) error {
	if v == nil {
		return fmt.Errorf("null plan field")
	}
	if t.Kind() == reflect.Struct {
		m, ok := v.(map[string]any)
		if !ok {
			return fmt.Errorf("expected object")
		}
		if len(m) != t.NumField() {
			return fmt.Errorf("unexpected field count")
		}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			key := strings.Split(f.Tag.Get("json"), ",")[0]
			child, ok := m[key]
			if !ok {
				return fmt.Errorf("missing field %s", key)
			}
			if err := checkShape(child, f.Type); err != nil {
				return err
			}
		}
	}
	if t.Kind() == reflect.Slice {
		a, ok := v.([]any)
		if !ok {
			return fmt.Errorf("expected array")
		}
		for _, child := range a {
			if err := checkShape(child, t.Elem()); err != nil {
				return err
			}
		}
	}
	return nil
}

// PlanSchema keeps the provider schema in sync with the typed wire contract.
func PlanSchema(value any) map[string]any { return typeSchema(reflect.TypeOf(value)) }
func typeSchema(t reflect.Type) map[string]any {
	switch t.Kind() {
	case reflect.Struct:
		properties := map[string]any{}
		required := []string{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			key := strings.Split(f.Tag.Get("json"), ",")[0]
			child := typeSchema(f.Type)
			if key == "query" {
				child["maxLength"] = 120
			}
			if key == "reason" {
				child["minLength"] = 1
				child["maxLength"] = 160
			}
			properties[key] = child
			required = append(required, key)
		}
		return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
	case reflect.Slice:
		return map[string]any{"type": "array", "items": typeSchema(t.Elem())}
	case reflect.Int, reflect.Uint64:
		return map[string]any{"type": "integer"}
	case reflect.Float64:
		return map[string]any{"type": "number"}
	default:
		return map[string]any{"type": "string"}
	}
}
