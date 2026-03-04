package controller

import (
	"net/http"
	"time"
)

func (c *Controller) cookieSameSite() http.SameSite {
	return http.SameSiteLaxMode
}

func (c *Controller) setAuthCookies(w http.ResponseWriter, accessToken string, accessExp time.Time, refreshToken string, refreshExp time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     "konvoq_access_token",
		Value:    accessToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   c.cfg.IsProduction,
		SameSite: c.cookieSameSite(),
		Expires:  accessExp,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "witzo_refresh_token",
		Value:    refreshToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   c.cfg.IsProduction,
		SameSite: c.cookieSameSite(),
		Expires:  refreshExp,
	})
}

func (c *Controller) clearAuthCookies(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "konvoq_access_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   c.cfg.IsProduction,
		SameSite: c.cookieSameSite(),
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "witzo_refresh_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   c.cfg.IsProduction,
		SameSite: c.cookieSameSite(),
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
}

func (c *Controller) setCSRFCookie(w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     "csrf_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   c.cfg.IsProduction,
		SameSite: c.cookieSameSite(),
		Expires:  expires,
	})
}
