# ログレベル設定の追加 設計

作成日: 2026-07-29
対象: `switchBotStore`（SwitchBot API からデバイス状態を収集し MySQL に保存する cron 起動型バッチ）

## 目的

`config.json` から出力ログのレベル閾値を設定できるようにし、通常運用でのログ量を絞れるようにする。

## 背景

現在、ログレベルは `internal/infra/logging/logging.go` にハードコードされている。

```go
func New(w io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo}))
}
```

ログ呼び出しの内訳は以下のとおり。

| レベル | 件数 | 主な内容 |
|---|---|---|
| Error | 4 | 実行失敗、アカウント単位の致命的エラー |
| Warn | 4 | デバイスの処理失敗、クローズ失敗 |
| Info | 7 | 起動各段階、デバイス1台ごとの処理結果、アカウント単位の集計 |
| Debug | 0 | — |

Info には `internal/adapter/presenter/slog_presenter.go` の「デバイスを処理しました」がデバイス1台ごとに出る。5分間隔の cron なら1日288回実行されるため、デバイス10台で1日約3,000行になる。

## スコープ

**実装するのは閾値の設定のみ。** Debug ログの呼び出しは新規に追加しない。

現状 Debug の呼び出しは0件であり、閾値に `debug` を指定しても Info と同じ出力にしかならない。目的はノイズ削減であり、詳細情報の追加ではないため、消費者のいないログ文を書くことは YAGNI に反する。将来 Debug ログが必要になった時点で追加すればよく、その際に本設計の変更は不要。

## 決定事項

| 項目 | 決定 | 理由 |
|---|---|---|
| 設定場所 | `config.json` の `log_level` | 既存の `log_dir` と同じ場所。cron 運用における恒久的な設定として自然 |
| 環境変数・CLI フラグでの上書き | **実装しない** | 現時点で用途がない（YAGNI） |
| 不正値の扱い | **起動を中止（exit 1）** | `database.host` や `accounts[].token` が空のときと同じ扱い。既存の検証と一貫させる |
| 未指定時の既定値 | `INFO` | 現行の振る舞いを保存する |
| 出力先ごとのレベル分け | **しない** | ハンドラが1つ（`io.MultiWriter`）であり、cron 運用では stderr も結局ファイルに落ちるため分ける実用的理由がない |

## 実装方式

`Config` のフィールドを `slog.Level` 型にし、`encoding/json` に直接パースさせる。

```go
type Config struct {
	Database Database   `json:"database"`
	Accounts []Account  `json:"accounts" validate:"required,min=1,dive"`
	LogDir   string     `json:"log_dir"`
	LogLevel slog.Level `json:"log_level"`
}
```

`slog.Level` は `encoding.TextUnmarshaler` を実装しているため、`json.Unmarshal` が文字列を直接変換する。実測で確認した挙動は以下のとおり。

| 入力 | 結果 |
|---|---|
| `"debug"` / `"info"` / `"warn"` / `"error"` | 対応するレベルに変換される |
| `"WARN"`（大文字） | `"warn"` と同じ。**大文字小文字を問わない** |
| キー自体を省略 | ゼロ値 = `INFO`（既定値が自動的に正しい） |
| `"WARNING"` / `"inof"`（不正値） | エラー → 起動中止 |
| `""`（空文字） | エラー → 起動中止 |
| `"INFO+2"` | slog のオフセット記法として有効 |

これにより変換コードも既定値の補完コードも不要になる。`internal/infra/config` が `log/slog` を import することになるが、`log/slog` は標準ライブラリであり、infra 層が標準ライブラリを使うことは依存規則に適合する。

### 検討したが採用しなかった方式

| 方式 | 不採用の理由 |
|---|---|
| 文字列 + validator の `oneof=debug info warn error` | `oneof` は完全一致のため **`"WARN"` と大文字で書くと弾かれる**。また「有効な値の定義」がタグと変換関数の2箇所に分かれ、片方だけ変更するとずれる |
| 文字列 + `logging.ParseLevel` に集約 | `config` が slog を知らずに済む点は綺麗だが、変換関数とそのテストが増える。`log/slog` は標準ライブラリであり、`logging` パッケージが既に使っている以上、`config` に閉じ込める価値がコード増加に見合わない |

`"INFO+2"` のようなオフセット記法が通る点は、slog が公式に用意している記法であり、書いた人は意図して書いているため実害はないと判断する。

## 変更するファイル

| ファイル | 変更内容 |
|---|---|
| `internal/infra/config/config.go` | `LogLevel slog.Level` フィールドを追加。`log/slog` を import |
| `internal/infra/logging/logging.go` | `New` と `Setup` がレベルを引数で受け取るよう変更 |
| `cmd/switchbotstore/main.go` | `cfg.LogLevel` を渡す（2箇所） |
| `config.json.example` | `log_level` の例を追加 |
| `README.md` | 設定項目の表に追加。`error` 指定時の注意を追記 |
| `internal/infra/config/config_test.go` | ログレベルのパースに関するテストを追加 |
| `internal/infra/logging/logging_test.go` | シグネチャ変更に伴う更新と、レベルによる抑止のテストを追加 |

### シグネチャの変更

```go
// 変更前
func New(w io.Writer) *slog.Logger
func Setup(logDir string, now time.Time) (logger *slog.Logger, closeFn func() error, err error)

// 変更後
func New(w io.Writer, level slog.Level) *slog.Logger
func Setup(logDir string, level slog.Level, now time.Time) (logger *slog.Logger, closeFn func() error, err error)
```

引数の順序は「設定由来の値（`logDir`, `level`）→ 注入する値（`now`）」とする。

`internal/adapter/presenter` のテストは自前でロガーを組み立てているため影響を受けない。

## 後方互換性

`log_level` を書かなければ `slog.Level` のゼロ値である `INFO` になる。**既存の `config.json` を一切変更せずに、現行とまったく同じ振る舞いのまま動く。**

## 既知の制約: `error` 指定時にログ劣化の警告が抑止される

`cmd/switchbotstore/main.go` には、`logging.Setup` が失敗した際に標準エラー出力のみへフォールバックして収集を続行する経路がある（データ収集を止めないため）。

```go
logger = logging.New(os.Stderr, cfg.LogLevel)          // ERROR レベル
logger.Warn("ログファイルの初期化に失敗しました...")      // 抑止される
```

`log_level: "error"` を指定していると、**ログファイルが書けなくなっても運用者はそれに気づけない。**

この挙動は意図的に受け入れる。ログレベルは事象の深刻度を表すものであって「この運用者にとっての重要度」ではない。フィルタを回避するために深刻度を水増しするのは筋が悪く、WARN を切ると決めた運用者は警告を見逃すことを受け入れたと解釈すべきである。

代わりに README に「`error` を指定すると、ログファイルへの書き込みに失敗した際の警告も抑止される」と明記する。

## 既知の制約: `log_level` が及ばない出力が2箇所ある

`cmd/switchbotstore/main.go` には、パッケージレベルの `slog` を直接使う箇所が2つある。

| 箇所 | 理由 |
|---|---|
| `main()` の `slog.Error("実行に失敗しました", ...)` | `run()` から戻った時点でログファイルは閉じられているため |
| `closeLog` の defer 内の `slog.Error("ログファイルのクローズに失敗しました", ...)` | まさにそのログファイルを閉じている最中のため |

これらは slog の既定ハンドラ（標準エラー出力・テキスト形式・INFO 閾値）を使うため、**`log_level` の設定を受けない。**

実害は小さい。どちらも Error レベルであり、`log_level` に何を設定しても（`error` を含め）閾値以上なので出力される。ただしテキスト形式である点は他の JSON 出力と揃っていない。これは本変更以前からある性質であり、修正はスコープ外とする。

## テスト方針

| 対象 | 検証内容 |
|---|---|
| `internal/infra/config` | `debug` / `info` / `warn` / `error` の各文字列が対応するレベルにパースされること。`"WARN"` のような大文字表記も通ること。**`log_level` を省略すると `INFO` になること**。不正値（`"WARNING"` 等）と空文字でエラーになること |
| `internal/infra/logging` | 指定したレベル未満のログが実際に出力されないこと（`warn` を指定したとき Info が出力されず Warn と Error は出力される） |
| 既存テスト | シグネチャ変更に伴う呼び出しの更新のみ。検証内容は変えない |

既存テストの検証内容を変えないことが、この変更で振る舞いを壊していないことの証拠になる。

## 完了条件

- [ ] `go build ./...` / `go test ./...` / `go vet ./...` が通り、`gofmt -l .` が空であること
- [ ] `log_level` を書かない既存の `config.json` で、現行と同じログが出ること
- [ ] `log_level: "warn"` で「デバイスを処理しました」の Info 行が出なくなること
- [ ] 不正な `log_level` で起動が中止され、終了コードが 1 になること
- [ ] `README.md` と `config.json.example` が実装と一致していること
- [ ] 依存規則が維持されていること（`internal/infra/config` が内部パッケージを import しない）

## スコープ外

- Debug ログ呼び出しの追加
- 環境変数・CLI フラグによる上書き
- 出力先（stderr / ファイル）ごとに異なるレベルを設定する機能
- ログ形式（JSON）の変更
