# golan-project

Go backend rewrite of `witzo-ai-automation-chatbot` with PostgreSQL + Redis persistence.

## Run

```powershell
cd golan-project
$env:ALL_PROXY=''
$env:HTTP_PROXY=''
$env:HTTPS_PROXY=''
$env:GIT_HTTP_PROXY=''
$env:GIT_HTTPS_PROXY=''
$env:DB_HOST='localhost'
$env:DB_PORT='5432'
$env:DB_NAME='auth_db'
$env:DB_USER='postgres'
$env:DB_PASSWORD='postgres'
$env:REDIS_HOST='localhost'
$env:REDIS_PORT='6379'
$env:JWT_SECRET='dev-jwt-secret'
$env:JWT_REFRESH_SECRET='dev-refresh-secret'
$env:ADMIN_JWT_SECRET='dev-admin-secret'
$env:COOKIE_SECRET='dev-cookie-secret'
go run ./cmd/server
```

Default server: `http://localhost:8080`

## Notes

- Runs DB migrations from `internal/migrations/sql` on startup (`AUTO_MIGRATE=true` by default).
- Route groups implemented: `/api/auth`, `/api/admin`, `/api/v1`, plus health routes.
- CSRF flow is implemented using Redis (`/api/auth/csrf-token` + `X-CSRF-Token` header).
- Auth/session, usage, chat storage, widget, leads, feedback, and admin endpoints use PostgreSQL.
- Default admin credentials come from env:
  - `ADMIN_EMAIL` (default `admin@witzo.local`)
  - `ADMIN_PASSWORD` (default `change-me-admin-password`)
