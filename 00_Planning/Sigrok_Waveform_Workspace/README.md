# Sigrok waveform workspace

Planning baseline: 2026-09-05. Research and brainstorming only, prepared without AEGIS at the user's request. Tasks below are proposed work, not implemented or verified product capabilities.

This integration is the NILO `0.2.0` development line. Keep the project at
`0.2.0` while the work packages land; assigning a later version is a separate
release decision after the integration is complete.

## Direction agreed with the user

Build a browser waveform workspace similar in appearance and functionality to PulseView, backed by the sigrok integration. The GUI, CLI and MCP use the same appliance services. PulseView remains a reference application and an interoperability target; embedding or remotely streaming its desktop window is not the chosen architecture.

Plugin installation and enablement belong to the **NILO appliance**. All users see the same installed/enabled plugin configuration, subject to RBAC. A user's zoom position or an analysis document does not create a separate bench configuration.

The scope covers the waveform acquisition and analysis capabilities offered by the deployed sigrok/PulseView stack, including digital, analog and mixed signals. The ATK analyzer is one live-test device, not the product's capability model. PWM outputs are excluded from every interface. Other sigrok device classes are to be classified explicitly, not silently presented as waveform instruments.

## Read in this order

1. [PulseView reference and evidence](01_PulseView_Reference.md): documented, source-observed and live-observed functionality; source locations and limitations.
2. [NILO workflow and layout](02_Workflow_and_Layout.md): navigation, capture workspace, saved waveforms, operations, accessibility and RBAC.
3. [Architecture and contracts](03_Architecture_and_Contracts.md): capability model, data handling, commands, concurrency, persistence and CLI/MCP coverage.
4. [Implementation tasks](04_Implementation_Tasks.md): work packages, dependencies and acceptance outcomes.
5. [Verification and decisions](05_Verification_and_Decisions.md): coverage matrix, performance investigation and remaining technical decisions.

This folder is the current planning baseline for the waveform expansion. The earlier initial integration plan was removed during planning cleanup after its first integration landed; its historical content remains in Git history. Any earlier ATK-only, fixed-channel, UART-only or small-preview scope is superseded by the broader target here. Existing AEGIS drafts have not been modified by this planning exercise.

## Can this be planned from help alone?

The web manual and local source provide enough evidence to plan the architecture and feature inventory. Running PulseView improves understanding of the interaction details. Both were used here: the already-open Windows application was inspected and its Demo device was used for one finite capture and a cursor overlay. No physical analyzer was exercised during this research.

The running executable was `D:/Programms/PulseView/pulseview.exe`. Its build identity was not established, so observations of it are not proof that it exactly matches the local fork. Source evidence refers to `F:/2026_PulseView` at commit `bcf7e93414dbca640abf8e4b47802b57c5794b84`. NILO gap analysis was made against local commit `09a411552bf2f3bd284608c554a1bdd9fe074950` and the inspected working files.

## Meaning of completion

Completion means useful end-to-end acquisition, waveform analysis, persistence and automation with explicit coverage of the feature inventory. A renderer showing a few hundred samples is an intermediate milestone. Driver-specific support and electrical acceptance must be reported separately from NILO integration acceptance. Repository administration tools are not product requirements.
