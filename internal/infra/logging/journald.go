// journald.go — ログ規約v1（2026-08-13制定）の priority 伝達。
package logging

import (
	"bytes"
	"io"
)

// journaldWriter は 1レコードごとに行頭へ syslog priority（"<N>"）を付けて書き出す。
//
// なぜ必要か：
//
//	systemd は stdout/stderr の行頭にある "<N>" を SyslogLevelPrefix（既定 on）で
//	解釈し、journald の PRIORITY フィールドに載せたうえでプレフィックスを取り除く。
//	これが無いとアプリのログは全行 priority 6（info）として記録され、
//	`journalctl -p err` も集約基盤（VictoriaLogs）の PRIORITY 絞り込みも効かない。
//	実測（2026-08-13・CT104）では 285万件のうち warning 以上が 142件しか無く、
//	アプリが出した WARN/ERROR はすべて info に埋もれていた。
//
// 実装の前提：
//
//	slog のハンドラは 1レコード = 1回の Write なので、渡されたバイト列から
//	レベルを判定してよい。JSON形式（"level":"ERROR"）と text形式（level=ERROR）の
//	両方を見る。
type journaldWriter struct{ w io.Writer }

// NewJournaldWriter は w への書き込みに priority プレフィックスを付ける Writer を返す。
func NewJournaldWriter(w io.Writer) io.Writer { return journaldWriter{w: w} }

var journaldPriorities = []struct {
	json, text []byte
	prefix     string
}{
	{[]byte(`"level":"ERROR"`), []byte("level=ERROR"), "<3>"},
	{[]byte(`"level":"WARN"`), []byte("level=WARN"), "<4>"},
	{[]byte(`"level":"DEBUG"`), []byte("level=DEBUG"), "<7>"},
}

func (jw journaldWriter) Write(p []byte) (int, error) {
	prefix := "<6>" // INFO（既定）
	for _, c := range journaldPriorities {
		if bytes.Contains(p, c.json) || bytes.Contains(p, c.text) {
			prefix = c.prefix
			break
		}
	}
	buf := make([]byte, 0, len(prefix)+len(p))
	buf = append(buf, prefix...)
	buf = append(buf, p...)
	if _, err := jw.w.Write(buf); err != nil {
		return 0, err
	}
	// io.Writer の契約どおり「受け取ったバイト数」を返す（プレフィックス分は数えない）。
	return len(p), nil
}
