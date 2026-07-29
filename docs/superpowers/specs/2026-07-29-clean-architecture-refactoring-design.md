# switchBotStore クリーンアーキテクチャ・リファクタリング設計

作成日: 2026-07-29
対象: `switchBotStore`（SwitchBot API からデバイス状態を収集し MySQL に保存する cron 起動型バッチ）

## 目的

1. ライブラリを採用して自前実装を減らし、読み手の認知負荷を下げる
2. クリーンアーキテクチャの依存規則に従い、依存の逆流を解消する
3. 外部から見た振る舞いは原則維持する（変更する2点は「振る舞いの変更」節に明記）

## 前提と制約

| 項目 | 内容 |
|---|---|
| DB の状態 | **実データあり。維持必須** |
| スキーマ変更 | **今回は行わない。** 既存カラム構成を厳守する |
| 今後の方向性 | デバイス制御（書き込み系ユースケース）を追加予定。境界はそれを見越して引く |
| ブランチ | `refactor/clean-architecture`（`master` から分岐） |

---

## 1. 現状の問題点

### 1.1 依存の逆流（クリーンアーキテクチャ違反）

`repository.Repository`（永続化の抽象＝内側）が `switchbot.DeviceInfo`（外部 API の DTO＝外側）を引数に取っている。

```go
// internal/repository/repository.go:14
UpsertDevice(accountID int64, dev switchbot.DeviceInfo, isVirtualInfrared bool) (int64, error)
```

内側の契約が外部サービスのレスポンス形式に縛られている。ドメインモデルが存在しない。

### 1.2 動かないコードのために払われている複雑さ

`internal/logger/logger.go`（106行）は日次ローテーションを goroutine + mutex + timer + `stopCh` + `sync.Once` で自前実装している。しかし本アプリは cron 起動で数秒で終了するため、**「翌日0時を待って切り替える goroutine」は実運用で一度も発火しない**。

### 1.3 ログ出力先の二段構え

`cmd/main.go:26-53` は「暫定ディレクトリ `logs/` で開く → config を読む → 違ったら閉じて開き直す」という二段構え。`closeLog` を間接参照で `defer` するトリック（21行目のコメント）まで必要になっている。

### 1.4 `log.Fatalf` により `defer` が実行されない（不具合）

`cmd/main.go` の 34, 57, 66, 80 行目で `log.Fatalf` を使用している。`log.Fatalf` は `os.Exit` を呼ぶため **`defer` が一切実行されない**。設定読み込み以降で失敗すると `defer closeLog()`（ログファイル）も `defer db.Close()`（DB 接続）も走らずに終了する。

### 1.5 実質無効化されている「初回起動」機構

```go
// internal/collector/collector.go:23-26
func (c *Collector) InitialCollect() error {
	log.Println("=== 初回データ収集を開始します ===")
	return c.Collect()   // Collect() をそのまま呼ぶだけ
}
```

`cmd/main.go:83-94` でも「初回なら `InitialCollect()` して `return`」だが、2回目以降も `Collect()` して `return` するため**動作は完全に同一**。`system_settings` / `IsFirstRun` / `MarkFirstRunDone` / `InitialCollect` の一式（約35行）は、ログが1行増える以外の効果を持たない。

### 1.6 その他

| # | 問題 | 該当箇所 |
|---|---|---|
| a | エラーラップが `%v` のため `errors.Is` / `errors.As` が機能しない | 全ファイル |
| b | アカウント単位のエラーを `lastErr` に上書きするため最後の1件しか残らない | `collector.go:36` |
| c | `context.Context` が皆無。HTTP・DB ともにキャンセル/タイムアウトを伝播できない | 全ファイル |
| d | `switchbot.Client` が `AccountToken()` / `AccountSecret()` を公開し、認証情報をゲッターで取り出している | `client.go:19-25` |
| e | `UpsertDevice(..., isVirtualInfrared bool)` の boolean trap | `repository.go:14` |
| f | `DeviceList` と `InfraredRemoteList` を別々のループで二重に処理 | `collector.go:56, 84` |
| g | ユースケースに `log.Printf` が8箇所散在し、責務が混在 | `collector.go` |
| h | DDL が Go の文字列リテラルで、バージョン管理されたマイグレーションがない | `database.go:36-103` |
| i | 手書きの `if` によるバリデーションとテストアサーション | `config.go:41-54` ほか |
| j | HTTP クライアントをアカウント数だけ生成している | `main.go:69-73` |

---

## 2. 採用するライブラリ

| 用途 | ライブラリ | 効果 |
|---|---|---|
| ORM | `gorm.io/gorm` + `gorm.io/driver/mysql` | 生 SQL の Upsert を `clause.OnConflict` で表現。`rows.Scan` の手書きが消える |
| マイグレーション | `github.com/pressly/goose/v3`（**ライブラリとして使用**） | DDL を `migrations/*.sql` へ。`embed.FS` で実行ファイルに同梱するため追加バイナリ不要 |
| 設定検証 | `github.com/go-playground/validator/v10` | `validate:"required"` タグ化。手書き `if` 14行が消える |
| テスト | `github.com/stretchr/testify` | `require` / `assert` でアサーションが宣言的になる |
| ログ | `log/slog`（Go 標準・追加依存なし） | 構造化ログ。`logger.go` 106行 → 約25行 |
| エラー集約 | `errors.Join`（Go 標準） | 部分失敗を全件保持 |

`go.mod` の `go` ディレクティブは、上記ライブラリが要求する最低バージョンに合わせて更新する（実装時に実際の要求値を確認して確定する。現在は `go 1.21`、インストール済みは Go 1.26.3）。

### 採用しなかったもの と その理由

| 候補 | 不採用の理由 |
|---|---|
| `lumberjack`（ログローテーション） | **cron で数秒終了するプロセスではサイズ肥大が起きず、働かない。** 日付をファイル名にするだけで日次分割は達成される |
| GORM の `AutoMigrate` | **実データがあるため、スキーマを自動変更されると危険。** goose を唯一の真実とする |
| `google/wire`（DI） | この規模では手書きのコンポジションルートの方が読みやすい |
| `spf13/cobra`（CLI） | 引数を取らないため不要 |
| `testcontainers-go` | DB 統合テストのために CI・実行時間のコストが見合わない |
| `testify/mock` | 手書きフェイクの方が短く読みやすい。現状の書き方でもある |

### GORM 採用に伴うリスクとその手当て

GORM を採用すると「GORM の構造体タグ」と「goose の DDL」でスキーマを二重管理することになる。この破綻を早期に検出するため、**マイグレーション直後にスキーマ整合性チェックを実行する**（詳細は 5.3 節）。

---

## 3. ディレクトリ構成

```
switchBotStore/
├── cmd/switchbotstore/main.go          # コンポジションルートのみ (~45行)
├── internal/
│   ├── domain/                         # 【最内層】標準ライブラリのみに依存
│   │   ├── account.go                  #   Account, AccountID, Credential
│   │   ├── device.go                   #   Device, DeviceID, DeviceRecordID, DeviceKind
│   │   ├── status.go                   #   StatusPayload, StatusSnapshot
│   │   └── errors.go                   #   ドメインエラー
│   ├── usecase/                        # 【ユースケース層】domain のみに依存
│   │   ├── port.go                     #   出力ポート（インターフェース定義）
│   │   ├── collect_status.go           #   ステータス収集ユースケース
│   │   └── report.go                   #   CollectReport ほか実行結果の値オブジェクト
│   ├── adapter/                        # 【アダプタ層】usecase + domain に依存
│   │   ├── switchbot/
│   │   │   ├── client.go               #   HTTP 呼び出し
│   │   │   ├── signer.go               #   HMAC-SHA256 署名
│   │   │   ├── dto.go                  #   API レスポンス構造体
│   │   │   └── mapper.go               #   DTO → domain 変換
│   │   ├── persistence/
│   │   │   ├── model.go                #   GORM モデル（永続化専用）
│   │   │   ├── mapper.go               #   GORM モデル ⇄ domain 変換
│   │   │   ├── store.go                #   ポート実装
│   │   │   └── verify.go               #   スキーマ整合性チェック
│   │   └── presenter/
│   │       └── slog_presenter.go       #   CollectReport → 構造化ログ
│   └── infra/                          # 【最外層】フレームワーク・ドライバ
│       ├── config/config.go            #   JSON 読込 + validator
│       ├── logging/logging.go          #   slog セットアップ
│       └── mysql/
│           ├── connect.go              #   GORM 接続
│           ├── migrate.go              #   goose 実行
│           └── migrations/*.sql        #   embed.FS で同梱
└── docs/superpowers/specs/
```

### 依存規則（Dependency Rule）

```
  infra ──────┐
              ▼
  adapter ──> usecase ──> domain
     ▲                       ▲
     └───────────────────────┘
```

| 層 | import してよい内部パッケージ | import してよい外部依存 |
|---|---|---|
| `domain` | なし | 標準ライブラリのみ（`encoding/json`, `time`） |
| `usecase` | `domain` のみ | 標準ライブラリのみ（`context`, `errors`, `time`）。**ログ関連は不可** |
| `adapter` | `usecase`, `domain` | 各種ライブラリ（GORM, slog ほか） |
| `infra` | なし | 外部ライブラリ（GORM, goose, validator, slog） |
| `cmd` | すべて（配線のため） | すべて |

**内側は外側を一切知らない。**

### 現行ファイルとの対応

| 現行 | 移行先 |
|---|---|
| `cmd/main.go` | `cmd/switchbotstore/main.go`（配線のみに縮小） |
| `internal/config/config.go` | `internal/infra/config/config.go` |
| `internal/logger/logger.go` | `internal/infra/logging/logging.go`（大幅縮小） |
| `internal/database/database.go` | `internal/infra/mysql/{connect,migrate}.go` + `migrations/*.sql` |
| `internal/repository/repository.go` | `internal/adapter/persistence/*` |
| `internal/switchbot/client.go` | `internal/adapter/switchbot/*` |
| `internal/collector/collector.go` | `internal/usecase/collect_status.go` + `internal/adapter/presenter/slog_presenter.go` |
| （新規） | `internal/domain/*` |

---

## 4. ドメインモデルとポート

### 4.1 `domain`

```go
// account.go
type AccountID int64
type Credential struct{ Token, Secret string }
type Account struct {
    Name       string
    Credential Credential
}

// device.go
type DeviceID string        // SwitchBot 上のデバイスID
type DeviceRecordID int64   // devices.id（DB の採番）

type DeviceKind int
const (
    DeviceKindPhysical       DeviceKind = iota  // 物理デバイス
    DeviceKindInfraredRemote                     // 仮想赤外線リモコン
)

type Device struct {
    ID                  DeviceID
    Name                string
    Type                string
    HubID               DeviceID
    Kind                DeviceKind
    CloudServiceEnabled bool
}

// StatusReadable はステータス取得が可能かを表すドメインルール。
// 赤外線リモコンには状態取得 API が存在せず、
// クラウドサービス無効のデバイスは API から状態を取得できない。
func (d Device) StatusReadable() bool {
    return d.Kind == DeviceKindPhysical && d.CloudServiceEnabled
}

// status.go
type StatusPayload json.RawMessage
type StatusSnapshot struct {
    Payload    StatusPayload
    RecordedAt time.Time
}
```

これにより問題 1.6-e（boolean trap）が解消し、`collector.go:64` に埋もれていた「クラウド無効ならスキップ」という判断がドメインの言葉として表現される。

### 4.2 `usecase` の出力ポート

```go
type DeviceGateway interface {
    ListDevices(ctx context.Context, cred domain.Credential) ([]domain.Device, error)
    FetchStatus(ctx context.Context, cred domain.Credential, id domain.DeviceID) (domain.StatusPayload, error)
}
type AccountStore interface {
    Save(ctx context.Context, a domain.Account) (domain.AccountID, error)
}
type DeviceStore interface {
    Save(ctx context.Context, accountID domain.AccountID, d domain.Device) (domain.DeviceRecordID, error)
}
type StatusStore interface {
    Append(ctx context.Context, id domain.DeviceRecordID, s domain.StatusSnapshot) error
}
type Clock interface{ Now() time.Time }
```

`Clock` の本番実装は `time.Now()` を返すだけの3行なので、専用パッケージを作らず `cmd/switchbotstore/main.go` に非公開型として置く。テストでは固定時刻を返すフェイクを注入する。

認証情報を**引数として渡す**形にしたことで、以下が同時に解決する。

- 問題 1.6-d: `switchbot.Client` から `AccountToken()` / `AccountSecret()` のゲッターが消える
- 問題 1.6-j: HTTP クライアントをアカウントごとに生成する必要がなくなり、1つを共有できる

**デバイス制御（将来）を見越した設計**: 制御は `DeviceGateway` に `SendCommand` を追加し、`usecase/control_device.go` を新設する形で入る。ドメインモデルと Store 群には変更が及ばない。

### 4.3 ユースケースはログを吐かず結果を値で返す

現状 `collector.go` には `log.Printf` が8箇所散在している（問題 1.6-g）。これらをすべて戻り値に移す。

```go
type Outcome int
const (
    OutcomeStored               Outcome = iota // 保存成功
    OutcomeSkippedCloudDisabled                // クラウドサービス無効でスキップ
    OutcomeRegisteredOnly                      // 赤外線リモコン：デバイス登録のみ
    OutcomeFailed                              // 失敗
)

type DeviceResult struct {
    Device  domain.Device
    Outcome Outcome
    Err     error
}
type AccountResult struct {
    AccountName string
    Devices     []DeviceResult
    Err         error // アカウント単位の致命的エラー
}
type CollectReport struct{ Accounts []AccountResult }

// FatalError はアカウント単位の致命的エラーを errors.Join で集約して返す。
// 1件もなければ nil。run() はこれをそのまま返し、非 nil なら exit 1 となる。
func (r CollectReport) FatalError() error
```

`Execute` の戻り値は `CollectReport` のみで、`error` を返さない。**アカウント単位のエラーは `AccountResult.Err` に、デバイス単位のエラーは `DeviceResult.Err` に格納され、すべてレポートの中に閉じる。** 終了コードの判定は `FatalError()` が一手に担う。これにより「戻り値の error」と「レポート内のエラー」が二重に存在する曖昧さを避ける。

`adapter/presenter/slog_presenter.go` が `CollectReport` を受け取ってログに整形する。この分離により、**ユースケースのテストがログ文字列に依存せず戻り値の検証だけで済む**。

---

## 5. データフローと主要な処理

### 5.1 起動から終了まで

```
main() → run() error
  │
  ├─ 1. config.Load("config.json")            infra/config   ── validator で検証
  │      ※ この時点ではログ出力先が未確定。失敗時は slog のデフォルト
  │        （stderr 出力）で報告される
  ├─ 2. logging.Setup(cfg.LogDir)             infra/logging  ── slog ハンドラ確定
  ├─ 3. mysql.Connect(ctx, cfg.Database)      infra/mysql    ── *gorm.DB
  ├─ 4. mysql.Migrate(db)                     infra/mysql    ── goose up
  ├─ 5. persistence.VerifySchema(db)          adapter        ── スキーマ整合性チェック
  │
  ├─ 6. 組み立て（コンポジションルート）
  │      gateway := switchbot.NewGateway(httpClient)
  │      store   := persistence.New(db)
  │      uc      := usecase.NewCollectStatus(gateway, store, clock)
  │      pres    := presenter.NewSlog(logger)
  │
  ├─ 7. report := uc.Execute(ctx, accounts)      ← error は返さない。全て report の中
  ├─ 8. pres.Present(report)
  └─ 9. return report.FatalError()               ← 非 nil なら exit 1
```

`main` は `func run() error` パターンを採用し、`log.Fatalf` を全廃する（問題 1.4 の修正）。

```go
func main() {
    if err := run(); err != nil {
        slog.Error("実行に失敗しました", "error", err)
        os.Exit(1)
    }
}
```

`ctx` は `signal.NotifyContext` で生成し、全層に伝播させる（問題 1.6-c の修正）。

### 5.2 ユースケース内部

```
CollectStatus.Execute(ctx, accounts)
  recordedAt := clock.Now()        ← 全デバイスで共有（同一バッチを同時刻でグルーピング）
  │
  └─ アカウントごとに:
       ├─ AccountStore.Save(account)            → accountID
       ├─ DeviceGateway.ListDevices(cred)       → []domain.Device
       └─ デバイスごとに:
            ├─ DeviceStore.Save(accountID, device)   → recordID
            ├─ device.StatusReadable() が false なら
            │     → Kind に応じて SkippedCloudDisabled / RegisteredOnly を記録して次へ
            ├─ DeviceGateway.FetchStatus(cred, device.ID)
            └─ StatusStore.Append(recordID, snapshot)  → Stored を記録
```

**ループの一本化**: 現在は `DeviceList` と `InfraredRemoteList` を `collector.go:56` と `collector.go:84` の別ループで二重に処理している（問題 1.6-f）。`adapter/switchbot/mapper.go` の時点で両者を `Kind` を持つ単一の `[]domain.Device` に畳み込むことで、ループが1つになり重複が消える。

**`recordedAt`**: 先頭で1回だけ取得して全デバイスで共有する現状の挙動は意図的なものと判断し維持する。`Clock` ポートで注入するためテストで固定できる。

### 5.3 永続化層（GORM）

GORM モデルは `internal/adapter/persistence/model.go` に閉じ込め、`gorm` タグを層の外に出さない。`domain` は GORM を知らない。

```go
type apiAccount struct {
    ID        int64  `gorm:"column:id;primaryKey"`
    Name      string `gorm:"column:name"`
    Token     string `gorm:"column:token"`
    Secret    string `gorm:"column:secret"`
    CreatedAt time.Time
    UpdatedAt time.Time
}
func (apiAccount) TableName() string { return "api_accounts" }
```

**Upsert は「INSERT ... ON DUPLICATE KEY UPDATE → 主キーを SELECT」の2クエリ方式を維持する。**

MySQL の `ON DUPLICATE KEY UPDATE` は、更新が発生した場合に `LAST_INSERT_ID()` を更新しない。そのため GORM が返す `model.ID` を信頼できない。`ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id)` というトリックで1クエリ化する方法もあるが、AUTO_INCREMENT を無駄に消費するうえ意図が読み取りにくいため採用しない。現行実装と同じ2クエリ方式を、GORM の API で読みやすく表現する。

**スキーマ整合性チェック**（`verify.go`）:

```go
// VerifySchema は goose 適用後に呼び、GORM モデルが期待するテーブルと
// 全カラムが実 DB に存在することを確認する。
// goose の DDL と GORM タグがズレた場合、起動時に必ず検出できる。
func VerifySchema(db *gorm.DB) error
```

`db.Migrator()` の `HasTable` / `ColumnTypes` を用い、全モデルのテーブルとカラムの存在を確認する。約20行のコストで、GORM 採用の主要リスク（二重管理の破綻）が本番で不可解な SQL エラーになる前に、起動時の明示的なエラーとして表面化する。

### 5.4 マイグレーション

現行の `database.go:36-103` の DDL を、そのまま `migrations/00001_initial_schema.sql` に移す。

```sql
-- +goose Up
CREATE TABLE IF NOT EXISTS api_accounts ( ... );
CREATE TABLE IF NOT EXISTS devices ( ... );
CREATE TABLE IF NOT EXISTS device_status_logs ( ... );
CREATE TABLE IF NOT EXISTS system_settings ( ... );
```

既存 DDL は全て `CREATE TABLE IF NOT EXISTS` のため、**実データのある DB では no-op になり、goose のバージョン1として記録されるだけ**。特別な baseline 処理は不要。

`system_settings` テーブルは、コードからの参照を削除した後も**マイグレーションからは削除しない**（実データ維持のため `DROP` しない）。

### 5.5 設定（validator）

```go
type Config struct {
    Database Database  `json:"database"`
    Accounts []Account `json:"accounts" validate:"required,min=1,dive"`
    LogDir   string    `json:"log_dir"`
}
type Database struct {
    Host string `json:"host" validate:"required"`
    Port int    `json:"port"`
    User string `json:"user"`
    Password string `json:"password"`
    Name string `json:"name"`
}
type Account struct {
    Name   string `json:"name"`
    Token  string `json:"token"  validate:"required"`
    Secret string `json:"secret" validate:"required"`
}
```

**バリデーション規則は現行と完全に同一とし、実装手段のみを手書き `if` から validator タグへ変更する。**

現行 `config.go:41-54` が検証しているのは「accounts が1件以上」「各 account の token と secret が非空」「database.host が非空」の3点のみ。`database.user` / `password` / `name` および `accounts[].name` は検証していない。ここに `required` を足すと**既存の `config.json` が突然通らなくなる可能性がある**ため、リファクタリングの範囲では規則を変えない。

`Port` が 0 のときに 3306 を補う処理は、`Load` 内で現行どおり明示的に行う（validator にデフォルト値機能はなく、この1箇所のために `creasty/defaults` を追加する価値はない）。

### 5.6 ログ

- **`log/slog` に統一。`slog.JSONHandler` を使用**
- 出力先は「ファイル + stderr」の `io.MultiWriter`（現行と同じ）
- ファイル名は日付（`2006-01-02.log`）。現行と同じ
- **自前ローテーション（goroutine・mutex・timer・stopCh・sync.Once）を全削除**（問題 1.2）。106行 → 約25行
- **二段構えを廃止**（問題 1.3）。config 読み込み前は stderr のみに出力する

  根拠は2つ。config.json が読めない状況では正しいログ出力先も不明であり、暫定 `logs/` に書くのは推測にすぎない。そして README には既に cron 設定例として `>> /path/to/logs/cron.log 2>&1` が記載されており、**stderr は元々ファイルに落ちる運用になっている**。

---

## 6. エラー処理

| 現状 | 変更後 |
|---|---|
| `fmt.Errorf("...: %v", err)` — 全箇所 | `fmt.Errorf("...: %w", err)` — `errors.Is` / `errors.As` が機能する |
| アカウント単位のエラーを `lastErr` に上書き（最後の1件しか残らない） | 各 `AccountResult.Err` に保持し、`CollectReport.FatalError()` が `errors.Join` で全件を集約 |
| デバイス単位の失敗は `log.Printf` して `continue`、全体は成功扱い | `DeviceResult.Err` に記録して継続（**方針は現状踏襲**） |
| `log.Fatalf` で `defer` を飛ばす | `run() error` パターンで `defer` を確実に実行 |

**粒度の考え方**: デバイス1台の失敗（オフライン等）は正常な運用状態なので収集全体を止めない。アカウント単位の失敗（認証エラー、API 全体のダウン）は、そのアカウントのデバイスが全滅するため `AccountResult.Err` として記録する。

---

## 7. 振る舞いの変更（意図的な2点）

リファクタリングは原則として振る舞いを保存するが、以下2点は合意のうえ意図的に変更する。

### 7.1 終了コード

- **現状**: アカウント単位の失敗でも `[WARN]` を出して終了コード 0
- **変更後**: アカウント単位の致命的エラーが1件でもあれば **exit 1**

cron / タスクスケジューラが失敗を検知できるようになる。デバイス1台の失敗（オフライン等）は従来どおり exit 0 のまま。

### 7.2 初回起動フラグ機構の削除

`system_settings` / `IsFirstRun` / `MarkFirstRunDone` / `InitialCollect` の一式（約35行）をコードから削除する（問題 1.5 のとおり動作に差がないため）。

- `system_settings` **テーブル自体は DB に残す**（実データ維持のため `DROP` しない）
- README の「初回起動時に全デバイスの初期データを自動取得」の記述を実態に合わせて修正する

---

## 8. テスト方針

| 対象 | 方法 |
|---|---|
| `domain` | 純粋関数のテーブル駆動テスト（`StatusReadable()` 等） |
| `usecase` | 手書きの小さなフェイク（Gateway / Store）を注入し、**`CollectReport` の中身を検証**。既存 `collector_test.go` の9ケースを移植・拡充 |
| `adapter/switchbot` | `httptest` で API をスタブ（既存 `client_test.go` を移植）。`signer.go` は署名アルゴリズムを独立してテスト |
| `adapter/persistence` | GORM モデル ⇄ domain の `mapper` を純粋関数としてテスト。**DB アクセス自体のテストは行わず**、起動時の `VerifySchema` に委ねる |
| `infra/config` | 既存 `config_test.go` を移植 |

- アサーションは `testify` の `require` / `assert` に統一する
- モックは生成ライブラリを使わず**手書きのフェイク**を維持する（現状の書き方であり、`testify/mock` の expectation 記述より短く読みやすい）
- 既存テストが検証している振る舞いは、移植後もすべて維持する

---

## 9. 移行手順

各ステップ完了時点で `go build ./...` と `go test ./...` が通る状態を保つ。

| # | 内容 |
|---|---|
| 1 | 依存追加（gorm, goose, validator, testify）+ 既存テストを testify へ書き換え（**振る舞い不変**） |
| 2 | slog 導入 / `logger.go` 簡素化 / `main` を `run() error` 化 |
| 3 | config に validator 導入 |
| 4 | `domain` 層を新設 |
| 5 | `usecase` 層を新設（`collector` から移行）、`CollectReport` 導入 |
| 6 | `adapter/switchbot` へ移行（signer / dto / mapper に分割） |
| 7 | GORM + goose 導入、`adapter/persistence` へ移行、`VerifySchema` 追加 |
| 8 | 初回起動フラグ一式を削除 |
| 9 | `main` をコンポジションルートに整理 |
| 10 | README を実態に合わせて更新 |

ステップ7でのみ DB スキーマに触れるが、5.4 節のとおり既存 DDL をそのまま流用するため実データのある DB では no-op になる。

ステップ10 で README に必要な修正は以下の4点。

1. **ビルドコマンド**: `go build -o switchbotstore ./cmd/` → `go build ./cmd/switchbotstore/`（バイナリ名がディレクトリ名から決まるため `-o` が不要になる）
2. **「初回データ収集」の記述**: 「初回起動時に全デバイスの初期データを自動取得します」は実態と異なる（毎回全デバイスを取得している）ため削除する
3. **テーブル設計の表**: `system_settings` の説明を「`initial_collect_done` フラグで初回起動を判定」から「現在はコードから参照されない旧テーブル（既存データ保持のため残置）」に書き換える
4. **動作フロー図**: 「[初回起動の場合] ... → 終了 / [2回目以降] ...」の分岐を削除し、単一のフローに書き直す。あわせて終了コードの仕様（7.1 節）を追記する

---

## 10. 完了条件

- [ ] `go build ./...` と `go test ./...` が通る
- [ ] `domain` が標準ライブラリ以外を import していない
- [ ] `usecase` が `domain` 以外の内部パッケージを import していない
- [ ] `usecase` に `log` / `slog` の呼び出しが存在しない
- [ ] `repository` 相当の契約が `switchbot` の型に依存していない
- [ ] `log.Fatalf` がコードベースに存在しない
- [ ] エラーラップが全て `%w` になっている
- [ ] 既存テストが検証していた振る舞いがすべて移植されている
- [ ] 実データのある DB に対して起動し、`VerifySchema` が通り、収集が成功する
- [ ] README が実態と一致している

## 11. 今回のスコープ外

- デバイス制御機能そのものの実装（境界を用意するのみ）
- DB スキーマの変更（カラム名・型・制約の見直し）
- `system_settings` テーブルの `DROP`
- HTTP リトライ / レート制限対応
- DB 統合テスト（testcontainers 等）
