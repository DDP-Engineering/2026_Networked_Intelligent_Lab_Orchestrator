# REQ-PLAT-e1a2a591 - Build the locked browser application

## Behavior

The Windows frontend bootstrap runs `npm ci` through the repository-local
Node.js distribution. The browser application is an ordinary locked npm
project; no package is installed globally and no generated dependency or build
directory is committed.

The production build contains static HTML, JavaScript, CSS, and related assets.
It does not start or embed a Node.js application server. Serving these assets
from Bench Core remains later integration work.

## Data / interface

- Project root: `05_Software/frontend`.
- Dependency restore: `npm ci`.
- Development server: `npm run dev`.
- Formatting check: `npm run format:check`.
- Lint: `npm run lint`.
- Unit tests: `npm run test:ci`.
- Production build: `npm run build`.
- Generated directories: `node_modules/`, `dist/`, and `coverage/`, all ignored.

All dependency versions are exact in `package.json` and resolved with integrity
metadata in `package-lock.json`.

## Acceptance

- A clean dependency restore accepts the committed lockfile without rewriting it.
- The production build emits `dist/index.html` and hashed JavaScript and CSS.
- The generated output contains no Node.js server entry point.
