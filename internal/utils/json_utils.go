package utils

import (
	"encoding/json"
	"errors"
)

// ParseJSON parses a JSON string into a map
func ParseJSON(jsonStr string) (map[string]interface{}, error) {
	if jsonStr == "" {
		return nil, errors.New("empty JSON string")
	}
	
	var result map[string]interface{}
	err := json.Unmarshal([]byte(jsonStr), &result)
	if err != nil {
		return nil, err
	}
	
	return result, nil
}

// ParseJSONInto parses a JSON string into the provided struct
func ParseJSONInto(jsonStr string, target interface{}) error {
	if jsonStr == "" {
		return errors.New("empty JSON string")
	}
	
	return json.Unmarshal([]byte(jsonStr), target)
}
