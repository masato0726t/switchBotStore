package domain

// DeviceID は SwitchBot が採番するデバイス識別子。
type DeviceID string

// DeviceRecordID は永続化されたデバイスの識別子（devices.id）。
type DeviceRecordID int64

// DeviceKind はデバイスの種別。
type DeviceKind int

const (
	// DeviceKindPhysical は物理デバイス。
	DeviceKindPhysical DeviceKind = iota
	// DeviceKindInfraredRemote は Hub に登録された仮想赤外線リモコン。
	DeviceKindInfraredRemote
)

// Device は SwitchBot に登録されたデバイス。
type Device struct {
	ID                  DeviceID
	Name                string
	Type                string
	HubID               DeviceID
	Kind                DeviceKind
	CloudServiceEnabled bool
}

// StatusReadable はこのデバイスからステータスを取得できるかを返す。
//
// 赤外線リモコンには SwitchBot API にステータス取得エンドポイントが存在せず、
// クラウドサービスが無効なデバイスは API 経由で状態を読めない。
func (d Device) StatusReadable() bool {
	return d.Kind == DeviceKindPhysical && d.CloudServiceEnabled
}
