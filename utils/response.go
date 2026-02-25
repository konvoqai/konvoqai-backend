package utils

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

func JSONOK(w http.ResponseWriter, payload map[string]interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(payload)
}

func JSONErr(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "message": message})
}

func DecodeJSON(r *http.Request, out interface{}) error {
	b, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return nil
	}
	return json.Unmarshal(b, out)
}

func NullString(v sql.NullString) interface{} {
	if !v.Valid {
		return nil
	}
	return v.String
}

func NullTime(v sql.NullTime) interface{} {
	if !v.Valid {
		return nil
	}
	return v.Time
}

func NullableInt64(v sql.NullInt64) interface{} {
	if !v.Valid {
		return nil
	}
	return v.Int64
}

func Nullable(v string) interface{} {
	t := strings.TrimSpace(v)
	if t == "" {
		return nil
	}
	return t
}
