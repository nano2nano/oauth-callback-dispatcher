# OAuth Callback Dispatcher

[![Release](https://img.shields.io/github/v/release/nano2nano/oauth-callback-dispatcher?sort=semver)](https://github.com/nano2nano/oauth-callback-dispatcher/releases)
[![License](https://img.shields.io/github/license/nano2nano/oauth-callback-dispatcher)](LICENSE)
[![Go version](https://img.shields.io/github/go-mod/go-version/nano2nano/oauth-callback-dispatcher)](go.mod)
[![Container](https://img.shields.io/badge/ghcr.io-oauth--callback--dispatcher-2496ed?logo=docker)](https://github.com/nano2nano/oauth-callback-dispatcher/pkgs/container/oauth-callback-dispatcher)

A tiny local-only dispatcher that lets multiple dynamic development environments
share a single OAuth `redirect_uri` registered with the upstream provider.

> [!WARNING]
> This tool is for **personal local development only**. Production use is prohibited.
>
> It does not provide availability guarantees, audit logs, durable storage,
> failure recovery, or multi-tenant isolation. In production and shared
> environments, give each environment its own normal OAuth `redirect_uri`.

---

## Table of Contents

- [Why](#why)
- [How it works](#how-it-works)
- [Installation](#installation)
  - [Pre-built binaries](#pre-built-binaries)
  - [Docker](#docker)
  - [`go install`](#go-install)
  - [Nix](#nix)
  - [From source](#from-source)
  - [Verifying signatures](#verifying-signatures)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [Client Changes](#client-changes)
- [API](#api)
- [Security Model](#security-model)
- [Portless Example](#portless-example)
- [Known Limits](#known-limits)
- [Development](#development)
- [License](#license)

---

## Why

OAuth providers require an exact, pre-registered callback URL. When you run
many short-lived development environments (feature branches, ephemeral
worktrees, preview hosts), you cannot register every new origin with the
provider. This dispatcher lets you register **one** fixed callback such as:

```text
https://oauth-dispatcher.myapp.localhost/auth/callback
```

and forwards each authorization response back to whichever local origin
started the flow.

## How it works

Each local app registers its generated OAuth `state` with the dispatcher
immediately before redirecting the browser to the provider. When the provider
returns to the dispatcher, the dispatcher looks up `state -> origin` in memory
and returns a `302` to the original app:

```text
[feat-a.myapp.localhost] -- POST /register --> [oauth-dispatcher.myapp.localhost]
[OAuth Provider] -------- GET /auth/callback -> [oauth-dispatcher] -- 302 --> [feat-a]/auth/callback
```

The dispatcher only forwards the authorization response query string. It never
exchanges tokens and must not see `client_secret`, `access_token`, or
`refresh_token`.

## Installation

Releases are published for Linux, macOS, and Windows on `amd64` and `arm64`.
All artifacts are signed with [cosign](https://docs.sigstore.dev/cosign/) using
keyless OIDC signing — see [Verifying signatures](#verifying-signatures).

### Pre-built binaries

Download the archive for your platform from the
[releases page](https://github.com/nano2nano/oauth-callback-dispatcher/releases/latest)
and extract the `oauth-callback-dispatcher` binary onto your `PATH`.

```bash
# Linux / macOS — replace VERSION, OS, and ARCH
VERSION=0.1.4
OS=linux              # or darwin
ARCH=amd64            # or arm64

curl -sSfL "https://github.com/nano2nano/oauth-callback-dispatcher/releases/download/v${VERSION}/oauth-callback-dispatcher_${VERSION}_${OS}_${ARCH}.tar.gz" \
  | tar -xz oauth-callback-dispatcher
sudo install -m 0755 oauth-callback-dispatcher /usr/local/bin/
```

```powershell
# Windows (PowerShell)
$Version = "0.1.4"
$Arch    = "amd64"   # or "arm64"
$Url     = "https://github.com/nano2nano/oauth-callback-dispatcher/releases/download/v$Version/oauth-callback-dispatcher_${Version}_windows_${Arch}.zip"

Invoke-WebRequest $Url -OutFile dispatcher.zip
Expand-Archive dispatcher.zip -DestinationPath .
```

### Docker

Multi-arch images (`linux/amd64`, `linux/arm64`) are published to GitHub
Container Registry. The container is a `FROM scratch` image and runs as a
non-root user.

```bash
docker run --rm -p 127.0.0.1:8888:8888 \
  -e ALLOWED_ORIGIN_PATTERN='^https://[a-z0-9-]+\.myapp\.localhost(:[0-9]+)?$' \
  -e LISTEN_HOST=0.0.0.0 \
  ghcr.io/nano2nano/oauth-callback-dispatcher:v0.1.4
```

`LISTEN_HOST=0.0.0.0` is required inside the container so the process binds to
the published port; the loopback restriction is enforced by the
`-p 127.0.0.1:8888:8888` host-side bind. Always pin to a specific tag —
available tags are listed on
[the GHCR package page](https://github.com/nano2nano/oauth-callback-dispatcher/pkgs/container/oauth-callback-dispatcher).

### `go install`

If you have Go 1.25 or newer:

```bash
go install github.com/nano2nano/oauth-callback-dispatcher/cmd/oauth-callback-dispatcher@latest
```

The binary will land in `$(go env GOBIN)` (or `$(go env GOPATH)/bin`).

### Nix

This repository is a Nix flake. Run the latest tagged release without
installing it:

```bash
nix run github:nano2nano/oauth-callback-dispatcher
```

Pin to a specific tag:

```bash
nix run github:nano2nano/oauth-callback-dispatcher/v0.1.4
```

Or add it as a flake input:

```nix
{
  inputs.oauth-callback-dispatcher.url = "github:nano2nano/oauth-callback-dispatcher/v0.1.4";
  # then reference packages.${system}.default in your output
}
```

### From source

```bash
git clone https://github.com/nano2nano/oauth-callback-dispatcher.git
cd oauth-callback-dispatcher
go build -o oauth-callback-dispatcher ./cmd/oauth-callback-dispatcher
```

The implementation targets Go 1.25+ and uses only the standard library, so
there are no third-party dependencies to vendor.

### Verifying signatures

Every release publishes a `checksums.txt` and `checksums.txt.sigstore.json`
bundle signed by GitHub Actions OIDC. Container manifests are signed in the
same way. Verify before installing:

```bash
# Verify the checksums file
cosign verify-blob \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp 'https://github.com/nano2nano/oauth-callback-dispatcher/\.github/workflows/release\.yml@refs/tags/v.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt

# Then check your downloaded archive against the (now trusted) checksums
sha256sum --check --ignore-missing checksums.txt
```

```bash
# Verify a container image
cosign verify ghcr.io/nano2nano/oauth-callback-dispatcher:v0.1.4 \
  --certificate-identity-regexp 'https://github.com/nano2nano/oauth-callback-dispatcher/\.github/workflows/release\.yml@refs/tags/v.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

## Quick Start

```bash
ALLOWED_ORIGIN_PATTERN='^https://[a-z0-9-]+\.myapp\.localhost(:[0-9]+)?$' \
oauth-callback-dispatcher
```

By default, the dispatcher listens on `127.0.0.1:8888`. Set
`LISTEN_HOST=0.0.0.0` only when you intentionally want to expose it beyond
loopback.

Register this URL with the OAuth provider:

```text
https://oauth-dispatcher.myapp.localhost/auth/callback
```

Before starting OAuth, register the same `state` value that you will send to
the provider:

```bash
curl -i -X POST 'https://oauth-dispatcher.myapp.localhost/register' \
  -H 'Content-Type: application/json' \
  -H 'X-OAuth-Callback-Dispatcher: register' \
  -d '{"state":"cryptographically-random-state","origin":"https://feat-a.myapp.localhost"}'
```

Then redirect the browser to the provider using the dispatcher callback URL as
`redirect_uri`. During token exchange, use the same dispatcher callback URL as
`redirect_uri`, because OAuth providers require an exact match.

## Configuration

| Name | Required | Default | Description |
|---|---:|---:|---|
| `ALLOWED_ORIGIN_PATTERN` | yes | none | Anchored regular expression for allowed client origins |
| `LISTEN_HOST` | no | `127.0.0.1` | Listen host. Use `0.0.0.0` only with an explicit local network threat model |
| `PORT` | no | `8888` | Listen port |
| `STATE_TTL_SECONDS` | no | `600` | State mapping lifetime |
| `MAX_STATE_ENTRIES` | no | `10000` | Maximum number of in-flight state mappings |
| `LOG_LEVEL` | no | `info` | `debug`, `info`, `warn`, or `error` |

## Client Changes

Applications do not need a client library.

1. Set the OAuth authorization URL `redirect_uri` to the dispatcher callback URL.
2. Set the backend token exchange `redirect_uri` to the same dispatcher callback URL.
3. Immediately before redirecting to the provider, `POST /register` with the generated `state` and the current app origin.

All existing state generation, state validation, token exchange, session
creation, and callback handling should remain in the client application.

## API

### `POST /register`

```json
{
  "state": "predefined-csrf-token-from-client",
  "origin": "https://feat-a.myapp.localhost"
}
```

Responses: `204 No Content`, `400 Bad Request`, or `409 Conflict` for
duplicate state.

`POST /register` requires `Content-Type: application/json`, the
`X-OAuth-Callback-Dispatcher: register` header, and a small JSON body. These
checks intentionally reject simple browser form posts and oversized
registration attempts.

### `GET /auth/callback`

Receives `state`, `code`, `error`, `scope`, `error_description`, and any other
provider query parameters. If `state` exists and has not expired, the
dispatcher redirects to:

```text
{origin}/auth/callback?{original query string}
```

The mapping is deleted immediately after successful lookup to prevent replay.

### `GET /healthz`

Returns `200 OK` with body `ok`.

## Security Model

The trust decision is the `origin` field in `POST /register` checked against
`ALLOWED_ORIGIN_PATTERN`. CORS headers and request `Origin` headers are
browser convenience features only and are not a security boundary.

When a browser sends an `Origin` header on `/register`, it must exactly match
the normalized JSON `origin`. Browser clients must send the
`X-OAuth-Callback-Dispatcher: register` header so that cross-origin
registrations require a CORS preflight.

The allowlist pattern must be anchored with `^` and `$`, and literal dots must
be escaped. For example:

```bash
ALLOWED_ORIGIN_PATTERN='^https://[a-z0-9-]+\.myapp\.localhost(:[0-9]+)?$'
```

These patterns are rejected or warned about:

```bash
# Rejected: not anchored
https://[a-z0-9-]+\.myapp\.localhost

# Rejected: unescaped dots
^https://[a-z0-9-]+.myapp.localhost$

# Allowed with warning: permits plain HTTP
^https?://[a-z0-9-]+\.myapp\.localhost(:[0-9]+)?$
```

Regex allowlist mistakes have caused real redirect validation vulnerabilities,
including Authentik CVE-2024-52289. This dispatcher revalidates the stored
origin during callback dispatch as a second defensive layer.

Use PKCE, especially when running over HTTP in local portless setups. PKCE
reduces the impact of an intercepted authorization code and is part of current
OAuth security best practice in RFC 9700.

Do not use predictable `state` values. If another local process can guess a
state value, it can register it first and force the real flow to fail with
`409 Conflict`.

This tool does not protect against malicious processes on the same machine
reading process memory, racing local requests, or tampering with local
networking. It is intentionally scoped to a single developer's local machine.

## Portless Example

Run the dispatcher behind portless as `oauth-dispatcher.myapp.localhost` and
allow only worktree hosts under the same local suffix:

```bash
PORT=8888 \
ALLOWED_ORIGIN_PATTERN='^https://[a-z0-9-]+\.myapp\.localhost(:[0-9]+)?$' \
oauth-callback-dispatcher
```

Use app origins such as:

```text
https://feat-a.myapp.localhost
https://exp-foo.myapp.localhost
```

## Known Limits

Do not use this in production. In-flight OAuth flows are lost when the
dispatcher restarts because mappings are in memory only. Team-shared use would
require shared mapping storage and a broader security model, which are
intentionally out of scope.

## Development

```bash
# Run tests
go test ./...

# Or with the provided Nix dev shell
nix develop
```

The Nix dev shell provides Go 1.25, `golangci-lint`, and `goreleaser`.

## License

[MIT](LICENSE)
