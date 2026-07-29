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
