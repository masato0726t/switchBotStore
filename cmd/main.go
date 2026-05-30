package main

import (
	"log"

	"switchBotStore/internal/collector"
	"switchBotStore/internal/config"
	"switchBotStore/internal/database"
	"switchBotStore/internal/logger"
	"switchBotStore/internal/repository"
	"switchBotStore/internal/switchbot"
)

// defaultLogDir は config.json を読み込む前に使う暫定ログディレクトリ。
// config.Load より先にログファイルを開くことで、起動時エラーもファイルに記録できる。
const defaultLogDir = "logs"

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmsgprefix)

	// closeLog を間接参照で defer することで、後から再代入しても正しい関数が呼ばれる
	var closeLog = func() {}
	defer func() { closeLog() }()

	// config.Load より前にデフォルトのログディレクトリでファイルを開く
	if cl, err := logger.Setup(defaultLogDir); err != nil {
		log.Printf("[WARN] デフォルトログの設定に失敗しました: %v", err)
	} else {
		closeLog = cl
	}

	cfg, err := config.Load("config.json")
	if err != nil {
		log.Fatalf("[ERROR] 設定ファイルの読み込みに失敗しました: %v", err)
	}
	log.Printf("[INFO] 設定を読み込みました (アカウント数: %d)", len(cfg.Accounts))

	// config.json の log_dir がデフォルトと異なる場合は切り替える
	if cfg.LogDir != defaultLogDir {
		closeLog()           // デフォルトのログファイルを閉じる
		closeLog = func() {} // フェイルセーフ：再設定に失敗しても defer が安全に呼ばれる

		if cfg.LogDir != "" {
			if cl, err := logger.Setup(cfg.LogDir); err != nil {
				log.Printf("[WARN] ログファイルの設定に失敗しました（標準出力のみ使用します）: %v", err)
			} else {
				closeLog = cl
				log.Printf("[INFO] ログ出力先: %s", cfg.LogDir)
			}
		}
	} else if cfg.LogDir != "" {
		log.Printf("[INFO] ログ出力先: %s", cfg.LogDir)
	}

	db, err := database.Connect(cfg.Database)
	if err != nil {
		log.Fatalf("[ERROR] データベースへの接続に失敗しました: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("[WARN] DB接続のクローズに失敗しました: %v", err)
		}
	}()

	if err := database.Migrate(db); err != nil {
		log.Fatalf("[ERROR] テーブルのマイグレーションに失敗しました: %v", err)
	}

	clients := make([]switchbot.Client, 0, len(cfg.Accounts))
	for _, acc := range cfg.Accounts {
		clients = append(clients, switchbot.NewClient(acc.Token, acc.Secret, acc.Name))
		log.Printf("[INFO] アカウント登録: %s", acc.Name)
	}

	repo := repository.NewSQL(db)
	col := collector.New(repo, clients)

	firstRun, err := database.IsFirstRun(db)
	if err != nil {
		log.Fatalf("[ERROR] 初回起動チェックに失敗しました: %v", err)
	}

	if firstRun {
		log.Println("[INFO] 初回起動を検出しました。全デバイスの初期データを収集します...")
		if err := col.InitialCollect(); err != nil {
			log.Printf("[WARN] 初期データ収集中にエラーが発生しました: %v", err)
		}
		if err := database.MarkFirstRunDone(db); err != nil {
			log.Printf("[WARN] 初回起動フラグの保存に失敗しました: %v", err)
		} else {
			log.Println("[INFO] 初期データ収集が完了しました")
		}
		return
	}

	log.Println("[INFO] データ収集を開始します")
	if err := col.Collect(); err != nil {
		log.Printf("[WARN] データ収集中にエラーが発生しました: %v", err)
	} else {
		log.Println("[INFO] データ収集が完了しました")
	}
}
