# NILO browser GUI

This project is the native Windows development frontend for NILO. It builds to
static assets for later serving by Bench Core; Node.js is a development and
build dependency, not an appliance runtime dependency.

The interface authenticates against the versioned Bench Core HTTP API with a
development bearer token and displays the reported backend status. It does not
yet render the integration or device inventory.

From the repository root, restore the locked dependency graph:

```powershell
.\02_Tools\bootstrap\bootstrap-frontend.ps1
```

Then run commands through the repository-local tool wrapper:

```powershell
.\02_Tools\env\with-tools.ps1 npm.cmd --prefix .\05_Software\nilo_gui run dev
.\02_Tools\env\with-tools.ps1 npm.cmd --prefix .\05_Software\nilo_gui run dev:mock
.\02_Tools\env\with-tools.ps1 npm.cmd --prefix .\05_Software\nilo_gui run format:check
.\02_Tools\env\with-tools.ps1 npm.cmd --prefix .\05_Software\nilo_gui run lint
.\02_Tools\env\with-tools.ps1 npm.cmd --prefix .\05_Software\nilo_gui run test:ci
.\02_Tools\env\with-tools.ps1 npm.cmd --prefix .\05_Software\nilo_gui run build
```

The development server binds to `127.0.0.1` and uses Vite's default port 5173
unless an explicit `-- --port <port>` argument is supplied. Generated
`node_modules/`, `coverage/`, and `dist/` directories are ignored and must not
be committed.

## How the browser reaches Bench Core

The interface only ever requests origin-relative paths such as
`/api/v1/status`. It never addresses Bench Core directly: a bearer credential is
not a CORS-safelisted header, so a cross-origin call would be preflighted, and
Bench Core answers no preflight and exposes no CORS headers.

In development the Vite server forwards `/api` to the configured Bench Core
endpoint, so the page at `127.0.0.1:5173` and the backend at `127.0.0.1:8090`
stay one origin as far as the browser is concerned. On an appliance Bench Core
serves the built assets itself, where those paths are already same-origin.

## Backend configuration

| Variable                  | Values                                                | Default                 |
| ------------------------- | ----------------------------------------------------- | ----------------------- |
| `VITE_NILO_API_MODE`      | `live`, `mock`                                        | `live`                  |
| `VITE_NILO_API_BASE_URL`  | absolute http or https URL                            | `http://127.0.0.1:8090` |
| `VITE_NILO_MOCK_SCENARIO` | `healthy`, `unauthorized`, `unavailable`, `malformed` | `healthy`               |

`VITE_NILO_API_BASE_URL` is the endpoint the development server forwards `/api`
to and the label the login screen shows; it is not fetched from the browser.

`.env` holds the live defaults for a Windows browser talking to Bench Core under
WSL2. `.env.mock` is loaded by `dev:mock` and `build:mock`, which pass
`--mode mock` to Vite. Neither file contains a credential. Override values in an
ignored `.env.local` or `.env.mock.local` rather than editing the committed
files; an unusable value stops startup with an error naming the variable.

## Authenticating

Issue a development token in WSL2 (see
`06_SoftwareDocu/Development_and_Simulation_Flow.md`, section 8.1):

```bash
../../02_Tools/env/with-tools.sh go run ./cmd/nilo development issue-token \
  --data-dir "$HOME/.local/state/nilo-development"
```

Paste the printed `token` into the login field. The token is held in memory for
the lifetime of the page only — never in `localStorage`, `sessionStorage`, or a
cookie — so a reload returns to the login screen. In mock mode any non-empty
token is accepted and nothing is contacted.

## Source layout

- `src/api/contract.ts` — the versioned `nilo.http/1` wire contract and its
  parser, shared by the client and the mock.
- `src/api/client.ts` — the typed Bench Core client, and `BenchCoreError` with
  its `unauthorized`, `unavailable`, and `malformed` causes.
- `src/api/mock.ts` — contract-built mock answers for development without a
  backend.
- `src/config/environment.ts` — resolves the mode and base URL above.
- `src/App.tsx` — the shell that owns the in-memory token and the connection
  state, with `src/components/` for the login and status screens.

Simulation is never evidence that a physical operation occurred; the interface
states this on every screen.
