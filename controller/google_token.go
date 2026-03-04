package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type googleTokenInfo struct {
	Email         string `json:"email"`
	EmailVerified string `json:"email_verified"`
	Name          string `json:"name"`
	Audience      string `json:"aud"`
	Issuer        string `json:"iss"`
	Subject       string `json:"sub"`
	ExpiresIn     string `json:"expires_in"`
}

func (c *Controller) verifyGoogleIDToken(ctx context.Context, idToken string) (googleTokenInfo, error) {
	var info googleTokenInfo
	token := strings.TrimSpace(idToken)
	if token == "" {
		return info, fmt.Errorf("google id token is required")
	}

	endpoint := "https://oauth2.googleapis.com/tokeninfo?id_token=" + url.QueryEscape(token)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return info, err
	}
	clientCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req = req.WithContext(clientCtx)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return info, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return info, fmt.Errorf("google tokeninfo status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return info, err
	}
	if strings.TrimSpace(info.Email) == "" {
		return info, fmt.Errorf("google token has no email")
	}
	if !googleBool(info.EmailVerified) {
		return info, fmt.Errorf("google email is not verified")
	}
	if !googleIssuerOK(info.Issuer) {
		return info, fmt.Errorf("invalid google token issuer")
	}
	clientID := strings.TrimSpace(c.cfg.GoogleClientID)
	if clientID == "" {
		return info, fmt.Errorf("google client id is not configured")
	}
	if strings.TrimSpace(info.Audience) != clientID {
		return info, fmt.Errorf("google token audience mismatch")
	}
	if strings.TrimSpace(info.ExpiresIn) != "" {
		if sec, convErr := strconv.Atoi(strings.TrimSpace(info.ExpiresIn)); convErr == nil && sec <= 0 {
			return info, fmt.Errorf("google token expired")
		}
	}
	return info, nil
}

func googleBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1", "yes":
		return true
	default:
		return false
	}
}

func googleIssuerOK(iss string) bool {
	switch strings.TrimSpace(strings.ToLower(iss)) {
	case "accounts.google.com", "https://accounts.google.com":
		return true
	default:
		return false
	}
}
