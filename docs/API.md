# API Nodra — v1

Base : `/api/v1`. Réponses JSON.

## Santé

`GET /api/v1/health` → `200 {"status":"ok"}` ou `503 {"status":"degraded","database":"down"}`.
