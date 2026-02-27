package controller

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/smtp"
	"net/url"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"konvoq-backend/utils"
)

// ── Email ──────────────────────────────────────────────────────────────────────

func (c *Controller) sendVerificationEmail(email, code string) {
	if strings.TrimSpace(c.cfg.EmailHost) == "" || strings.TrimSpace(c.cfg.EmailUser) == "" {
		return
	}
	subject := "Your Witzo Verification Code"
	html := buildVerificationEmailHTML(code, c.cfg.VerifyCodeMinutes)
	text := fmt.Sprintf("Witzo AI - Verify Your Email\n\nYour verification code is: %s\nThis code expires in %d minutes.\n\nIf you did not request this code, please ignore this email.", code, c.cfg.VerifyCodeMinutes)
	c.sendEmail(email, subject, html, text)
}

func (c *Controller) sendWelcomeEmail(email string) {
	if strings.TrimSpace(c.cfg.EmailHost) == "" || strings.TrimSpace(c.cfg.EmailUser) == "" {
		return
	}
	subject := "Welcome to Witzo AI"
	html := buildWelcomeEmailHTML(email)
	text := buildWelcomeEmailText(email)
	c.sendEmail(email, subject, html, text)
}

func (c *Controller) sendFollowUpEmail(visitorEmail, visitorName, widgetOwnerName string) {
	if strings.TrimSpace(c.cfg.EmailHost) == "" || strings.TrimSpace(c.cfg.EmailUser) == "" {
		return
	}
	from := strings.TrimSpace(widgetOwnerName)
	if from == "" {
		from = "Witzo"
	}
	subject := "Thanks for chatting with " + from + "!"
	html := buildFollowUpEmailHTML(visitorName, from)
	text := buildFollowUpEmailText(visitorName, from)
	c.sendEmail(visitorEmail, subject, html, text)
}

func (c *Controller) sendEmail(to, subject, htmlBody, textBody string) {
	go func() {
		from := c.cfg.EmailFrom
		if from == "" {
			from = c.cfg.EmailUser
		}
		boundary := utils.RandomID("boundary")
		msg := "From: " + from + "\r\n" +
			"To: " + to + "\r\n" +
			"Subject: " + subject + "\r\n" +
			"MIME-Version: 1.0\r\n" +
			"Content-Type: multipart/alternative; boundary=\"" + boundary + "\"\r\n\r\n" +
			"--" + boundary + "\r\n" +
			"Content-Type: text/plain; charset=UTF-8\r\n\r\n" + textBody + "\r\n\r\n" +
			"--" + boundary + "\r\n" +
			"Content-Type: text/html; charset=UTF-8\r\n\r\n" + htmlBody + "\r\n\r\n" +
			"--" + boundary + "--"
		addr := fmt.Sprintf("%s:%d", c.cfg.EmailHost, c.cfg.EmailPort)
		auth := smtp.PlainAuth("", c.cfg.EmailUser, c.cfg.EmailPassword, c.cfg.EmailHost)
		if err := smtp.SendMail(addr, auth, from, []string{to}, []byte(msg)); err != nil {
			c.logger.Error("email send failed", "to", to, "subject", subject, "error", err)
		}
	}()
}

func buildVerificationEmailHTML(code string, expiryMinutes int) string {
	return `<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Your Verification Code</title>
  </head>
  <body style="margin:0;padding:0;background:#f3f7fb;font-family:Segoe UI,Roboto,Arial,sans-serif;color:#10263d;">
    <table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0" style="background:#f3f7fb;padding:24px 12px;">
      <tr><td align="center">
        <table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0" style="max-width:620px;background:#ffffff;border-radius:20px;overflow:hidden;box-shadow:0 16px 40px rgba(16,38,61,0.12);">
          <tr><td style="padding:0;background:linear-gradient(135deg,#0f172a 0%,#1e3a8a 45%,#0ea5e9 100%);">
            <table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0">
              <tr><td style="padding:28px 28px 20px 28px;">
                <div style="font-size:14px;letter-spacing:2px;text-transform:uppercase;color:#cde8ff;font-weight:600;">Witzo AI</div>
                <h1 style="margin:10px 0 0 0;font-size:28px;line-height:1.2;color:#ffffff;font-weight:700;">Verify Your Email</h1>
                <p style="margin:12px 0 0 0;color:#d7ebff;font-size:15px;line-height:1.55;">Use the one-time code below to continue securely.</p>
              </td></tr>
            </table>
          </td></tr>
          <tr><td style="padding:28px 28px 10px 28px;">
            <p style="margin:0 0 16px 0;font-size:15px;line-height:1.6;color:#324b64;">Enter this verification code in the login window:</p>
            <table role="presentation" cellspacing="0" cellpadding="0" border="0" width="100%" style="background:#f8fbff;border:1px solid #d7e8f8;border-radius:14px;">
              <tr><td align="center" style="padding:22px 12px;">
                <div style="font-size:36px;letter-spacing:10px;font-weight:800;color:#0b4a8f;">` + code + `</div>
              </td></tr>
            </table>
            <p style="margin:16px 0 0 0;font-size:14px;line-height:1.6;color:#4f6a84;">This code expires in <strong>` + fmt.Sprintf("%d", expiryMinutes) + ` minutes</strong>.</p>
            <p style="margin:8px 0 0 0;font-size:14px;line-height:1.6;color:#4f6a84;">If you did not request this, you can safely ignore this email.</p>
          </td></tr>
          <tr><td style="padding:18px 28px 28px 28px;">
            <table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0" style="border-top:1px solid #e4edf6;padding-top:14px;">
              <tr><td style="font-size:12px;line-height:1.6;color:#7b91a8;">This is an automated security message from Witzo AI. Please do not reply to this email.</td></tr>
            </table>
          </td></tr>
        </table>
      </td></tr>
    </table>
  </body>
</html>`
}

func buildWelcomeEmailHTML(email string) string {
	displayName := displayNameFromEmail(email)
	return `<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Welcome to Witzo AI</title>
  </head>
  <body style="margin:0;padding:0;background:#f3f6fb;font-family:Segoe UI,Roboto,Arial,sans-serif;color:#0f2238;">
    <table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0" style="background:#f3f6fb;padding:24px 12px;">
      <tr><td align="center">
        <table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0" style="max-width:640px;background:#ffffff;border-radius:20px;overflow:hidden;box-shadow:0 16px 40px rgba(15,34,56,0.12);">
          <tr><td style="background:linear-gradient(130deg,#0f172a 0%,#1d4ed8 45%,#22d3ee 100%);padding:30px 30px 26px 30px;">
            <div style="font-size:13px;letter-spacing:2px;text-transform:uppercase;color:#cae4ff;font-weight:700;">Witzo AI</div>
            <h1 style="margin:10px 0 0 0;font-size:30px;line-height:1.2;color:#ffffff;font-weight:800;">Welcome aboard</h1>
            <p style="margin:12px 0 0 0;color:#d7ecff;font-size:15px;line-height:1.6;">Your account is now active and ready to use.</p>
          </td></tr>
          <tr><td style="padding:28px 30px 8px 30px;">
            <p style="margin:0;font-size:16px;line-height:1.7;color:#324a62;">Hi ` + displayName + `,</p>
            <p style="margin:14px 0 0 0;font-size:15px;line-height:1.8;color:#4d657e;">
              Thanks for signing up for Witzo AI. Your email has been verified successfully. You can now create your chatbot, connect data sources, and deploy your widget.
            </p>
            <table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0" style="margin:20px 0;background:#f8fbff;border:1px solid #d7e8f8;border-radius:14px;">
              <tr><td style="padding:16px 18px;">
                <p style="margin:0 0 8px 0;font-size:14px;font-weight:700;color:#0b4a8f;">Quick start</p>
                <p style="margin:0;font-size:14px;line-height:1.7;color:#4d657e;">1. Add your website data source<br />2. Train your assistant<br />3. Copy and embed the widget code</p>
              </td></tr>
            </table>
          </td></tr>
          <tr><td style="padding:18px 30px 30px 30px;">
            <table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0" style="border-top:1px solid #e4edf6;padding-top:14px;">
              <tr><td style="font-size:12px;line-height:1.7;color:#7b91a8;">Need help? Reply to this email and our team will assist you.</td></tr>
            </table>
          </td></tr>
        </table>
      </td></tr>
    </table>
  </body>
</html>`
}

func buildWelcomeEmailText(email string) string {
	displayName := displayNameFromEmail(email)
	return "Welcome to Witzo AI\n\nHi " + displayName + ",\nYour account has been verified successfully.\n\nQuick start:\n1. Add your website data source\n2. Train your assistant\n3. Copy and embed the widget code\n\nNeed help? Reply to this email."
}

func buildFollowUpEmailHTML(visitorName, from string) string {
	greeting := "Hi there"
	if strings.TrimSpace(visitorName) != "" {
		greeting = "Hi " + visitorName
	}
	return `<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Thanks for chatting!</title>
  </head>
  <body style="margin:0;padding:0;background:#f3f6fb;font-family:Segoe UI,Roboto,Arial,sans-serif;color:#0f2238;">
    <table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0" style="background:#f3f6fb;padding:24px 12px;">
      <tr><td align="center">
        <table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0" style="max-width:640px;background:#ffffff;border-radius:20px;overflow:hidden;box-shadow:0 16px 40px rgba(15,34,56,0.12);">
          <tr><td style="background:linear-gradient(130deg,#0f172a 0%,#1d4ed8 45%,#22d3ee 100%);padding:30px 30px 26px 30px;">
            <div style="font-size:13px;letter-spacing:2px;text-transform:uppercase;color:#cae4ff;font-weight:700;">` + from + `</div>
            <h1 style="margin:10px 0 0 0;font-size:30px;line-height:1.2;color:#ffffff;font-weight:800;">Thanks for chatting!</h1>
            <p style="margin:12px 0 0 0;color:#d7ecff;font-size:15px;line-height:1.6;">We appreciate you reaching out.</p>
          </td></tr>
          <tr><td style="padding:28px 30px 8px 30px;">
            <p style="margin:0;font-size:16px;line-height:1.7;color:#324a62;">` + greeting + `,</p>
            <p style="margin:14px 0 0 0;font-size:15px;line-height:1.8;color:#4d657e;">
              Thank you for reaching out and chatting with us today. We hope we were able to help answer your questions.
            </p>
            <p style="margin:14px 0 0 0;font-size:15px;line-height:1.8;color:#4d657e;">
              If you have any further questions or need additional assistance, please don't hesitate to get in touch.
            </p>
            <table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0" style="margin:20px 0;background:#f8fbff;border:1px solid #d7e8f8;border-radius:14px;">
              <tr><td style="padding:16px 18px;">
                <p style="margin:0;font-size:14px;line-height:1.7;color:#4d657e;">
                  This email was sent because you recently chatted with <strong>` + from + `</strong>. If you didn't initiate this conversation, you can safely ignore this email.
                </p>
              </td></tr>
            </table>
          </td></tr>
          <tr><td style="padding:18px 30px 30px 30px;">
            <table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0" style="border-top:1px solid #e4edf6;padding-top:14px;">
              <tr><td style="font-size:12px;line-height:1.7;color:#7b91a8;">
                Powered by <a href="https://witzo.ai" style="color:#1d4ed8;text-decoration:none;">Witzo AI</a>
              </td></tr>
            </table>
          </td></tr>
        </table>
      </td></tr>
    </table>
  </body>
</html>`
}

func buildFollowUpEmailText(visitorName, from string) string {
	greeting := "Hi there"
	if strings.TrimSpace(visitorName) != "" {
		greeting = "Hi " + visitorName
	}
	return "Thanks for chatting with " + from + "!\n\n" +
		greeting + ",\n\n" +
		"Thank you for reaching out and chatting with us today. We hope we were able to help answer your questions.\n\n" +
		"If you have any further questions or need additional assistance, please don't hesitate to get in touch.\n\n" +
		"— " + from + "\n\nPowered by Witzo AI"
}

func displayNameFromEmail(email string) string {
	parts := strings.SplitN(email, "@", 2)
	local := parts[0]
	if local == "" {
		return "there"
	}
	words := strings.FieldsFunc(local, func(r rune) bool {
		return r == '.' || r == '_' || r == '-'
	})
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + strings.ToLower(w[1:])
		}
	}
	result := strings.Join(words, " ")
	if result == "" {
		return "there"
	}
	return result
}

// ── OpenAI ─────────────────────────────────────────────────────────────────────

func (c *Controller) openAIChat(message string) (string, error) {
	if strings.TrimSpace(c.cfg.OpenAIAPIKey) == "" {
		return "", nil
	}
	payload := map[string]interface{}{
		"model": c.cfg.OpenAIModel,
		"messages": []map[string]string{
			{"role": "system", "content": "You are Witzo AI assistant."},
			{"role": "user", "content": message},
		},
	}
	b, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, "https://api.openai.com/v1/chat/completions", bytes.NewReader(b))
	if err != nil {
		c.logger.Error("openai request build failed", "error", err)
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.OpenAIAPIKey)
	req.Header.Set("Content-Type", "application/json")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req = req.WithContext(ctx)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.logger.Warn("openai request failed", "error", err)
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		err := fmt.Errorf("openai status %d", resp.StatusCode)
		c.logger.Warn("openai returned non-success status", "status_code", resp.StatusCode, "response", strings.TrimSpace(string(body)))
		return "", err
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		c.logger.Warn("openai decode failed", "error", err)
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", nil
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}

func (c *Controller) openAIAnswerWithContext(query string, matches []map[string]interface{}) (string, error) {
	if strings.TrimSpace(c.cfg.OpenAIAPIKey) == "" {
		return "", nil
	}
	contextParts := make([]string, 0, len(matches))
	for _, m := range matches {
		meta, ok := m["metadata"].(map[string]interface{})
		if !ok {
			continue
		}
		text, _ := meta["text"].(string)
		sourceURL, _ := meta["url"].(string)
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		if len(text) > 1000 {
			text = text[:1000]
		}
		if strings.TrimSpace(sourceURL) != "" {
			contextParts = append(contextParts, fmt.Sprintf("Source: %s\n%s", sourceURL, text))
		} else {
			contextParts = append(contextParts, text)
		}
	}
	contextBlock := "No indexed context was retrieved."
	if len(contextParts) > 0 {
		contextBlock = strings.Join(contextParts, "\n\n---\n\n")
	}
	prompt := fmt.Sprintf(`You are a helpful assistant for this business.
Use only the context below to answer the question.
If the answer is not in the context, reply exactly:
"I don't have information about that. Please contact support."

Context:
%s

User Question: %s`, contextBlock, query)
	return c.openAIChat(prompt)
}

func (c *Controller) openAIEmbedding(input string, dimensions int) ([]float64, error) {
	if strings.TrimSpace(c.cfg.OpenAIAPIKey) == "" {
		return nil, nil
	}
	payload := map[string]interface{}{"model": "text-embedding-3-small", "input": input}
	if dimensions > 0 {
		payload["dimensions"] = dimensions
	}
	b, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, "https://api.openai.com/v1/embeddings", bytes.NewReader(b))
	if err != nil {
		c.logger.Error("openai embedding request build failed", "error", err)
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.OpenAIAPIKey)
	req.Header.Set("Content-Type", "application/json")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req = req.WithContext(ctx)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.logger.Warn("openai embedding request failed", "error", err)
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		err := fmt.Errorf("embedding status %d", resp.StatusCode)
		c.logger.Warn("openai embedding returned non-success status", "status_code", resp.StatusCode, "response", strings.TrimSpace(string(body)))
		return nil, err
	}
	var out struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		c.logger.Warn("openai embedding decode failed", "error", err)
		return nil, err
	}
	if len(out.Data) == 0 {
		return nil, nil
	}
	return out.Data[0].Embedding, nil
}

// ── Pinecone ───────────────────────────────────────────────────────────────────

type pineconeIndexInfo struct {
	Host      string
	Dimension int
}

func (c *Controller) pineconeIndexInfo() (pineconeIndexInfo, error) {
	info := pineconeIndexInfo{
		Host:      normalizePineconeHost(c.cfg.PineconeHost),
		Dimension: c.cfg.PineconeDimension,
	}

	if strings.TrimSpace(c.cfg.PineconeIndexName) == "" || strings.TrimSpace(c.cfg.PineconeAPIKey) == "" {
		if info.Host == "" {
			return info, errors.New("missing pinecone index configuration")
		}
		return info, nil
	}

	resolved, err := c.resolvePineconeIndexInfoFromControlPlane(c.cfg.PineconeIndexName)
	if err != nil {
		if info.Host != "" {
			c.logger.Warn("pinecone index metadata resolve failed; using explicit host override",
				"index", c.cfg.PineconeIndexName,
				"error", err,
			)
			return info, nil
		}
		return info, err
	}

	if info.Host == "" {
		info.Host = resolved.Host
	}
	if info.Dimension <= 0 {
		info.Dimension = resolved.Dimension
	}

	return info, nil
}

func normalizePineconeHost(raw string) string {
	host := strings.TrimSpace(raw)
	if host == "" {
		return ""
	}
	if strings.HasPrefix(host, "https://") || strings.HasPrefix(host, "http://") {
		return host
	}
	return "https://" + host
}

func (c *Controller) resolvePineconeIndexInfoFromControlPlane(indexName string) (pineconeIndexInfo, error) {
	var info pineconeIndexInfo
	endpoint := "https://api.pinecone.io/indexes/" + url.PathEscape(strings.TrimSpace(indexName))
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return info, err
	}
	req.Header.Set("Api-Key", c.cfg.PineconeAPIKey)
	req.Header.Set("Accept", "application/json")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return info, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return info, fmt.Errorf("describe index status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var out struct {
		Host      string `json:"host"`
		Dimension int    `json:"dimension"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return info, err
	}
	host := normalizePineconeHost(out.Host)
	if host == "" {
		return info, errors.New("empty host from pinecone control api")
	}
	info.Host = host
	info.Dimension = out.Dimension
	return info, nil
}

type ragChunk struct {
	URL        string
	PageTitle  string
	Text       string
	ChunkIndex int
	WidgetKey  string
}

func pineconeNamespace(userID string) string {
	return "user_" + strings.ReplaceAll(strings.TrimSpace(userID), "-", "")
}

func (c *Controller) userWidgetKey(userID string) string {
	var widgetKey string
	if err := c.db.QueryRow(`SELECT widget_key FROM widget_keys WHERE user_id=$1`, userID).Scan(&widgetKey); err != nil {
		return ""
	}
	return strings.TrimSpace(widgetKey)
}

func (c *Controller) pineconeUpsert(userID, sourceURL, content string) error {
	chunks := []ragChunk{{
		URL:        sourceURL,
		PageTitle:  "",
		Text:       content,
		ChunkIndex: 0,
		WidgetKey:  c.userWidgetKey(userID),
	}}
	return c.pineconeUpsertChunks(userID, chunks)
}

func (c *Controller) pineconeUpsertChunks(userID string, chunks []ragChunk) error {
	if strings.TrimSpace(c.cfg.PineconeAPIKey) == "" {
		return nil
	}
	indexInfo, err := c.pineconeIndexInfo()
	if err != nil {
		return err
	}
	if indexInfo.Host == "" {
		return errors.New("pinecone host could not be resolved; set PINECONE_HOST or verify PINECONE_INDEX_NAME")
	}

	namespace := pineconeNamespace(userID)
	vectors := make([]map[string]interface{}, 0, len(chunks))
	for _, chunk := range chunks {
		text := strings.TrimSpace(chunk.Text)
		if text == "" {
			continue
		}
		emb, err := c.openAIEmbedding(text, indexInfo.Dimension)
		if err != nil || len(emb) == 0 {
			if err != nil {
				c.logger.Warn("pinecone upsert embedding failed", "user_id", userID, "source_url", chunk.URL, "error", err)
			}
			continue
		}

		vectors = append(vectors, map[string]interface{}{
			"id":     utils.RandomID("pc"),
			"values": emb,
			"metadata": map[string]interface{}{
				"user_id":     userID,
				"url":         chunk.URL,
				"pageTitle":   strings.TrimSpace(chunk.PageTitle),
				"chunkIndex":  chunk.ChunkIndex,
				"text":        text,
				"widgetKey":   strings.TrimSpace(chunk.WidgetKey),
				"namespaceId": namespace,
			},
		})
	}

	if len(vectors) == 0 {
		return nil
	}

	payload := map[string]interface{}{
		"namespace": namespace,
		"vectors":   vectors,
	}
	b, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, indexInfo.Host+"/vectors/upsert", bytes.NewReader(b))
	if err != nil {
		c.logger.Error("pinecone upsert request build failed", "error", err)
		return err
	}
	req.Header.Set("Api-Key", c.cfg.PineconeAPIKey)
	req.Header.Set("Content-Type", "application/json")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req = req.WithContext(ctx)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.logger.Warn("pinecone upsert request failed", "user_id", userID, "namespace", namespace, "error", err)
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		err := fmt.Errorf("pinecone upsert status %d: %s", resp.StatusCode, string(body))
		c.logger.Warn("pinecone upsert returned non-success status", "status_code", resp.StatusCode, "user_id", userID, "namespace", namespace)
		return err
	}
	return nil
}

func (c *Controller) pineconeDeleteNamespace(userID string) error {
	if strings.TrimSpace(c.cfg.PineconeAPIKey) == "" {
		return nil
	}
	indexInfo, err := c.pineconeIndexInfo()
	if err != nil {
		return err
	}
	if indexInfo.Host == "" {
		return nil
	}
	namespace := pineconeNamespace(userID)
	payload := map[string]interface{}{
		"namespace": namespace,
		"deleteAll": true,
	}
	b, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, indexInfo.Host+"/vectors/delete", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Api-Key", c.cfg.PineconeAPIKey)
	req.Header.Set("Content-Type", "application/json")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		trimmed := strings.TrimSpace(string(body))
		if resp.StatusCode == http.StatusNotFound {
			lower := strings.ToLower(trimmed)
			if strings.Contains(lower, "namespace not found") || strings.Contains(lower, `"code":5`) {
				return nil
			}
		}
		return fmt.Errorf("pinecone namespace delete status %d: %s", resp.StatusCode, trimmed)
	}
	return nil
}

func (c *Controller) pineconeQuery(userID, query string, topK int) ([]map[string]interface{}, error) {
	if strings.TrimSpace(c.cfg.PineconeAPIKey) == "" {
		return nil, nil
	}
	indexInfo, err := c.pineconeIndexInfo()
	if err != nil {
		return nil, err
	}
	if indexInfo.Host == "" {
		return nil, nil
	}
	emb, err := c.openAIEmbedding(query, indexInfo.Dimension)
	if err != nil || len(emb) == 0 {
		if err != nil {
			c.logger.Warn("pinecone query embedding failed", "user_id", userID, "error", err)
		}
		return nil, err
	}
	if topK <= 0 {
		topK = 5
	}
	namespace := pineconeNamespace(userID)
	payload := map[string]interface{}{
		"namespace":       namespace,
		"vector":          emb,
		"topK":            topK,
		"includeMetadata": true,
	}
	b, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, indexInfo.Host+"/query", bytes.NewReader(b))
	if err != nil {
		c.logger.Error("pinecone query request build failed", "error", err)
		return nil, err
	}
	req.Header.Set("Api-Key", c.cfg.PineconeAPIKey)
	req.Header.Set("Content-Type", "application/json")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req = req.WithContext(ctx)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.logger.Warn("pinecone query request failed", "user_id", userID, "error", err)
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		err := fmt.Errorf("pinecone query status %d", resp.StatusCode)
		c.logger.Warn("pinecone query returned non-success status", "status_code", resp.StatusCode, "user_id", userID)
		return nil, err
	}
	var out struct {
		Matches []map[string]interface{} `json:"matches"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		c.logger.Warn("pinecone query decode failed", "user_id", userID, "error", err)
		return nil, err
	}
	return out.Matches, nil
}

// ── URL scraper ────────────────────────────────────────────────────────────────

func (c *Controller) extractTextFromURL(source string) (string, error) {
	u, err := url.Parse(source)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("invalid url")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	req.Header.Set("User-Agent", "WitzoGoBot/1.0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.logger.Warn("source scrape request failed", "url", source, "error", err)
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		c.logger.Warn("source scrape returned non-success status", "url", source, "status_code", resp.StatusCode)
		return "", fmt.Errorf("scrape status %d", resp.StatusCode)
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	text := strings.ReplaceAll(string(data), "\n", " ")
	text = strings.ReplaceAll(text, "\r", " ")
	text = strings.Join(strings.Fields(text), " ")
	if len(text) > 5000 {
		text = text[:5000]
	}
	return text, nil
}

// ── Webhooks ───────────────────────────────────────────────────────────────────

func (c *Controller) queueWebhookEvent(userID, leadID, eventType string, payload map[string]interface{}) {
	b, _ := json.Marshal(payload)
	if _, err := c.db.Exec(`INSERT INTO lead_webhook_events (user_id,lead_id,config_id,event_type,payload,status,max_attempts)
		SELECT $1,$2,c.id,$3,$4::jsonb,'pending',8 FROM lead_webhook_configs c WHERE c.user_id=$1 AND c.is_active=TRUE`,
		userID, utils.Nullable(leadID), eventType, string(b)); err != nil {
		c.logger.Warn("queue webhook event insert failed", "user_id", userID, "lead_id", leadID, "event_type", eventType, "error", err)
	}
}

func webhookSignature(secret, timestamp, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "." + body))
	return hex.EncodeToString(mac.Sum(nil))
}

func (c *Controller) processPendingWebhookEvents(ctx context.Context) {
	rows, err := c.db.QueryContext(ctx, `SELECT e.id,e.user_id,e.config_id,e.event_type,e.payload::text,e.attempts,e.max_attempts,c.webhook_url,c.signing_secret
		FROM lead_webhook_events e JOIN lead_webhook_configs c ON c.id=e.config_id
		WHERE e.status IN ('pending','retrying') AND e.next_attempt_at <= CURRENT_TIMESTAMP AND c.is_active=TRUE
		ORDER BY e.created_at ASC LIMIT 50`)
	if err != nil {
		c.logger.Error("failed to fetch pending webhook events", "error", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id, userID, configID, eventType, payloadText, webhookURL, secret string
		var attempts, maxAttempts int
		if err := rows.Scan(&id, &userID, &configID, &eventType, &payloadText, &attempts, &maxAttempts, &webhookURL, &secret); err != nil {
			c.logger.Error("failed to scan webhook event row", "error", err)
			continue
		}
		nextAttempts := attempts + 1
		if _, err := c.db.ExecContext(ctx, `UPDATE lead_webhook_events SET status='processing',attempts=attempts+1,last_attempt_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, id); err != nil {
			c.logger.Warn("failed to mark webhook event as processing", "event_id", id, "error", err)
		}
		timestamp := fmt.Sprintf("%d", time.Now().Unix())
		eventBody := fmt.Sprintf(`{"id":"%s","type":"%s","occurredAt":"%s","payload":%s}`,
			id, eventType, time.Now().UTC().Format(time.RFC3339), payloadText)
		sig := webhookSignature(secret, timestamp, eventBody)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, strings.NewReader(eventBody))
		if err != nil {
			c.logger.Warn("failed to build webhook request", "event_id", id, "webhook_url", webhookURL, "error", err)
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Witzo-Event-Id", id)
		req.Header.Set("X-Witzo-Event-Type", eventType)
		req.Header.Set("X-Witzo-Timestamp", timestamp)
		req.Header.Set("X-Witzo-Signature", "sha256="+sig)
		resp, err := http.DefaultClient.Do(req)
		if err == nil && resp != nil {
			_ = resp.Body.Close()
		}
		if err == nil && resp != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			if _, err := c.db.ExecContext(ctx, `UPDATE lead_webhook_events SET status='delivered',delivered_at=CURRENT_TIMESTAMP,response_status=$2,last_error=NULL,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, id, resp.StatusCode); err != nil {
				c.logger.Warn("failed to mark webhook event delivered", "event_id", id, "error", err)
			}
			continue
		}
		status := "retrying"
		if nextAttempts >= maxAttempts {
			status = "dead"
		}
		errorMsg := "delivery failed"
		if err != nil {
			errorMsg = err.Error()
		}
		var responseStatus interface{} = nil
		if resp != nil {
			responseStatus = resp.StatusCode
			errorMsg = fmt.Sprintf("webhook status %d", resp.StatusCode)
		}
		delayMs := int(math.Min(600000, float64(5000*int(math.Pow(2, float64(nextAttempts-1))))))
		if _, err := c.db.ExecContext(ctx, `UPDATE lead_webhook_events SET status=$2,last_error=$3,response_status=$4,next_attempt_at=CASE WHEN $2='dead' THEN next_attempt_at ELSE CURRENT_TIMESTAMP + ($5 || ' milliseconds')::interval END,updated_at=CURRENT_TIMESTAMP WHERE id=$1`,
			id, status, errorMsg, responseStatus, delayMs); err != nil {
			c.logger.Warn("failed to update webhook event retry state", "event_id", id, "error", err)
		}
		c.logger.Warn("webhook delivery attempt failed",
			"event_id", id,
			"user_id", userID,
			"config_id", configID,
			"status", status,
			"attempt", nextAttempts,
			"max_attempts", maxAttempts,
			"error", errorMsg,
			"response_status", responseStatus,
		)
	}
}

// ── Analytics flush ────────────────────────────────────────────────────────────

func (c *Controller) flushWidgetAnalytics(ctx context.Context) {
	if c.redis == nil {
		return
	}
	for i := 0; i < 200; i++ {
		v, err := c.redis.RPop(ctx, "widget:analytics:buffer").Result()
		if err != nil {
			if !errors.Is(err, redis.Nil) {
				c.logger.Warn("failed to pop analytics event from redis", "error", err)
			}
			break
		}
		var evt struct {
			WidgetKeyID int64                  `json:"widget_key_id"`
			EventType   string                 `json:"event_type"`
			EventData   map[string]interface{} `json:"event_data"`
			IP          string                 `json:"ip_address"`
			UA          string                 `json:"user_agent"`
			Referer     string                 `json:"referer_url"`
		}
		if err := json.Unmarshal([]byte(v), &evt); err != nil || evt.WidgetKeyID == 0 {
			if err != nil {
				c.logger.Warn("failed to decode buffered analytics event", "error", err)
			}
			continue
		}
		data, _ := json.Marshal(evt.EventData)
		if _, err := c.db.ExecContext(ctx, `INSERT INTO widget_analytics (widget_key_id,event_type,event_data,ip_address,user_agent,referer_url) VALUES ($1,$2,$3::jsonb,$4,$5,$6)`,
			evt.WidgetKeyID, evt.EventType, string(data),
			utils.Nullable(evt.IP), utils.Nullable(evt.UA), utils.Nullable(evt.Referer)); err != nil {
			c.logger.Warn("failed to persist analytics event", "widget_key_id", evt.WidgetKeyID, "event_type", evt.EventType, "error", err)
		}
	}
}

// ── Background workers ─────────────────────────────────────────────────────────

func (c *Controller) StartBackgroundWorkers(ctx context.Context) {
	if c.cfg.AnalyticsFlushIntervalSec <= 0 {
		c.cfg.AnalyticsFlushIntervalSec = 60
	}
	if c.cfg.WebhookProcessIntervalSec <= 0 {
		c.cfg.WebhookProcessIntervalSec = 30
	}
	analyticsTicker := time.NewTicker(time.Duration(c.cfg.AnalyticsFlushIntervalSec) * time.Second)
	webhookTicker := time.NewTicker(time.Duration(c.cfg.WebhookProcessIntervalSec) * time.Second)
	maintenanceTicker := time.NewTicker(24 * time.Hour)
	c.logger.Info("background workers started",
		"analytics_interval_sec", c.cfg.AnalyticsFlushIntervalSec,
		"webhook_interval_sec", c.cfg.WebhookProcessIntervalSec,
		"maintenance_interval_hours", 24,
	)

	go func() {
		defer analyticsTicker.Stop()
		defer webhookTicker.Stop()
		defer maintenanceTicker.Stop()
		defer c.logger.Info("background workers stopped")
		for {
			select {
			case <-ctx.Done():
				return
			case <-analyticsTicker.C:
				c.flushWidgetAnalytics(context.Background())
			case <-webhookTicker.C:
				c.processPendingWebhookEvents(context.Background())
			case <-maintenanceTicker.C:
				if _, err := c.db.Exec(`UPDATE sessions SET is_revoked=TRUE WHERE (refresh_token_expires_at < CURRENT_TIMESTAMP OR refresh_token_expires_at IS NULL) AND is_revoked=FALSE`); err != nil {
					c.logger.Warn("maintenance task failed: revoke expired sessions", "error", err)
				}
				if _, err := c.db.Exec(`DELETE FROM verification_codes WHERE expires_at < CURRENT_TIMESTAMP - INTERVAL '1 day'`); err != nil {
					c.logger.Warn("maintenance task failed: cleanup verification codes", "error", err)
				}
				if _, err := c.db.Exec(`DELETE FROM widget_analytics WHERE created_at < CURRENT_TIMESTAMP - INTERVAL '90 days'`); err != nil {
					c.logger.Warn("maintenance task failed: cleanup widget analytics", "error", err)
				}
				if _, err := c.db.Exec(`DELETE FROM sessions WHERE is_revoked=TRUE AND updated_at < CURRENT_TIMESTAMP - INTERVAL '30 days'`); err != nil {
					c.logger.Warn("maintenance task failed: cleanup revoked sessions", "error", err)
				}
			}
		}
	}()
}
