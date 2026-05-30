package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Logger は標準出力とファイルへの同時ログ出力と日次ローテーションを管理する
type Logger struct {
	logDir string
	file   *os.File
	mu     sync.Mutex
	stopCh chan struct{}
}

// Setup はログ出力先を設定し、終了時に呼ぶクローズ関数を返す。
// logDir が空の場合は標準出力のみに出力し、ファイルは作成しない。
func Setup(logDir string) (func(), error) {
	if logDir == "" {
		return func() {}, nil
	}

	l := &Logger{
		logDir: logDir,
		stopCh: make(chan struct{}),
	}

	if err := l.openFile(time.Now()); err != nil {
		return nil, err
	}

	go l.rotateDailyLoop()

	var once sync.Once
	return func() {
		once.Do(func() {
			close(l.stopCh)
			l.mu.Lock()
			defer l.mu.Unlock()
			if l.file != nil {
				l.file.Close()
				l.file = nil
			}
		})
	}, nil
}

// openFile は指定した日付のログファイルを開き、log.SetOutput を更新する
func (l *Logger) openFile(t time.Time) error {
	if err := os.MkdirAll(l.logDir, 0755); err != nil {
		return fmt.Errorf("ログディレクトリの作成に失敗 (%s): %v", l.logDir, err)
	}

	filename := filepath.Join(l.logDir, t.Format("2006-01-02")+".log")
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("ログファイルのオープンに失敗 (%s): %v", filename, err)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		l.file.Close()
	}
	l.file = f
	log.SetOutput(io.MultiWriter(os.Stdout, f))

	return nil
}

// rotateDailyLoop は翌日0時になるたびに新しいログファイルへ切り替える
func (l *Logger) rotateDailyLoop() {
	for {
		now := time.Now()
		tomorrow := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 1, 0, now.Location())

		select {
		case <-time.After(tomorrow.Sub(now)):
			if err := l.openFile(time.Now()); err != nil {
				log.Printf("[WARN] ログファイルのローテーションに失敗しました: %v", err)
			} else {
				log.Printf("[INFO] ログファイルをローテーションしました")
			}
		case <-l.stopCh:
			return
		}
	}
}
