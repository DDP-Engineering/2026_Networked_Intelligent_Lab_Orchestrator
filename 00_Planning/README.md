# Planning

Project goals, architecture decisions, milestones, and implementation plans.

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
- [TODO](TODO.md): mixed backlog; older status statements need reconciliation
  before selecting new work.
- [GUI design references](GUI_Design/README.md): retained visual direction.

Completed implementation plans and their entry prompts were removed on
2026-09-05. The initial sigrok plan was also removed after its first integration
landed and the waveform plan superseded its next scope. Historical documents
remain available in Git history. Current implementation evidence and remaining
hardware acceptance are recorded in `06_SoftwareDocu/`, including the
[network runbook](../06_SoftwareDocu/Network_Operations.md).
