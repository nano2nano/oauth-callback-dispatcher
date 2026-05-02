# 技術選定 — OAuth Callback Dispatcher

**日付**: 2026-05-03
**前提仕様**: [`docs/SPEC.md`](../../SPEC.md)
**対象**: 個人ローカル開発専用の軽量 OAuth callback ディスパッチャ

## 1. 決定事項(サマリ)

| 項目 | 採用 |
|---|---|
| 言語 | **Go 1.23+** |
| HTTP / ルーティング | `net/http`(標準ライブラリのみ、Go 1.22+ の method/path 拡張を利用) |
| JSON | `encoding/json`(標準) |
| 正規表現 | `regexp`(標準) |
| ロガー | `log/slog`(標準)+ secret フィルタ用カスタム Handler |
| 設定読み込み | `os.Getenv` 直読み(ライブラリ追加なし) |
| テスト | `go test` + `net/http/httptest`(標準) |
| リリース自動化 | **GoReleaser** → GitHub Releases + Homebrew tap |
| Nix 配布 | **flake.nix**(`buildGoModule`)+ devShell |
| CI | GitHub Actions(test → tag で goreleaser release) |

**外部 Go 依存はゼロを目標**(`go.mod` の require 行が空に近い状態)とする。

## 2. 配布チャネル

仕様 §1 で「個人ローカル開発専用」と明記しつつ、Q1 の答えとして **B. Nix ユーザー中心 + C. 一般開発者向け** をスコープに含める。

| チャネル | 用途 | 実装手段 |
|---|---|---|
| `nix run github:nano2nano/oauth-callback-dispatcher` | NixOS / nix-darwin / Home Manager ユーザー | `flake.nix`(`buildGoModule`、`vendorHash`)|
| GitHub Releases(tarball / zip) | 一般開発者(Linux / macOS / Windows、amd64 / arm64) | GoReleaser |
| Homebrew(自前 tap) | macOS / Linux Homebrew ユーザー | GoReleaser の `brews:` ブロック → `homebrew-tap` リポジトリへ自動 push |
| Docker(任意) | コンテナ前提のユーザー | GoReleaser の `dockers:` ブロック(`FROM scratch`、~10 MB)|

クロスビルドは CGO 不要なので `GOOS`/`GOARCH` のマトリクスのみで完結する。

## 3. なぜ Go か(Rust 比較)

仕様の特性に対する適合度で Go を採用する。

### 3.1 仕様規模と言語のフィット感

エンドポイント 3 つ・in-memory map・regex 検証・CORS のみ。**サードパーティ依存ゼロでも全仕様を満たせる**のは Go(stdlib に HTTP server が入っている)であり、Rust では axum + tokio + serde + tracing が必須になる。本件で Rust の所有権・型システムが守る不変条件は、Go の単純な構造でも壊しようがない。

### 3.2 配布コスト

| | Go | Rust |
|---|---|---|
| クロスコンパイル | `GOOS=darwin GOARCH=arm64 go build` 一発、CGO_ENABLED=0 で純静的 | `cargo-zigbuild` / `cross` 経由(`dist` が裏で吸収) |
| Nix flake | `buildGoModule` + `vendorHash` 1 行 | `crane`(複数 derivation 構成)or `rustPlatform.buildRustPackage` |
| クリーンビルド時間 | 数秒 | 1〜3 分 |

仕様 §10 の sanity check テストを高速に反復回せる Go の方が開発体験で優位。

### 3.3 読み手の母数

GitHub に置いた Issue / PR で他者がパッチを書ける確率は Go の方が高い。本ツールは「OAuth provider が wildcard を許さないので困っている開発者」が対象であり、その層は言語横断で散らばっているため、読みやすさ優先で Go を選ぶ。

### 3.4 Rust が選ばれる条件(本件では満たさない)

- 既存の Rust エコシステムに組み込みたい
- 性能がクリティカル(本件は in-memory map の lookup のみ、無関係)
- バイナリサイズを 2〜3 MB まで詰めたい(本件では 5〜10 MB で十分)

これらに該当しないため Go を採用する。

## 4. ライブラリ・パッケージ詳細

### 4.1 標準ライブラリのみで完結する根拠

| 仕様要件 | 利用するパッケージ |
|---|---|
| `POST /register`、`GET /auth/callback`、`GET /healthz` のルーティング | `net/http`(`mux.Handle("POST /register", ...)` の Go 1.22+ 構文)|
| JSON リクエスト body のパース | `encoding/json` |
| `ALLOWED_ORIGIN_PATTERN` の compile・適用 | `regexp` |
| TTL 付き in-memory map | 自前 struct + `sync.RWMutex` + `time.Time` |
| 60 秒ごとの掃除ジョブ | `time.Ticker` + `context.Context` でのキャンセル |
| 構造化ログ(secret フィルタ) | `log/slog` + 自前の `slog.Handler` ラッパで `code` / `state` / `access_token` / `refresh_token` キーを drop |
| CORS preflight | 自前 middleware(数十行) |
| 起動時 sanity check | 自前(`regexp.Compile` + bypass 候補テーブル) |
| テスト | `net/http/httptest`、`testing` |

### 4.2 採用しないもの(YAGNI)

- ルーター: `chi` / `gorilla/mux` / `gin` — Go 1.22+ stdlib で十分
- 設定ライブラリ: `viper` / `envconfig` — env var 4 つだけなので `os.Getenv` で足りる
- ロガー: `zerolog` / `zap` — `log/slog` で構造化ログは可能、secret フィルタも自前で書ける
- DI コンテナ / フレームワーク — 不要
- メトリクス(Prometheus 等)— 個人ローカル前提でスコープ外
- 永続化(SQLite / Redis 等)— 仕様 §7.1 で明示的に禁止

### 4.3 開発時のみ使うツール(ビルド成果物には入らない)

| ツール | 用途 |
|---|---|
| `golangci-lint` | lint(CI のみ)|
| GoReleaser | リリース自動化(主に CI、ローカルでも `goreleaser release --snapshot --clean` で動作確認可)|
| Nix(devShell) | ローカル開発環境のピン留め |

## 5. プロジェクト構成(初期)

```
oauth-callback-dispatcher/
├── docs/
│   ├── SPEC.md
│   └── superpowers/specs/2026-05-03-tech-stack-design.md
├── cmd/
│   └── oauth-callback-dispatcher/
│       └── main.go              # エントリポイント、env 読み込み、起動
├── internal/
│   ├── server/                  # HTTP ハンドラ、ルーティング、CORS
│   ├── store/                   # state → origin の TTL マップ
│   ├── allowlist/               # ALLOWED_ORIGIN_PATTERN の compile + sanity check
│   └── logging/                 # secret フィルタ slog handler
├── go.mod
├── go.sum                       # 依存ゼロなら空に近い
├── flake.nix
├── flake.lock
├── .goreleaser.yaml
├── .github/workflows/
│   ├── test.yml
│   └── release.yml
├── README.md
├── CLAUDE.md
└── LICENSE                      # MIT(仕様 §12)
```

`internal/` 配下を 4 パッケージに分けるのは、仕様 §7.4 で示唆された「将来の構造化検証併用」を後付けしやすくするため(`allowlist` パッケージの内部 API を差し替えれば良い)。

## 6. リリース・配布の運用フロー

### 6.1 開発フロー

1. `nix develop` で Go 1.23 + golangci-lint + goreleaser のピン留めシェルに入る
2. `go test ./...` で動作確認
3. push → GitHub Actions の `test.yml` で再検証

### 6.2 リリースフロー

1. `git tag v0.x.y && git push --tags`
2. GitHub Actions の `release.yml` が GoReleaser を起動
3. GoReleaser が以下を一括生成:
   - 各プラットフォームのバイナリ(Linux/macOS/Windows × amd64/arm64)
   - GitHub Releases に tarball / zip / SHA256SUMS をアップロード
   - `homebrew-tap` リポジトリに Formula を自動 push
   - (任意で)Docker イメージを ghcr.io に push

### 6.3 Nix ユーザー側の利用

```bash
nix run github:nano2nano/oauth-callback-dispatcher -- --help
```

`flake.nix` は以下を export:

- `packages.${system}.default` — `buildGoModule` 成果物
- `apps.${system}.default` — `nix run` 用エントリ
- `devShells.${system}.default` — Go 1.23、golangci-lint、goreleaser

## 7. テスト戦略

仕様 §10 をすべて `go test` で実行する。

| カテゴリ | 実装 |
|---|---|
| 正常系(`/register` → `/auth/callback` のリダイレクト)| `httptest.NewServer` でディスパッチャを起動し、ハンドラを直接呼ぶ |
| 異常系(未登録 state、TTL 切れ、ホワイトリスト外、重複 state)| 同上 + テーブル駆動テスト |
| TTL の時間制御 | 内部の `time.Now` を `func() time.Time` で差し替え可能にして、テスト時に進める |
| 起動時 sanity check | `allowlist.Validate` を直接呼ぶユニットテスト + bypass 候補のテーブル(`https://evil.com` 等)|
| ログに secret が出ないこと | `slog.NewJSONHandler` の出力を `bytes.Buffer` で捕まえて、`code` / `access_token` 等のキーが含まれないことを検査 |

外部リソース(実 OAuth provider 等)に依存するテストは作らない。**全テストが `go test ./...` で完結する**ことを目標。

## 8. リスクと未決事項

| リスク | 緩和策 |
|---|---|
| Go 1.22+ の routing 構文に依存 | `go.mod` で `go 1.23` を明示。CI は Go 1.23 以上のみで回す |
| GoReleaser の GitHub Actions token スコープ不足(homebrew-tap への push で発生しがち) | 別 PAT または GitHub App token を `HOMEBREW_TAP_GITHUB_TOKEN` として設定する手順を README に書く |
| Nix flake の `vendorHash` の更新忘れ | CI で `nix build .#default` を回して PR 時点で検出 |
| 依存ゼロ方針のドリフト | `go.mod` の require 行を CI で grep して 0 件であることを assert(将来必要になったら方針見直し)|

「依存ゼロを assert する CI チェック」は最初は入れず、設計ドキュメントの方針として明記するに留める。途中で必要になれば後付けする。

## 9. 採用しないもの(明示)

- Rust(理由は §3)
- HTTP フレームワーク(Echo / Gin / Chi / Fiber)
- ORM / DB クライアント(永続化なし)
- メトリクス・トレーシング(個人ローカル前提)
- WebSocket / SSE(callback は単発の HTTP 302 で完結)
- 自前の self-update 機能(`brew upgrade` / `nix flake update` に委ねる)

## 10. 次ステップ

このドキュメント承認後、`writing-plans` skill で実装計画を起こす。実装計画では以下の粒度で TDD タスクに分解する想定:

1. プロジェクト初期化(`go.mod`、ディレクトリ、空の `main.go`)
2. `internal/allowlist` — sanity check と Validate(仕様 §7.3)
3. `internal/store` — TTL マップ
4. `internal/logging` — secret フィルタ slog handler
5. `internal/server` — `/register`、`/auth/callback`、`/healthz`、CORS
6. `cmd/...` — env 読み込みと起動
7. `flake.nix` + devShell
8. `.goreleaser.yaml` + GitHub Actions
9. `README.md`(仕様 §11 の必須記載事項)
