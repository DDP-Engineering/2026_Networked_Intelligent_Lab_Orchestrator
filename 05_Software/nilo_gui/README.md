# NILO browser GUI

This project is the native Windows development frontend for NILO. It builds to
static assets for later serving by Bench Core; Node.js is a development and
build dependency, not an appliance runtime dependency.

The current scaffold proves the frontend toolchain and persistent simulation
identification. It does not authenticate, call the backend, or represent live
integration or device state yet.

From the repository root, restore the locked dependency graph:

```powershell
.\02_Tools\bootstrap\bootstrap-frontend.ps1
```

Then run commands through the repository-local tool wrapper:

```powershell
.\02_Tools\env\with-tools.ps1 npm.cmd --prefix .\05_Software\nilo_gui run dev
.\02_Tools\env\with-tools.ps1 npm.cmd --prefix .\05_Software\nilo_gui run format:check
.\02_Tools\env\with-tools.ps1 npm.cmd --prefix .\05_Software\nilo_gui run lint
.\02_Tools\env\with-tools.ps1 npm.cmd --prefix .\05_Software\nilo_gui run test:ci
.\02_Tools\env\with-tools.ps1 npm.cmd --prefix .\05_Software\nilo_gui run build
```

The development server binds to `127.0.0.1` and uses Vite's default port 5173
unless an explicit `-- --port <port>` argument is supplied. Generated
`node_modules/`, `coverage/`, and `dist/` directories are ignored and must not
be committed.
