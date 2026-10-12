# Changelog

## [Unreleased]

** Features: **

- Added "OpenAI Compatible" provider for llama.cpp, vLLM, LM Studio and any other OpenAI API compatible server (configurable base URL, optional API key) - [#2](https://github.com/hkdb/otui/issues/2)

** Maintenance & Bug Fixes: **

- Scroll position is kept when a response completes if you scrolled up while it was streaming
- Locked sessions can now be force opened (D) when the lock was left behind by a crash, including the last session on startup
- Fixed MCP tool names being rejected by OpenAI, Anthropic and OpenAI compatible APIs (namespaced tool names are now encoded the same way for every provider)
- Fixed parameterless MCP tools sending an invalid schema to OpenAI compatible APIs
- Sessions are now locked when restored on startup or newly created
- Fixed the "Custom Settings" setup wizard saving `require_approval = false` (MCP tools ran without asking), `enable_multi_step = false`, `max_iterations = 0` and zeroed compaction thresholds instead of the defaults. Affected profiles are repaired automatically on the next launch
- Added "Tool Approval" to the Settings screen to turn tool approval prompts on/off
- Fixed context window ("max:" in the header) falling back to 8192 for most models: it's now queried from the server (Ollama loaded context / num_ctx / trained length, llama.cpp, vLLM, LM Studio, OpenRouter, Anthropic) instead of guessed from the model name
- Context window lookups no longer block the UI and use the session's own provider
- Context usage % now updates when the model or its context window changes
- Fixed crash on startup after entering the correct passphrase for a passphrase protected SSH key (also when switching to such a data directory in Settings)
- Fixed the startup passphrase prompt repeating forever without explanation: a wrong passphrase now shows "Incorrect passphrase", and other credential errors (e.g. credentials encrypted with a different SSH key) show an error instead of prompting again
- Fixed crashes when the terminal is small or resized very small (model selector, session manager, settings, code blocks, chat view); below 40×12 a "Terminal too small" message is shown instead
- Model and session lists adapt to narrow terminals (names keep at least 10 columns, less important columns are dropped instead of wrapping)
- Fixed long names with non-ASCII characters or emoji being cut in the middle of a character
- Fixed a crash when compacting a session after cancelling a response with Esc
- Fixed compaction hiding the wrong messages in sessions with tool calls (system messages were counted as conversation messages)
- Compaction no longer re-summarizes already compacted history; a failed summary keeps the previous one instead of replacing it with a generic note
- Fixed auto-compaction removing the wrong message (e.g. a pending tool permission prompt) when it finished
- The compaction notice now shows more accurate context usage

---

## [v0.08.02] - 2026-04-13

** Feature: **

- Notification on completion of response

---

## [v0.08.01] - 2026-03-22

** Maintenance & Bug Fixes: **

- Fixed session switch cleanup
- Increased test coverage

---

## [v0.08.00] - 2026-03-20

** Features: **

- Added context window tracking
- Added ability to scroll while streaming response
- Added auth header for auth proxy protected ollama instances

---

## [v0.07.00] - 2026-02-01

** Features: **

- Custom keybindings. See details at [docs/KEYBINDINGS.md](https://github.com/hkdb/otui/blob/main/docs/KEYBINDINGS.md)

---

## [v0.06.00] - 2026-01-31

** Features: **

Adjusted scroll keybindings for easier navigation:
- Changed alt+j/k and alt+up/down to one line scroll
- Changed alt+shift+j/k and alt+shift+up/down to half page scroll
- alt+PgUp/PgDown remained full page scroll

---

## [v0.05.01] - 2026-01-19

** Maintenance & Bug Fixes: **

- Added option to force delete lock file to handle stale lock file situations

[v0.05.01]: https://github.com/hkdb/otui/releases/tag/v0.05.01

---

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

