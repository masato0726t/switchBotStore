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
