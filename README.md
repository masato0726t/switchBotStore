# switchBotStore

SwitchBot APIからデバイスのステータスを収集し、MySQLに保存するデータ収集ツールです。  
複数のSwitchBotアカウントを一括管理できます。  
実行するたびに1回収集して終了するため、cron やタスクスケジューラと組み合わせて定期実行します。

## 機能

- **複数アカウント対応**: 設定ファイルに複数のSwitchBot APIアカウントを登録可能
- **1回実行型**: 起動するたびに収集を1回行って終了する。定期実行はOSのcron/タスクスケジューラに委ねる
- **マイグレーション**: 起動時に goose で未適用のマイグレーションを自動適用（実行ファイルに同梱）
- **スキーマ検証**: マイグレーション後、コードが期待するテーブル・カラムが存在するかを起動時に検証
- **構造化ログ**: JSON 形式で標準エラー出力とファイルに出力（`log_dir/YYYY-MM-DD.log`）。`log_level` で出力する下限レベルを設定可能

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
  "log_dir": "logs"
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

`log_dir` を指定しない場合、または指定先への書き込みに失敗した場合は、標準エラー出力のみにログを出力し、ログファイルは作成しません。

`log_level` を指定しない場合は `info` として扱われます。デバイス1台ごとの処理結果は `info` で出力されるため、収集対象が多い場合は `warn` を指定するとログ量を抑えられます。

**`error` を指定する場合の注意**: ログファイルへの書き込みに失敗した際の警告（`warn`）も抑止されます。ログ出力先の不備に気づけなくなるため、通常は `warn` までに留めることを推奨します。

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
| `0` | 正常終了。デバイス1台の失敗（オフライン等）は含まれる |
| `1` | 設定・DB接続・マイグレーション・スキーマ検証の失敗、またはアカウント単位の致命的エラー（認証失敗、デバイス一覧の取得失敗など）が1件以上 |

cron やタスクスケジューラから失敗を検知できるよう、アカウント単位の失敗は終了コード 1 になります。

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

設計の詳細は `docs/superpowers/specs/2026-07-29-clean-architecture-refactoring-design.md` を参照してください。

## テスト実行

```bash
go test ./... -v
```

## 注意事項

- **クラウドサービス無効デバイス**: SwitchBotアプリでクラウドサービスを有効にしていないデバイスはステータス取得ができないためスキップされます（デバイス登録は行われます）
- **赤外線リモコン**: SwitchBot API にステータス取得エンドポイントがないため、デバイス登録のみ行われます
- **`config.json` は `.gitignore` 対象**: APIトークンを含むため、Gitには含まれません。`config.json.example` をコピーして使用してください
- **赤外線リモコンの種別**: SwitchBot API は赤外線リモコンの種別を `remoteType` で返しますが、本ツールは `deviceType` を読むため `devices.device_type` は空文字で保存されます（現行の挙動を維持）
