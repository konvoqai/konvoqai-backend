package handler

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (c *Handler) UploadDocument(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	name, size, mime, err := readUploadedFile(r, "document")
	if err != nil {
		jsonErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var id string
	err = c.db.QueryRow(`INSERT INTO documents (user_id,file_name,file_size,mime_type) VALUES ($1,$2,$3,$4) RETURNING id`, claims.UserID, name, size, nullable(mime)).Scan(&id)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "db error")
		return
	}
	// Index text content in Pinecone for RAG (TXT and CSV supported natively)
	if fhs := r.MultipartForm.File["document"]; len(fhs) > 0 {
		if f, openErr := fhs[0].Open(); openErr == nil {
			if data, readErr := io.ReadAll(io.LimitReader(f, 2<<20)); readErr == nil {
				docID, docName, docMime := id, name, mime
				go func() {
					if text := extractDocumentText(docName, docMime, data); text != "" {
						_ = c.pineconeUpsert(claims.UserID, "doc:"+docID, text)
					}
				}()
			}
			f.Close()
		}
	}
	jsonOK(w, map[string]interface{}{"success": true, "document": map[string]interface{}{"id": id, "fileName": name, "size": size, "mimeType": mime}})
}

func (c *Handler) UploadMultipleDocuments(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	if err := r.ParseMultipartForm(40 << 20); err != nil {
		jsonErr(w, http.StatusBadRequest, "invalid multipart form")
		return
	}
	files := r.MultipartForm.File["documents"]
	items := []map[string]interface{}{}
	for _, fh := range files {
		name, size, mime := docFromHeader(fh)
		var id string
		_ = c.db.QueryRow(`INSERT INTO documents (user_id,file_name,file_size,mime_type) VALUES ($1,$2,$3,$4) RETURNING id`, claims.UserID, name, size, nullable(mime)).Scan(&id)
		items = append(items, map[string]interface{}{"id": id, "fileName": name, "size": size, "mimeType": mime})
		// Index text content in Pinecone for RAG
		if id != "" {
			if f, openErr := fh.Open(); openErr == nil {
				if data, readErr := io.ReadAll(io.LimitReader(f, 2<<20)); readErr == nil {
					docID, docName, docMime := id, name, mime
					go func() {
						if text := extractDocumentText(docName, docMime, data); text != "" {
							_ = c.pineconeUpsert(claims.UserID, "doc:"+docID, text)
						}
					}()
				}
				f.Close()
			}
		}
	}
	jsonOK(w, map[string]interface{}{"success": true, "documents": items})
}

// extractDocumentText extracts plain text from uploaded file bytes.
// Supports TXT and CSV natively. PDF/DOCX/XLSX require additional Go libraries
// (ledongthuc/pdf, nguyenthenguyen/docx, xuri/excelize) and are not yet implemented.
func extractDocumentText(filename, mime string, data []byte) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch {
	case ext == ".txt" || strings.Contains(mime, "text/plain"):
		return strings.TrimSpace(string(data))
	case ext == ".csv" || strings.Contains(mime, "text/csv") || strings.Contains(mime, "application/csv"):
		return extractCSVText(data)
	default:
		return ""
	}
}

func extractCSVText(data []byte) string {
	reader := csv.NewReader(bytes.NewReader(data))
	reader.LazyQuotes = true
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return strings.TrimSpace(string(data))
	}
	var sb strings.Builder
	for _, row := range records {
		sb.WriteString(strings.Join(row, " | "))
		sb.WriteString("\n")
	}
	return strings.TrimSpace(sb.String())
}
func (c *Handler) CreateWidget(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	var body struct {
		Name        string                 `json:"name"`
		Theme       string                 `json:"theme"`
		Primary     string                 `json:"primaryColor"`
		WelcomeText string                 `json:"welcomeText"`
		Settings    map[string]interface{} `json:"settings"`
	}
	_ = decodeJSON(r, &body)
	cfg := map[string]interface{}{"theme": body.Theme, "primaryColor": body.Primary, "welcomeText": body.WelcomeText}
	for k, v := range body.Settings {
		cfg[k] = v
	}
	cfgJSON, _ := json.Marshal(cfg)
	widgetKey := randomID("wk")
	var id int64
	err := c.db.QueryRow(`INSERT INTO widget_keys (user_id,widget_key,widget_name,widget_config,is_active) VALUES ($1,$2,$3,$4::jsonb,TRUE)
		ON CONFLICT (user_id) DO UPDATE SET widget_key=EXCLUDED.widget_key,widget_name=EXCLUDED.widget_name,widget_config=EXCLUDED.widget_config,is_active=TRUE,updated_at=CURRENT_TIMESTAMP
		RETURNING id`, claims.UserID, widgetKey, coalesce(body.Name, "My Chat Widget"), string(cfgJSON)).Scan(&id)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "db error")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	_ = c.redis.Del(ctx, "widget:"+widgetKey).Err()
	jsonOK(w, map[string]interface{}{"success": true, "widget": map[string]interface{}{"id": id, "userId": claims.UserID, "widgetKey": widgetKey, "name": coalesce(body.Name, "My Chat Widget"), "settings": cfg}})
}

func coalesce(v, d string) string {
	if strings.TrimSpace(v) == "" {
		return d
	}
	return v
}

func (c *Handler) GetWidget(w http.ResponseWriter, _ *http.Request, claims TokenClaims, _ UserRecord) {
	var id int64
	var key, name string
	var active bool
	var cfgRaw []byte
	var created, updated time.Time
	err := c.db.QueryRow(`SELECT id,widget_key,widget_name,is_active,widget_config,created_at,updated_at FROM widget_keys WHERE user_id=$1`, claims.UserID).Scan(&id, &key, &name, &active, &cfgRaw, &created, &updated)
	if err != nil {
		jsonErr(w, http.StatusNotFound, "widget not found")
		return
	}
	cfg := map[string]interface{}{}
	_ = json.Unmarshal(cfgRaw, &cfg)
	jsonOK(w, map[string]interface{}{"success": true, "widget": map[string]interface{}{"id": id, "userId": claims.UserID, "widgetKey": key, "name": name, "isActive": active, "settings": cfg, "createdAt": created, "updatedAt": updated}})
}

func (c *Handler) UpdateWidget(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	var body map[string]interface{}
	if err := decodeJSON(r, &body); err != nil {
		jsonErr(w, http.StatusBadRequest, "invalid payload")
		return
	}
	name, _ := body["name"].(string)
	cfgJSON, _ := json.Marshal(body)
	_, err := c.db.Exec(`UPDATE widget_keys SET widget_name=COALESCE($2,widget_name),widget_config=COALESCE($3::jsonb,widget_config),updated_at=CURRENT_TIMESTAMP WHERE user_id=$1`, claims.UserID, nullable(name), string(cfgJSON))
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "db error")
		return
	}
	c.GetWidget(w, r, claims, UserRecord{})
}

func (c *Handler) RegenerateWidget(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	newKey := randomID("wk")
	_, err := c.db.Exec(`UPDATE widget_keys SET widget_key=$2,updated_at=CURRENT_TIMESTAMP WHERE user_id=$1`, claims.UserID, newKey)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "db error")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	_ = c.redis.Del(ctx, "widget:"+newKey).Err()
	c.GetWidget(w, r, claims, UserRecord{})
}

func (c *Handler) DeleteWidget(w http.ResponseWriter, _ *http.Request, claims TokenClaims, _ UserRecord) {
	_, _ = c.db.Exec(`DELETE FROM widget_keys WHERE user_id=$1`, claims.UserID)
	jsonOK(w, map[string]interface{}{"success": true})
}

func (c *Handler) WidgetAnalytics(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	limit := 100
	if q := r.URL.Query().Get("limit"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	rows, err := c.db.Query(`SELECT wa.event_type,wa.event_data,wa.created_at FROM widget_analytics wa JOIN widget_keys wk ON wk.id=wa.widget_key_id WHERE wk.user_id=$1 ORDER BY wa.created_at DESC LIMIT $2`, claims.UserID, limit)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()
	items := []map[string]interface{}{}
	for rows.Next() {
		var et string
		var data []byte
		var created time.Time
		_ = rows.Scan(&et, &data, &created)
		m := map[string]interface{}{}
		_ = json.Unmarshal(data, &m)
		items = append(items, map[string]interface{}{"eventType": et, "eventData": m, "createdAt": created})
	}
	jsonOK(w, map[string]interface{}{"success": true, "analytics": items})
}
