# switchBotStore

SwitchBot APIからデバイスのステータスを収集し、MySQLに保存するデータ収集ツールです。  
複数のSwitchBotアカウントを一括管理できます。  
実行するたびに1回収集して終了するため、cron やタスクスケジューラと組み合わせて定期実行します。

## 機能

- **複数アカウント対応**: 設定ファイルに複数のSwitchBot APIアカウントを登録可能
- **1回実行型**: 起動するたびに収集を1回行って終了する。定期実行はOSのcron/タスクスケジューラに委ねる
- **マイグレーション**: 起動時に goose で未適用のマイグレーションを自動適用（実行ファイルに同梱）
- **スキーマ検証**: マイグレーション後、コードが期待するテーブル・カラムが存在するかを起動時に検証
- **構造化ログ**: JSON 形式で標準エラー出力とファイルに出力（`log_dir/YYYY-MM-DD.log`）。`log_level` で出力する下限レベルを設定可能。API がエラーを返したときは HTTP ステータス・応答本文・レート制限ヘッダーを個別のフィールドとして残す
- **レート制限への対応**: `429 Too Many Requests` を受けたら警告として記録し、終了コード `0` のまま次の実行まで待機時間を空ける（1分から倍にして最大15分）

## テーブル設計

```
api_accounts        SwitchBot APIアカウント（tokenをユニークキーとして管理）
    └── devices     デバイス情報（アカウントごと）
        └── device_status_logs  収集ログ（JSONで保存、デバイス種別を問わず対応）
```

| テーブル | 説明 |
|---|---|
| `api_accounts` | APIトークン・シークレットを管理 |
| `devices` | デバイスID・名前・種別・クラウドサービス有効フラグ等 |
| `device_status_logs` | 収集したステータスをJSONで保存（`recorded_at` でインデックス）|
| `system_settings` | 現在はコードから参照されない旧テーブル（既存データ保持のため残置）|

## 動作環境

- Go 1.26.3 以上
- MySQL 5.7 以上（JSON型サポートのため）

## セットアップ

### 1. SwitchBot APIトークンの取得

1. SwitchBotアプリ → **プロフィール** → **設定** → **アプリバージョン** を7回タップ
2. **開発者向けオプション** → **トークン** と **シークレット** をコピー

### 2. MySQLデータベースの作成

```sql
CREATE DATABASE switchbot_db CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
```

### 3. 設定ファイルの作成

```bash
cp config.json.example config.json
```

`config.json` を編集して接続情報とAPIトークンを設定します。

```json
{
  "database": {
    "host": "localhost",
    "port": 3306,
    "user": "root",
    "password": "your_password",
    "name": "switchbot_db"
  },
  "accounts": [
    {
      "name": "メインアカウント",
      "token": "your_switchbot_token",
      "secret": "your_switchbot_secret"
    }
  ],
  "log_dir": "logs",
  "device_list_cache_ttl": "24h"
}
```

| フィールド | 説明 | デフォルト |
|---|---|---|
| `database.host` | MySQLホスト | 必須 |
| `database.port` | MySQLポート | `3306` |
| `database.user` | MySQLユーザー | 必須 |
| `database.password` | MySQLパスワード | 必須 |
| `database.name` | データベース名 | 必須 |
| `accounts[].name` | アカウント識別名（任意） | 必須 |
| `accounts[].token` | SwitchBot APIトークン | 必須 |
| `accounts[].secret` | SwitchBot APIシークレット | 必須 |
| `log_dir` | ログ出力ディレクトリ | 未指定（デフォルト値なし） |
| `log_level` | 出力するログの下限レベル。`debug` / `info` / `warn` / `error` のいずれか（大文字小文字は問いません） | 未指定（`info` 相当） |
| `device_list_cache_ttl` | デバイス一覧を API から取り直す間隔（`time.ParseDuration` の書式。例 `24h` / `30m`）。`"0"` でキャッシュを使わない | `24h` |

#### デバイス一覧のキャッシュ

1回の実行で消費する SwitchBot API の呼び出しは「デバイス一覧 1回 + ステータス取得（対象デバイス数）回」です。デバイス構成はめったに変わらないため、一度取得した一覧は `devices` テーブルに残っているものを再利用し、`device_list_cache_ttl` を過ぎたときだけ API から取り直します。1分間隔で実行する場合、`/v1.1/devices` の呼び出しは 1,440回/日 から 1回/日 まで減ります。

キャッシュを使った実行では `devices` テーブルへの登録もやり直しません。デバイスを追加・改名した直後に反映させたい場合は、期限を待たずに次のコマンドで取り直せます。

```bash
./switchbotstore -refresh-devices
```

一覧を API から取り直した実行のうち、全デバイスを `devices` へ登録できたときだけ同期時刻（`api_accounts.devices_synced_at`）を進めます。1台でも登録に失敗した場合は時刻を据え置き、次回もう一度 API から取り直します。

#### レート制限（HTTP 429）の扱い

SwitchBot API が呼び出し回数の制限で `429 Too Many Requests` を返したとき、このツールは次のように動きます。

1. **終了コードは `0` のまま**にします。制限はこちら側の不具合ではなく、待てば直るものだからです。毎分 `1` を返すと systemd がユニットを `failed` にし、制限が続くあいだ警報が鳴り続けてしまいます。
2. **ログには `WARN` で残します。** 応答の内訳（HTTP ステータス・応答本文・`X-RateLimit-Remaining`）をそれぞれ独立したフィールドとして出すので、ログだけを見て状況を判断できます。
3. **次の実行までの待機時間を空けます。** 1分から始めて2分・4分・8分と倍にし、**15分で頭打ち**にします。待機中は API を1回も呼びません。
4. **制限が解けたら自動で戻ります。** 待機が明けた回に成功すれば、待機状態を消して `INFO` で「レート制限から復帰しました」を出します。

待機時間の上限を15分にしているのは、同じホストで動く `auto-air-conditioner` がセンサー値を有効とみなす上限が20分（`sensor_max_age_min`）だからです。上限をそれより短くしておけば、復帰に最大15分気づかなくても、次の収集で入る値はエアコン制御にとって十分新しいままです。

待機の状態は `api_accounts` の3列（`rate_limited_until` / `rate_limited_since` / `rate_limit_failures`）に持ちます。1回実行して終了する作りなので、待機の状態をプロセス内に持てないためです。現在の状態は次で確認できます。

```sql
SELECT name, rate_limited_until, rate_limited_since, rate_limit_failures FROM api_accounts;
```

レート制限とみなすのは **HTTP 429 だけ**です。応答 JSON の `statusCode` が `190` のものは、デバイスの内部エラーやオフラインでも返るため、これを制限とみなすとデバイスの故障を「待てば直るもの」として黙って見送ってしまいます。

`log_dir` を指定しない場合、または指定先への書き込みに失敗した場合は、標準エラー出力のみにログを出力し、ログファイルは作成しません。

`log_level` を指定しない場合は `info` として扱われます。デバイス1台ごとの処理結果は `info` で出力されるため、収集対象が多い場合は `warn` を指定するとログ量を抑えられます。

現時点で `debug` レベルのログ出力は実装されていないため、`debug` を指定しても `info` と同じ内容になります。

**`error` を指定する場合の注意**: ログファイルへの書き込みに失敗した際の警告（`warn`）に加え、デバイス単位の処理失敗やDB接続のクローズ失敗といった `warn` 出力もすべて抑止されます。特にデバイス単位の失敗は終了コードに影響しないため（終了コードは `0` のまま）、`error` を指定しているとログからもcronの終了コード監視からも検知できなくなります。通常は `warn` までに留めることを推奨します。

### 4. ビルドと実行

```bash
# ビルド (config.json と同じディレクトリに出力)
go build ./cmd/switchbotstore/

# 手動実行（1回収集して終了）
./switchbotstore
```

エントリポイントが `cmd/switchbotstore/` に移ったため、バイナリ名はディレクトリ名から決まり、`-o` の指定は不要です。

### 5. 定期実行の設定

#### Linux / Mac（cron）

```bash
crontab -e
```

以下を追加します（5分ごとに実行する例）。

```cron
*/5 * * * * cd /path/to && ./switchbotstore >> /path/to/logs/cron.log 2>&1
```

`config.json` は実行ファイルの位置ではなく**カレントワーキングディレクトリ**からの相対パスで解決されるため、`cd` で作業ディレクトリを `config.json` と同じ場所に移してから実行します。

設定ファイルの読み込みやログの初期化に失敗した場合は、ログファイルがまだ無いため標準エラー出力にのみエラーが出力されます。それ以降の失敗（DB接続・マイグレーション・スキーマ検証・収集）はログファイルと標準エラー出力の両方に出力されるため、起動失敗の原因を確実に残すには `2>&1` のリダイレクトが必要です。

#### Windows（タスクスケジューラ）

1. タスクスケジューラを開く
2. **タスクの作成** → **トリガー** → 「5分ごと」に設定
3. **操作** → `switchbotstore.exe` のフルパスを指定（作業ディレクトリも `config.json` と同じ場所に設定）

## 動作フロー

```
起動（cron/タスクスケジューラにより定期呼び出し）
 │
 ├─ config.json 読み込み・検証
 ├─ ログ出力先の設定
 ├─ MySQL 接続
 ├─ マイグレーション適用（goose）
 ├─ スキーマ検証
 │
 └─ アカウントごとに
      ├─ アカウント登録
      ├─ デバイス一覧取得
      └─ デバイスごとにステータス取得 → DB保存
```

### 終了コード

| コード | 条件 |
|---|---|
| `0` | 正常終了。デバイス1台の失敗（オフライン等）と、レート制限（HTTP 429）による見送りは含まれる |
| `1` | 設定・DB接続・マイグレーション・スキーマ検証の失敗、またはアカウント単位の致命的エラー（認証失敗、デバイス一覧の取得失敗など）が1件以上 |

cron やタスクスケジューラから失敗を検知できるよう、アカウント単位の失敗は終了コード 1 になります。

レート制限だけは例外で、終了コード `0` を返します。制限はこちら側の不具合ではなく待てば直るもので、毎分 `1` を返すと警報が鳴り続けるためです。見送ったことは `WARN` のログ行として残るので、ログ側で検知してください（詳細は「レート制限（HTTP 429）の扱い」）。

## アーキテクチャ

クリーンアーキテクチャの依存規則に従い、内側は外側を一切 import しません。

```
  cmd
   ├──> infra
   └──> adapter ──> usecase ──> domain
           │                       ▲
           └───────────────────────┘
```

矢印は依存の向き（起点が終点に依存する）です。右にあるものほど内側です（`adapter` は `usecase` 経由だけでなく `domain` にも直接依存します）。

| ディレクトリ | 責務 | import してよいもの |
|---|---|---|
| `internal/domain` | ドメインモデル | 標準ライブラリのみ |
| `internal/usecase` | ユースケースと出力ポート。ログは吐かず結果を値で返す | `domain` のみ |
| `internal/adapter` | SwitchBot API / GORM 永続化 / ログ整形 | `usecase`, `domain` |
| `internal/infra` | 設定・ログ・DB接続 | 内部パッケージを import しない |
| `cmd/switchbotstore` | コンポジションルート（配線） | すべて |

DB スキーマの正は `internal/infra/mysql/migrations/*.sql`（goose）にあります。
`internal/adapter/persistence` の GORM モデルはそこへのマッピングにすぎず、
両者のズレは起動時の `VerifySchema` が検出します。

## テスト実行

```bash
go test ./... -v
```

## 注意事項

- **クラウドサービス無効デバイス**: SwitchBotアプリでクラウドサービスを有効にしていないデバイスはステータス取得ができないためスキップされます（デバイス登録は行われます）
- **赤外線リモコン**: SwitchBot API にステータス取得エンドポイントがないため、デバイス登録のみ行われます
- **`config.json` は `.gitignore` 対象**: APIトークンを含むため、Gitには含まれません。`config.json.example` をコピーして使用してください
- **デバイス種別の保存**: SwitchBot API は物理デバイスの種別を `deviceType`、赤外線リモコンの種別を `remoteType` で返します。本ツールは両方を読んで `devices.device_type` に保存します（`Air Conditioner` / `DIY Light` など）
