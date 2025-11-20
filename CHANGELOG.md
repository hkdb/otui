# Changelog

## [v0.05.00] - 2025-11-20

** Features: **

- Added tool use permission request dialog
- Enhanced tool use indication
- Added multi-steps response handling
- Improved tool calling leakage handling

[v0.05.00]: https://github.com/hkdb/otui/releases/tag/v0.05.00

---

## [v0.04.00] - 2025-11-17

** Features: **

- Support separately managed remote MCP servers
- Support for MCPs using streamable HTTP connection transport type
- Added dynamic env var: `{{OTUI_SESSION_ID}}, {{OTUI_SESSION_NAME}}, {{OTUI_DATA_DIR}}, & {{OTUI_USER}}`
- Added ability to parse args and env var default values from registry
- Encrypt senstive env var for mcp plugins
- Added the ability to edit custom plugin

**Maintenance & Bug Fixes:**

- Changed Plugin Manager's "Installed" tab to show name of plugin instead of description
- Added some padding between end of plugin list and footer in Plugin Manager
- Delayed clearing of "waiting for response" spinner for models that stream empty chunks in the beginning
- Added the missing python3-venv package in Docker image

[v0.04.00]: https://github.com/hkdb/otui/releases/tag/v0.04.00

---

## [v0.03.04] - 2025-11-13

**Maintenance & Bug Fixes:**

- Corrected get script header
- Added video in Github Pages
- Updated release doc

No changes were made to the binary. v0.03.03 users can skip this release.

[v0.03.04]: https://github.com/hkdb/otui/releases/tag/v0.03.04

---

## [v0.03.03] - 2025-11-13

**Maintenance & Bug Fixes:**

- Fixes regression that prevented npm from working in container image

[v0.03.03]: https://github.com/hkdb/otui/releases/tag/v0.03.03

---

## [v0.03.02] - 2025-11-12

**Maintenance & Bug Fixes:**

- Updated container image to support macos
- Updated container image to have more reliable permissions
- Updated README.md and docs/CONTAINERIZE.md

[v0.03.02]: https://github.com/hkdb/otui/releases/tag/v0.03.02

---

## [v0.03.01] - 2025-11-11

**Maintenance & Bug Fixes:**

- Fixes wrong provider loaded on startup if provider is not ollama
- Fixes current model and tool support indicator for providers that are not ollama
- Fixes export confirmation acknowledgement

[v0.03.01]: https://github.com/hkdb/otui/releases/tag/v0.03.01

---

## [v0.03.00] - 2025-11-11

**Feature(s):**

- Added docker image and instructions in README.

**Maintenance & Bug Fixes:**

- Sizable improvements to MCP use stability
- Fixes save settings regression
- Fixes stale lock files after switching data (profile) dir
- Fixes minor UI bugs

[v0.03.00]: https://github.com/hkdb/otui/releases/tag/v0.03.00

---

## [v0.02.00] - 2025-11-10

**Feature(s):**

- Multi-Provider support (OpenRouter, Anthropic, OpenAI)
- SSH key encryption to encrypt API keys and prepare for encrypted sessions
- A better flow to create new data dirs within OTUI

**Maintenance:**

- More reliable data dir switching
- Minor Bug fixes
- Code cleanup
- Removed overly complicated quit logic
- Minor distribution landing page fixes

[v0.02.00]: https://github.com/hkdb/otui/releases/tag/v0.02.00

---

## [v0.01.00] - 2025-11-06

First ALPHA release of OTUI

- Core functionality (Ollama Only)

[v0.01.00]: https://github.com/hkdb/otui/releases/tag/v0.01.00

---

