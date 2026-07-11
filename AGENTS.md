# AI Agent Instructions

Before working on any code in this repository, you MUST:

1. **Read the project context**: `openspec/config.yaml` — contains tech stack, architecture, conventions, and domain info.

2. **Read the relevant spec**: Before modifying a feature, check `openspec/specs/<feature-name>/spec.md` for requirements and edge cases. There are 27 specs covering all major features.

3. **Check the change history**: Before fixing a bug, check `openspec/changes/archive/` for prior fixes. Many edge cases have been documented there (e.g., `TestConnection` endpoints-map bug, Gemini 2.5 Pro empty response, logging overhaul).

4. **Archive every change before pushing**: After making code changes and committing, create a dated archive entry under `openspec/changes/archive/YYYY-MM-DD-<slug>/` with `.openspec.yaml`, `proposal.md`, and `tasks.md`. Follow the same format as existing entries. Commit the archive separately before `git push`. Do NOT skip this step even if the user doesn't ask.

5. **Do NOT skip these steps** even if the user doesn't explicitly ask. They are critical for understanding the codebase and avoiding regressions.

## Key Architecture Facts (from openspec/config.yaml)

- **Backend**: Go 1.24+, Gin, GORM, SQLite/PostgreSQL
- **Frontend**: Vue 3, TypeScript, Vite, Element Plus
- **Pattern**: Hub-and-spoke protocol conversion (UnifiedRequest/Response)
- **Protocol plugins**: openai, anthropic, gemini, deepseek, openrouter
- **MCP proxy**: JSON-RPC 2.0 over HTTP/SSE + stdio
- **Auth**: API key (sk-/sk-ant-/AIza prefix) + session-based admin
- **Config**: config.yaml + AG_ prefixed env var overrides

## Quick Reference: Common Pitfalls

- Provider endpoints are stored in `endpoints` JSON column, NOT flat columns (`openai_base_url`, etc.)
- `TestConnection` MUST read from `req.Endpoints` map, not just flat fields
- `findStoredAPIKey()` MUST check `endpoints` JSON before flat column fallback
- Debug page `RunTest` uses maxTokens=1024; provider page uses maxTokens=5
- `/api/v1/debug/server-logs` is DEBUG level — not visible in default log level
- Success HTTP requests use `resp_bytes=%d` not `resp=%q` (response body not logged)
- Gemini 2.5 Pro is a thinking model; maxOutputTokens must be large enough for thoughts+output
