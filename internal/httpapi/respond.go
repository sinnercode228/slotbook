package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"time"
)

// maxBodyBytes bounds every request body; the largest real request (a
// confirmation with contact details) is well under 2 KB.
const maxBodyBytes = 64 << 10

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorBody{errorDetail{Code: code, Message: message}})
}

// apiError is an error that already knows its HTTP response.
type apiError struct {
	status  int
	code    string
	message string
}

func (e *apiError) Error() string { return e.message }

func badRequest(format string, args ...any) error {
	return &apiError{http.StatusBadRequest, "bad_request", fmt.Sprintf(format, args...)}
}

// decodeJSON reads exactly one JSON object with known fields from a body of
// at most maxBodyBytes.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return decodeError(err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return badRequest("request body must contain a single JSON object")
		}
		return decodeError(err)
	}
	return nil
}

// decodeError turns a decoding failure into a 400 that names the offending
// field in JSON terms. The decoder's own messages mention Go types and time
// layouts, which mean nothing to a client and describe the server's insides.
func decodeError(err error) error {
	var (
		tooLarge *http.MaxBytesError
		syntax   *json.SyntaxError
		mismatch *json.UnmarshalTypeError
	)
	switch {
	case errors.As(err, &tooLarge):
		return &apiError{http.StatusRequestEntityTooLarge, "payload_too_large", fmt.Sprintf("request body is larger than %d bytes", tooLarge.Limit)}
	case errors.Is(err, io.EOF):
		return badRequest("request body is empty")
	case errors.Is(err, io.ErrUnexpectedEOF):
		return badRequest("invalid JSON: the body ends early")
	case errors.As(err, &syntax):
		return badRequest("invalid JSON at byte %d", syntax.Offset)
	case errors.As(err, &mismatch):
		if mismatch.Field == "" {
			return badRequest("request body must be a JSON object")
		}
		return badRequest("%s must be %s", mismatch.Field, jsonKind(mismatch.Type))
	case errors.Is(err, errBadTimestamp):
		return badRequest("times must be RFC 3339 strings, such as 2026-10-14T10:00:00+03:00")
	case strings.HasPrefix(err.Error(), "json: unknown field "):
		// encoding/json has no error type for this one.
		return badRequest("%s", strings.TrimPrefix(err.Error(), "json: "))
	default:
		return badRequest("request body does not have the expected shape")
	}
}

// jsonKind describes the JSON value a Go type decodes from.
func jsonKind(t reflect.Type) string {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "an integer"
	case reflect.Float32, reflect.Float64:
		return "a number"
	case reflect.Bool:
		return "true or false"
	case reflect.String:
		return "a string"
	case reflect.Slice, reflect.Array:
		return "an array"
	default:
		return "an object"
	}
}

var errBadTimestamp = errors.New("not an RFC 3339 time")

// timestamp is a time in a request body. It decodes like time.Time but fails
// with errBadTimestamp, so that the client never sees Go's layout string.
type timestamp struct {
	time.Time
}

func (t *timestamp) UnmarshalJSON(b []byte) error {
	if err := t.Time.UnmarshalJSON(b); err != nil {
		return errBadTimestamp
	}
	return nil
}
