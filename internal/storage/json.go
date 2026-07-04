package storage

import "encoding/json"

func mustMarshal(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func mustUnmarshal(data string, v interface{}) error {
	return json.Unmarshal([]byte(data), v)
}
