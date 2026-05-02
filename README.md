# OAuth Callback Dispatcher

> Warning: this tool is for personal local development only. Production use is prohibited.
>
> It does not provide availability guarantees, audit logs, durable storage, failure recovery, or multi-tenant isolation. In production and shared environments, give each environment its own normal OAuth `redirect_uri`.

OAuth Callback Dispatcher lets multiple dynamic development environments use OAuth providers that require an exact, pre-registered callback URL. Register one fixed provider callback such as:

```text
https://oauth-dispatcher.myapp.localhost/auth/callback
```

Each local app registers its generated OAuth `state` with the dispatcher immediately before redirecting the browser to the provider. When the provider returns to the dispatcher, the dispatcher looks up `state -> origin` in memory and returns a `302` to the original app:

```text
[feat-a.myapp.localhost] -- POST /register --> [oauth-dispatcher.myapp.localhost]
[OAuth Provider] -------- GET /auth/callback -> [oauth-dispatcher] -- 302 --> [feat-a]/auth/callback
```

The dispatcher only forwards the authorization response query string. It never exchanges tokens and must not see `client_secret`, `access_token`, or `refresh_token`.

## Quick Start

```bash
ALLOWED_ORIGIN_PATTERN='^https://[a-z0-9-]+\.myapp\.localhost(:[0-9]+)?$' \
oauth-callback-dispatcher
```

Environment variables:

| Name | Required | Default | Description |
|---|---:|---:|---|
| `ALLOWED_ORIGIN_PATTERN` | yes | none | Anchored regular expression for allowed client origins |
| `PORT` | no | `8888` | Listen port |
| `STATE_TTL_SECONDS` | no | `600` | State mapping lifetime |
| `LOG_LEVEL` | no | `info` | `debug`, `info`, `warn`, or `error` |

Register this URL with the OAuth provider:

```text
https://oauth-dispatcher.myapp.localhost/auth/callback
```

Before starting OAuth, register the same `state` value that you will send to the provider:

```bash
curl -i -X POST 'https://oauth-dispatcher.myapp.localhost/register' \
  -H 'Content-Type: application/json' \
  -d '{"state":"cryptographically-random-state","origin":"https://feat-a.myapp.localhost"}'
```

Then redirect the browser to the provider using the dispatcher callback URL as `redirect_uri`. During token exchange, use the same dispatcher callback URL as `redirect_uri`, because OAuth providers require an exact match.

## Client Changes

Applications do not need a client library.

1. Set the OAuth authorization URL `redirect_uri` to the dispatcher callback URL.
2. Set the backend token exchange `redirect_uri` to the same dispatcher callback URL.
3. Immediately before redirecting to the provider, `POST /register` with the generated `state` and the current app origin.

All existing state generation, state validation, token exchange, session creation, and callback handling should remain in the client application.

## API

`POST /register`

```json
{
  "state": "predefined-csrf-token-from-client",
  "origin": "https://feat-a.myapp.localhost"
}
```

Responses: `204 No Content`, `400 Bad Request`, or `409 Conflict` for duplicate state.

`GET /auth/callback`

Receives `state`, `code`, `error`, `scope`, `error_description`, and any other provider query parameters. If `state` exists and has not expired, the dispatcher redirects to:

```text
{origin}/auth/callback?{original query string}
```

The mapping is deleted immediately after successful lookup to prevent replay.

`GET /healthz`

Returns `200 OK` with body `ok`.

## Security Model

The trust decision is the `origin` field in `POST /register` checked against `ALLOWED_ORIGIN_PATTERN`. CORS headers and request `Origin` headers are browser convenience features only and are not a security boundary.

The allowlist pattern must be anchored with `^` and `$`, and literal dots must be escaped. For example:

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

Regex allowlist mistakes have caused real redirect validation vulnerabilities, including Authentik CVE-2024-52289. This dispatcher revalidates the stored origin during callback dispatch as a second defensive layer.

Use PKCE, especially when running over HTTP in local portless setups. PKCE reduces the impact of an intercepted authorization code and is part of current OAuth security best practice in RFC 9700.

Do not use predictable `state` values. If another local process can guess a state value, it can register it first and force the real flow to fail with `409 Conflict`.

This tool does not protect against malicious processes on the same machine reading process memory, racing local requests, or tampering with local networking. It is intentionally scoped to a single developer's local machine.

## Portless Example

Run the dispatcher behind portless as `oauth-dispatcher.myapp.localhost` and allow only worktree hosts under the same local suffix:

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

Do not use this in production. In-flight OAuth flows are lost when the dispatcher restarts because mappings are in memory only. Team-shared use would require shared mapping storage and a broader security model, which are intentionally out of scope.

## Development

```bash
go test ./...
```

The implementation targets Go 1.25+ and uses only the standard library.

## License

MIT
