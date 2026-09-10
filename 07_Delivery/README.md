# Delivery

Release artifacts, installation bundles, release notes, and the official
documentation that accompanies the application.

## Official documentation

- [NILO — Command Line and Authentication](NILO_CLI_and_Authentication.md) -
  the administrator's reference for the `nilo` command line, the fixed role and
  permission model, first-run setup, browser sessions versus API tokens, the
  HTTP authentication routes, audit coverage, upgrade behavior, and what this
  release does not yet provide.
- [Dual-path management verification](NILO_Dual_Path_Management_Verification.md) -
  the implemented security contract, automated acceptance evidence, base-state
  caveat, and remaining Raspberry Pi hardware checks.


## Optional integration bundles

Set `NILO_BUNDLE_SIGROK=1` when building an edition to include the sigrok catalog alongside its binaries. The same catalog can be deployed later; installation and enablement remain explicit. See [Sigrok integration](../06_SoftwareDocu/Sigrok_Integration.md).
