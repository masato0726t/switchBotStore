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
			Name:       a.Name,
			Credential: domain.Credential{Token: a.Token, Secret: a.Secret},
		})
	}
	return result
}
