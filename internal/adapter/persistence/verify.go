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
//
// 検証するのはテーブルとカラムの「存在」のみで、カラムの型・NULL 制約・
// UNIQUE KEY や外部キーなどの制約は検証しない。特に uq_token /
// uq_account_device といったユニークキーが失われても本関数は検出できず、
// その場合 ON DUPLICATE KEY UPDATE による重複防止が効かなくなり、
// 気づかないまま重複行が増え続ける可能性がある。
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
