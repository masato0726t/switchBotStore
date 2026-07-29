package persistence

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"switchBotStore/internal/domain"
)

// このファイルは Store が組み立てる SQL を、MySQL に接続せずに検証する。
//
// Store は本番 DB に書き込む唯一のコードだが、CI 環境に MySQL が無いため
// 往復テストは書けない。GORM の DryRun セッションは SQL の組み立てまでを行い
// 実行だけを省くので、発行されるはずの SQL 文字列とバインド値を取り出せる。
//
// 期待値は移行元の internal/repository/repository.go の SQL から導いている。
// このリファクタリングは振る舞いを保存する方針であり、ON DUPLICATE KEY UPDATE
// の更新対象カラムや WHERE 条件がずれると、本番で静かに誤ったデータが入る。

// capturedStatement は組み立てられた1文と、そのバインド値。
type capturedStatement struct {
	SQL  string
	Vars []any
}

// sqlCapture は Store が発行した SQL を発行順に記録する。
type sqlCapture struct {
	statements []capturedStatement
}

// only は記録が1文だけであることを確認して、それを返す。
func (c *sqlCapture) only(t *testing.T) capturedStatement {
	t.Helper()
	require.Len(t, c.statements, 1, "発行された SQL は1文であるはず")
	return c.statements[0]
}

// pair は記録が2文であることを確認して、順に返す。
func (c *sqlCapture) pair(t *testing.T) (insert, selectStmt capturedStatement) {
	t.Helper()
	require.Len(t, c.statements, 2,
		"Upsert は「INSERT ... ON DUPLICATE KEY UPDATE」と「id の再 SELECT」の2文であるはず")
	return c.statements[0], c.statements[1]
}

// newDryRunStore は DB 接続を持たない Store と、発行される SQL の記録先を返す。
//
// DryRun だけでは足りず SkipDefaultTransaction も必要になる。GORM は既定で
// Create をトランザクションで囲むが、BEGIN は DryRun の対象外で実接続を要求するため。
func newDryRunStore(t *testing.T) (*Store, *sqlCapture) {
	t.Helper()

	db, err := gorm.Open(
		gormmysql.New(gormmysql.Config{
			DriverName:                "mysql",
			DSN:                       "",
			SkipInitializeWithVersion: true, // サーバーへのバージョン問い合わせを行わない
		}),
		&gorm.Config{
			DryRun:                 true,
			SkipDefaultTransaction: true,
			DisableAutomaticPing:   true,
			Logger:                 gormlogger.Discard,
		},
	)
	require.NoError(t, err, "DryRun 用の GORM は DB 無しで開けるはず")

	capture := &sqlCapture{}
	record := func(d *gorm.DB) {
		capture.statements = append(capture.statements, capturedStatement{
			SQL:  d.Statement.SQL.String(),
			Vars: d.Statement.Vars,
		})
	}
	require.NoError(t, db.Callback().Create().After("gorm:create").Register("test:capture", record))
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:capture", record))

	return New(db), capture
}

// updateClause は ON DUPLICATE KEY UPDATE 以降の部分を返す。
//
// 更新対象カラムだけを検査するために必要。例えば is_virtual_infrared は
// INSERT のカラム一覧には正しく現れるため、SQL 全体に対する検査では
// 「更新対象に入っていないこと」を確認できない。
func updateClause(t *testing.T, sql string) string {
	t.Helper()
	const marker = "ON DUPLICATE KEY UPDATE"
	i := strings.Index(sql, marker)
	require.NotEqual(t, -1, i, "ON DUPLICATE KEY UPDATE が含まれているはず: %s", sql)
	return sql[i+len(marker):]
}

// assertNotUpdated は指定カラムが更新対象に含まれていないことを検証する。
//
// カラム名の素朴な部分一致は使えない。GORM はカラムを `col`=VALUES(`col`) の形で
// 出力するため、例えば "device_id" は "hub_device_id" にも含まれてしまう。
// バッククォートまで含めた形で照合する。
func assertNotUpdated(t *testing.T, sql, column, reason string) {
	t.Helper()
	assert.NotContains(t, updateClause(t, sql), "`"+column+"`=", reason)
}

func TestSaveAccount_発行するSQL(t *testing.T) {
	store, capture := newDryRunStore(t)

	_, err := store.SaveAccount(context.Background(), domain.Account{
		Name:       "メインアカウント",
		Credential: domain.Credential{Token: "tok1", Secret: "sec1"},
	})
	require.NoError(t, err)

	insert, selectStmt := capture.pair(t)

	assert.Equal(t,
		"INSERT INTO `api_accounts` (`name`,`token`,`secret`,`created_at`,`updated_at`) "+
			"VALUES (?,?,?,?,?) "+
			"ON DUPLICATE KEY UPDATE `name`=VALUES(`name`),`secret`=VALUES(`secret`)",
		insert.SQL)

	// 競合キーである token を更新してしまうと、別アカウントの行を書き換えうる。
	assertNotUpdated(t, insert.SQL, "token", "token は競合キーなので更新対象に含めてはならない")
	assertNotUpdated(t, insert.SQL, "updated_at",
		"updated_at を代入すると中身が同じでも毎回書き込みが発生する。列定義の ON UPDATE CURRENT_TIMESTAMP に任せる")

	require.Len(t, insert.Vars, 5)
	assert.Equal(t, "メインアカウント", insert.Vars[0])
	assert.Equal(t, "tok1", insert.Vars[1])
	assert.Equal(t, "sec1", insert.Vars[2])

	assert.Equal(t, "SELECT `id` FROM `api_accounts` WHERE token = ? LIMIT ?", selectStmt.SQL)
	require.Len(t, selectStmt.Vars, 2)
	assert.Equal(t, "tok1", selectStmt.Vars[0], "id は token で引き直す")
}

func TestSaveDevice_発行するSQL(t *testing.T) {
	store, capture := newDryRunStore(t)

	_, err := store.SaveDevice(context.Background(), domain.AccountID(7), domain.Device{
		ID:                  "d1",
		Name:                "温湿度計",
		Type:                "Meter",
		HubID:               "hub1",
		Kind:                domain.DeviceKindPhysical,
		CloudServiceEnabled: true,
	})
	require.NoError(t, err)

	insert, selectStmt := capture.pair(t)

	assert.Equal(t,
		"INSERT INTO `devices` "+
			"(`api_account_id`,`device_id`,`device_name`,`device_type`,`hub_device_id`,"+
			"`enable_cloud_service`,`is_virtual_infrared`,`created_at`,`updated_at`) "+
			"VALUES (?,?,?,?,?,?,?,?,?) "+
			"ON DUPLICATE KEY UPDATE "+
			"`device_name`=VALUES(`device_name`),`device_type`=VALUES(`device_type`),"+
			"`hub_device_id`=VALUES(`hub_device_id`),"+
			"`enable_cloud_service`=VALUES(`enable_cloud_service`)",
		insert.SQL)

	// 移行元の repository.go も is_virtual_infrared を更新対象に含めていない。
	// INSERT のカラム一覧には現れるため、更新句だけを取り出して検査する。
	assertNotUpdated(t, insert.SQL, "is_virtual_infrared",
		"is_virtual_infrared は移行元と同じく更新対象に含めない")
	// 収集は毎分走る。updated_at を代入すると中身が同じでも MySQL が「変化した行」と
	// みなし、全デバイスぶんの書き込みが毎分発生する。列定義の
	// ON UPDATE CURRENT_TIMESTAMP に任せれば、内容が変わったときだけ更新される。
	assertNotUpdated(t, insert.SQL, "updated_at",
		"updated_at は更新対象に含めない（ON UPDATE CURRENT_TIMESTAMP に任せる）")
	assertNotUpdated(t, insert.SQL, "api_account_id",
		"api_account_id は競合キーなので更新対象に含めてはならない")
	assertNotUpdated(t, insert.SQL, "device_id",
		"device_id は競合キーなので更新対象に含めてはならない")

	require.Len(t, insert.Vars, 9)
	assert.Equal(t, int64(7), insert.Vars[0])
	assert.Equal(t, "d1", insert.Vars[1])
	assert.Equal(t, "温湿度計", insert.Vars[2])
	assert.Equal(t, "Meter", insert.Vars[3])
	assert.Equal(t, "hub1", insert.Vars[4])
	assert.Equal(t, true, insert.Vars[5], "enable_cloud_service")
	assert.Equal(t, false, insert.Vars[6], "is_virtual_infrared（物理デバイスなので false）")

	assert.Equal(t,
		"SELECT `id` FROM `devices` WHERE api_account_id = ? AND device_id = ? LIMIT ?",
		selectStmt.SQL)
	require.Len(t, selectStmt.Vars, 3)
	assert.Equal(t, int64(7), selectStmt.Vars[0], "id はアカウントとデバイスの複合キーで引き直す")
	assert.Equal(t, "d1", selectStmt.Vars[1])
}

func TestSaveDevice_赤外線リモコンはフラグが立つ(t *testing.T) {
	store, capture := newDryRunStore(t)

	_, err := store.SaveDevice(context.Background(), domain.AccountID(1), domain.Device{
		ID:   "ir1",
		Name: "エアコン",
		Kind: domain.DeviceKindInfraredRemote,
	})
	require.NoError(t, err)

	insert, _ := capture.pair(t)
	require.Len(t, insert.Vars, 9)
	assert.Equal(t, true, insert.Vars[6], "is_virtual_infrared")
}

func TestAppendStatus_発行するSQL(t *testing.T) {
	store, capture := newDryRunStore(t)

	recordedAt := time.Date(2026, 7, 29, 12, 0, 0, 0, time.Local)
	err := store.AppendStatus(context.Background(), domain.DeviceRecordID(42), domain.StatusSnapshot{
		Payload:    []byte(`{"temperature":25.5}`),
		RecordedAt: recordedAt,
	})
	require.NoError(t, err)

	insert := capture.only(t)

	assert.Equal(t,
		"INSERT INTO `device_status_logs` (`device_id`,`status_data`,`recorded_at`,`created_at`) "+
			"VALUES (?,?,?,?)",
		insert.SQL)

	// 収集ログは常に新しい行を足す。既存行を書き換えてはならない。
	assert.NotContains(t, insert.SQL, "ON DUPLICATE KEY UPDATE",
		"ステータスログは追記のみで、Upsert してはならない")

	require.Len(t, insert.Vars, 4)
	assert.Equal(t, int64(42), insert.Vars[0])
	assert.Equal(t, `{"temperature":25.5}`, insert.Vars[1], "status_data は文字列として渡す")
	assert.Equal(t, recordedAt, insert.Vars[2], "recorded_at は渡された時刻をそのまま使う")
}
