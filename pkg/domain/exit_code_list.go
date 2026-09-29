package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// ExitCodeList represents process exit codes, supporting flexible JSON unmarshaling
// from standard integer arrays ([0, 1]), string arrays (["0", "1"]), key/value maps
// ({"0": "ok", "1": "error"}), or single integers/strings (0, "0").
type ExitCodeList []int

// UnmarshalJSON implements custom JSON unmarshaling for resilient schema parsing.
func (e *ExitCodeList) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		*e = nil
		return nil
	}

	// 1. Standard integer slice: [0, 1]
	var ints []int
	if err := json.Unmarshal(trimmed, &ints); err == nil {
		*e = ints
		return nil
	}

	// 2. Heterogeneous or string slice: ["0", "1"] or [0, "1"]
	var anySlice []any
	if err := json.Unmarshal(trimmed, &anySlice); err == nil {
		var res []int
		for _, item := range anySlice {
			switch v := item.(type) {
			case float64:
				res = append(res, int(v))
			case string:
				if val, sErr := strconv.Atoi(strings.TrimSpace(v)); sErr == nil {
					res = append(res, val)
				}
			}
		}
		*e = res
		return nil
	}

	// 3. Map/dictionary: {"0": "success", "1": "error"} or {"success": 0}
	var anyMap map[string]any
	if err := json.Unmarshal(trimmed, &anyMap); err == nil {
		seen := make(map[int]bool)
		var res []int
		for k, v := range anyMap {
			if val, sErr := strconv.Atoi(strings.TrimSpace(k)); sErr == nil {
				if !seen[val] {
					seen[val] = true
					res = append(res, val)
				}
			} else if num, ok := v.(float64); ok {
				val := int(num)
				if !seen[val] {
					seen[val] = true
					res = append(res, val)
				}
			} else if str, ok := v.(string); ok {
				if val, sErr := strconv.Atoi(strings.TrimSpace(str)); sErr == nil {
					if !seen[val] {
						seen[val] = true
						res = append(res, val)
					}
				}
			}
		}
		sort.Ints(res)
		*e = res
		return nil
	}

	// 4. Single integer or numeric string: 0 or "0"
	var singleNum float64
	if err := json.Unmarshal(trimmed, &singleNum); err == nil {
		*e = []int{int(singleNum)}
		return nil
	}
	var singleStr string
	if err := json.Unmarshal(trimmed, &singleStr); err == nil {
		if val, sErr := strconv.Atoi(strings.TrimSpace(singleStr)); sErr == nil {
			*e = []int{val}
			return nil
		}
	}

	return fmt.Errorf("cannot unmarshal %s into ExitCodeList", string(trimmed))
}
