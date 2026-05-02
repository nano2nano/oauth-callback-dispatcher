# OAuth Callback Dispatcher — 仕様書

## 1. 概要

> ⚠️ **本ツールは個人ローカルの開発環境専用です。本番環境・ステージング環境・チーム共有インフラでの運用は想定していません。**
> 本番では各環境がそれぞれ独自の `redirect_uri` を持つ通常の OAuth フローを使ってください。
> 詳細は §4 と §11(README 必須記載事項)を参照。

OAuth プロバイダ(Google など)の `redirect_uri` 制約を回避し、**複数の動的環境(git worktree、プレビューデプロイ、devcontainer 等)で OAuth 認証を並行して使えるようにする**、フレームワーク非依存の軽量プロキシサーバ。

## 2. 解決する問題

OAuth プロバイダの典型的な制約により、動的に増減する開発環境で OAuth が使えない:

- Google は `redirect_uri` の wildcard 登録を許可しない
- `*.localhost` のようなサブドメイン形式は登録不可(public TLD 必須)
- `redirect_uri` は事前登録した値と完全一致が必要
- worktree のような高頻度生成環境で都度 Google Console を編集するのは非現実的

## 3. 解決アプローチ

OAuth プロバイダには **単一の固定 callback URL** だけを登録し、そこに常駐するディスパッチャが認可コードを本来の環境にルーティングする。

ディスパッチャは認可開始時に `state → origin` のマッピングをメモリ内に保持し、callback 受信時に state をキーとして元の origin を解決して 302 リダイレクトする。

## 4. 非機能要件

- **フレームワーク非依存**: 任意の言語・フレームワーク(Next.js、Rails、Django、Phoenix 等)から利用可能
- **クライアントライブラリ不要**: アプリ側は HTTP 呼び出しのみで完結
- **トークンを保持しない**: ディスパッチャは `code` を素通しするだけで、`access_token` や `refresh_token`、`client_secret` には触れない
- **開発環境専用(本番運用は明示的にスコープ外)**: 本ツールは可用性・監査ログ・耐障害性・マルチテナント分離のいずれも提供しない。本番では各環境がそれぞれ独自の `redirect_uri` を持つ通常の OAuth フローを使う
- **個人完結**: 1 開発者のローカルマシンで動かすことを想定。チーム共有は将来課題

## 5. システム構成

```
[各 worktree / 環境]              [ディスパッチャ]                [OAuth Provider]
feat-a.myapp.localhost            oauth-dispatcher                Google etc.
feat-b.myapp.localhost     ←→     .myapp.localhost          ←→
exp-foo.myapp.localhost           (固定サブドメイン)
```

> ※ 上図は **個人ローカル開発専用** の構成。チーム間で共有したり本番にデプロイしたりしない。

ディスパッチャは OAuth Provider に登録される唯一の `redirect_uri` を提供する。各環境は自身の `redirect_uri` 設定をディスパッチャの URL に向けるだけで利用できる。

## 6. ディスパッチャの API

### 6.1 `POST /register`

認可開始の直前にクライアントが呼び出す。state と origin の対応をディスパッチャに記録する。

**リクエスト:**

```json
{
  "state": "predefined-csrf-token-from-client",
  "origin": "https://feat-a.myapp.localhost"
}
```

**レスポンス:**

- 成功: `204 No Content`
- origin がホワイトリスト外: `400 Bad Request`
- state または origin の欠如: `400 Bad Request`
- state の重複(既に同じ state が登録済み): `409 Conflict`

**CORS:**

- ホワイトリストに合致する origin からのリクエストを許可
- `Access-Control-Allow-Origin` を動的に設定
- preflight (`OPTIONS`) に対応
- `Content-Type: application/json` を許可
- ※ CORS と `Origin` ヘッダは **信頼の根拠ではない**(§7.3 参照)。ブラウザ利便性のために設定するのみ

### 6.2 `GET /auth/callback`

OAuth Provider から認可コードを受け取り、本来の環境にリダイレクトする。

**クエリパラメータ:**

- `state` (必須): クライアントが `/register` で登録した値と同じ
- `code` (任意): OAuth 認可コード(成功時)
- `error` (任意): OAuth エラーコード(失敗時)
- その他のパラメータ(`scope`, `error_description` 等)もすべて素通しする

**レスポンス:**

- state 該当エントリあり: `302 Found` で `{origin}/auth/callback?{全クエリパラメータ}` へリダイレクト
- state 該当エントリなし or TTL 切れ: `400 Bad Request`(エラー説明 HTML)
- リダイレクト後、該当エントリは即座に削除する(replay 防止)

### 6.3 `GET /healthz`

死活監視用エンドポイント。

**レスポンス:** `200 OK`、ボディ `ok`

## 7. ディスパッチャの内部仕様

### 7.1 マッピング保持

- データ構造: in-memory の連想配列(キー: `state`、値: `{ origin, expiresAt }`)
- TTL: 600 秒(10 分)
- 掃除: 60 秒ごとに走るバックグラウンドジョブで失効エントリを削除
- callback 成功時は対応エントリを即削除(replay 防止)
- 永続化なし。ディスパッチャ再起動でマッピングは消える(個人ローカル前提では許容)

### 7.2 origin ホワイトリスト

- 環境変数 `ALLOWED_ORIGIN_PATTERN` で正規表現を受け取る
- 例: `^https://[a-z0-9-]+\.myapp\.localhost(:\d+)?$`
- 環境変数未設定時はディスパッチャを起動失敗させる(危険なデフォルトを避ける)
- すべての受け入れ判定(`/register` の origin、`/auth/callback` のリダイレクト先)で必ず適用

### 7.3 セキュリティ要件

**必須:**

- `/register` の origin パラメータは `ALLOWED_ORIGIN_PATTERN` で厳密検証
- callback リダイレクト時、マップから取り出した origin に対しても再度 `ALLOWED_ORIGIN_PATTERN` を検証(防御的プログラミング、Authentik CVE-2024-52289 と同型のミスを二段で防ぐ)
- state は OAuth 仕様通り、クライアントが推測不可能なランダム値として生成する前提
- ディスパッチャは state を改竄しない、検証もしない(クライアント側の既存 state 検証ロジックを尊重)
- `code` パラメータをログに出力しない
- HTTPS 推奨だが、ローカル開発の HTTP も許容(portless 環境では HTTPS が標準)

**`ALLOWED_ORIGIN_PATTERN` の起動時 sanity check(strict — 失敗時は起動を中止):**

正規表現の典型ミスは過去に CVE 化している(例: Authentik CVE-2024-52289、ドット未エスケープによる検証 bypass)。ディスパッチャは起動時に以下のチェックを行い、いずれかに該当するパターンであれば **起動を失敗させる**:

- パターンが `^` で始まり `$` で終わっていない(部分一致 bypass の可能性)
- リテラルのドット `.` がエスケープされていない(`https://app\.example\.com` は OK、`https://app.example.com` は NG)
- 内部の sanity test として、明らかな bypass 候補(`https://evil.com`、`https://attacker.com.myapp.localhost.evil.com`、`https://myapp.localhost.evil.com` 等)を入力したときに reject されるか検査
- `https?://` のような平文 HTTP も許容する書き方は **warn ログのみ**(完全禁止はしない、portless 環境で HTTP の場合があるため)

**信頼境界に関する明示:**

- CORS ヘッダおよびリクエストの `Origin` ヘッダは **信頼の根拠ではない**(サーバ直叩きで容易に偽装可能)。信頼の根拠は `/register` の body の `origin` フィールドに対する `ALLOWED_ORIGIN_PATTERN` 適用のみ
- 同一マシン上の他プロセスがマップに対して race / 盗聴を行う可能性は防御対象外(個人ローカル前提)

**state の取り扱いと DoS 警告:**

- state は client 側で cryptographically random に生成される前提とする
- 予測可能な state を使うと、攻撃者が先回りで `/register` を呼んで `409 Conflict` を発生させ、正規の register を拒否する DoS が成立する
- 個人ローカル前提では許容するが、README に明示的に警告する

**PKCE 強く推奨:**

- HTTP 上で運用する場合(portless 等)、client 側で **PKCE の利用を強く推奨**(RFC 9700 準拠)
- code が傍受されても token 交換が成立しないよう、攻撃面を縮小する

**禁止事項:**

- 任意 URL へのリダイレクト機能を提供しない(ホワイトリスト外には絶対に飛ばさない)
- token 交換を代行しない(ディスパッチャは `code` の運搬のみ)
- マッピングを永続化しない(ディスクや外部 KV ストアへの書き出しなし)
- state、code、access_token、refresh_token、client_secret、その他認証情報をログに出さない

### 7.4 構造化検証併用(将来拡張・本仕様には含めない)

regex 一本ではミスが起きやすいため、将来的には URL パース → ホスト末尾一致による構造化検証(例: `ALLOWED_HOST_SUFFIXES=myapp.localhost,foo.myapp.localhost`)の併用も検討する。本仕様では実装しないが、設計上後付け可能であるよう内部 API を分離しておく。

## 8. 環境変数

| 変数名 | 必須 | 説明 |
|---|---|---|
| `PORT` | 任意 | リッスンポート(デフォルト 8888) |
| `ALLOWED_ORIGIN_PATTERN` | 必須 | リダイレクト許可 origin の正規表現。起動時 sanity check に通る必要あり(§7.3) |
| `STATE_TTL_SECONDS` | 任意 | state エントリの TTL(デフォルト 600) |
| `LOG_LEVEL` | 任意 | `debug` / `info` / `warn` / `error`(デフォルト `info`) |

## 9. クライアント側の利用方法

ディスパッチャは特定のクライアントライブラリを必要としない。任意のアプリは以下の 2 点だけを行う。

### 9.1 redirect_uri の差し替え

OAuth 認可 URL の `redirect_uri` パラメータと、トークン交換時の `redirect_uri` パラメータを、ディスパッチャの callback URL(例: `https://oauth-dispatcher.myapp.localhost/auth/callback`)に変更する。環境変数経由で吸収する想定。

### 9.2 認可開始直前の登録 POST

認可 URL へ遷移する直前に、ディスパッチャの `/register` へ以下の HTTP リクエストを送る:

```
POST {DISPATCHER_URL}/register
Content-Type: application/json

{
  "state":  "<認可 URL に渡す state と同じ値>",
  "origin": "<クライアントの origin>"
}
```

`204 No Content` を受け取ってから認可 URL へ遷移する。それ以外のロジック(state 生成、callback での state 検証、code の BE への POST、token 交換、JWT 発行、sessionID 紐付け等)は **完全に既存のまま** 動作する。

### 9.3 BE 側の差分

BE がトークン交換時に OAuth Provider へ送る `redirect_uri` も、ディスパッチャの URL に揃える(完全一致が要求されるため)。環境変数による切り替えで対応。

## 10. テスト要件

最低限カバーすべきテストケース:

**正常系:**

- `/register` でホワイトリスト合致 origin を登録できる
- `/auth/callback` で登録済み state に対して正しい origin にリダイレクトされる
- callback 時のクエリパラメータ(code、scope 等)が素通しでリダイレクト先に渡される
- callback 成功後、マッピングエントリが削除される(同じ state での 2 回目の callback は失敗する)

**異常系:**

- ホワイトリスト外の origin で `/register` が 400 を返す
- 未登録の state で `/auth/callback` が 400 を返す
- TTL 切れ後の state で `/auth/callback` が 400 を返す
- マッピング取り出し後の origin がホワイトリストに合致しない場合 400 を返す(防御層の二重チェック)
- 重複 state での `/register` が 409 を返す

**設定:**

- `ALLOWED_ORIGIN_PATTERN` 未設定で起動失敗する
- `ALLOWED_ORIGIN_PATTERN` の正規表現が compile 不可能な場合に起動失敗する

**起動時 sanity check(§7.3):**

- 未 anchor のパターン(例: `https://[a-z0-9-]+\.myapp\.localhost`)で起動失敗する
- ドット未エスケープのパターン(例: `^https://[a-z0-9-]+.myapp.localhost$`)で起動失敗する
- bypass 候補入力(`https://evil.com`、`https://attacker.com.myapp.localhost.evil.com` 等)を reject できないパターンで起動失敗する
- `https?://` を許容するパターンでは起動成功するが warn ログが出る

**ログ:**

- LOG_LEVEL=debug でも `code`、`state`、`access_token`、`refresh_token` が一切ログに出ないことを検査する

## 11. ドキュメント要件

`README.md` に最低限含める内容:

> ⚠️ **本ツールは個人ローカルの開発環境専用です。本番運用は禁止します。**
> ※ README の冒頭に上記の警告を必ず明記する

1. **問題提起**: なぜこのツールが必要か(OAuth の wildcard 不可問題、worktree 並列開発)
2. **解決アプローチ**: state ベースのメモリマップ方式の概要図
3. **クイックスタート**:
   - ディスパッチャの起動方法
   - OAuth Provider への登録 URL 例
   - クライアント側のコード差分(2 箇所)
4. **セキュリティモデル**:
   - 何を保護するか / しないか
   - **本番禁止の理由**: 可用性・耐障害性・監査ログ・マルチテナント分離のいずれも提供せず、in-memory のため再起動でフロー消失する。本番では各環境ごとに通常の OAuth フローを使う
   - origin ホワイトリストの重要性と、regex の典型ミス例(unescaped dot、未 anchor、`https?` での HTTP 許容)。CVE の実例(Authentik CVE-2024-52289)へのリンク
   - **PKCE の併用を強く推奨**(HTTP 環境では特に。RFC 9700 準拠)
   - CORS ヘッダ・`Origin` ヘッダは信頼の根拠ではないこと
   - 同一マシン上の悪意あるプロセスからの保護はしない(個人ローカル前提)
5. **既知の制約**:
   - 本番環境では使わない
   - ディスパッチャ再起動でフロー進行中のセッションが失われる
   - チーム共有時の注意点(マッピングのプロセス間共有が必要)
   - 同一ホスト上の他プロセスからのマップ盗聴・改竄は防御対象外
   - state を予測可能にすると DoS リスクあり(client は cryptographically random な state を必ず使うこと)
6. **portless との組み合わせ例**:
   - portless が払い出す `*.myapp.localhost` 配下にディスパッチャを置く
   - `ALLOWED_ORIGIN_PATTERN` の設定例

## 12. ライセンス

MIT License
