package ucodec

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

func StructToKeyValue(v interface{}) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return "", err
	}

	pairs := make([]string, 0, len(m))
	typeKey := "type"
	if typeVal, exists := m[typeKey]; exists {
		pairs = append(pairs, fmt.Sprintf("%s=%v", typeKey, typeVal))
	}

	keys := make([]string, 0, len(m))
	for key := range m {
		if key == typeKey {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		pairs = append(pairs, fmt.Sprintf("%s=%v", key, m[key]))
	}

	return strings.Join(pairs, ","), nil
}

