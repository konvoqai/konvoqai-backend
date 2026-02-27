package controller

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"konvoq-backend/utils"
)

func (c *Controller) ListDocuments(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	rows, err := c.db.Query(`SELECT id,file_name,file_size,mime_type,created_at FROM documents WHERE user_id=$1 ORDER BY created_at DESC`, claims.UserID)
	if err != nil {
		c.logRequestError(r, "list documents query failed", err, "user_id", claims.UserID)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()

	items := make([]map[string]interface{}, 0)
	for rows.Next() {
		var id, fileName string
		var fileSize int64
		var mime sql.NullString
		var createdAt time.Time
		if err := rows.Scan(&id, &fileName, &fileSize, &mime, &createdAt); err != nil {
			c.logRequestWarn(r, "list documents row scan failed", err, "user_id", claims.UserID)
			continue
		}
		items = append(items, map[string]interface{}{
			"id":        id,
			"fileName":  fileName,
			"fileSize":  fileSize,
			"mimeType":  utils.NullString(mime),
			"createdAt": createdAt,
		})
	}

	utils.JSONOK(w, map[string]interface{}{"success": true, "documents": items})
}

func (c *Controller) UploadDocument(w http.ResponseWriter, r *http.Request, claims TokenClaims, user UserRecord) {
	limits := limitsForPlan(user.PlanType)
	var currentDocs int
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM documents WHERE user_id=$1`, claims.UserID).Scan(&currentDocs); err != nil {
		c.logRequestError(r, "document upload count query failed", err, "user_id", claims.UserID)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if currentDocs >= limits.Documents {
		w.WriteHeader(http.StatusPaymentRequired)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success":      false,
			"message":      "document limit reached for your plan",
			"limitReached": true,
			"usage": map[string]interface{}{
				"used":  currentDocs,
				"limit": limits.Documents,
			},
		})
		return
	}

	name, size, mime, err := readUploadedFile(r, "document")
	if err != nil {
		utils.JSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var id string
	err = c.db.QueryRow(`INSERT INTO documents (user_id,file_name,file_size,mime_type) VALUES ($1,$2,$3,$4) RETURNING id`,
		claims.UserID, name, size, utils.Nullable(mime)).Scan(&id)
	if err != nil {
		c.logRequestError(r, "upload document insert failed", err, "user_id", claims.UserID, "file_name", name)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if fhs := r.MultipartForm.File["document"]; len(fhs) > 0 {
		if f, openErr := fhs[0].Open(); openErr == nil {
			if data, readErr := io.ReadAll(io.LimitReader(f, 2<<20)); readErr == nil {
				docID, docName, docMime := id, name, mime
				go func() {
					if text := extractDocumentText(docName, docMime, data); text != "" {
						if err := c.pineconeUpsert(claims.UserID, "doc:"+docID, text); err != nil {
							c.logger.Warn("document index upsert failed", "user_id", claims.UserID, "document_id", docID, "error", err)
						}
					}
				}()
			}
			f.Close()
		}
	}
	utils.JSONOK(w, map[string]interface{}{"success": true, "document": map[string]interface{}{"id": id, "fileName": name, "size": size, "mimeType": mime}})
}

func (c *Controller) UploadMultipleDocuments(w http.ResponseWriter, r *http.Request, claims TokenClaims, user UserRecord) {
	if err := r.ParseMultipartForm(40 << 20); err != nil {
		utils.JSONErr(w, http.StatusBadRequest, "invalid multipart form")
		return
	}
	files := r.MultipartForm.File["documents"]
	limits := limitsForPlan(user.PlanType)
	var currentDocs int
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM documents WHERE user_id=$1`, claims.UserID).Scan(&currentDocs); err != nil {
		c.logRequestError(r, "batch upload count query failed", err, "user_id", claims.UserID)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if currentDocs+len(files) > limits.Documents {
		w.WriteHeader(http.StatusPaymentRequired)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success":      false,
			"message":      "document limit reached for your plan",
			"limitReached": true,
			"usage": map[string]interface{}{
				"used":  currentDocs,
				"limit": limits.Documents,
			},
		})
		return
	}

	items := []map[string]interface{}{}
	for _, fh := range files {
		name, size, mime := docFromHeader(fh)
		var id string
		if err := c.db.QueryRow(`INSERT INTO documents (user_id,file_name,file_size,mime_type) VALUES ($1,$2,$3,$4) RETURNING id`,
			claims.UserID, name, size, utils.Nullable(mime)).Scan(&id); err != nil {
			c.logRequestWarn(r, "batch upload document insert failed", err, "user_id", claims.UserID, "file_name", name)
		}
		items = append(items, map[string]interface{}{"id": id, "fileName": name, "size": size, "mimeType": mime})
		if id != "" {
			if f, openErr := fh.Open(); openErr == nil {
				if data, readErr := io.ReadAll(io.LimitReader(f, 2<<20)); readErr == nil {
					docID, docName, docMime := id, name, mime
					go func() {
						if text := extractDocumentText(docName, docMime, data); text != "" {
							if err := c.pineconeUpsert(claims.UserID, "doc:"+docID, text); err != nil {
								c.logger.Warn("document batch index upsert failed", "user_id", claims.UserID, "document_id", docID, "error", err)
							}
						}
					}()
				}
				f.Close()
			}
		}
	}
	utils.JSONOK(w, map[string]interface{}{"success": true, "documents": items})
}

func (c *Controller) DeleteDocument(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		utils.JSONErr(w, http.StatusBadRequest, "document id is required")
		return
	}

	res, err := c.db.Exec(`DELETE FROM documents WHERE id=$1 AND user_id=$2`, id, claims.UserID)
	if err != nil {
		c.logRequestError(r, "delete document failed", err, "user_id", claims.UserID, "document_id", id)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		utils.JSONErr(w, http.StatusNotFound, "document not found")
		return
	}

	utils.JSONOK(w, map[string]interface{}{"success": true})
}

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
