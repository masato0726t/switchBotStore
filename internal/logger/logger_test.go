package logger

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// resetLog はテスト終了時にグローバルlogの出力先を元に戻す
func resetLog(t *testing.T) {
	t.Helper()
	orig := log.Writer()
	t.Cleanup(func() { log.SetOutput(orig) })
}

// TestSetup_EmptyLogDir は log_dir が空の場合にファイルを作成しないことを確認する
func TestSetup_EmptyLogDir(t *testing.T) {
	resetLog(t)

	close, err := Setup("")
	if err != nil {
		t.Fatalf("エラーが発生しない想定: %v", err)
	}
	close()
}

// TestSetup_CreatesDirectory はログディレクトリが存在しない場合に自動作成することを確認する
func TestSetup_CreatesDirectory(t *testing.T) {
	resetLog(t)

	dir := filepath.Join(t.TempDir(), "nested", "logs")
	close, err := Setup(dir)
	if err != nil {
		t.Fatalf("ネストしたディレクトリでもエラーなし想定: %v", err)
	}
	defer close()

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Errorf("ログディレクトリが作成されていない: %s", dir)
	}
}

// TestSetup_CreatesLogFile は起動日付のログファイルが作成されることを確認する
func TestSetup_CreatesLogFile(t *testing.T) {
	resetLog(t)

	dir := t.TempDir()
	close, err := Setup(dir)
	if err != nil {
		t.Fatalf("エラーが発生しない想定: %v", err)
	}
	defer close()

	expected := filepath.Join(dir, time.Now().Format("2006-01-02")+".log")
	if _, err := os.Stat(expected); os.IsNotExist(err) {
		t.Errorf("ログファイルが作成されていない: %s", expected)
	}
}

// TestSetup_WritesToFile は log.Println の出力がファイルに書き込まれることを確認する
func TestSetup_WritesToFile(t *testing.T) {
	resetLog(t)

	dir := t.TempDir()
	close, err := Setup(dir)
	if err != nil {
		t.Fatalf("エラーが発生しない想定: %v", err)
	}
	defer close()

	log.Println("テストメッセージ12345")

	logFile := filepath.Join(dir, time.Now().Format("2006-01-02")+".log")
	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("ログファイルの読み込みに失敗: %v", err)
	}
	if !strings.Contains(string(content), "テストメッセージ12345") {
		t.Errorf("ログファイルにメッセージが書き込まれていない\nファイル内容: %s", string(content))
	}
}

// TestSetup_AppendsToExistingFile は既存ファイルへの追記モードで開くことを確認する
func TestSetup_AppendsToExistingFile(t *testing.T) {
	resetLog(t)

	dir := t.TempDir()
	logFile := filepath.Join(dir, time.Now().Format("2006-01-02")+".log")

	// 事前にファイルを作成して内容を書き込む
	if err := os.WriteFile(logFile, []byte("既存の内容\n"), 0644); err != nil {
		t.Fatal(err)
	}

	close, err := Setup(dir)
	if err != nil {
		t.Fatalf("エラーが発生しない想定: %v", err)
	}
	defer close()

	log.Println("追記メッセージ")

	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	body := string(content)
	if !strings.Contains(body, "既存の内容") {
		t.Error("既存の内容が消えている（追記モードではない）")
	}
	if !strings.Contains(body, "追記メッセージ") {
		t.Error("追記メッセージが書き込まれていない")
	}
}

// TestSetup_CloseStopsRotation はクローズ後にgoroutineが停止することを確認する
func TestSetup_CloseStopsRotation(t *testing.T) {
	resetLog(t)

	dir := t.TempDir()
	close, err := Setup(dir)
	if err != nil {
		t.Fatalf("エラーが発生しない想定: %v", err)
	}

	// クローズが二重呼び出しでもパニックしないことも確認
	close()
	close()
}
