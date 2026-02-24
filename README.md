# konvoq-backend

Go backend rewrite of `konvoq-ai-automation-chatbot` with PostgreSQL + Redis persistence.

## Run Server

```powershell
cd d:\vivek-aa\konvoqai-backend
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
go run main.go
```

Default server: `http://localhost:8080`

## Run Migrations

```powershell
cd d:\vivek-aa\konvoqai-backend
go run ./migrate
```

`go run ./migrate` auto-loads `.env` from project root, so no `$env:` command is needed for normal local usage.

## Notes

- Runs DB migrations from `migrations/sql` with `go run ./migrate`.
- Optional startup migration: set `AUTO_MIGRATE=true` to run migrations on server start.
- Route groups implemented: `/api/auth`, `/api/admin`, `/api/v1`, plus health routes.
- CSRF flow is implemented using Redis (`/api/auth/csrf-token` + `X-CSRF-Token` header).
- Auth/session, usage, chat storage, widget, leads, feedback, and admin endpoints use PostgreSQL.
- Default admin credentials come from env:
  - `ADMIN_EMAIL` (default `admin@konvoq.local`)
  - `ADMIN_PASSWORD` (default `change-me-admin-password`)

