# NILO Assistant Go Bench Core

This Go module is the NILO Bench Core. External Python integrations connect
through the versioned integration boundary rather than becoming part of the
core process. Increment 1 currently provides the portable foundation and
`nilo version`; it provides no hardware behavior or MCP service.

Linux ARM64 is the first target. Linux x86_64 and Windows x86_64 are verified
portable targets for later delivery.
