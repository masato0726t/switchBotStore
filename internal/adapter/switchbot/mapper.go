package switchbot

import "switchBotStore/internal/domain"

// toDomainDevices は API のデバイス一覧を domain.Device のスライスへ畳み込む。
//
// API は物理デバイスと赤外線リモコンを別々の配列で返すが、以降の処理では
// DeviceKind で区別された単一のリストとして扱う。
func toDomainDevices(body deviceListBody) []domain.Device {
	devices := make([]domain.Device, 0, len(body.DeviceList)+len(body.InfraredRemoteList))
	for _, d := range body.DeviceList {
		devices = append(devices, toDomainDevice(d, domain.DeviceKindPhysical))
	}
	for _, d := range body.InfraredRemoteList {
		devices = append(devices, toDomainDevice(d, domain.DeviceKindInfraredRemote))
	}
	return devices
}

func toDomainDevice(d deviceInfo, kind domain.DeviceKind) domain.Device {
	return domain.Device{
		ID:                  domain.DeviceID(d.DeviceID),
		Name:                d.DeviceName,
		Type:                d.DeviceType,
		HubID:               domain.DeviceID(d.HubDeviceID),
		Kind:                kind,
		CloudServiceEnabled: d.EnableCloudService,
	}
}
