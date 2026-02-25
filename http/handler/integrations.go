package handler

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/smtp"
	"net/url"
	"strings"
	"time"
)

func (c *Handler) sendVerificationEmail(email, code string) {
	if strings.TrimSpace(c.cfg.EmailHost) == "" || strings.TrimSpace(c.cfg.EmailUser) == "" {
		return
	}
	subject := "Your Witzo Verification Code"
	html := buildVerificationEmailHTML(code, c.cfg.VerifyCodeMinutes)
	text := fmt.Sprintf("Witzo AI - Verify Your Email\n\nYour verification code is: %s\nThis code expires in %d minutes.\n\nIf you did not request this code, please ignore this email.", code, c.cfg.VerifyCodeMinutes)
	c.sendEmail(email, subject, html, text)
}

func (c *Handler) sendWelcomeEmail(email string) {
	if strings.TrimSpace(c.cfg.EmailHost) == "" || strings.TrimSpace(c.cfg.EmailUser) == "" {
		return
	}
	subject := "Welcome to Witzo AI"
	html := buildWelcomeEmailHTML(email)
	text := buildWelcomeEmailText(email)
	c.sendEmail(email, subject, html, text)
}

func (c *Handler) sendFollowUpEmail(visitorEmail, visitorName, widgetOwnerName string) {
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

func (c *Handler) sendEmail(to, subject, htmlBody, textBody string) {
	go func() {
		from := c.cfg.EmailFrom
		if from == "" {
			from = c.cfg.EmailUser
		}
		boundary := randomID("boundary")
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
		_ = smtp.SendMail(addr, auth, from, []string{to}, []byte(msg))
	}()
}

// -- Email HTML builders --------------------------------------------------------

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
              If you have any further questions or need additional assistance, please don't hesitate to get in touch — we're always happy to help.
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

// displayNameFromEmail converts an email local-part into a presentable display name.
// e.g. "john.doe@example.com" ? "John Doe"
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

func (c *Handler) openAIChat(message string) (string, error) {
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
	req, _ := http.NewRequest(http.MethodPost, "https://api.openai.com/v1/chat/completions", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+c.cfg.OpenAIAPIKey)
	req.Header.Set("Content-Type", "application/json")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req = req.WithContext(ctx)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("openai status %d", resp.StatusCode)
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", nil
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}

func (c *Handler) openAIAnswerWithContext(query string, matches []map[string]interface{}) (string, error) {
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
		url, _ := meta["url"].(string)
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		if len(text) > 1000 {
			text = text[:1000]
		}
		if strings.TrimSpace(url) != "" {
			contextParts = append(contextParts, fmt.Sprintf("Source: %s\n%s", url, text))
		} else {
			contextParts = append(contextParts, text)
		}
	}
	contextBlock := "No indexed context was retrieved."
	if len(contextParts) > 0 {
		contextBlock = strings.Join(contextParts, "\n\n---\n\n")
	}
	prompt := fmt.Sprintf("Question:\n%s\n\nKnowledge Base Context:\n%s\n\nAnswer using only supported facts from context when possible. If context is insufficient, say so briefly.", query, contextBlock)
	return c.openAIChat(prompt)
}

func (c *Handler) openAIEmbedding(input string) ([]float64, error) {
	if strings.TrimSpace(c.cfg.OpenAIAPIKey) == "" {
		return nil, nil
	}
	payload := map[string]interface{}{"model": "text-embedding-3-small", "input": input}
	b, _ := json.Marshal(payload)
	req, _ := http.NewRequest(http.MethodPost, "https://api.openai.com/v1/embeddings", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+c.cfg.OpenAIAPIKey)
	req.Header.Set("Content-Type", "application/json")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req = req.WithContext(ctx)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("embedding status %d", resp.StatusCode)
	}
	var out struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if len(out.Data) == 0 {
		return nil, nil
	}
	return out.Data[0].Embedding, nil
}

func (c *Handler) pineconeHost() string {
	if strings.TrimSpace(c.cfg.PineconeIndexName) == "" || strings.TrimSpace(c.cfg.PineconeEnvironment) == "" {
		return ""
	}
	return fmt.Sprintf("https://%s-%s.svc.pinecone.io", c.cfg.PineconeIndexName, c.cfg.PineconeEnvironment)
}

func (c *Handler) pineconeUpsert(userID, sourceURL, content string) error {
	if strings.TrimSpace(c.cfg.PineconeAPIKey) == "" {
		return nil
	}
	host := c.pineconeHost()
	if host == "" {
		return nil
	}
	emb, err := c.openAIEmbedding(content)
	if err != nil || len(emb) == 0 {
		return err
	}
	id := randomID("pc")
	payload := map[string]interface{}{
		"vectors": []map[string]interface{}{{
			"id":     id,
			"values": emb,
			"metadata": map[string]interface{}{
				"user_id": userID,
				"url":     sourceURL,
				"text":    content,
			},
		}},
	}
	b, _ := json.Marshal(payload)
	req, _ := http.NewRequest(http.MethodPost, host+"/vectors/upsert", bytes.NewReader(b))
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
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("pinecone upsert status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func (c *Handler) pineconeQuery(userID, query string, topK int) ([]map[string]interface{}, error) {
	if strings.TrimSpace(c.cfg.PineconeAPIKey) == "" {
		return nil, nil
	}
	host := c.pineconeHost()
	if host == "" {
		return nil, nil
	}
	emb, err := c.openAIEmbedding(query)
	if err != nil || len(emb) == 0 {
		return nil, err
	}
	if topK <= 0 {
		topK = 3
	}
	payload := map[string]interface{}{
		"vector":          emb,
		"topK":            topK,
		"includeMetadata": true,
		"filter": map[string]interface{}{
			"user_id": map[string]interface{}{"$eq": userID},
		},
	}
	b, _ := json.Marshal(payload)
	req, _ := http.NewRequest(http.MethodPost, host+"/query", bytes.NewReader(b))
	req.Header.Set("Api-Key", c.cfg.PineconeAPIKey)
	req.Header.Set("Content-Type", "application/json")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req = req.WithContext(ctx)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("pinecone query status %d", resp.StatusCode)
	}
	var out struct {
		Matches []map[string]interface{} `json:"matches"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Matches, nil
}

func (c *Handler) extractTextFromURL(source string) (string, error) {
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
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
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

func (c *Handler) queueWebhookEvent(userID, leadID, eventType string, payload map[string]interface{}) {
	b, _ := json.Marshal(payload)
	_, _ = c.db.Exec(`INSERT INTO lead_webhook_events (user_id,lead_id,config_id,event_type,payload,status,max_attempts)
		SELECT $1,$2,c.id,$3,$4::jsonb,'pending',8 FROM lead_webhook_configs c WHERE c.user_id=$1 AND c.is_active=TRUE`, userID, nullable(leadID), eventType, string(b))
}

func webhookSignature(secret, timestamp, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "." + body))
	return hex.EncodeToString(mac.Sum(nil))
}

func (c *Handler) processPendingWebhookEvents(ctx context.Context) {
	rows, err := c.db.QueryContext(ctx, `SELECT e.id,e.user_id,e.config_id,e.event_type,e.payload::text,e.attempts,e.max_attempts,c.webhook_url,c.signing_secret
		FROM lead_webhook_events e JOIN lead_webhook_configs c ON c.id=e.config_id
		WHERE e.status IN ('pending','retrying') AND e.next_attempt_at <= CURRENT_TIMESTAMP AND c.is_active=TRUE
		ORDER BY e.created_at ASC LIMIT 50`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id, userID, configID, eventType, payloadText, webhookURL, secret string
		var attempts, maxAttempts int
		_ = rows.Scan(&id, &userID, &configID, &eventType, &payloadText, &attempts, &maxAttempts, &webhookURL, &secret)
		nextAttempts := attempts + 1
		_, _ = c.db.ExecContext(ctx, `UPDATE lead_webhook_events SET status='processing',attempts=attempts+1,last_attempt_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, id)
		timestamp := fmt.Sprintf("%d", time.Now().Unix())
		eventBody := fmt.Sprintf(`{"id":"%s","type":"%s","occurredAt":"%s","payload":%s}`,
			id, eventType, time.Now().UTC().Format(time.RFC3339), payloadText)
		sig := webhookSignature(secret, timestamp, eventBody)
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, strings.NewReader(eventBody))
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
			_, _ = c.db.ExecContext(ctx, `UPDATE lead_webhook_events SET status='delivered',delivered_at=CURRENT_TIMESTAMP,response_status=$2,last_error=NULL,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, id, resp.StatusCode)
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
		_, _ = c.db.ExecContext(ctx, `UPDATE lead_webhook_events SET status=$2,last_error=$3,response_status=$4,next_attempt_at=CASE WHEN $2='dead' THEN next_attempt_at ELSE CURRENT_TIMESTAMP + ($5 || ' milliseconds')::interval END,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, id, status, errorMsg, responseStatus, delayMs)
	}
}

func (c *Handler) flushWidgetAnalytics(ctx context.Context) {
	if c.redis == nil {
		return
	}
	for i := 0; i < 200; i++ {
		v, err := c.redis.RPop(ctx, "widget:analytics:buffer").Result()
		if err != nil {
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
		if json.Unmarshal([]byte(v), &evt) != nil || evt.WidgetKeyID == 0 {
			continue
		}
		data, _ := json.Marshal(evt.EventData)
		_, _ = c.db.ExecContext(ctx, `INSERT INTO widget_analytics (widget_key_id,event_type,event_data,ip_address,user_agent,referer_url) VALUES ($1,$2,$3::jsonb,$4,$5,$6)`, evt.WidgetKeyID, evt.EventType, string(data), nullable(evt.IP), nullable(evt.UA), nullable(evt.Referer))
	}
}

func (c *Handler) StartBackgroundWorkers(ctx context.Context) {
	if c.cfg.AnalyticsFlushIntervalSec <= 0 {
		c.cfg.AnalyticsFlushIntervalSec = 60
	}
	if c.cfg.WebhookProcessIntervalSec <= 0 {
		c.cfg.WebhookProcessIntervalSec = 30
	}
	analyticsTicker := time.NewTicker(time.Duration(c.cfg.AnalyticsFlushIntervalSec) * time.Second)
	webhookTicker := time.NewTicker(time.Duration(c.cfg.WebhookProcessIntervalSec) * time.Second)
	maintenanceTicker := time.NewTicker(24 * time.Hour)

	go func() {
		defer analyticsTicker.Stop()
		defer webhookTicker.Stop()
		defer maintenanceTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-analyticsTicker.C:
				c.flushWidgetAnalytics(context.Background())
			case <-webhookTicker.C:
				c.processPendingWebhookEvents(context.Background())
			case <-maintenanceTicker.C:
				_, _ = c.db.Exec(`UPDATE sessions SET is_revoked=TRUE WHERE (refresh_token_expires_at < CURRENT_TIMESTAMP OR refresh_token_expires_at IS NULL) AND is_revoked=FALSE`)
				_, _ = c.db.Exec(`DELETE FROM verification_codes WHERE expires_at < CURRENT_TIMESTAMP - INTERVAL '1 day'`)
				_, _ = c.db.Exec(`DELETE FROM widget_analytics WHERE created_at < CURRENT_TIMESTAMP - INTERVAL '90 days'`)
				_, _ = c.db.Exec(`DELETE FROM sessions WHERE is_revoked=TRUE AND updated_at < CURRENT_TIMESTAMP - INTERVAL '30 days'`)
			}
		}
	}()
}
