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
//
// level 未満のレベルのログは出力されない。
func New(w io.Writer, level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level}))
}

// Setup は logDir に「YYYY-MM-DD.log」を開き、標準エラー出力とファイルの
// 両方へ JSON 形式で出力する slog.Logger を返す。
//
// level 未満のレベルのログは、標準エラー出力とファイルの両方で出力されない。
// ハンドラは1つなので出力先ごとにレベルを分けることはできない。
//
// logDir が空の場合は標準エラー出力のみに出力し、ファイルは作らない。
// 戻り値の closeFn はプロセス終了時に必ず呼ぶこと（複数回呼んでも安全）。
//
// 本アプリは cron から起動されて数秒で終了するため、実行中のローテーションは
// 行わない。日付をファイル名にすることで日次分割が達成される。
func Setup(logDir string, level slog.Level, now time.Time) (logger *slog.Logger, closeFn func() error, err error) {
	// ログ規約v1（2026-08-13）：stderr へは syslog priority プレフィックス付きで書く。
	// ファイル側には付けない（journald 以外が読むと "<3>" がノイズになるため）。
	stderr := NewJournaldWriter(os.Stderr)

	if logDir == "" {
		return New(stderr, level), func() error { return nil }, nil
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

	return New(io.MultiWriter(stderr, f), level), closeFn, nil
}
