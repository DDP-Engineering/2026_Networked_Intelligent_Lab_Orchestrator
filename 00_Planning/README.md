# Planning

Project goals, architecture decisions, milestones, and implementation plans.

- [Agent and MCP connection setup](Agent_MCP_Onboarding_Plan.md): direct client setup
  from a bench companion on a home/corporate LAN or through the network's VPN;
  optional MCP-only HTTP, retained HTTPS management, client qualification and a
  deferred companion fallback. No AEGIS workflow.
- [Repository tools simplification](Tools_Simplification_Plan.md): six public
  commands implemented, duplicate launchers consolidated and local gates passed;
  retains the current stack and offline/acceptance boundaries. No AEGIS workflow.
- [Go to Python backend migration](Go_to_Python_Migration_Plan.md): Python/FastAPI
  Core, CLI and helper with uv and Ruff; staged parity, offline packaging and
  cutover gates. Retains React; Camera and Serial are in scope, Sigrok is excluded.
- [Modbus master/client plugin](Modbus_Plugin_Plan.md): RTU, TCP and RTU over TCP,
  shared connection/workspace definitions and register maps, polling/write controls,
  saved logs/search/exports, and ten implementation work packages for Codex.
- [USB to Serial plugin](USB_to_Serial_Plugin_Plan.md): terminal, session
  approval, bounded RX/TX, saved logs and retention; implementation uses AEGIS,
  with physical Pi acceptance after the user supplies an adapter and requests deployment.
- [Sigrok waveform workspace](Sigrok_Waveform_Workspace/README.md):
  current PulseView feature research, NILO workflow and contracts, and a
  multi-task implementation plan for the native browser waveform interface.
- [Planning overview](Overview.md), [system build plan](System_Build_Plan.md)
  and [remote bench MVP](Remote_Bench_MVP_PRD.md): broader product direction
  and work beyond the implemented foundations.
- [Development and build environment plan](Development_Build_Environment_Plan.md):
  Windows/WSL2 and native Ubuntu OCI build boundary, locked toolchain, Git LFS offline archive,
  dual-remote preservation and acceptance gates that precede further feature
  implementation.
- [Packaging and delivery plan](Packaging_and_Delivery_Plan.md): one package per
  architecture containing NILO and every integration, an offline installer per
  target, a directly writable Raspberry Pi image, and the work packages that
  produce them.
- [Camera plugin plan](Camera_Plugin_Plan.md): the first feature plugin after
  sigrok. USB cameras as named managed devices, snapshots with device
  controls, processing and overlays for humans and agents alike, the bench
  core generalisation this needs, and a later live preview. Temporary; delete
  it when its work packages are delivered.
- [TODO](TODO.md): mixed backlog; older status statements need reconciliation
  before selecting new work.
- [GUI design references](GUI_Design/README.md): retained visual direction.

Completed implementation plans and their entry prompts were removed on
2026-09-05. The initial sigrok plan was also removed after its first integration
landed and the waveform plan superseded its next scope. Historical documents
remain available in Git history. Current implementation evidence and remaining
hardware acceptance are recorded in `06_SoftwareDocu/`, including the
[network runbook](../06_SoftwareDocu/Network_Operations.md).
