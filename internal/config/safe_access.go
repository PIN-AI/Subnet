package config

// SafeGetString safely retrieves a string value from a nested map
func SafeGetString(m map[string]interface{}, keys ...string) (string, bool) {
	if m == nil || len(keys) == 0 {
		return "", false
	}

	var current interface{} = m
	for i, key := range keys {
		currentMap, ok := current.(map[string]interface{})
		if !ok {
			return "", false
		}

		val, exists := currentMap[key]
		if !exists {
			return "", false
		}

		// If this is the last key, try to get string value
		if i == len(keys)-1 {
			str, ok := val.(string)
			return str, ok
		}

		current = val
	}

	return "", false
}

// SafeGetBool safely retrieves a bool value from a nested map
func SafeGetBool(m map[string]interface{}, keys ...string) (bool, bool) {
	if m == nil || len(keys) == 0 {
		return false, false
	}

	var current interface{} = m
	for i, key := range keys {
		currentMap, ok := current.(map[string]interface{})
		if !ok {
			return false, false
		}

		val, exists := currentMap[key]
		if !exists {
			return false, false
		}

		// If this is the last key, try to get bool value
		if i == len(keys)-1 {
			b, ok := val.(bool)
			return b, ok
		}

		current = val
	}

	return false, false
}

// SafeGetInt safely retrieves an int value from a nested map
func SafeGetInt(m map[string]interface{}, keys ...string) (int, bool) {
	if m == nil || len(keys) == 0 {
		return 0, false
	}

	var current interface{} = m
	for i, key := range keys {
		currentMap, ok := current.(map[string]interface{})
		if !ok {
			return 0, false
		}

		val, exists := currentMap[key]
		if !exists {
			return 0, false
		}

		// If this is the last key, try to get int value
		if i == len(keys)-1 {
			// Try different number types
			switch v := val.(type) {
			case int:
				return v, true
			case int64:
				return int(v), true
			case float64:
				return int(v), true
			default:
				return 0, false
			}
		}

		current = val
	}

	return 0, false
}

// SafeGetMap safely retrieves a map from a nested structure
func SafeGetMap(m map[string]interface{}, keys ...string) (map[string]interface{}, bool) {
	if m == nil || len(keys) == 0 {
		return nil, false
	}

	var current interface{} = m
	for _, key := range keys {
		currentMap, ok := current.(map[string]interface{})
		if !ok {
			return nil, false
		}

		val, exists := currentMap[key]
		if !exists {
			return nil, false
		}

		current = val
	}

	result, ok := current.(map[string]interface{})
	return result, ok
}
