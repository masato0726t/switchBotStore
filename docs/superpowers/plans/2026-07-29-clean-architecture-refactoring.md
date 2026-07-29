# switchBotStore クリーンアーキテクチャ移行 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** SwitchBot データ収集バッチ `switchBotStore` を、GORM / goose / validator / testify / log-slog を採用したクリーンアーキテクチャ4層構成に再編し、依存の逆流と死にコードを解消する。

**Architecture:** `domain`（純粋なドメインモデル）→ `usecase`（出力ポートとユースケース。ログを吐かず `CollectReport` を値で返す）→ `adapter`（SwitchBot HTTP / GORM 永続化 / slog プレゼンタ）→ `infra`（設定・ログ・DB接続）の4層。内側は外側を一切 import しない。既存パッケージは新パッケージが揃うまで残し、最終タスクでまとめて削除することでビルドとテストを常に緑に保つ。

**Tech Stack:** Go 1.26.3 / GORM v1.31.2 / goose v3.27.3 / validator v10.30.3 / testify v1.11.1 / log/slog（標準）/ MySQL

**設計仕様書:** `docs/superpowers/specs/2026-07-29-clean-architecture-refactoring-design.md`

## Global Constraints

以下は全タスクの要件に暗黙的に含まれる。

- **既存 MySQL には実データがあり、維持必須。DB スキーマ（テーブル名・カラム名・型・制約）を一切変更しない。** カラムの追加・削除・リネームを行うタスクは存在しない
- `go.mod` の go ディレクティブは **`go 1.26.3`**（インストール済み最新版）
- 依存バージョンは固定: `gorm.io/gorm v1.31.2` / `gorm.io/driver/mysql v1.6.0` / `github.com/pressly/goose/v3 v3.27.3` / `github.com/go-playground/validator/v10 v10.30.3` / `github.com/stretchr/testify v1.11.1`
- モジュールパスは `switchBotStore`（変更しない）。内部 import は `switchBotStore/internal/...`
- **依存規則**: `domain` は標準ライブラリのみ。`usecase` は `domain` のみ（ログ関連の import 禁止）。`adapter` は `usecase` と `domain`。`infra` は内部パッケージを import しない
- エラーラップは必ず `%w`。`%v` を使わない
- `log.Fatalf` / `log.Fatal` / `os.Exit` を `main` 関数以外で使わない
- コメントと識別子以外の文字列（ログメッセージ・エラーメッセージ）は日本語
- 各タスク完了時点で `go build ./...` と `go test ./...` が通ること
- テストのアサーションは `testify` の `require`（失敗したら即中断）/ `assert`（失敗しても継続）を使う

## ファイル構成

| ファイル | 責務 | 担当タスク |
|---|---|---|
| `go.mod` / `go.sum` | 依存とGoバージョン | 1 |
| `internal/domain/account.go` | `Account` / `AccountID` / `Credential` | 2 |
| `internal/domain/device.go` | `Device` / `DeviceID` / `DeviceRecordID` / `DeviceKind` / `StatusReadable()` | 2 |
| `internal/domain/status.go` | `StatusPayload` / `StatusSnapshot` | 2 |
| `internal/usecase/port.go` | 出力ポート `DeviceGateway` / `Repository` / `Clock` | 3 |
| `internal/usecase/report.go` | `Outcome` / `DeviceResult` / `AccountResult` / `CollectReport` | 3 |
| `internal/usecase/collect_status.go` | `CollectStatus` ユースケース | 3 |
| `internal/adapter/switchbot/signer.go` | HMAC-SHA256 署名生成 | 4 |
| `internal/adapter/switchbot/dto.go` | API レスポンス構造体 | 4 |
| `internal/adapter/switchbot/mapper.go` | DTO → `domain.Device` 変換 | 4 |
| `internal/adapter/switchbot/client.go` | `DeviceGateway` の HTTP 実装 | 4 |
| `internal/infra/config/config.go` | `config.json` 読込 + validator | 5 |
| `internal/infra/logging/logging.go` | slog セットアップ | 6 |
| `internal/infra/mysql/connect.go` | GORM 接続 | 7 |
| `internal/infra/mysql/migrate.go` | goose 実行 | 7 |
| `internal/infra/mysql/migrations/00001_initial_schema.sql` | 現行 DDL | 7 |
| `internal/adapter/persistence/model.go` | GORM モデル | 8 |
| `internal/adapter/persistence/mapper.go` | GORM モデル ⇄ domain | 8 |
| `internal/adapter/persistence/store.go` | `Repository` の GORM 実装 | 8 |
| `internal/adapter/persistence/verify.go` | スキーマ整合性チェック | 8 |
| `internal/adapter/presenter/slog_presenter.go` | `CollectReport` → 構造化ログ | 9 |
| `cmd/switchbotstore/main.go` | コンポジションルート | 10 |
| `README.md` | ドキュメント | 11 |

## 仕様書からの逸脱（実装上の理由があるもの）

| 逸脱 | 理由 |
|---|---|
| 出力ポートを `AccountStore` / `DeviceStore` / `StatusStore` の3つに分けず、`Repository` 1つに統合 | **Go はメソッドのオーバーロードを許さない**ため、単一の実装型が `Save` を2つ持てない。メソッド名を `SaveAccount` / `SaveDevice` / `AppendStatus` に区別する（仕様書 §4.2 に反映済み） |
| `internal/domain/errors.go` を作らない | ユースケースが `StatusReadable()` で事前分岐するため、ドメインエラーを返す経路が存在しない。消費者のいないエラー変数は YAGNI（仕様書 §4.1 に反映済み） |
| 既存テスト全4本を先に testify 化せず、`config_test.go` と `client_test.go` の2本のみ | 残り2本（`collector_test.go` / `logger_test.go`）は新パッケージで全面的に書き直すため、書き換えてから捨てるのは無駄。移設先でそのまま活きる2本のみ先行して変換する |

## 既知の制限（今回は修正しない）

- **赤外線リモコンの `device_type` が空になる**: SwitchBot API は `infraredRemoteList` の要素に `deviceType` ではなく `remoteType` を返す。現行コードは両方を同じ構造体で `deviceType` としてパースしているため、赤外線リモコンの `device_type` カラムは空文字で保存されている。本リファクタリングは振る舞いを保存する方針のため、この挙動をそのまま維持する。修正する場合は別タスクとして扱うこと

## トレーサビリティ（仕様書 §1 の問題点 → 解決するタスク）

| 仕様書 | 問題 | タスク | 解決方法 |
|---|---|---|---|
| §1.1 | 永続化の契約が `switchbot.DeviceInfo` に依存（依存の逆流） | 3, 8 | `usecase.Repository` を `domain` の型だけで定義し、`persistence` が実装する |
| §1.2 | 発火しない日次ローテーション106行 | 6 | 日付ファイル名のみにし、goroutine / mutex / timer を全削除 |
| §1.3 | ログ出力先の二段構え | 6, 10 | config 読込前は slog デフォルト（stderr）。切り替えは1回のみ |
| §1.4 | `log.Fatalf` で `defer` が飛ぶ | 10 | `run() error` パターン。`os.Exit` は `main` のみ |
| §1.5 | 実質無効な初回起動フラグ機構 | 10 | 一式削除（テーブルは残置） |
| §1.6-a | `%v` でのエラーラップ | 全タスク | Global Constraints で `%w` 必須。Task 10 Step 7 で機械的に検証 |
| §1.6-b | `lastErr` 上書きで最後の1件しか残らない | 3 | `AccountResult.Err` に保持し `FatalError()` が `errors.Join` |
| §1.6-c | `context.Context` が皆無 | 3, 4, 7, 8, 10 | 全ポートが `ctx` を受け取る。`signal.NotifyContext` から伝播 |
| §1.6-d | 認証情報のゲッター露出 | 3, 4 | `domain.Credential` をメソッド引数で渡す |
| §1.6-e | `isVirtualInfrared bool` の boolean trap | 2 | `domain.DeviceKind` で表現する |
| §1.6-f | 物理／赤外線の二重ループ | 4 | `toDomainDevices` が単一リストへ畳み込む |
| §1.6-g | ユースケースに `log.Printf` が8箇所 | 3, 9 | ユースケースは `CollectReport` を返し、presenter が整形する |
| §1.6-h | DDL が Go の文字列リテラル | 7 | goose の `migrations/*.sql` + `embed.FS` |
| §1.6-i | 手書き `if` のバリデーション／アサーション | 1, 5 | validator タグ + testify |
| §1.6-j | HTTP クライアントをアカウント数だけ生成 | 4 | `Gateway` 1インスタンスを共有する |

---

### Task 1: 依存の追加と Go バージョン更新

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`
- Modify: `internal/config/config_test.go`（testify 化）
- Modify: `internal/switchbot/client_test.go`（testify 化）

**Interfaces:**
- Consumes: なし（最初のタスク）
- Produces: 後続タスクが `github.com/stretchr/testify/require` と `github.com/stretchr/testify/assert` を import できる状態

このタスクではプロダクションコードを一切変更しない。依存を追加し、後続タスクで移設される2本のテストをアサーションライブラリに載せ替えるだけ。

- [ ] **Step 1: 現状のテストが通ることを確認する（ベースライン）**

Run: `go test ./...`
Expected: `ok` が4パッケージ（collector / config / logger / switchbot）、`no test files` が3パッケージ

- [ ] **Step 2: Go バージョンと依存を追加する**

```bash
go mod edit -go=1.26.3
go get github.com/stretchr/testify@v1.11.1
```

`gorm` / `goose` / `validator` はこの時点では **追加しない**。まだ import していないため `go mod tidy` で削除されてしまう。それぞれ最初に使うタスク（5・7・8）で追加する。

- [ ] **Step 3: `go.mod` の内容を確認する**

Run: `cat go.mod`
Expected: `go 1.26.3` の行があり、`github.com/stretchr/testify v1.11.1` が `require` に入っていること。この時点ではまだ import していないため `// indirect` が付いていてもよい（Step 4 以降で import すると外れる）

- [ ] **Step 4: `internal/config/config_test.go` を testify 化する**

既存の `if ... { t.Errorf(...) }` を `require` / `assert` に置き換える。**テストケースの数と検証内容は一切変えない。** 変換の型は以下のとおり。

```go
// 変換前 → 変換後
// if err != nil { t.Fatalf("...: %v", err) }        → require.NoError(t, err)
// if err == nil { t.Error("...") }                   → require.Error(t, err)
// if got != want { t.Errorf("... = %v, want %v") }   → assert.Equal(t, want, got)
// if len(x) != n { t.Errorf(...) }                   → assert.Len(t, x, n)
```

import ブロックに以下を追加する。

```go
import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)
```

`assert.Equal` は第2引数が期待値、第3引数が実際の値である点に注意（`t.Errorf` の並びと逆になりやすい）。

- [ ] **Step 5: config のテストが通ることを確認する**

Run: `go test ./internal/config/ -v`
Expected: 変換前と同じテスト名がすべて PASS

- [ ] **Step 6: `internal/switchbot/client_test.go` を testify 化する**

Step 4 と同じ変換を適用する。`httptest` サーバーの記述と `c.apiBase = srv.URL` の代入はそのまま残す。

- [ ] **Step 7: switchbot のテストが通ることを確認する**

Run: `go test ./internal/switchbot/ -v`
Expected: 変換前と同じテスト名がすべて PASS

- [ ] **Step 8: 全体のビルドとテストを確認する**

Run: `go build ./... && go test ./...`
Expected: ビルド成功、全パッケージ `ok`

- [ ] **Step 9: コミット**

```bash
git add go.mod go.sum internal/config/config_test.go internal/switchbot/client_test.go
git commit -m "chore: Go 1.26.3 へ更新し testify を導入"
```

---

### Task 2: domain 層の新設

**Files:**
- Create: `internal/domain/account.go`
- Create: `internal/domain/device.go`
- Create: `internal/domain/status.go`
- Test: `internal/domain/device_test.go`

**Interfaces:**
- Consumes: なし
- Produces: 以下の型と関数。Task 3・4・8 がこれらを使う。
  - `domain.AccountID`（`int64` の名前付き型）
  - `domain.Credential{Token, Secret string}`
  - `domain.Account{Name string; Credential Credential}`
  - `domain.DeviceID`（`string` の名前付き型）
  - `domain.DeviceRecordID`（`int64` の名前付き型）
  - `domain.DeviceKind`（`int` の名前付き型）、定数 `DeviceKindPhysical` / `DeviceKindInfraredRemote`
  - `domain.Device{ID DeviceID; Name, Type string; HubID DeviceID; Kind DeviceKind; CloudServiceEnabled bool}`
  - `func (d Device) StatusReadable() bool`
  - `domain.StatusPayload`（`json.RawMessage` の名前付き型）
  - `domain.StatusSnapshot{Payload StatusPayload; RecordedAt time.Time}`

- [ ] **Step 1: 失敗するテストを書く**

Create `internal/domain/device_test.go`:

```go
package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"switchBotStore/internal/domain"
)

func TestDevice_StatusReadable(t *testing.T) {
	tests := []struct {
		name string
		dev  domain.Device
		want bool
	}{
		{
			name: "物理デバイスでクラウド有効なら取得できる",
			dev:  domain.Device{Kind: domain.DeviceKindPhysical, CloudServiceEnabled: true},
			want: true,
		},
		{
			name: "物理デバイスでもクラウド無効なら取得できない",
			dev:  domain.Device{Kind: domain.DeviceKindPhysical, CloudServiceEnabled: false},
			want: false,
		},
		{
			name: "赤外線リモコンはクラウド有効でも取得できない",
			dev:  domain.Device{Kind: domain.DeviceKindInfraredRemote, CloudServiceEnabled: true},
			want: false,
		},
		{
			name: "赤外線リモコンでクラウド無効なら取得できない",
			dev:  domain.Device{Kind: domain.DeviceKindInfraredRemote, CloudServiceEnabled: false},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.dev.StatusReadable())
		})
	}
}
```

- [ ] **Step 2: テストが失敗することを確認する**

Run: `go test ./internal/domain/`
Expected: FAIL。`no required module provides package switchBotStore/internal/domain` またはビルドエラー

- [ ] **Step 3: `internal/domain/account.go` を作る**

```go
// Package domain は SwitchBot データ収集のドメインモデルを定義する。
//
// このパッケージは標準ライブラリ以外に依存しない。他の内部パッケージも
// import してはならない（クリーンアーキテクチャの最内層）。
package domain

// AccountID は永続化された API アカウントの識別子（api_accounts.id）。
type AccountID int64

// Credential は SwitchBot API の認証情報。
type Credential struct {
	Token  string
	Secret string
}

// Account は SwitchBot API のアカウント。
type Account struct {
	Name       string
	Credential Credential
}
```

- [ ] **Step 4: `internal/domain/device.go` を作る**

```go
package domain

// DeviceID は SwitchBot が採番するデバイス識別子。
type DeviceID string

// DeviceRecordID は永続化されたデバイスの識別子（devices.id）。
type DeviceRecordID int64

// DeviceKind はデバイスの種別。
type DeviceKind int

const (
	// DeviceKindPhysical は物理デバイス。
	DeviceKindPhysical DeviceKind = iota
	// DeviceKindInfraredRemote は Hub に登録された仮想赤外線リモコン。
	DeviceKindInfraredRemote
)

// Device は SwitchBot に登録されたデバイス。
type Device struct {
	ID                  DeviceID
	Name                string
	Type                string
	HubID               DeviceID
	Kind                DeviceKind
	CloudServiceEnabled bool
}

// StatusReadable はこのデバイスからステータスを取得できるかを返す。
//
// 赤外線リモコンには SwitchBot API にステータス取得エンドポイントが存在せず、
// クラウドサービスが無効なデバイスは API 経由で状態を読めない。
func (d Device) StatusReadable() bool {
	return d.Kind == DeviceKindPhysical && d.CloudServiceEnabled
}
```

- [ ] **Step 5: `internal/domain/status.go` を作る**

```go
package domain

import (
	"encoding/json"
	"time"
)

// StatusPayload は SwitchBot API が返すデバイスステータスの生 JSON。
//
// デバイス種別ごとにフィールドが大きく異なるため、構造化せずそのまま保持し、
// DB の JSON カラムへ格納する。
type StatusPayload json.RawMessage

// StatusSnapshot はある時点で収集したデバイスステータス。
type StatusSnapshot struct {
	Payload    StatusPayload
	RecordedAt time.Time
}
```

- [ ] **Step 6: テストが通ることを確認する**

Run: `go test ./internal/domain/ -v`
Expected: `TestDevice_StatusReadable` の4サブテストがすべて PASS

- [ ] **Step 7: 依存規則を確認する**

Run: `go list -deps ./internal/domain/ | grep switchBotStore`
Expected: `switchBotStore/internal/domain` の1行のみ（他の内部パッケージが出たら依存規則違反）

- [ ] **Step 8: 全体のビルドとテストを確認する**

Run: `go build ./... && go test ./...`
Expected: ビルド成功、全パッケージ `ok`

- [ ] **Step 9: コミット**

```bash
git add internal/domain/
git commit -m "feat: ドメインモデル層を追加"
```

---

### Task 3: usecase 層の新設

**Files:**
- Create: `internal/usecase/port.go`
- Create: `internal/usecase/report.go`
- Create: `internal/usecase/collect_status.go`
- Test: `internal/usecase/collect_status_test.go`

**Interfaces:**
- Consumes: Task 2 の `domain.*` 全型
- Produces: Task 4・8・9・10 が使う以下。
  - `usecase.DeviceGateway` インターフェース: `ListDevices(ctx context.Context, cred domain.Credential) ([]domain.Device, error)` / `FetchStatus(ctx context.Context, cred domain.Credential, id domain.DeviceID) (domain.StatusPayload, error)`
  - `usecase.Repository` インターフェース: `SaveAccount(ctx context.Context, a domain.Account) (domain.AccountID, error)` / `SaveDevice(ctx context.Context, accountID domain.AccountID, d domain.Device) (domain.DeviceRecordID, error)` / `AppendStatus(ctx context.Context, id domain.DeviceRecordID, s domain.StatusSnapshot) error`
  - `usecase.Clock` インターフェース: `Now() time.Time`
  - `usecase.Outcome`（`int`）、定数 `OutcomeStored` / `OutcomeSkippedCloudDisabled` / `OutcomeRegisteredOnly` / `OutcomeFailed`、`func (o Outcome) String() string`
  - `usecase.DeviceResult{Device domain.Device; Outcome Outcome; Err error}`
  - `usecase.AccountResult{AccountName string; Devices []DeviceResult; Err error}`
  - `usecase.CollectReport{Accounts []AccountResult}`、`func (r CollectReport) FatalError() error`
  - `func NewCollectStatus(gateway DeviceGateway, repo Repository, clock Clock) *CollectStatus`
  - `func (uc *CollectStatus) Execute(ctx context.Context, accounts []domain.Account) CollectReport`

- [ ] **Step 1: 出力ポートを定義する**

Create `internal/usecase/port.go`:

```go
// Package usecase はアプリケーション固有のユースケースを実装する。
//
// domain 以外の内部パッケージには依存しない。ログ出力も行わず、処理結果は
// CollectReport として呼び出し元に返す（ログ整形は adapter/presenter の責務）。
package usecase

import (
	"context"
	"time"

	"switchBotStore/internal/domain"
)

// DeviceGateway は SwitchBot API へのアクセスを抽象化する出力ポート。
//
// 認証情報を引数で受け取るため、実装は全アカウントで1インスタンスを共有できる。
type DeviceGateway interface {
	// ListDevices はアカウントに登録された全デバイスを返す。
	// 物理デバイスと赤外線リモコンは DeviceKind で区別された単一のリストになる。
	ListDevices(ctx context.Context, cred domain.Credential) ([]domain.Device, error)

	// FetchStatus は指定デバイスの現在のステータスを返す。
	FetchStatus(ctx context.Context, cred domain.Credential, id domain.DeviceID) (domain.StatusPayload, error)
}

// Repository は収集結果の永続化を抽象化する出力ポート。
type Repository interface {
	// SaveAccount はアカウントを登録し、既に存在する場合は更新して ID を返す。
	SaveAccount(ctx context.Context, a domain.Account) (domain.AccountID, error)

	// SaveDevice はデバイスを登録し、既に存在する場合は更新して ID を返す。
	SaveDevice(ctx context.Context, accountID domain.AccountID, d domain.Device) (domain.DeviceRecordID, error)

	// AppendStatus はステータス収集ログを1件追加する。
	AppendStatus(ctx context.Context, id domain.DeviceRecordID, s domain.StatusSnapshot) error
}

// Clock は現在時刻の取得を抽象化する（テストで固定するため）。
type Clock interface {
	Now() time.Time
}
```

- [ ] **Step 2: 実行結果の型を定義する**

Create `internal/usecase/report.go`:

```go
package usecase

import (
	"errors"

	"switchBotStore/internal/domain"
)

// Outcome は1デバイスに対する収集処理の結末。
type Outcome int

const (
	// OutcomeStored はステータスを取得して保存できたことを表す。
	OutcomeStored Outcome = iota
	// OutcomeSkippedCloudDisabled はクラウドサービスが無効なためステータス取得を省いたことを表す。
	OutcomeSkippedCloudDisabled
	// OutcomeRegisteredOnly は赤外線リモコンのためデバイス登録のみ行ったことを表す。
	OutcomeRegisteredOnly
	// OutcomeFailed はデバイス単位の処理が失敗したことを表す。
	OutcomeFailed
)

// String はログ出力用の識別子を返す。
func (o Outcome) String() string {
	switch o {
	case OutcomeStored:
		return "stored"
	case OutcomeSkippedCloudDisabled:
		return "skipped_cloud_disabled"
	case OutcomeRegisteredOnly:
		return "registered_only"
	case OutcomeFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// DeviceResult は1デバイスに対する処理結果。
type DeviceResult struct {
	Device  domain.Device
	Outcome Outcome
	Err     error
}

// AccountResult は1アカウントに対する処理結果。
//
// Err はアカウント単位の致命的エラー（認証失敗、デバイス一覧の取得失敗など）。
// 個々のデバイスの失敗は Devices[i].Err に入り、Err には現れない。
type AccountResult struct {
	AccountName string
	Devices     []DeviceResult
	Err         error
}

// CollectReport は収集処理全体の結果。
type CollectReport struct {
	Accounts []AccountResult
}

// FatalError はアカウント単位の致命的エラーを集約して返す。1件もなければ nil。
//
// 呼び出し元はこの戻り値をそのままプロセスの終了コード判定に使う
// （非 nil なら exit 1）。デバイス1台の失敗は含まれないため、
// オフラインのデバイスがあっても正常終了する。
func (r CollectReport) FatalError() error {
	errs := make([]error, 0, len(r.Accounts))
	for _, a := range r.Accounts {
		if a.Err != nil {
			errs = append(errs, a.Err)
		}
	}
	return errors.Join(errs...)
}
```

`errors.Join` は要素が0件のとき nil を返すため、明示的な長さチェックは不要。

- [ ] **Step 3: 失敗するテストを書く**

Create `internal/usecase/collect_status_test.go`:

```go
package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"switchBotStore/internal/domain"
	"switchBotStore/internal/usecase"
)

// ---------- フェイク ----------

// fakeGateway は usecase.DeviceGateway のテスト用実装。
type fakeGateway struct {
	devices    map[string][]domain.Device // token → デバイス一覧
	listErr    map[string]error           // token → ListDevices が返すエラー
	statuses   map[domain.DeviceID]domain.StatusPayload
	statusErrs map[domain.DeviceID]error
}

func (g *fakeGateway) ListDevices(_ context.Context, cred domain.Credential) ([]domain.Device, error) {
	if err, ok := g.listErr[cred.Token]; ok {
		return nil, err
	}
	return g.devices[cred.Token], nil
}

func (g *fakeGateway) FetchStatus(_ context.Context, _ domain.Credential, id domain.DeviceID) (domain.StatusPayload, error) {
	if err, ok := g.statusErrs[id]; ok {
		return nil, err
	}
	return g.statuses[id], nil
}

// savedStatus は fakeRepo が記録した1件のステータス保存。
type savedStatus struct {
	deviceRecordID domain.DeviceRecordID
	snapshot       domain.StatusSnapshot
}

// fakeRepo は usecase.Repository のテスト用実装。
type fakeRepo struct {
	accountIDs map[string]domain.AccountID     // token → ID
	deviceIDs  map[string]domain.DeviceRecordID // "accountID/deviceID" → ID
	statuses   []savedStatus

	accountErr error
	deviceErr  error
	statusErr  error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		accountIDs: make(map[string]domain.AccountID),
		deviceIDs:  make(map[string]domain.DeviceRecordID),
	}
}

func (r *fakeRepo) SaveAccount(_ context.Context, a domain.Account) (domain.AccountID, error) {
	if r.accountErr != nil {
		return 0, r.accountErr
	}
	if id, ok := r.accountIDs[a.Credential.Token]; ok {
		return id, nil
	}
	id := domain.AccountID(len(r.accountIDs) + 1)
	r.accountIDs[a.Credential.Token] = id
	return id, nil
}

func (r *fakeRepo) SaveDevice(_ context.Context, accountID domain.AccountID, d domain.Device) (domain.DeviceRecordID, error) {
	if r.deviceErr != nil {
		return 0, r.deviceErr
	}
	key := fmt.Sprintf("%d/%s", accountID, d.ID)
	if id, ok := r.deviceIDs[key]; ok {
		return id, nil
	}
	id := domain.DeviceRecordID(len(r.deviceIDs) + 1)
	r.deviceIDs[key] = id
	return id, nil
}

func (r *fakeRepo) AppendStatus(_ context.Context, id domain.DeviceRecordID, s domain.StatusSnapshot) error {
	if r.statusErr != nil {
		return r.statusErr
	}
	r.statuses = append(r.statuses, savedStatus{deviceRecordID: id, snapshot: s})
	return nil
}

// fixedClock は常に同じ時刻を返す usecase.Clock。
type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

// ---------- ヘルパー ----------

var testNow = time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)

func physicalDevice(id, name string) domain.Device {
	return domain.Device{
		ID:                  domain.DeviceID(id),
		Name:                name,
		Type:                "Meter",
		Kind:                domain.DeviceKindPhysical,
		CloudServiceEnabled: true,
	}
}

func account(name, token string) domain.Account {
	return domain.Account{Name: name, Credential: domain.Credential{Token: token, Secret: "sec"}}
}

// outcomes は結果から Outcome の並びだけを取り出す。
func outcomes(r usecase.AccountResult) []usecase.Outcome {
	got := make([]usecase.Outcome, 0, len(r.Devices))
	for _, d := range r.Devices {
		got = append(got, d.Outcome)
	}
	return got
}

// ---------- テスト ----------

func TestExecute_物理デバイスのステータスを保存する(t *testing.T) {
	dev := physicalDevice("d1", "温湿度計")
	gw := &fakeGateway{
		devices:  map[string][]domain.Device{"tok1": {dev}},
		statuses: map[domain.DeviceID]domain.StatusPayload{"d1": []byte(`{"temperature":25.5}`)},
	}
	repo := newFakeRepo()

	uc := usecase.NewCollectStatus(gw, repo, fixedClock{t: testNow})
	report := uc.Execute(context.Background(), []domain.Account{account("acc1", "tok1")})

	require.NoError(t, report.FatalError())
	require.Len(t, report.Accounts, 1)
	assert.Equal(t, []usecase.Outcome{usecase.OutcomeStored}, outcomes(report.Accounts[0]))

	require.Len(t, repo.statuses, 1)
	assert.Equal(t, testNow, repo.statuses[0].snapshot.RecordedAt)
	assert.JSONEq(t, `{"temperature":25.5}`, string(repo.statuses[0].snapshot.Payload))
}

func TestExecute_クラウド無効のデバイスは登録のみでスキップする(t *testing.T) {
	disabled := physicalDevice("d1", "Bot")
	disabled.CloudServiceEnabled = false

	gw := &fakeGateway{
		devices:  map[string][]domain.Device{"tok1": {disabled, physicalDevice("d2", "Meter")}},
		statuses: map[domain.DeviceID]domain.StatusPayload{"d2": []byte(`{}`)},
	}
	repo := newFakeRepo()

	uc := usecase.NewCollectStatus(gw, repo, fixedClock{t: testNow})
	report := uc.Execute(context.Background(), []domain.Account{account("acc1", "tok1")})

	require.NoError(t, report.FatalError())
	assert.Equal(t,
		[]usecase.Outcome{usecase.OutcomeSkippedCloudDisabled, usecase.OutcomeStored},
		outcomes(report.Accounts[0]))
	assert.Len(t, repo.deviceIDs, 2, "スキップしたデバイスも登録される")
	assert.Len(t, repo.statuses, 1)
}

func TestExecute_赤外線リモコンは登録のみ行う(t *testing.T) {
	ir := domain.Device{
		ID:                  "ir1",
		Name:                "エアコン",
		Kind:                domain.DeviceKindInfraredRemote,
		CloudServiceEnabled: true,
	}
	gw := &fakeGateway{devices: map[string][]domain.Device{"tok1": {ir}}}
	repo := newFakeRepo()

	uc := usecase.NewCollectStatus(gw, repo, fixedClock{t: testNow})
	report := uc.Execute(context.Background(), []domain.Account{account("acc1", "tok1")})

	require.NoError(t, report.FatalError())
	assert.Equal(t, []usecase.Outcome{usecase.OutcomeRegisteredOnly}, outcomes(report.Accounts[0]))
	assert.Len(t, repo.deviceIDs, 1)
	assert.Empty(t, repo.statuses, "赤外線リモコンはステータスを保存しない")
}

func TestExecute_複数アカウントを処理する(t *testing.T) {
	gw := &fakeGateway{
		devices: map[string][]domain.Device{
			"tok1": {physicalDevice("d1", "A")},
			"tok2": {physicalDevice("d2", "B")},
		},
		statuses: map[domain.DeviceID]domain.StatusPayload{
			"d1": []byte(`{}`),
			"d2": []byte(`{}`),
		},
	}
	repo := newFakeRepo()

	uc := usecase.NewCollectStatus(gw, repo, fixedClock{t: testNow})
	report := uc.Execute(context.Background(), []domain.Account{
		account("acc1", "tok1"),
		account("acc2", "tok2"),
	})

	require.NoError(t, report.FatalError())
	require.Len(t, report.Accounts, 2)
	assert.Len(t, repo.accountIDs, 2)
	assert.Len(t, repo.statuses, 2)
}

func TestExecute_デバイス一覧の取得失敗はアカウント単位の致命的エラーになる(t *testing.T) {
	wantErr := errors.New("API エラー")
	gw := &fakeGateway{listErr: map[string]error{"tok1": wantErr}}
	repo := newFakeRepo()

	uc := usecase.NewCollectStatus(gw, repo, fixedClock{t: testNow})
	report := uc.Execute(context.Background(), []domain.Account{account("acc1", "tok1")})

	require.Error(t, report.FatalError())
	assert.ErrorIs(t, report.Accounts[0].Err, wantErr, "元のエラーが %w で包まれている")
	assert.Empty(t, report.Accounts[0].Devices)
}

func TestExecute_アカウント登録の失敗はアカウント単位の致命的エラーになる(t *testing.T) {
	wantErr := errors.New("DB エラー")
	gw := &fakeGateway{devices: map[string][]domain.Device{"tok1": {physicalDevice("d1", "A")}}}
	repo := newFakeRepo()
	repo.accountErr = wantErr

	uc := usecase.NewCollectStatus(gw, repo, fixedClock{t: testNow})
	report := uc.Execute(context.Background(), []domain.Account{account("acc1", "tok1")})

	require.Error(t, report.FatalError())
	assert.ErrorIs(t, report.Accounts[0].Err, wantErr)
}

func TestExecute_ステータス取得の失敗は他のデバイスを止めない(t *testing.T) {
	gw := &fakeGateway{
		devices: map[string][]domain.Device{
			"tok1": {physicalDevice("d1", "失敗"), physicalDevice("d2", "成功")},
		},
		statuses:   map[domain.DeviceID]domain.StatusPayload{"d2": []byte(`{}`)},
		statusErrs: map[domain.DeviceID]error{"d1": errors.New("デバイスオフライン")},
	}
	repo := newFakeRepo()

	uc := usecase.NewCollectStatus(gw, repo, fixedClock{t: testNow})
	report := uc.Execute(context.Background(), []domain.Account{account("acc1", "tok1")})

	require.NoError(t, report.FatalError(), "デバイス単位の失敗は致命的エラーにしない")
	assert.Equal(t,
		[]usecase.Outcome{usecase.OutcomeFailed, usecase.OutcomeStored},
		outcomes(report.Accounts[0]))
	assert.Error(t, report.Accounts[0].Devices[0].Err)
	assert.Len(t, repo.statuses, 1)
}

func TestExecute_ステータス保存の失敗は他のデバイスを止めない(t *testing.T) {
	gw := &fakeGateway{
		devices: map[string][]domain.Device{
			"tok1": {physicalDevice("d1", "A"), physicalDevice("d2", "B")},
		},
		statuses: map[domain.DeviceID]domain.StatusPayload{"d1": []byte(`{}`), "d2": []byte(`{}`)},
	}
	repo := newFakeRepo()
	repo.statusErr = errors.New("DB 書き込みエラー")

	uc := usecase.NewCollectStatus(gw, repo, fixedClock{t: testNow})
	report := uc.Execute(context.Background(), []domain.Account{account("acc1", "tok1")})

	require.NoError(t, report.FatalError())
	assert.Equal(t,
		[]usecase.Outcome{usecase.OutcomeFailed, usecase.OutcomeFailed},
		outcomes(report.Accounts[0]))
	assert.Empty(t, repo.statuses)
}

func TestExecute_デバイス登録の失敗はそのデバイスのみ失敗にする(t *testing.T) {
	gw := &fakeGateway{devices: map[string][]domain.Device{"tok1": {physicalDevice("d1", "A")}}}
	repo := newFakeRepo()
	repo.deviceErr = errors.New("DB エラー")

	uc := usecase.NewCollectStatus(gw, repo, fixedClock{t: testNow})
	report := uc.Execute(context.Background(), []domain.Account{account("acc1", "tok1")})

	require.NoError(t, report.FatalError())
	assert.Equal(t, []usecase.Outcome{usecase.OutcomeFailed}, outcomes(report.Accounts[0]))
}

func TestExecute_全デバイスが同じ収集時刻を共有する(t *testing.T) {
	gw := &fakeGateway{
		devices: map[string][]domain.Device{
			"tok1": {physicalDevice("d1", "A"), physicalDevice("d2", "B")},
		},
		statuses: map[domain.DeviceID]domain.StatusPayload{"d1": []byte(`{}`), "d2": []byte(`{}`)},
	}
	repo := newFakeRepo()

	uc := usecase.NewCollectStatus(gw, repo, fixedClock{t: testNow})
	uc.Execute(context.Background(), []domain.Account{account("acc1", "tok1")})

	require.Len(t, repo.statuses, 2)
	assert.Equal(t, repo.statuses[0].snapshot.RecordedAt, repo.statuses[1].snapshot.RecordedAt)
}

func TestFatalError_複数アカウントのエラーを全件保持する(t *testing.T) {
	err1 := errors.New("アカウント1の失敗")
	err2 := errors.New("アカウント2の失敗")

	report := usecase.CollectReport{Accounts: []usecase.AccountResult{
		{AccountName: "a1", Err: err1},
		{AccountName: "a2", Err: err2},
	}}

	joined := report.FatalError()
	require.Error(t, joined)
	assert.ErrorIs(t, joined, err1)
	assert.ErrorIs(t, joined, err2, "最後の1件だけでなく全件が保持される")
}

func TestFatalError_エラーがなければnilを返す(t *testing.T) {
	report := usecase.CollectReport{Accounts: []usecase.AccountResult{{AccountName: "a1"}}}
	assert.NoError(t, report.FatalError())
}
```

- [ ] **Step 4: テストが失敗することを確認する**

Run: `go test ./internal/usecase/`
Expected: FAIL。`undefined: usecase.NewCollectStatus`

- [ ] **Step 5: ユースケースを実装する**

Create `internal/usecase/collect_status.go`:

```go
package usecase

import (
	"context"
	"fmt"
	"time"

	"switchBotStore/internal/domain"
)

// CollectStatus は全アカウントのデバイスステータスを収集して永続化するユースケース。
type CollectStatus struct {
	gateway DeviceGateway
	repo    Repository
	clock   Clock
}

// NewCollectStatus は CollectStatus を生成する。
func NewCollectStatus(gateway DeviceGateway, repo Repository, clock Clock) *CollectStatus {
	return &CollectStatus{gateway: gateway, repo: repo, clock: clock}
}

// Execute は accounts の全デバイスからステータスを収集する。
//
// エラーは戻り値ではなく CollectReport に格納される。デバイス1台の失敗では
// 処理を止めず、アカウント単位の失敗のみ AccountResult.Err に記録して
// 次のアカウントへ進む。
func (uc *CollectStatus) Execute(ctx context.Context, accounts []domain.Account) CollectReport {
	// 同一バッチのログを同じ時刻でグルーピングできるよう、先頭で1回だけ取得する。
	recordedAt := uc.clock.Now()

	report := CollectReport{Accounts: make([]AccountResult, 0, len(accounts))}
	for _, acc := range accounts {
		report.Accounts = append(report.Accounts, uc.collectAccount(ctx, acc, recordedAt))
	}
	return report
}

func (uc *CollectStatus) collectAccount(ctx context.Context, acc domain.Account, recordedAt time.Time) AccountResult {
	result := AccountResult{AccountName: acc.Name}

	accountID, err := uc.repo.SaveAccount(ctx, acc)
	if err != nil {
		result.Err = fmt.Errorf("アカウントの登録に失敗しました: %w", err)
		return result
	}

	devices, err := uc.gateway.ListDevices(ctx, acc.Credential)
	if err != nil {
		result.Err = fmt.Errorf("デバイス一覧の取得に失敗しました: %w", err)
		return result
	}

	result.Devices = make([]DeviceResult, 0, len(devices))
	for _, dev := range devices {
		result.Devices = append(result.Devices,
			uc.collectDevice(ctx, acc.Credential, accountID, dev, recordedAt))
	}
	return result
}

func (uc *CollectStatus) collectDevice(
	ctx context.Context,
	cred domain.Credential,
	accountID domain.AccountID,
	dev domain.Device,
	recordedAt time.Time,
) DeviceResult {
	recordID, err := uc.repo.SaveDevice(ctx, accountID, dev)
	if err != nil {
		return failed(dev, fmt.Errorf("デバイスの登録に失敗しました: %w", err))
	}

	// ステータスを取得できないデバイスは、登録だけ済ませて次へ進む。
	if !dev.StatusReadable() {
		return DeviceResult{Device: dev, Outcome: skipOutcome(dev)}
	}

	payload, err := uc.gateway.FetchStatus(ctx, cred, dev.ID)
	if err != nil {
		return failed(dev, fmt.Errorf("ステータスの取得に失敗しました: %w", err))
	}

	snapshot := domain.StatusSnapshot{Payload: payload, RecordedAt: recordedAt}
	if err := uc.repo.AppendStatus(ctx, recordID, snapshot); err != nil {
		return failed(dev, fmt.Errorf("ステータスログの保存に失敗しました: %w", err))
	}

	return DeviceResult{Device: dev, Outcome: OutcomeStored}
}

func failed(dev domain.Device, err error) DeviceResult {
	return DeviceResult{Device: dev, Outcome: OutcomeFailed, Err: err}
}

// skipOutcome はステータスを取得しなかった理由を Outcome として返す。
func skipOutcome(dev domain.Device) Outcome {
	if dev.Kind == domain.DeviceKindInfraredRemote {
		return OutcomeRegisteredOnly
	}
	return OutcomeSkippedCloudDisabled
}
```

- [ ] **Step 6: テストが通ることを確認する**

Run: `go test ./internal/usecase/ -v`
Expected: 12個のテストがすべて PASS

- [ ] **Step 7: 依存規則を確認する**

Run: `go list -deps ./internal/usecase/ | grep switchBotStore`
Expected: `switchBotStore/internal/domain` と `switchBotStore/internal/usecase` の2行のみ

Run: `grep -rn "log" internal/usecase/*.go | grep -v "_test.go"`
Expected: 出力なし（ユースケース層はログを吐かない）

- [ ] **Step 8: 全体のビルドとテストを確認する**

Run: `go build ./... && go test ./...`
Expected: ビルド成功、全パッケージ `ok`

- [ ] **Step 9: コミット**

```bash
git add internal/usecase/
git commit -m "feat: ユースケース層を追加 (結果を CollectReport で返す)"
```

---

### Task 4: SwitchBot アダプタの新設

**Files:**
- Create: `internal/adapter/switchbot/signer.go`
- Create: `internal/adapter/switchbot/dto.go`
- Create: `internal/adapter/switchbot/mapper.go`
- Create: `internal/adapter/switchbot/client.go`
- Test: `internal/adapter/switchbot/signer_test.go`
- Test: `internal/adapter/switchbot/mapper_test.go`
- Test: `internal/adapter/switchbot/client_test.go`

**Interfaces:**
- Consumes: Task 2 の `domain.*`、Task 3 の `usecase.DeviceGateway`（実装対象）
- Produces: Task 10 が使う `func NewGateway() *Gateway` と `switchbot.DefaultAPIBase`。`*Gateway` は `usecase.DeviceGateway` を満たす

- [ ] **Step 1: 署名生成の失敗するテストを書く**

Create `internal/adapter/switchbot/signer_test.go`:

```go
package switchbot

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSigner_署名が仕様どおり生成される(t *testing.T) {
	const (
		token  = "mytoken"
		secret = "mysecret"
		nonce  = "fixed-nonce"
	)
	fixed := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	s := &signer{now: func() time.Time { return fixed }}
	got := s.signWithNonce(token, secret, nonce)

	ts := strconv.FormatInt(fixed.UnixMilli(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(token + ts + nonce))
	want := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	assert.Equal(t, want, got.Sign)
	assert.Equal(t, token, got.Authorization)
	assert.Equal(t, nonce, got.Nonce)
	assert.Equal(t, ts, got.Timestamp)
}

func TestSigner_必須項目が空でない(t *testing.T) {
	s := newSigner()
	got, err := s.sign("tok", "sec")

	require.NoError(t, err)
	assert.NotEmpty(t, got.Authorization)
	assert.NotEmpty(t, got.Sign)
	assert.NotEmpty(t, got.Nonce)
	assert.NotEmpty(t, got.Timestamp)
}

func TestSigner_nonceは毎回変わる(t *testing.T) {
	s := newSigner()

	first, err := s.sign("tok", "sec")
	require.NoError(t, err)
	second, err := s.sign("tok", "sec")
	require.NoError(t, err)

	assert.NotEqual(t, first.Nonce, second.Nonce)
}
```

- [ ] **Step 2: テストが失敗することを確認する**

Run: `go test ./internal/adapter/switchbot/`
Expected: FAIL（パッケージが存在しない）

- [ ] **Step 3: 署名生成を実装する**

Create `internal/adapter/switchbot/signer.go`:

```go
// Package switchbot は SwitchBot API v1.1 に対する usecase.DeviceGateway の実装。
package switchbot

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"time"
)

// authHeaders は SwitchBot API が要求する認証ヘッダーの値一式。
type authHeaders struct {
	Authorization string
	Sign          string
	Nonce         string
	Timestamp     string
}

// signer は SwitchBot API の認証ヘッダーを生成する。
//
// 署名仕様: Base64( HMAC-SHA256( token + timestamp_ms + nonce, secret ) )
type signer struct {
	now      func() time.Time
	newNonce func() (string, error)
}

func newSigner() *signer {
	return &signer{now: time.Now, newNonce: randomNonce}
}

// sign はランダムな nonce を生成して認証ヘッダーを返す。
func (s *signer) sign(token, secret string) (authHeaders, error) {
	nonce, err := s.newNonce()
	if err != nil {
		return authHeaders{}, err
	}
	return s.signWithNonce(token, secret, nonce), nil
}

// signWithNonce は nonce を指定して認証ヘッダーを生成する（テストで固定するため分離）。
func (s *signer) signWithNonce(token, secret, nonce string) authHeaders {
	ts := strconv.FormatInt(s.now().UnixMilli(), 10)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(token + ts + nonce))

	return authHeaders{
		Authorization: token,
		Sign:          base64.StdEncoding.EncodeToString(mac.Sum(nil)),
		Nonce:         nonce,
		Timestamp:     ts,
	}
}

// randomNonce は UUID 形式のランダム文字列を返す。
func randomNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("nonce の生成に失敗しました: %w", err)
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}
```

`signer_test.go` の `&signer{now: ...}` は `newNonce` を設定していないが、`signWithNonce` は `newNonce` を使わないため nil のままで問題ない。

- [ ] **Step 4: 署名のテストが通ることを確認する**

Run: `go test ./internal/adapter/switchbot/ -run TestSigner -v`
Expected: 3テストが PASS

- [ ] **Step 5: DTO と変換のテストを書く**

Create `internal/adapter/switchbot/mapper_test.go`:

```go
package switchbot

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"switchBotStore/internal/domain"
)

func TestToDomainDevices_物理と赤外線を単一リストに畳み込む(t *testing.T) {
	body := deviceListBody{
		DeviceList: []deviceInfo{
			{DeviceID: "d1", DeviceName: "温湿度計", DeviceType: "Meter", HubDeviceID: "hub1", EnableCloudService: true},
		},
		InfraredRemoteList: []deviceInfo{
			{DeviceID: "ir1", DeviceName: "エアコン", DeviceType: "Air Conditioner"},
		},
	}

	got := toDomainDevices(body)

	require.Len(t, got, 2)

	assert.Equal(t, domain.Device{
		ID:                  "d1",
		Name:                "温湿度計",
		Type:                "Meter",
		HubID:               "hub1",
		Kind:                domain.DeviceKindPhysical,
		CloudServiceEnabled: true,
	}, got[0])

	assert.Equal(t, domain.DeviceKindInfraredRemote, got[1].Kind)
	assert.Equal(t, domain.DeviceID("ir1"), got[1].ID)
	assert.False(t, got[1].StatusReadable(), "赤外線リモコンはステータスを取得できない")
}

func TestToDomainDevices_空のレスポンスで空スライスを返す(t *testing.T) {
	got := toDomainDevices(deviceListBody{})
	assert.Empty(t, got)
}
```

- [ ] **Step 6: DTO を実装する**

Create `internal/adapter/switchbot/dto.go`:

```go
package switchbot

import "encoding/json"

// statusCodeSuccess は SwitchBot API が成功時に返す statusCode。
const statusCodeSuccess = 100

// apiResponse は SwitchBot API 共通のレスポンス封筒。
type apiResponse struct {
	StatusCode int             `json:"statusCode"`
	Message    string          `json:"message"`
	Body       json.RawMessage `json:"body"`
}

// deviceListBody は GET /v1.1/devices のレスポンス body。
type deviceListBody struct {
	DeviceList         []deviceInfo `json:"deviceList"`
	InfraredRemoteList []deviceInfo `json:"infraredRemoteList"`
}

// deviceInfo は API が返すデバイス1件の情報。
//
// 注意: SwitchBot API は infraredRemoteList の要素に deviceType ではなく
// remoteType を返すため、赤外線リモコンの DeviceType は空文字になる。
// これは現行実装から引き継いだ挙動であり、本リファクタリングでは変更しない。
type deviceInfo struct {
	DeviceID           string `json:"deviceId"`
	DeviceName         string `json:"deviceName"`
	DeviceType         string `json:"deviceType"`
	HubDeviceID        string `json:"hubDeviceId"`
	EnableCloudService bool   `json:"enableCloudService"`
}
```

- [ ] **Step 7: 変換を実装する**

Create `internal/adapter/switchbot/mapper.go`:

```go
package switchbot

import "switchBotStore/internal/domain"

// toDomainDevices は API のデバイス一覧を domain.Device のスライスへ畳み込む。
//
// API は物理デバイスと赤外線リモコンを別々の配列で返すが、以降の処理では
// DeviceKind で区別された単一のリストとして扱う。
func toDomainDevices(body deviceListBody) []domain.Device {
	devices := make([]domain.Device, 0, len(body.DeviceList)+len(body.InfraredRemoteList))
	for _, d := range body.DeviceList {
		devices = append(devices, toDomainDevice(d, domain.DeviceKindPhysical))
	}
	for _, d := range body.InfraredRemoteList {
		devices = append(devices, toDomainDevice(d, domain.DeviceKindInfraredRemote))
	}
	return devices
}

func toDomainDevice(d deviceInfo, kind domain.DeviceKind) domain.Device {
	return domain.Device{
		ID:                  domain.DeviceID(d.DeviceID),
		Name:                d.DeviceName,
		Type:                d.DeviceType,
		HubID:               domain.DeviceID(d.HubDeviceID),
		Kind:                kind,
		CloudServiceEnabled: d.EnableCloudService,
	}
}
```

- [ ] **Step 8: 変換のテストが通ることを確認する**

Run: `go test ./internal/adapter/switchbot/ -run TestToDomainDevices -v`
Expected: 2テストが PASS

- [ ] **Step 9: HTTP クライアントのテストを書く**

Create `internal/adapter/switchbot/client_test.go`:

```go
package switchbot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"switchBotStore/internal/domain"
)

var testCred = domain.Credential{Token: "tok", Secret: "sec"}

// newTestGateway は httptest サーバーを向く Gateway を返す。
func newTestGateway(t *testing.T, handler http.HandlerFunc) *Gateway {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	g := NewGateway()
	g.apiBase = srv.URL
	return g
}

func TestListDevices_物理と赤外線をまとめて返す(t *testing.T) {
	g := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1.1/devices", r.URL.Path)
		assert.NotEmpty(t, r.Header.Get("Authorization"))
		assert.NotEmpty(t, r.Header.Get("sign"))
		assert.NotEmpty(t, r.Header.Get("nonce"))
		assert.NotEmpty(t, r.Header.Get("t"))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"statusCode": 100,
			"message": "success",
			"body": {
				"deviceList": [
					{"deviceId":"d1","deviceName":"温湿度計","deviceType":"Meter","enableCloudService":true},
					{"deviceId":"d2","deviceName":"スマートプラグ","deviceType":"Plug Mini (US)","enableCloudService":true}
				],
				"infraredRemoteList": [
					{"deviceId":"ir1","deviceName":"エアコン"}
				]
			}
		}`))
	})

	devices, err := g.ListDevices(context.Background(), testCred)

	require.NoError(t, err)
	require.Len(t, devices, 3)
	assert.Equal(t, domain.DeviceID("d1"), devices[0].ID)
	assert.Equal(t, domain.DeviceKindPhysical, devices[0].Kind)
	assert.Equal(t, domain.DeviceKindInfraredRemote, devices[2].Kind)
}

func TestListDevices_APIエラーを返す(t *testing.T) {
	g := newTestGateway(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"statusCode": 401, "message": "Unauthorized"}`))
	})

	_, err := g.ListDevices(context.Background(), testCred)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "401")
}

func TestListDevices_ネットワークエラーを返す(t *testing.T) {
	g := NewGateway()
	g.apiBase = "http://127.0.0.1:1"

	_, err := g.ListDevices(context.Background(), testCred)
	require.Error(t, err)
}

func TestListDevices_パースできないレスポンスでエラーを返す(t *testing.T) {
	g := newTestGateway(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`これはJSONではない`))
	})

	_, err := g.ListDevices(context.Background(), testCred)
	require.Error(t, err)
}

func TestFetchStatus_生のJSONを返す(t *testing.T) {
	const statusBody = `{"deviceId":"d1","deviceType":"Meter","temperature":25.5,"humidity":60}`

	g := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1.1/devices/d1/status", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"statusCode":100,"message":"success","body":` + statusBody + `}`))
	})

	payload, err := g.FetchStatus(context.Background(), testCred, "d1")

	require.NoError(t, err)
	assert.JSONEq(t, statusBody, string(payload))

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(payload, &decoded))
	assert.Equal(t, "d1", decoded["deviceId"])
}

func TestFetchStatus_APIエラーを返す(t *testing.T) {
	g := newTestGateway(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"statusCode":190,"message":"Device Not Found"}`))
	})

	_, err := g.FetchStatus(context.Background(), testCred, "unknown")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Device Not Found")
}

func TestFetchStatus_キャンセル済みcontextでエラーを返す(t *testing.T) {
	g := newTestGateway(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"statusCode":100,"body":{}}`))
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := g.FetchStatus(ctx, testCred, "d1")
	require.Error(t, err)
}
```

- [ ] **Step 10: HTTP クライアントを実装する**

Create `internal/adapter/switchbot/client.go`:

```go
package switchbot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"switchBotStore/internal/domain"
	"switchBotStore/internal/usecase"
)

// DefaultAPIBase は SwitchBot API のベース URL。
const DefaultAPIBase = "https://api.switch-bot.com"

// defaultTimeout は1リクエストあたりのタイムアウト。
const defaultTimeout = 30 * time.Second

// Gateway が出力ポートを満たすことをコンパイル時に検証する。
var _ usecase.DeviceGateway = (*Gateway)(nil)

// Gateway は SwitchBot API v1.1 に対する usecase.DeviceGateway の実装。
//
// 認証情報をメソッド引数で受け取るため、全アカウントで1インスタンスを共有できる。
type Gateway struct {
	httpClient *http.Client
	apiBase    string
	signer     *signer
}

// NewGateway は既定の HTTP クライアントで Gateway を生成する。
func NewGateway() *Gateway {
	return &Gateway{
		httpClient: &http.Client{Timeout: defaultTimeout},
		apiBase:    DefaultAPIBase,
		signer:     newSigner(),
	}
}

// ListDevices はアカウントに登録された全デバイスを返す。
func (g *Gateway) ListDevices(ctx context.Context, cred domain.Credential) ([]domain.Device, error) {
	body, err := g.get(ctx, cred, "/v1.1/devices")
	if err != nil {
		return nil, err
	}

	var list deviceListBody
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("デバイス一覧のパースに失敗しました: %w", err)
	}
	return toDomainDevices(list), nil
}

// FetchStatus は指定デバイスの現在のステータスを生の JSON で返す。
func (g *Gateway) FetchStatus(ctx context.Context, cred domain.Credential, id domain.DeviceID) (domain.StatusPayload, error) {
	body, err := g.get(ctx, cred, "/v1.1/devices/"+string(id)+"/status")
	if err != nil {
		return nil, err
	}
	return domain.StatusPayload(body), nil
}

// get は認証ヘッダーを付けて GET し、レスポンス封筒の body 部分を返す。
func (g *Gateway) get(ctx context.Context, cred domain.Credential, path string) (json.RawMessage, error) {
	headers, err := g.signer.sign(cred.Token, cred.Secret)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.apiBase+path, nil)
	if err != nil {
		return nil, fmt.Errorf("リクエストの生成に失敗しました: %w", err)
	}
	req.Header.Set("Authorization", headers.Authorization)
	req.Header.Set("sign", headers.Sign)
	req.Header.Set("nonce", headers.Nonce)
	req.Header.Set("t", headers.Timestamp)
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP リクエストに失敗しました: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("レスポンスの読み込みに失敗しました: %w", err)
	}

	var apiResp apiResponse
	if err := json.Unmarshal(raw, &apiResp); err != nil {
		return nil, fmt.Errorf("レスポンスのパースに失敗しました (body=%s): %w", string(raw), err)
	}
	if apiResp.StatusCode != statusCodeSuccess {
		return nil, fmt.Errorf("SwitchBot API がエラーを返しました (statusCode=%d): %s",
			apiResp.StatusCode, apiResp.Message)
	}
	return apiResp.Body, nil
}
```

- [ ] **Step 11: 全テストが通ることを確認する**

Run: `go test ./internal/adapter/switchbot/ -v`
Expected: signer 3件 + mapper 2件 + client 7件、合計12テストが PASS

`var _ usecase.DeviceGateway = (*Gateway)(nil)` により、ポートを満たしていなければここでコンパイルエラーになる。

- [ ] **Step 12: 依存規則を確認する**

Run: `go list -deps ./internal/adapter/switchbot/ | grep switchBotStore`
Expected: `domain` / `usecase` / `adapter/switchbot` の3行のみ

- [ ] **Step 13: 全体のビルドとテストを確認する**

Run: `go build ./... && go test ./...`
Expected: ビルド成功、全パッケージ `ok`

- [ ] **Step 14: コミット**

```bash
git add internal/adapter/switchbot/
git commit -m "feat: SwitchBot API アダプタを追加 (署名・DTO・変換を分離)"
```

---

### Task 5: 設定パッケージの新設（validator 導入）

**Files:**
- Create: `internal/infra/config/config.go`
- Test: `internal/infra/config/config_test.go`
- Modify: `go.mod` / `go.sum`

**Interfaces:**
- Consumes: なし（infra 層は内部パッケージを import しない）
- Produces: Task 10 が使う以下。
  - `config.Config{Database Database; Accounts []Account; LogDir string}`
  - `config.Database{Host string; Port int; User, Password, Name string}`
  - `config.Account{Name, Token, Secret string}`
  - `func Load(path string) (*Config, error)`
  - `const DefaultPort = 3306`

**検証規則は現行実装と完全に同一にする。** 現行 `internal/config/config.go:41-54` が検証しているのは「accounts が1件以上」「各 account の token と secret が非空」「database.host が非空」の3点のみ。`database.user` / `password` / `name` と `accounts[].name` は検証しない。ここに `required` を足すと既存の `config.json` が読めなくなる恐れがある。

- [ ] **Step 1: validator を追加する**

```bash
go get github.com/go-playground/validator/v10@v10.30.3
```

- [ ] **Step 2: 失敗するテストを書く**

Create `internal/infra/config/config_test.go`:

```go
package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"switchBotStore/internal/infra/config"
)

// writeConfig は一時ディレクトリに config.json を書き、そのパスを返す。
func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

const validConfig = `{
  "database": {"host":"localhost","port":3306,"user":"root","password":"pw","name":"switchbot_db"},
  "accounts": [{"name":"メイン","token":"tok1","secret":"sec1"}],
  "log_dir": "logs"
}`

func TestLoad_正常な設定を読み込む(t *testing.T) {
	cfg, err := config.Load(writeConfig(t, validConfig))

	require.NoError(t, err)
	assert.Equal(t, "localhost", cfg.Database.Host)
	assert.Equal(t, 3306, cfg.Database.Port)
	assert.Equal(t, "switchbot_db", cfg.Database.Name)
	assert.Equal(t, "logs", cfg.LogDir)
	require.Len(t, cfg.Accounts, 1)
	assert.Equal(t, "tok1", cfg.Accounts[0].Token)
	assert.Equal(t, "sec1", cfg.Accounts[0].Secret)
}

func TestLoad_port未指定なら3306を補う(t *testing.T) {
	cfg, err := config.Load(writeConfig(t, `{
	  "database": {"host":"localhost","user":"root","password":"pw","name":"db"},
	  "accounts": [{"name":"a","token":"t","secret":"s"}]
	}`))

	require.NoError(t, err)
	assert.Equal(t, config.DefaultPort, cfg.Database.Port)
}

func TestLoad_ファイルが存在しないとエラー(t *testing.T) {
	_, err := config.Load(filepath.Join(t.TempDir(), "missing.json"))
	require.Error(t, err)
}

func TestLoad_不正なJSONでエラー(t *testing.T) {
	_, err := config.Load(writeConfig(t, `{ これはJSONではない`))
	require.Error(t, err)
}

func TestLoad_検証エラーになるケース(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name: "accounts が空配列",
			content: `{"database":{"host":"h"},"accounts":[]}`,
		},
		{
			name: "accounts が未指定",
			content: `{"database":{"host":"h"}}`,
		},
		{
			name: "token が空",
			content: `{"database":{"host":"h"},"accounts":[{"name":"a","token":"","secret":"s"}]}`,
		},
		{
			name: "secret が空",
			content: `{"database":{"host":"h"},"accounts":[{"name":"a","token":"t","secret":""}]}`,
		},
		{
			name: "database.host が空",
			content: `{"database":{"host":""},"accounts":[{"name":"a","token":"t","secret":"s"}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := config.Load(writeConfig(t, tt.content))
			require.Error(t, err)
		})
	}
}

func TestLoad_現行実装が検証しない項目は空でも通る(t *testing.T) {
	// database.user / password / name と accounts[].name は現行実装が
	// 検証していないため、validator 導入後も通らなければならない。
	cfg, err := config.Load(writeConfig(t, `{
	  "database": {"host":"localhost"},
	  "accounts": [{"token":"t","secret":"s"}]
	}`))

	require.NoError(t, err)
	assert.Empty(t, cfg.Database.User)
	assert.Empty(t, cfg.Database.Password)
	assert.Empty(t, cfg.Database.Name)
	assert.Empty(t, cfg.Accounts[0].Name)
}

func TestLoad_複数アカウントを読み込む(t *testing.T) {
	cfg, err := config.Load(writeConfig(t, `{
	  "database": {"host":"h"},
	  "accounts": [
	    {"name":"メイン","token":"t1","secret":"s1"},
	    {"name":"サブ","token":"t2","secret":"s2"}
	  ]
	}`))

	require.NoError(t, err)
	require.Len(t, cfg.Accounts, 2)
	assert.Equal(t, "t2", cfg.Accounts[1].Token)
}
```

- [ ] **Step 3: テストが失敗することを確認する**

Run: `go test ./internal/infra/config/`
Expected: FAIL（パッケージが存在しない）

- [ ] **Step 4: 設定パッケージを実装する**

Create `internal/infra/config/config.go`:

```go
// Package config は config.json の読み込みと検証を行う。
package config

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/go-playground/validator/v10"
)

// DefaultPort は database.port 未指定時に使う MySQL のポート。
const DefaultPort = 3306

// Config は config.json の内容。
type Config struct {
	Database Database  `json:"database"`
	Accounts []Account `json:"accounts" validate:"required,min=1,dive"`
	LogDir   string    `json:"log_dir"`
}

// Database は MySQL への接続情報。
//
// validate タグは現行実装の検証内容をそのまま写したもの。host 以外を
// required にすると既存の config.json が読めなくなる恐れがあるため、
// 本リファクタリングでは検証を強化しない。
type Database struct {
	Host     string `json:"host" validate:"required"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

// Account は SwitchBot API アカウント1件分の設定。
type Account struct {
	Name   string `json:"name"`
	Token  string `json:"token" validate:"required"`
	Secret string `json:"secret" validate:"required"`
}

// Load は path の JSON を読み込み、検証してから返す。
func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("設定ファイルを開けません (%s): %w", path, err)
	}
	defer f.Close()

	cfg := &Config{}
	if err := json.NewDecoder(f).Decode(cfg); err != nil {
		return nil, fmt.Errorf("設定ファイルのパースに失敗しました (%s): %w", path, err)
	}

	if err := validator.New().Struct(cfg); err != nil {
		return nil, fmt.Errorf("設定ファイルの内容が不正です (%s): %w", path, err)
	}

	if cfg.Database.Port == 0 {
		cfg.Database.Port = DefaultPort
	}
	return cfg, nil
}
```

- [ ] **Step 5: テストが通ることを確認する**

Run: `go test ./internal/infra/config/ -v`
Expected: 7つのトップレベルテスト（`TestLoad_検証エラーになるケース` は5サブテスト）がすべて PASS

- [ ] **Step 6: 全体のビルドとテストを確認する**

Run: `go build ./... && go test ./...`
Expected: ビルド成功、全パッケージ `ok`（旧 `internal/config` も残っているので両方通る）

- [ ] **Step 7: コミット**

```bash
git add go.mod go.sum internal/infra/config/
git commit -m "feat: 設定パッケージを infra 層へ移設し validator を導入"
```

---

### Task 6: ログパッケージの新設（slog 導入）

**Files:**
- Create: `internal/infra/logging/logging.go`
- Test: `internal/infra/logging/logging_test.go`

**Interfaces:**
- Consumes: なし
- Produces: Task 9・10 が使う以下。
  - `func New(w io.Writer) *slog.Logger`
  - `func Setup(logDir string, now time.Time) (logger *slog.Logger, closeFn func() error, err error)`

現行 `internal/logger/logger.go` の日次ローテーション（goroutine / mutex / timer / stopCh / sync.Once）は、cron 起動で数秒で終了するプロセスでは一度も発火しないため**全削除**する。ファイル名を日付にすることで日次分割は達成される。

- [ ] **Step 1: 失敗するテストを書く**

Create `internal/infra/logging/logging_test.go`:

```go
package logging_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"switchBotStore/internal/infra/logging"
)

var testNow = time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)

func TestNew_JSON形式で出力する(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.New(&buf)

	logger.Info("保存しました", "device", "温湿度計", "count", 3)

	var entry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
	assert.Equal(t, "保存しました", entry["msg"])
	assert.Equal(t, "INFO", entry["level"])
	assert.Equal(t, "温湿度計", entry["device"])
	assert.Equal(t, float64(3), entry["count"])
}

func TestSetup_日付名のログファイルを作る(t *testing.T) {
	dir := t.TempDir()

	logger, closeFn, err := logging.Setup(dir, testNow)
	require.NoError(t, err)
	t.Cleanup(func() { _ = closeFn() })

	logger.Info("テストメッセージ")
	require.NoError(t, closeFn())

	path := filepath.Join(dir, "2026-07-29.log")
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(content), "テストメッセージ")
}

func TestSetup_存在しないディレクトリを作る(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "logs")

	_, closeFn, err := logging.Setup(dir, testNow)
	require.NoError(t, err)
	t.Cleanup(func() { _ = closeFn() })

	assert.DirExists(t, dir)
}

func TestSetup_既存ファイルに追記する(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "2026-07-29.log")
	require.NoError(t, os.WriteFile(path, []byte("既存の行\n"), 0o644))

	logger, closeFn, err := logging.Setup(dir, testNow)
	require.NoError(t, err)
	logger.Info("追記した行")
	require.NoError(t, closeFn())

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(content), "既存の行")
	assert.Contains(t, string(content), "追記した行")
}

func TestSetup_logDirが空ならファイルを作らない(t *testing.T) {
	dir := t.TempDir()

	logger, closeFn, err := logging.Setup("", testNow)
	require.NoError(t, err)
	require.NotNil(t, logger)
	require.NoError(t, closeFn())

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestSetup_closeを二重に呼んでもパニックしない(t *testing.T) {
	logger, closeFn, err := logging.Setup(t.TempDir(), testNow)
	require.NoError(t, err)
	logger.Info("何か")

	require.NoError(t, closeFn())
	assert.NotPanics(t, func() { _ = closeFn() })
}
```

- [ ] **Step 2: テストが失敗することを確認する**

Run: `go test ./internal/infra/logging/`
Expected: FAIL（パッケージが存在しない）

- [ ] **Step 3: ログパッケージを実装する**

Create `internal/infra/logging/logging.go`:

```go
// Package logging はアプリケーションのログ出力先を設定する。
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// New は w へ JSON 形式で書き出す slog.Logger を返す。
func New(w io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

// Setup は logDir に「YYYY-MM-DD.log」を開き、標準エラー出力とファイルの
// 両方へ JSON 形式で出力する slog.Logger を返す。
//
// logDir が空の場合は標準エラー出力のみに出力し、ファイルは作らない。
// 戻り値の closeFn はプロセス終了時に必ず呼ぶこと（複数回呼んでも安全）。
//
// 本アプリは cron から起動されて数秒で終了するため、実行中のローテーションは
// 行わない。日付をファイル名にすることで日次分割が達成される。
func Setup(logDir string, now time.Time) (logger *slog.Logger, closeFn func() error, err error) {
	if logDir == "" {
		return New(os.Stderr), func() error { return nil }, nil
	}

	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, nil, fmt.Errorf("ログディレクトリの作成に失敗しました (%s): %w", logDir, err)
	}

	path := filepath.Join(logDir, now.Format("2006-01-02")+".log")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("ログファイルを開けません (%s): %w", path, err)
	}

	closed := false
	closeFn = func() error {
		if closed {
			return nil
		}
		closed = true
		return f.Close()
	}

	return New(io.MultiWriter(os.Stderr, f)), closeFn, nil
}
```

- [ ] **Step 4: テストが通ることを確認する**

Run: `go test ./internal/infra/logging/ -v`
Expected: 6テストがすべて PASS

- [ ] **Step 5: 全体のビルドとテストを確認する**

Run: `go build ./... && go test ./...`
Expected: ビルド成功、全パッケージ `ok`

- [ ] **Step 6: コミット**

```bash
git add internal/infra/logging/
git commit -m "feat: slog によるログパッケージを追加 (自前ローテーションを廃止)"
```

---

### Task 7: MySQL 接続と goose マイグレーション

**Files:**
- Create: `internal/infra/mysql/connect.go`
- Create: `internal/infra/mysql/migrate.go`
- Create: `internal/infra/mysql/migrations/00001_initial_schema.sql`
- Test: `internal/infra/mysql/connect_test.go`
- Modify: `go.mod` / `go.sum`

**Interfaces:**
- Consumes: なし
- Produces: Task 10 が使う以下。
  - `mysql.Config{Host string; Port int; User, Password, Name string}`
  - `func (c Config) DSN() string`
  - `func Connect(ctx context.Context, cfg Config) (db *gorm.DB, closeFn func() error, err error)`
  - `func Migrate(ctx context.Context, db *gorm.DB) error`

- [ ] **Step 1: GORM と goose を追加する**

```bash
go get gorm.io/gorm@v1.31.2 gorm.io/driver/mysql@v1.6.0 github.com/pressly/goose/v3@v3.27.3
```

- [ ] **Step 2: DSN のテストを書く**

Create `internal/infra/mysql/connect_test.go`:

```go
package mysql_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"switchBotStore/internal/infra/mysql"
)

func TestConfig_DSN(t *testing.T) {
	cfg := mysql.Config{
		Host:     "localhost",
		Port:     3306,
		User:     "root",
		Password: "pw",
		Name:     "switchbot_db",
	}

	// 現行実装 (internal/database/database.go:15-16) と同一の DSN を生成すること。
	assert.Equal(t,
		"root:pw@tcp(localhost:3306)/switchbot_db?parseTime=true&loc=Local&charset=utf8mb4",
		cfg.DSN())
}

func TestConnect_接続できないホストでエラーを返す(t *testing.T) {
	cfg := mysql.Config{Host: "127.0.0.1", Port: 1, User: "u", Password: "p", Name: "d"}

	_, _, err := mysql.Connect(t.Context(), cfg)
	assert.Error(t, err)
}
```

`t.Context()` は Go 1.24 以降で利用できる。

- [ ] **Step 3: テストが失敗することを確認する**

Run: `go test ./internal/infra/mysql/`
Expected: FAIL（パッケージが存在しない）

- [ ] **Step 4: 接続処理を実装する**

Create `internal/infra/mysql/connect.go`:

```go
// Package mysql は MySQL への接続とスキーマのマイグレーションを行う。
package mysql

import (
	"context"
	"fmt"
	"time"

	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// コネクションプールの設定。現行実装 (internal/database/database.go:27-29) と同じ値。
const (
	maxOpenConns    = 10
	maxIdleConns    = 5
	connMaxLifetime = time.Hour
)

// Config は MySQL への接続情報。
type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	Name     string
}

// DSN は go-sql-driver/mysql 形式の接続文字列を返す。
func (c Config) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&loc=Local&charset=utf8mb4",
		c.User, c.Password, c.Host, c.Port, c.Name)
}

// Connect は MySQL に接続し、疎通確認まで済んだ *gorm.DB を返す。
//
// 戻り値の closeFn はプロセス終了時に必ず呼ぶこと。
func Connect(ctx context.Context, cfg Config) (db *gorm.DB, closeFn func() error, err error) {
	db, err = gorm.Open(gormmysql.Open(cfg.DSN()), &gorm.Config{
		// アプリのログは slog に一本化するため、GORM 自身のログは捨てる。
		Logger: gormlogger.Discard,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("DB 接続の初期化に失敗しました: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, nil, fmt.Errorf("DB ハンドルの取得に失敗しました: %w", err)
	}
	sqlDB.SetMaxOpenConns(maxOpenConns)
	sqlDB.SetMaxIdleConns(maxIdleConns)
	sqlDB.SetConnMaxLifetime(connMaxLifetime)

	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, nil, fmt.Errorf("DB への疎通確認に失敗しました: %w", err)
	}

	return db, sqlDB.Close, nil
}
```

- [ ] **Step 5: マイグレーション SQL を作る**

Create `internal/infra/mysql/migrations/00001_initial_schema.sql`:

現行 `internal/database/database.go:41-93` の DDL をそのまま移す。**1文字も変更しないこと**（実データのあるテーブルとの差異を作らないため）。

```sql
-- +goose Up
-- 現行 internal/database/database.go の DDL をそのまま移設したもの。
-- 全て IF NOT EXISTS のため、既にテーブルが存在する DB では no-op になり、
-- goose のバージョン1として記録されるだけになる。
CREATE TABLE IF NOT EXISTS api_accounts (
    id         INT AUTO_INCREMENT PRIMARY KEY,
    name       VARCHAR(255)  NOT NULL COMMENT 'アカウント識別名',
    token      VARCHAR(255)  NOT NULL COMMENT 'SwitchBot APIトークン',
    secret     VARCHAR(255)  NOT NULL COMMENT 'SwitchBot APIシークレット',
    created_at TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uq_token (token)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='SwitchBot APIアカウント';

CREATE TABLE IF NOT EXISTS devices (
    id                   INT AUTO_INCREMENT PRIMARY KEY,
    api_account_id       INT          NOT NULL COMMENT 'api_accounts.id',
    device_id            VARCHAR(255) NOT NULL COMMENT 'SwitchBot デバイスID',
    device_name          VARCHAR(255)          COMMENT 'デバイス名',
    device_type          VARCHAR(100)          COMMENT 'デバイス種別',
    hub_device_id        VARCHAR(255)          COMMENT '接続先ハブのデバイスID',
    enable_cloud_service TINYINT(1)   NOT NULL DEFAULT 0 COMMENT 'クラウドサービス有効フラグ',
    is_virtual_infrared  TINYINT(1)   NOT NULL DEFAULT 0 COMMENT '仮想赤外線リモコンフラグ',
    created_at           TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at           TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (api_account_id) REFERENCES api_accounts(id),
    UNIQUE KEY uq_account_device (api_account_id, device_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='SwitchBot デバイス情報';

CREATE TABLE IF NOT EXISTS device_status_logs (
    id          BIGINT    AUTO_INCREMENT PRIMARY KEY,
    device_id   INT       NOT NULL COMMENT 'devices.id',
    status_data JSON      NOT NULL COMMENT 'APIから取得したステータスデータ(JSON)',
    recorded_at DATETIME  NOT NULL COMMENT 'データ収集日時',
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (device_id) REFERENCES devices(id),
    INDEX idx_device_recorded (device_id, recorded_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='デバイスステータス収集ログ';

CREATE TABLE IF NOT EXISTS system_settings (
    id            INT AUTO_INCREMENT PRIMARY KEY,
    setting_key   VARCHAR(100) NOT NULL COMMENT '設定キー',
    setting_value TEXT                  COMMENT '設定値',
    created_at    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uq_setting_key (setting_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='システム設定';

-- +goose Down
-- 実データを失うため、意図的にロールバックを行わない。
SELECT 1;
```

`system_settings` はコードから参照しなくなるが、既存データを保持するためテーブルは残す。

- [ ] **Step 6: マイグレーション実行を実装する**

Create `internal/infra/mysql/migrate.go`:

```go
package mysql

import (
	"context"
	"embed"
	"fmt"

	"github.com/pressly/goose/v3"
	"gorm.io/gorm"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// migrationDir は migrationFS 内のマイグレーション配置ディレクトリ。
const migrationDir = "migrations"

// Migrate は未適用のマイグレーションを適用する。
//
// マイグレーションは実行ファイルに埋め込まれているため、別途 goose バイナリを
// 配布する必要はない。
func Migrate(ctx context.Context, db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("DB ハンドルの取得に失敗しました: %w", err)
	}

	goose.SetBaseFS(migrationFS)
	// goose 自身の標準出力へのログは抑止し、アプリのログを slog に一本化する。
	goose.SetLogger(goose.NopLogger())

	if err := goose.SetDialect("mysql"); err != nil {
		return fmt.Errorf("goose の dialect 設定に失敗しました: %w", err)
	}
	if err := goose.UpContext(ctx, sqlDB, migrationDir); err != nil {
		return fmt.Errorf("マイグレーションの適用に失敗しました: %w", err)
	}
	return nil
}
```

- [ ] **Step 7: テストが通ることを確認する**

Run: `go test ./internal/infra/mysql/ -v`
Expected: 2テストが PASS

- [ ] **Step 8: マイグレーションが実行ファイルに埋め込まれることを確認する**

Run: `go build ./... && go vet ./internal/infra/mysql/`
Expected: エラーなし（`//go:embed` のパスが誤っているとここで失敗する）

- [ ] **Step 9: 全体のビルドとテストを確認する**

Run: `go build ./... && go test ./...`
Expected: ビルド成功、全パッケージ `ok`

- [ ] **Step 10: コミット**

```bash
git add go.mod go.sum internal/infra/mysql/
git commit -m "feat: GORM 接続と goose マイグレーションを追加"
```

---

### Task 8: 永続化アダプタの新設（GORM 実装）

**Files:**
- Create: `internal/adapter/persistence/model.go`
- Create: `internal/adapter/persistence/mapper.go`
- Create: `internal/adapter/persistence/store.go`
- Create: `internal/adapter/persistence/verify.go`
- Test: `internal/adapter/persistence/mapper_test.go`
- Test: `internal/adapter/persistence/verify_test.go`

**Interfaces:**
- Consumes: Task 2 の `domain.*`、Task 3 の `usecase.Repository`（実装対象）、Task 7 の `*gorm.DB`
- Produces: Task 10 が使う以下。
  - `func New(db *gorm.DB) *Store`（`*Store` は `usecase.Repository` を満たす）
  - `func VerifySchema(db *gorm.DB) error`

**DB アクセス自体のテストは行わない**（仕様書 §8）。純粋関数である `mapper` と、GORM モデルからカラム名を導出する部分のみテストする。スキーマとモデルのズレは起動時の `VerifySchema` が検出する。

- [ ] **Step 1: GORM モデルを定義する**

Create `internal/adapter/persistence/model.go`:

```go
// Package persistence は GORM による usecase.Repository の実装。
//
// GORM のタグはこのパッケージの外へ出さない。domain は GORM を知らない。
// スキーマの正は internal/infra/mysql/migrations/*.sql 側にあり、ここの
// モデルはそこへのマッピングにすぎない（VerifySchema がズレを検出する）。
package persistence

import "time"

// apiAccountModel は api_accounts テーブルの1行。
type apiAccountModel struct {
	ID        int64     `gorm:"column:id;primaryKey;autoIncrement"`
	Name      string    `gorm:"column:name"`
	Token     string    `gorm:"column:token"`
	Secret    string    `gorm:"column:secret"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

// TableName は GORM が使うテーブル名を返す。
func (apiAccountModel) TableName() string { return "api_accounts" }

// deviceModel は devices テーブルの1行。
type deviceModel struct {
	ID                 int64     `gorm:"column:id;primaryKey;autoIncrement"`
	APIAccountID       int64     `gorm:"column:api_account_id"`
	DeviceID           string    `gorm:"column:device_id"`
	DeviceName         string    `gorm:"column:device_name"`
	DeviceType         string    `gorm:"column:device_type"`
	HubDeviceID        string    `gorm:"column:hub_device_id"`
	EnableCloudService bool      `gorm:"column:enable_cloud_service"`
	IsVirtualInfrared  bool      `gorm:"column:is_virtual_infrared"`
	CreatedAt          time.Time `gorm:"column:created_at"`
	UpdatedAt          time.Time `gorm:"column:updated_at"`
}

// TableName は GORM が使うテーブル名を返す。
func (deviceModel) TableName() string { return "devices" }

// deviceStatusLogModel は device_status_logs テーブルの1行。
type deviceStatusLogModel struct {
	ID         int64     `gorm:"column:id;primaryKey;autoIncrement"`
	DeviceID   int64     `gorm:"column:device_id"`
	StatusData string    `gorm:"column:status_data"`
	RecordedAt time.Time `gorm:"column:recorded_at"`
	CreatedAt  time.Time `gorm:"column:created_at"`
}

// TableName は GORM が使うテーブル名を返す。
func (deviceStatusLogModel) TableName() string { return "device_status_logs" }

// allModels は VerifySchema が検査する全モデル。
// 新しいモデルを追加したらここにも足すこと。
func allModels() []any {
	return []any{
		apiAccountModel{},
		deviceModel{},
		deviceStatusLogModel{},
	}
}
```

- [ ] **Step 2: 変換のテストを書く**

Create `internal/adapter/persistence/mapper_test.go`:

```go
package persistence

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"switchBotStore/internal/domain"
)

func TestToAccountModel(t *testing.T) {
	got := toAccountModel(domain.Account{
		Name:       "メインアカウント",
		Credential: domain.Credential{Token: "tok", Secret: "sec"},
	})

	assert.Equal(t, "メインアカウント", got.Name)
	assert.Equal(t, "tok", got.Token)
	assert.Equal(t, "sec", got.Secret)
}

func TestToDeviceModel_物理デバイス(t *testing.T) {
	got := toDeviceModel(domain.AccountID(7), domain.Device{
		ID:                  "d1",
		Name:                "温湿度計",
		Type:                "Meter",
		HubID:               "hub1",
		Kind:                domain.DeviceKindPhysical,
		CloudServiceEnabled: true,
	})

	assert.Equal(t, int64(7), got.APIAccountID)
	assert.Equal(t, "d1", got.DeviceID)
	assert.Equal(t, "温湿度計", got.DeviceName)
	assert.Equal(t, "Meter", got.DeviceType)
	assert.Equal(t, "hub1", got.HubDeviceID)
	assert.True(t, got.EnableCloudService)
	assert.False(t, got.IsVirtualInfrared)
}

func TestToDeviceModel_赤外線リモコンはフラグが立つ(t *testing.T) {
	got := toDeviceModel(domain.AccountID(1), domain.Device{
		ID:   "ir1",
		Kind: domain.DeviceKindInfraredRemote,
	})

	assert.True(t, got.IsVirtualInfrared)
}

func TestToStatusLogModel(t *testing.T) {
	at := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)

	got := toStatusLogModel(domain.DeviceRecordID(42), domain.StatusSnapshot{
		Payload:    []byte(`{"temperature":25.5}`),
		RecordedAt: at,
	})

	assert.Equal(t, int64(42), got.DeviceID)
	assert.JSONEq(t, `{"temperature":25.5}`, got.StatusData)
	assert.Equal(t, at, got.RecordedAt)
}
```

- [ ] **Step 3: テストが失敗することを確認する**

Run: `go test ./internal/adapter/persistence/`
Expected: FAIL。`undefined: toAccountModel`

- [ ] **Step 4: 変換を実装する**

Create `internal/adapter/persistence/mapper.go`:

```go
package persistence

import "switchBotStore/internal/domain"

func toAccountModel(a domain.Account) apiAccountModel {
	return apiAccountModel{
		Name:   a.Name,
		Token:  a.Credential.Token,
		Secret: a.Credential.Secret,
	}
}

func toDeviceModel(accountID domain.AccountID, d domain.Device) deviceModel {
	return deviceModel{
		APIAccountID:       int64(accountID),
		DeviceID:           string(d.ID),
		DeviceName:         d.Name,
		DeviceType:         d.Type,
		HubDeviceID:        string(d.HubID),
		EnableCloudService: d.CloudServiceEnabled,
		IsVirtualInfrared:  d.Kind == domain.DeviceKindInfraredRemote,
	}
}

func toStatusLogModel(id domain.DeviceRecordID, s domain.StatusSnapshot) deviceStatusLogModel {
	return deviceStatusLogModel{
		DeviceID:   int64(id),
		StatusData: string(s.Payload),
		RecordedAt: s.RecordedAt,
	}
}
```

- [ ] **Step 5: 変換のテストが通ることを確認する**

Run: `go test ./internal/adapter/persistence/ -v`
Expected: 4テストが PASS

- [ ] **Step 6: Repository 実装を書く**

Create `internal/adapter/persistence/store.go`:

```go
package persistence

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"switchBotStore/internal/domain"
	"switchBotStore/internal/usecase"
)

// Store が出力ポートを満たすことをコンパイル時に検証する。
var _ usecase.Repository = (*Store)(nil)

// Store は usecase.Repository の GORM 実装。
type Store struct {
	db *gorm.DB
}

// New は Store を生成する。
func New(db *gorm.DB) *Store {
	return &Store{db: db}
}

// SaveAccount はアカウントを登録し、token が同一の行が既にあれば更新する。
//
// MySQL の ON DUPLICATE KEY UPDATE は更新時に LAST_INSERT_ID() を更新しない
// ため、INSERT が返す ID は信頼できない。そのため id を SELECT し直す
// 2 クエリ方式を採る（現行実装と同じ）。
func (s *Store) SaveAccount(ctx context.Context, a domain.Account) (domain.AccountID, error) {
	m := toAccountModel(a)

	err := s.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "token"}},
			DoUpdates: clause.AssignmentColumns([]string{"name", "secret", "updated_at"}),
		}).
		Create(&m).Error
	if err != nil {
		return 0, fmt.Errorf("アカウントの保存に失敗しました: %w", err)
	}

	// 受け皿はモデル構造体にする。単一カラムをプリミティブ型へ直接
	// Take する書き方は GORM の挙動が分かりにくいため使わない。
	var saved apiAccountModel
	err = s.db.WithContext(ctx).
		Select("id").
		Where("token = ?", a.Credential.Token).
		Take(&saved).Error
	if err != nil {
		return 0, fmt.Errorf("アカウント ID の取得に失敗しました: %w", err)
	}
	return domain.AccountID(saved.ID), nil
}

// SaveDevice はデバイスを登録し、(api_account_id, device_id) が同一の行が
// 既にあれば更新する。ID の取得方法は SaveAccount と同じ理由で 2 クエリ。
func (s *Store) SaveDevice(ctx context.Context, accountID domain.AccountID, d domain.Device) (domain.DeviceRecordID, error) {
	m := toDeviceModel(accountID, d)

	err := s.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "api_account_id"}, {Name: "device_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"device_name", "device_type", "hub_device_id", "enable_cloud_service", "updated_at",
			}),
		}).
		Create(&m).Error
	if err != nil {
		return 0, fmt.Errorf("デバイスの保存に失敗しました: %w", err)
	}

	var saved deviceModel
	err = s.db.WithContext(ctx).
		Select("id").
		Where("api_account_id = ? AND device_id = ?", int64(accountID), string(d.ID)).
		Take(&saved).Error
	if err != nil {
		return 0, fmt.Errorf("デバイス ID の取得に失敗しました: %w", err)
	}
	return domain.DeviceRecordID(saved.ID), nil
}

// AppendStatus はステータス収集ログを1件追加する。
func (s *Store) AppendStatus(ctx context.Context, id domain.DeviceRecordID, snapshot domain.StatusSnapshot) error {
	m := toStatusLogModel(id, snapshot)
	if err := s.db.WithContext(ctx).Create(&m).Error; err != nil {
		return fmt.Errorf("ステータスログの保存に失敗しました: %w", err)
	}
	return nil
}
```

現行実装の `is_virtual_infrared` は更新対象に含まれていない（`repository.go:50-55`）。同じ挙動を保つため `DoUpdates` にも含めていない。

- [ ] **Step 7: スキーマ整合性チェックのテストを書く**

Create `internal/adapter/persistence/verify_test.go`:

```go
package persistence

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpectedSchema_モデルからテーブル名とカラム名を導出する(t *testing.T) {
	tests := []struct {
		model       any
		wantTable   string
		wantColumns []string
	}{
		{
			model:     apiAccountModel{},
			wantTable: "api_accounts",
			wantColumns: []string{
				"id", "name", "token", "secret", "created_at", "updated_at",
			},
		},
		{
			model:     deviceModel{},
			wantTable: "devices",
			wantColumns: []string{
				"id", "api_account_id", "device_id", "device_name", "device_type",
				"hub_device_id", "enable_cloud_service", "is_virtual_infrared",
				"created_at", "updated_at",
			},
		},
		{
			model:     deviceStatusLogModel{},
			wantTable: "device_status_logs",
			wantColumns: []string{
				"id", "device_id", "status_data", "recorded_at", "created_at",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.wantTable, func(t *testing.T) {
			table, columns, err := expectedSchema(tt.model)

			require.NoError(t, err)
			assert.Equal(t, tt.wantTable, table)
			assert.ElementsMatch(t, tt.wantColumns, columns)
		})
	}
}

func TestAllModels_全モデルを列挙している(t *testing.T) {
	assert.Len(t, allModels(), 3)
}
```

- [ ] **Step 8: スキーマ整合性チェックを実装する**

Create `internal/adapter/persistence/verify.go`:

```go
package persistence

import (
	"fmt"
	"sync"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// VerifySchema は GORM モデルが期待するテーブルとカラムが実 DB に
// 存在するかを検証する。
//
// スキーマの正は internal/infra/mysql/migrations/*.sql (goose) 側にあり、
// このパッケージの GORM モデルはそこへのマッピングにすぎない。両者がズレた
// 場合に、実行時の不可解な SQL エラーではなく起動時の明示的なエラーとして
// 検出するために存在する。マイグレーション適用の直後に呼ぶこと。
func VerifySchema(db *gorm.DB) error {
	migrator := db.Migrator()

	for _, model := range allModels() {
		table, columns, err := expectedSchema(model)
		if err != nil {
			return fmt.Errorf("GORM モデルの解析に失敗しました: %w", err)
		}

		if !migrator.HasTable(model) {
			return fmt.Errorf("テーブルが存在しません: %s", table)
		}
		for _, col := range columns {
			if !migrator.HasColumn(model, col) {
				return fmt.Errorf("カラムが存在しません: %s.%s", table, col)
			}
		}
	}
	return nil
}

// expectedSchema は GORM モデルからテーブル名と全カラム名を導出する。
// DB 接続を必要としないため単体でテストできる。
func expectedSchema(model any) (table string, columns []string, err error) {
	parsed, err := schema.Parse(model, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		return "", nil, err
	}

	columns = make([]string, 0, len(parsed.Fields))
	for _, f := range parsed.Fields {
		if f.DBName != "" {
			columns = append(columns, f.DBName)
		}
	}
	return parsed.Table, columns, nil
}
```

- [ ] **Step 9: テストが通ることを確認する**

Run: `go test ./internal/adapter/persistence/ -v`
Expected: mapper 4件 + verify 2件（うち1件は3サブテスト）がすべて PASS

- [ ] **Step 10: 依存規則を確認する**

Run: `go list -deps ./internal/adapter/persistence/ | grep switchBotStore`
Expected: `domain` / `usecase` / `adapter/persistence` の3行のみ（`infra` が出たら依存規則違反）

- [ ] **Step 11: 全体のビルドとテストを確認する**

Run: `go build ./... && go test ./...`
Expected: ビルド成功、全パッケージ `ok`

- [ ] **Step 12: コミット**

```bash
git add internal/adapter/persistence/
git commit -m "feat: GORM による永続化アダプタとスキーマ整合性チェックを追加"
```

---

### Task 9: プレゼンタの新設

**Files:**
- Create: `internal/adapter/presenter/slog_presenter.go`
- Test: `internal/adapter/presenter/slog_presenter_test.go`

**Interfaces:**
- Consumes: Task 3 の `usecase.CollectReport` / `usecase.Outcome` 系
- Produces: Task 10 が使う `func NewSlog(logger *slog.Logger) *SlogPresenter` と `func (p *SlogPresenter) Present(report usecase.CollectReport)`

現行 `collector.go` に散在していた8箇所の `log.Printf` は、ここに集約される。

- [ ] **Step 1: 失敗するテストを書く**

Create `internal/adapter/presenter/slog_presenter_test.go`:

```go
package presenter_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"switchBotStore/internal/adapter/presenter"
	"switchBotStore/internal/domain"
	"switchBotStore/internal/usecase"
)

// newTestLogger は buf へ JSON 形式で書き出す logger を返す。
// adapter 層のテストから infra 層を import しないよう slog を直接使う。
func newTestLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

// logEntries は JSON Lines 形式のログをパースして返す。
func logEntries(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()

	entries := make([]map[string]any, 0)
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var e map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &e), "行: %s", line)
		entries = append(entries, e)
	}
	return entries
}

func TestPresent_保存したデバイスをINFOで記録する(t *testing.T) {
	var buf bytes.Buffer
	p := presenter.NewSlog(newTestLogger(&buf))

	p.Present(usecase.CollectReport{Accounts: []usecase.AccountResult{{
		AccountName: "メインアカウント",
		Devices: []usecase.DeviceResult{{
			Device:  domain.Device{ID: "d1", Name: "温湿度計", Type: "Meter"},
			Outcome: usecase.OutcomeStored,
		}},
	}}})

	entries := logEntries(t, &buf)
	require.NotEmpty(t, entries)

	found := false
	for _, e := range entries {
		if e["device"] == "温湿度計" {
			found = true
			assert.Equal(t, "INFO", e["level"])
			assert.Equal(t, "メインアカウント", e["account"])
			assert.Equal(t, "stored", e["outcome"])
			assert.Equal(t, "Meter", e["type"])
		}
	}
	assert.True(t, found, "デバイスのログが出力されていない")
}

func TestPresent_デバイスの失敗をWARNで記録する(t *testing.T) {
	var buf bytes.Buffer
	p := presenter.NewSlog(newTestLogger(&buf))

	p.Present(usecase.CollectReport{Accounts: []usecase.AccountResult{{
		AccountName: "acc1",
		Devices: []usecase.DeviceResult{{
			Device:  domain.Device{ID: "d1", Name: "玄関Bot"},
			Outcome: usecase.OutcomeFailed,
			Err:     errors.New("デバイスオフライン"),
		}},
	}}})

	entries := logEntries(t, &buf)

	found := false
	for _, e := range entries {
		if e["device"] == "玄関Bot" {
			found = true
			assert.Equal(t, "WARN", e["level"])
			assert.Contains(t, e["error"], "デバイスオフライン")
		}
	}
	assert.True(t, found, "失敗デバイスのログが出力されていない")
}

func TestPresent_アカウントの致命的エラーをERRORで記録する(t *testing.T) {
	var buf bytes.Buffer
	p := presenter.NewSlog(newTestLogger(&buf))

	p.Present(usecase.CollectReport{Accounts: []usecase.AccountResult{{
		AccountName: "壊れたアカウント",
		Err:         errors.New("認証に失敗しました"),
	}}})

	entries := logEntries(t, &buf)

	found := false
	for _, e := range entries {
		if e["level"] == "ERROR" {
			found = true
			assert.Equal(t, "壊れたアカウント", e["account"])
			assert.Contains(t, e["error"], "認証に失敗しました")
		}
	}
	assert.True(t, found, "致命的エラーのログが出力されていない")
}

func TestPresent_アカウントごとに集計を出力する(t *testing.T) {
	var buf bytes.Buffer
	p := presenter.NewSlog(newTestLogger(&buf))

	p.Present(usecase.CollectReport{Accounts: []usecase.AccountResult{{
		AccountName: "acc1",
		Devices: []usecase.DeviceResult{
			{Device: domain.Device{Name: "a"}, Outcome: usecase.OutcomeStored},
			{Device: domain.Device{Name: "b"}, Outcome: usecase.OutcomeStored},
			{Device: domain.Device{Name: "c"}, Outcome: usecase.OutcomeSkippedCloudDisabled},
			{Device: domain.Device{Name: "d"}, Outcome: usecase.OutcomeRegisteredOnly},
			{Device: domain.Device{Name: "e"}, Outcome: usecase.OutcomeFailed, Err: errors.New("x")},
		},
	}}})

	entries := logEntries(t, &buf)

	found := false
	for _, e := range entries {
		if e["msg"] == "アカウントの収集が完了しました" {
			found = true
			assert.Equal(t, float64(2), e["stored"])
			assert.Equal(t, float64(1), e["skipped"])
			assert.Equal(t, float64(1), e["registered_only"])
			assert.Equal(t, float64(1), e["failed"])
		}
	}
	assert.True(t, found, "集計ログが出力されていない")
}

func TestPresent_空のレポートでもパニックしない(t *testing.T) {
	var buf bytes.Buffer
	p := presenter.NewSlog(newTestLogger(&buf))

	assert.NotPanics(t, func() {
		p.Present(usecase.CollectReport{})
	})
}
```

- [ ] **Step 2: テストが失敗することを確認する**

Run: `go test ./internal/adapter/presenter/`
Expected: FAIL（パッケージが存在しない）

- [ ] **Step 3: プレゼンタを実装する**

Create `internal/adapter/presenter/slog_presenter.go`:

```go
// Package presenter はユースケースの実行結果を人間と機械が読める形に整形する。
package presenter

import (
	"log/slog"

	"switchBotStore/internal/usecase"
)

// SlogPresenter は CollectReport を構造化ログとして出力する。
type SlogPresenter struct {
	logger *slog.Logger
}

// NewSlog は SlogPresenter を生成する。
func NewSlog(logger *slog.Logger) *SlogPresenter {
	return &SlogPresenter{logger: logger}
}

// Present はレポート全体をログに書き出す。
func (p *SlogPresenter) Present(report usecase.CollectReport) {
	for _, acc := range report.Accounts {
		p.presentAccount(acc)
	}
}

func (p *SlogPresenter) presentAccount(acc usecase.AccountResult) {
	if acc.Err != nil {
		p.logger.Error("アカウントの収集に失敗しました",
			"account", acc.AccountName,
			"error", acc.Err.Error())
		return
	}

	for _, dev := range acc.Devices {
		p.presentDevice(acc.AccountName, dev)
	}

	counts := tally(acc.Devices)
	p.logger.Info("アカウントの収集が完了しました",
		"account", acc.AccountName,
		"total", len(acc.Devices),
		"stored", counts[usecase.OutcomeStored],
		"skipped", counts[usecase.OutcomeSkippedCloudDisabled],
		"registered_only", counts[usecase.OutcomeRegisteredOnly],
		"failed", counts[usecase.OutcomeFailed])
}

func (p *SlogPresenter) presentDevice(accountName string, dev usecase.DeviceResult) {
	attrs := []any{
		"account", accountName,
		"device", dev.Device.Name,
		"device_id", string(dev.Device.ID),
		"type", dev.Device.Type,
		"outcome", dev.Outcome.String(),
	}

	if dev.Err != nil {
		p.logger.Warn("デバイスの処理に失敗しました", append(attrs, "error", dev.Err.Error())...)
		return
	}
	p.logger.Info("デバイスを処理しました", attrs...)
}

// tally は Outcome ごとの件数を数える。
func tally(devices []usecase.DeviceResult) map[usecase.Outcome]int {
	counts := make(map[usecase.Outcome]int, 4)
	for _, d := range devices {
		counts[d.Outcome]++
	}
	return counts
}
```

- [ ] **Step 4: テストが通ることを確認する**

Run: `go test ./internal/adapter/presenter/ -v`
Expected: 5テストがすべて PASS

- [ ] **Step 5: 全体のビルドとテストを確認する**

Run: `go build ./... && go test ./...`
Expected: ビルド成功、全パッケージ `ok`

- [ ] **Step 6: コミット**

```bash
git add internal/adapter/presenter/
git commit -m "feat: CollectReport を構造化ログへ整形するプレゼンタを追加"
```

---

### Task 10: コンポジションルートへの切り替えと旧パッケージの削除

**Files:**
- Create: `cmd/switchbotstore/main.go`
- Delete: `cmd/main.go`
- Delete: `internal/collector/`（`collector.go` / `collector_test.go`）
- Delete: `internal/config/`（`config.go` / `config_test.go`）
- Delete: `internal/database/`（`database.go`）
- Delete: `internal/logger/`（`logger.go` / `logger_test.go`）
- Delete: `internal/repository/`（`repository.go`）
- Delete: `internal/switchbot/`（`client.go` / `client_test.go`）
- Modify: `go.mod` / `go.sum`

**Interfaces:**
- Consumes: Task 4 の `switchbot.NewGateway()`、Task 5 の `config.Load()`、Task 6 の `logging.Setup()`、Task 7 の `mysql.Connect()` / `mysql.Migrate()`、Task 8 の `persistence.New()` / `persistence.VerifySchema()`、Task 9 の `presenter.NewSlog()`、Task 3 の `usecase.NewCollectStatus()`
- Produces: 実行可能バイナリ

このタスクで**初回起動フラグ機構（`system_settings` / `IsFirstRun` / `MarkFirstRunDone` / `InitialCollect`）が削除される**。動作に差がないため（仕様書 §1.5）。`system_settings` テーブル自体は DB に残る。

- [ ] **Step 1: コンポジションルートを書く**

Create `cmd/switchbotstore/main.go`:

```go
// Command switchbotstore は SwitchBot API からデバイスのステータスを収集し
// MySQL に保存する。1回実行して終了するため、cron やタスクスケジューラから
// 定期的に呼び出して使う。
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"switchBotStore/internal/adapter/persistence"
	"switchBotStore/internal/adapter/presenter"
	"switchBotStore/internal/adapter/switchbot"
	"switchBotStore/internal/domain"
	"switchBotStore/internal/infra/config"
	"switchBotStore/internal/infra/logging"
	"switchBotStore/internal/infra/mysql"
	"switchBotStore/internal/usecase"
)

// configPath は設定ファイルのパス。実行ファイルと同じディレクトリに置く。
const configPath = "config.json"

// systemClock は usecase.Clock の本番実装。
type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

func main() {
	if err := run(); err != nil {
		// ログ出力先が未確定の段階で失敗する可能性があるため、
		// slog のデフォルト (標準エラー出力) で報告する。
		slog.Error("実行に失敗しました", "error", err)
		os.Exit(1)
	}
}

// run はアプリケーション全体を組み立てて実行する。
//
// main から defer が確実に実行されるよう、処理は必ずこの関数に置く
// (log.Fatalf や os.Exit を途中で呼ぶと defer が飛ばされる)。
func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("設定の読み込みに失敗しました: %w", err)
	}

	logger, closeLog, err := logging.Setup(cfg.LogDir, time.Now())
	if err != nil {
		return fmt.Errorf("ログの初期化に失敗しました: %w", err)
	}
	defer func() {
		if err := closeLog(); err != nil {
			slog.Error("ログファイルのクローズに失敗しました", "error", err)
		}
	}()

	logger.Info("設定を読み込みました",
		"accounts", len(cfg.Accounts),
		"log_dir", cfg.LogDir)

	db, closeDB, err := mysql.Connect(ctx, mysql.Config{
		Host:     cfg.Database.Host,
		Port:     cfg.Database.Port,
		User:     cfg.Database.User,
		Password: cfg.Database.Password,
		Name:     cfg.Database.Name,
	})
	if err != nil {
		return fmt.Errorf("データベースへの接続に失敗しました: %w", err)
	}
	defer func() {
		if err := closeDB(); err != nil {
			logger.Warn("DB 接続のクローズに失敗しました", "error", err)
		}
	}()
	logger.Info("データベースに接続しました", "database", cfg.Database.Name)

	if err := mysql.Migrate(ctx, db); err != nil {
		return fmt.Errorf("マイグレーションに失敗しました: %w", err)
	}
	if err := persistence.VerifySchema(db); err != nil {
		return fmt.Errorf("スキーマの検証に失敗しました: %w", err)
	}
	logger.Info("スキーマを確認しました")

	uc := usecase.NewCollectStatus(
		switchbot.NewGateway(),
		persistence.New(db),
		systemClock{},
	)

	logger.Info("データ収集を開始します")
	report := uc.Execute(ctx, toDomainAccounts(cfg.Accounts))
	presenter.NewSlog(logger).Present(report)

	if err := report.FatalError(); err != nil {
		return fmt.Errorf("データ収集中に致命的なエラーが発生しました: %w", err)
	}
	logger.Info("データ収集が完了しました")
	return nil
}

// toDomainAccounts は設定上のアカウントをドメインモデルへ変換する。
func toDomainAccounts(accounts []config.Account) []domain.Account {
	result := make([]domain.Account, 0, len(accounts))
	for _, a := range accounts {
		result = append(result, domain.Account{
			Name: a.Name,
			Credential: domain.Credential{Token: a.Token, Secret: a.Secret},
		})
	}
	return result
}
```

- [ ] **Step 2: 新しいエントリポイントがビルドできることを確認する**

Run: `go build ./cmd/switchbotstore/`
Expected: エラーなし（`switchbotstore.exe` が生成される）

- [ ] **Step 3: 生成されたバイナリを削除する**

```bash
rm -f switchbotstore switchbotstore.exe
```

`.gitignore` に `*.exe` があるためコミットされないが、作業ディレクトリを汚さないよう消しておく。

- [ ] **Step 4: 旧パッケージを削除する**

```bash
rm -rf cmd/main.go internal/collector internal/config internal/database internal/logger internal/repository internal/switchbot
```

`internal/switchbot` は削除するが、新しい `internal/adapter/switchbot` は残ることに注意。

- [ ] **Step 5: 依存を整理する**

```bash
go mod tidy
```

旧 `internal/database` が直接使っていた `github.com/go-sql-driver/mysql` は、`gorm.io/driver/mysql` が間接依存として引き続き必要とするため `go.mod` に残る（`// indirect` が付く場合がある）。

- [ ] **Step 6: ビルドと全テストを確認する**

Run: `go build ./... && go test ./...`
Expected: ビルド成功。テストは `domain` / `usecase` / `adapter/switchbot` / `adapter/persistence` / `adapter/presenter` / `infra/config` / `infra/logging` / `infra/mysql` が `ok`、`cmd/switchbotstore` は `no test files`

- [ ] **Step 7: 完了条件を機械的に検証する**

```bash
echo "--- log.Fatalf が残っていないこと (出力なしが正常) ---"
grep -rn "log.Fatal" --include="*.go" . || echo "OK: なし"

echo "--- %v によるエラーラップが残っていないこと (出力なしが正常) ---"
grep -rn 'Errorf(".*: %v"' --include="*.go" . || echo "OK: なし"

echo "--- domain が他の内部パッケージに依存しないこと (1行のみが正常) ---"
go list -deps ./internal/domain/ | grep switchBotStore

echo "--- usecase が domain 以外の内部パッケージに依存しないこと (2行のみが正常) ---"
go list -deps ./internal/usecase/ | grep switchBotStore

echo "--- usecase がログを吐かないこと (出力なしが正常) ---"
grep -rn "log/slog\|\"log\"" internal/usecase/ || echo "OK: なし"
```

Expected: すべて期待どおり（`log.Fatal` なし、`%v` ラップなし、`domain` は1行、`usecase` は2行、`usecase` にログの import なし）

- [ ] **Step 8: コミット**

```bash
git add -A
git commit -m "refactor: コンポジションルートへ切り替え旧パッケージを削除

初回起動フラグ機構 (system_settings / IsFirstRun / InitialCollect) は
動作に差がないため削除した。system_settings テーブル自体は既存データ
保持のため DB に残す。"
```

---

### Task 11: README の更新と実データでの動作確認

**Files:**
- Modify: `README.md`

**Interfaces:**
- Consumes: Task 10 までの全成果
- Produces: なし（最終タスク）

- [ ] **Step 1: 動作環境の記述を更新する**

`README.md` の「## 動作環境」節を以下に置き換える。

```markdown
## 動作環境

- Go 1.26.3 以上
- MySQL 5.7 以上（JSON型サポートのため）
```

- [ ] **Step 2: ビルドコマンドを更新する**

「### 4. ビルドと実行」節のコードブロックを以下に置き換える。

````markdown
```bash
# ビルド (config.json と同じディレクトリに出力)
go build ./cmd/switchbotstore/

# 手動実行（1回収集して終了）
./switchbotstore
```
````

エントリポイントが `cmd/switchbotstore/` に移ったため、バイナリ名はディレクトリ名から決まり `-o` の指定が不要になった。

- [ ] **Step 3: 「初回データ収集」の記述を削除する**

冒頭の説明文から「初回起動時に全デバイスの初期データを自動取得します。」の一文を削除し、「## 機能」節から以下の2項目を削除する。

- `- **初回データ収集**: 初回起動時に全デバイスのステータスを即座に取得・保存`
- `- **テーブル自動作成**: 起動時にテーブルが存在しない場合は自動で作成`

代わりに「## 機能」節へ以下を追加する。

```markdown
- **マイグレーション**: 起動時に goose で未適用のマイグレーションを自動適用（実行ファイルに同梱）
- **スキーマ検証**: マイグレーション後、コードが期待するテーブル・カラムが存在するかを起動時に検証
- **構造化ログ**: JSON 形式で標準エラー出力とファイルに出力（`log_dir/YYYY-MM-DD.log`）
```

- [ ] **Step 4: テーブル設計の表を更新する**

`system_settings` の行を以下に置き換える。

```markdown
| `system_settings` | 現在はコードから参照されない旧テーブル（既存データ保持のため残置）|
```

「## テーブル設計」冒頭のツリー図から `system_settings     初回起動フラグ等のシステム設定` の行も削除する。

- [ ] **Step 5: 動作フロー図を更新する**

「## 動作フロー」節のコードブロックを以下に置き換える。

````markdown
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
| `1` | 設定・DB接続・マイグレーションの失敗、またはアカウント単位の致命的エラー（認証失敗、デバイス一覧の取得失敗など）が1件以上 |

cron やタスクスケジューラから失敗を検知できるよう、アカウント単位の失敗は終了コード 1 になる。
````

- [ ] **Step 6: アーキテクチャの節を追加する**

「## テスト実行」節の直前に以下を挿入する。

````markdown
## アーキテクチャ

クリーンアーキテクチャの依存規則に従い、内側は外側を一切 import しない。

```
  infra ──────┐
              ▼
  adapter ──> usecase ──> domain
     ▲                       ▲
     └───────────────────────┘
```

| ディレクトリ | 責務 | import してよいもの |
|---|---|---|
| `internal/domain` | ドメインモデル | 標準ライブラリのみ |
| `internal/usecase` | ユースケースと出力ポート。ログは吐かず結果を値で返す | `domain` のみ |
| `internal/adapter` | SwitchBot API / GORM 永続化 / ログ整形 | `usecase`, `domain` |
| `internal/infra` | 設定・ログ・DB接続 | 内部パッケージを import しない |
| `cmd/switchbotstore` | コンポジションルート（配線） | すべて |

DB スキーマの正は `internal/infra/mysql/migrations/*.sql`（goose）にある。
`internal/adapter/persistence` の GORM モデルはそこへのマッピングにすぎず、
両者のズレは起動時の `VerifySchema` が検出する。

設計の詳細は `docs/superpowers/specs/2026-07-29-clean-architecture-refactoring-design.md` を参照。
````

- [ ] **Step 7: 注意事項を更新する**

「## 注意事項」節の末尾に以下を追加する。

```markdown
- **赤外線リモコンの種別**: SwitchBot API は赤外線リモコンの種別を `remoteType` で返すが、本ツールは `deviceType` を読むため `devices.device_type` は空文字で保存される（現行の挙動を維持）
```

- [ ] **Step 8: README の内容を最終確認する**

Run: `cat README.md`
Expected: ビルドコマンドが `go build ./cmd/switchbotstore/`、動作環境が Go 1.26.3 以上、初回データ収集の記述が消えていること

- [ ] **Step 9: 全体のビルドとテストを最終確認する**

Run: `go build ./... && go test ./... && go vet ./...`
Expected: すべてエラーなし

- [ ] **Step 10: コミット**

```bash
git add README.md
git commit -m "docs: README をリファクタリング後の実態に合わせて更新"
```

- [ ] **Step 11: 実データのある DB に対して動作確認する**

**これは人間による確認が必要なステップ。** 自動化せず、ユーザーに実行を依頼すること。

```bash
go build ./cmd/switchbotstore/
./switchbotstore
```

確認事項:

1. 終了コードが `0` であること（`echo $?`）
2. `logs/YYYY-MM-DD.log` に JSON 形式のログが出ていること
3. ログに `"msg":"スキーマを確認しました"` が含まれること（`VerifySchema` 通過）
4. ログに `"msg":"アカウントの収集が完了しました"` と `stored` の件数が含まれること
5. DB で新しい行が増えていること:
   ```sql
   SELECT COUNT(*), MAX(recorded_at) FROM device_status_logs;
   SELECT * FROM goose_db_version;  -- version_id=1 が applied されている
   ```
6. **既存データが失われていないこと**:
   ```sql
   SELECT COUNT(*) FROM api_accounts;
   SELECT COUNT(*) FROM devices;
   SELECT COUNT(*) FROM system_settings;  -- 旧テーブルも残っている
   ```
7. **MySQL ドライバ更新の影響がないこと**（Task 7 のレビュー指摘への対応）:

   `go-sql-driver/mysql` が v1.7.1 から v1.10.0 に上がっている。`gorm.io/driver/mysql v1.6.0` が v1.8.1 以上を要求するため不可避で、v1.8.1 に固定しても下記の変更は避けられない（変更が入ったのは v1.8.0 のため）。

   v1.8.0 の変更のうち本ツールに関係し得るのは、接続時のコレーション決定が `SET NAMES <charset> COLLATE <collation>` 方式に変わり、charset のみ指定した場合はサーバー既定のコレーションが使われる点。以下を確認する。

   ```sql
   -- 日本語のデバイス名・アカウント名が文字化けせず保存・取得できているか
   SELECT id, name FROM api_accounts;
   SELECT id, device_name, device_type FROM devices WHERE device_name <> '';

   -- 接続コレーションとテーブルのコレーションを確認
   SHOW VARIABLES LIKE 'collation_connection';
   SELECT TABLE_NAME, TABLE_COLLATION FROM information_schema.TABLES
     WHERE TABLE_SCHEMA = DATABASE();
   ```

   日本語が化けている、または既存行の `name` / `device_name` が読めない場合は停止してユーザーに報告すること。

いずれかが期待どおりでない場合は、そこで停止してユーザーに報告すること。

---

## 完了条件

- [ ] `go build ./...` / `go test ./...` / `go vet ./...` がすべて通る
- [ ] `internal/domain` が他の内部パッケージを import していない
- [ ] `internal/usecase` が `domain` 以外の内部パッケージを import していない
- [ ] `internal/usecase` にログ関連の import がない
- [ ] 永続化の契約（`usecase.Repository`）が `switchbot` の型に依存していない
- [ ] `log.Fatal` / `log.Fatalf` がコードベースに存在しない
- [ ] エラーラップがすべて `%w`
- [ ] 旧パッケージ（`internal/collector` / `config` / `database` / `logger` / `repository` / `switchbot`）が削除されている
- [ ] 実データのある DB に対して起動し、`VerifySchema` が通り、収集が成功し、既存データが失われていない
- [ ] README が実態と一致している
