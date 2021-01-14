package util

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
)

var (
	// Log print to stdout
	Log = log.New(os.Stdout, "", log.Ldate|log.Ltime|log.Lshortfile)
)

// JSONPut resp json
func JSONPut(w http.ResponseWriter, v interface{}) (int, error) {
	bs, err := json.Marshal(v)
	if err != nil {
		return 0, err
	}
	h := w.Header()
	h.Set("Content-Type", "application/json; charset=utf-8")
	return w.Write(bs)
}
