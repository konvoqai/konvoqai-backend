package middleware

import "net/http"

func WithAuth[C any, U any](
	authenticate func(*http.Request) (C, U, error),
	requireCSRF func(*http.Request) error,
	onError func(http.ResponseWriter, int, string),
	next func(http.ResponseWriter, *http.Request, C, U),
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, user, err := authenticate(r)
		if err != nil {
			onError(w, http.StatusUnauthorized, err.Error())
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if err := requireCSRF(r); err != nil {
				onError(w, http.StatusForbidden, err.Error())
				return
			}
		}
		next(w, r, claims, user)
	}
}

func WithAdmin(
	validate func(*http.Request) error,
	onError func(http.ResponseWriter, int, string),
	next http.HandlerFunc,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := validate(r); err != nil {
			onError(w, http.StatusUnauthorized, err.Error())
			return
		}
		next(w, r)
	}
}
