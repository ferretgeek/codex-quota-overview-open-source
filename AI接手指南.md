# AI Handover Guide

## Project purpose

This project is a local Windows quota dashboard for Codex-style auth JSON files.

Main goals:

- import auth folders from the browser
- scan large JSON sets recursively
- query live quota from the upstream usage endpoint
- render dashboard, account list, and window details
- keep scanning manual-only unless a human explicitly clicks a scan action

## Tech stack

- Backend: Go
- Frontend: React + TypeScript + Vite
- Runtime model: backend serves API and static frontend assets

## Key backend files

- `backend/cmd/server/main.go`
- `backend/internal/app/server.go`
- `backend/internal/app/scanner.go`
- `backend/internal/app/types.go`

## Key frontend files

- `web/src/App.tsx`
- `web/src/api.ts`
- `web/src/components/DashboardOverview.tsx`
- `web/src/components/AccountList.tsx`
- `web/src/components/WindowExplorer.tsx`
- `web/src/components/SettingsPanel.tsx`
- `web/src/hooks/useAccountsPage.ts`

## Important implementation rules

1. Scanning is manual only.
2. Page refresh must not auto-start a scan.
3. Large account results must be paged server-side.
4. Imported files must preserve safe relative structure.
5. Result snapshots are persisted to disk and restored by `resultId`.

## Quota parsing logic

Upstream endpoint:

- `https://chatgpt.com/backend-api/wham/usage`

Current conservative rules:

1. read `rate_limit`
2. read `code_review_rate_limit`
3. read `additional_rate_limits`
4. prefer `remaining_percent`
5. otherwise compute `100 - used_percent`
6. take the minimum remaining percentage across all windows

## Concurrency strategy

Recommended concurrency is based on CPU threads.

Current rule:

- detect `N` CPU threads -> recommend `N * 20` workers
- if CPU detection fails -> default to `20`

Effective concurrency is still clamped by actual task count.

## Suggested reading order

1. `README.md`
2. `操作说明.txt`
3. `backend/cmd/server/main.go`
4. `backend/internal/app/server.go`
5. `backend/internal/app/scanner.go`
6. `web/src/App.tsx`
7. `web/src/api.ts`

## Local verification

Backend:

```powershell
cd backend
go test ./...
go vet ./...
```

Frontend:

```powershell
cd web
npm install
npm run build
```

## Safety note

Do not commit:

- real auth JSON files
- imported folder data
- scan results
- logs
- local runtime PID files

