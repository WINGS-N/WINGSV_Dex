# AGENTS.md

Working guide for AI agents in the WINGSV_Dex repository. Read this first. When a rule is marked
HARD RULE or MANDATORY, follow it exactly - the user has corrected these before in the sibling
WINGS V repo.

## 1. What this is

WINGSV_Dex is the Linux desktop port of the WINGS V Android VPN client, built with Wails v3
(Go backend + Vue 3 frontend, system WebKitGTK). The MVP targets ONLY the VK TURN + WireGuard
mode. Feature scope: read wingsv:// links, profiles, WG auto-provision, and a VK TURN settings
screen that mirrors the Android app (same switches/dropdowns and the same default values).

Ecosystem context (sibling repos, out of scope here): the Android client
(../../../android/projects/apps/WINGSV), the Go/Vue panel v.wingsnet.org, and the vk-turn-proxy
node. All under the WINGS-N GitHub org.

## 2. How it works (data plane)

Local kernel WireGuard sends UDP to 127.0.0.1:9000 -> the vkturn child process (vk-turn-proxy
client) wraps it in DTLS-over-TURN with SRTP-mimicry through VK calls -> peer server unwraps and
forwards to real WireGuard. The app drives vkturn over a local AppControl gRPC on a unix socket:
Configure (all settings), SetVKCookies (VK web session for account mode), Provision (WG config
for managed profiles), StreamTelemetry.

## 3. Layout

- main.go - application.New + windows + services (bound methods exposed to Vue).
- internal/vktp - vkturn child-process manager + AppControl gRPC client.
- internal/wg - kernel WireGuard via wgctrl + netlink (up/down, peer, routes, DNS, protect-mark).
- internal/wingsv - wingsv:// codec (decode/encode) + Config model.
- internal/config - VK TURN settings model (port of ProxySettings) + profile store.
- internal/cookies - native VK cookie capture from WebKitGTK (Linux).
- internal/preview - wingsv:// / vless:// preview (ported from v.wingsnet.org).
- external/vk-turn-proxy - git submodule; build task compiles ./client to bin/vkturn (GOOS=linux).
- external/wingsv-proto - git subtree (wingsv.proto, appcontrol.proto, control.proto).
- frontend - Vue 3 + Vite; vendored Samsung/One-UI design system from v.wingsnet.org.
- scripts/generate_proto.sh - Go stub codegen (adapted from v.wingsnet.org).

The plan with milestones M0-M7 lives outside the repo at
~/.claude/plans/lexical-snacking-key.md.

## 4. Build, run, lint

- Package manager is pnpm. HARD RULE: never npm/yarn. Commit pnpm-lock.yaml.
- Build: `task build` (GOOS defaults to host; linux). Binary -> bin/wingsv-dex.
- Dev: `task dev`. Toolchain: go, wails3 (v3 alpha), task, protoc + protoc-gen-go(-grpc),
  pnpm, node, webkitgtk (GTK4/webkitgtk-6.0 on this host), pkexec.
- Frontend deps install and build run through the pnpm variants in build/Taskfile.yml
  (PACKAGE_MANAGER=pnpm set in the root Taskfile.yml).

## 5. Pre-commit gates (MANDATORY)

- Go: `gofmt -w` (or gofmt -l must be empty) on changed .go files, and `go build ./...`.
- Vue/TS/CSS: `pnpm format` in frontend (prettier) - mirror v.wingsnet.org config (semi,
  single-quote, printWidth 120, tabWidth 2). Verify before committing.

## 6. Commit conventions

- Format: `[scope] short lowercase imperative` - single subject line, NO colon after the scope,
  NO body unless explicitly requested, NO Co-Authored-By or any AI-mention trailer.
- Pick the narrowest accurate scope, e.g. [vktp], [wg], [wingsv], [config], [frontend], [proto],
  [build], [ci], [docs], [ignore]. Submodule pointer bump: `[external/vk-turn-proxy] bump`.
- HARD RULE: do NOT `git push` without explicit user confirmation, even for lint/CI fixes.
  Commit, then stop and report; wait for the user to say push.

## 7. Code-text style

- ASCII only in anything written into the source tree (code, comments): use `-` not em/en-dash,
  straight quotes, `...` not the ellipsis char, words not arrow glyphs.
- No markdown in code comments: plain prose, name symbols by writing their name. README and *.md
  docs are markdown and stay markdown (this file included).
- Comments only for genuinely non-obvious WHY (protocol gotchas, kernel quirks). Do not restate
  what the code does; default to no comment.

## 8. Embedding and proto

- Embedding third-party code: git submodule + a build task that produces the artifact
  (e.g. external/vk-turn-proxy -> bin/vkturn). Do not vendor-copy source.
- Proto: git subtree at external/wingsv-proto + scripts/generate_proto.sh for codegen
  (mirrors v.wingsnet.org). control.proto is re-synced from the vk-turn-proxy repo by the script.
- Config field DEFAULTS must match the Android app exactly (source of truth: ProxySettings.java +
  AppPrefs.getSettings). Notably turnSessionMode defaults to "mu" at runtime.
