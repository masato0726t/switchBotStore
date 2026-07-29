package domain

import (
	"encoding/json"
	"time"
)

// StatusPayload は SwitchBot API が返すデバイスステータスの生 JSON。
//
// デバイス種別ごとにフィールドが大きく異なるため、構造化せずそのまま保持し、
// DB の JSON カラムへ格納する。
type StatusPayload json.RawMessage

// StatusSnapshot はある時点で収集したデバイスステータス。
type StatusSnapshot struct {
	Payload    StatusPayload
	RecordedAt time.Time
}
