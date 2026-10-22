package main

import (
	"encoding/json"
	"io"
)

func emitJSONError(w io.Writer, err error) {
	_ = json.NewEncoder(w).Encode(map[string]string{
		"event":   "error",
		"code":    "transfer",
		"message": err.Error(),
	})
}
