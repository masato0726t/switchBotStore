# switchBotStore

SwitchBot APIからデバイスのステータスを定期収集し、MySQLに保存するデータ収集ツールです。  
複数のSwitchBotアカウントを一括管理でき、初回起動時に全デバイスの初期データを自動取得します。

## 機能

- **複数アカウント対応**: 設定ファイルに複数のSwitchBot APIアカウントを登録可能
- **テーブル自動作成**: 起動時にテーブルが存在しない場合は自動で作成
- **初回データ収集**: 初回起動時に全デバイスのステータスを即座に取得・保存
- **定期収集**: 設定した間隔（デフォルト5分）でデータを収集し続ける
- **グレースフルシャットダウン**: `Ctrl+C` または `SIGTERM` で安全に終了

## テーブル設計

```
api_accounts        SwitchBot APIアカウント（tokenをユニークキーとして管理）
    └── devices     デバイス情報（アカウントごと）
        └── device_status_logs  収集ログ（JSONで保存、デバイス種別を問わず対応）
system_settings     初回起動フラグ等のシステム設定
```

| テーブル | 説明 |
|---|---|
| `api_accounts` | APIトークン・シークレットを管理 |
| `devices` | デバイスID・名前・種別・クラウドサービス有効フラグ等 |
| `device_status_logs` | 収集したステータスをJSONで保存（`recorded_at` でインデックス）|
| `system_settings` | `initial_collect_done` フラグで初回起動を判定 |

## 動作環境

- Go 1.21 以上
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
  "collect_interval_minutes": 5
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
| `collect_interval_minutes` | 収集間隔（分） | `5` |

### 4. ビルドと実行

```bash
# ビルド (config.json と同じディレクトリに出力)
go build -o switchbotstore ./cmd/

# 実行
./switchbotstore
```

## 動作フロー

```
起動
 │
 ├─ config.json 読み込み
 ├─ MySQL 接続
 ├─ テーブル作成（IF NOT EXISTS）
 │
 ├─ [初回起動の場合] 全デバイスのステータスを即座に取得・保存
 │
 └─ 定期収集ループ（collect_interval_minutes 間隔）
      ├─ アカウント1: デバイス一覧取得 → ステータス取得 → DB保存
      ├─ アカウント2: デバイス一覧取得 → ステータス取得 → DB保存
      └─ ...
```

## テスト実行

```bash
go test ./... -v
```

## 注意事項

- **クラウドサービス無効デバイス**: SwitchBotアプリでクラウドサービスを有効にしていないデバイスはステータス取得ができないためスキップされます（デバイス登録は行われます）
- **赤外線リモコン**: SwitchBot API にステータス取得エンドポイントがないため、デバイス登録のみ行われます
- **`config.json` は `.gitignore` 対象**: APIトークンを含むため、Gitには含まれません。`config.json.example` をコピーして使用してください
