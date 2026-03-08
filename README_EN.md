# Codex Quota Overview

[简体中文](./README.md)

A local Windows quota dashboard for bulk-importing auth `JSON` files, querying live Codex quota, and visualizing total quota, remaining quota, loss, account lists, and window-level details.

## Highlights

- Import auth folders directly from the browser
- Pick multiple folders progressively, then import and scan them in one batch
- Recursively scan large `JSON` datasets
- Auto-calculate recommended concurrency from CPU threads
- Server-side pagination for large result sets
- Persist scan snapshots so page refresh does not auto-trigger a new scan
- Export CSV, clear stats, and clear imported directories

## Screenshots

> The screenshots below are sanitized.

### 1. Light theme overview

![Light theme overview](./docs/images/demo-01-light.png)

### 2. Dark theme overview

![Dark theme overview](./docs/images/demo-02-dark.png)

## Requirements

- Windows 10 / Windows 11
- Go 1.25+
- Node.js 18+
- npm

> If you are a regular end user, download the packaged runtime build from GitHub Releases instead of cloning the source repository.

## Quick Start

### Option 1: Double-click scripts

1. Run `一键安装环境.bat`
2. Run `一键启动服务.bat`
3. Open `http://127.0.0.1:8787`
4. Pick folders in the UI and click scan

### Option 2: Development mode

Backend:

```powershell
cd backend
go run .\cmd\server -open-browser=false
```

Frontend:

```powershell
cd web
npm install
npm run dev
```

## Project Layout

```text
.
├─ backend/
│  ├─ cmd/server/
│  └─ internal/app/
├─ web/
│  ├─ src/
│  └─ public/
├─ docs/images/
├─ 一键安装环境.bat
├─ 一键启动服务.bat
├─ 一键停止服务.bat
├─ 操作说明.txt
└─ AI接手指南.md
```

## Main Endpoints

- `GET /api/health`
- `GET /api/meta`
- `POST /api/import-folder`
- `POST /api/scan-job`
- `POST /api/refresh-job`
- `GET /api/job?id=...`
- `GET /api/accounts?resultId=...`
- `POST /api/clear-imported-files`
- `POST /api/clear-stats`
- `GET /api/export.csv`

## Validation

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

## Open Source Notes

- This repository does not include any real credentials, imported pools, scan results, or runtime logs
- Do not commit real auth files or runtime output directories
- Packaged runtime builds should be published through GitHub Releases

