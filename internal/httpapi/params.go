package httpapi

import (
	"net/http"
	"strconv"

	"github.com/sinnercode228/slotbook/internal/slots"
)

func pathID(r *http.Request, name string) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || id <= 0 {
		return 0, badRequest("%s must be a positive integer", name)
	}
	return id, nil
}

// queryID reads an optional positive integer parameter; absent means 0.
func queryID(r *http.Request, name string) (int64, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return 0, nil
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, badRequest("%s must be a positive integer", name)
	}
	return id, nil
}

func requiredQueryID(r *http.Request, name string) (int64, error) {
	id, err := queryID(r, name)
	if err == nil && id == 0 {
		return 0, badRequest("%s is required", name)
	}
	return id, err
}

// queryDate reads an optional YYYY-MM-DD parameter.
func queryDate(r *http.Request, name string) (slots.Date, bool, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return slots.Date{}, false, nil
	}
	d, err := slots.ParseDate(raw)
	if err != nil {
		return slots.Date{}, false, badRequest("%s: %v", name, err)
	}
	return d, true, nil
}
