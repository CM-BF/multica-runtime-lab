package cli

import (
	"encoding/json"
	"reflect"
	"strings"
)

// Unknown fields belong to the loaded object, not the destination file. Known
// fields are excluded even when omitted on save, so explicit deletion wins.
func unknownConfigFields(data []byte, typed any) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	typ := reflect.TypeOf(typed)
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name == "-" {
			continue
		}
		if name == "" {
			name = field.Name
		}
		for key := range fields {
			if strings.EqualFold(key, name) {
				delete(fields, key)
			}
		}
	}
	return fields, nil
}

func marshalConfigFields(typed any, unknown map[string]json.RawMessage) ([]byte, error) {
	raw, err := json.Marshal(typed)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	for key, value := range unknown {
		fields[key] = value
	}
	return json.Marshal(fields)
}

func (c *CLIConfig) UnmarshalJSON(data []byte) error {
	type plain CLIConfig
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	unknown, err := unknownConfigFields(data, decoded)
	if err != nil {
		return err
	}
	decoded.unknownFields = unknown
	*c = CLIConfig(decoded)
	return nil
}
func (c CLIConfig) MarshalJSON() ([]byte, error) {
	type plain CLIConfig
	return marshalConfigFields(plain(c), c.unknownFields)
}

func (c *BackendOverrides) UnmarshalJSON(data []byte) error {
	type plain BackendOverrides
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	unknown, err := unknownConfigFields(data, decoded)
	if err != nil {
		return err
	}
	decoded.unknownFields = unknown
	*c = BackendOverrides(decoded)
	return nil
}
func (c BackendOverrides) MarshalJSON() ([]byte, error) {
	type plain BackendOverrides
	return marshalConfigFields(plain(c), c.unknownFields)
}

func (c *OpenClawOverride) UnmarshalJSON(data []byte) error {
	type plain OpenClawOverride
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	unknown, err := unknownConfigFields(data, decoded)
	if err != nil {
		return err
	}
	decoded.unknownFields = unknown
	*c = OpenClawOverride(decoded)
	return nil
}
func (c OpenClawOverride) MarshalJSON() ([]byte, error) {
	type plain OpenClawOverride
	return marshalConfigFields(plain(c), c.unknownFields)
}
