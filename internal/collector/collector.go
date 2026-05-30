package collector

import (
	"fmt"
	"log"
	"time"

	"switchBotStore/internal/repository"
	"switchBotStore/internal/switchbot"
)

// Collector はSwitchBot APIからデータを収集してDBに保存する
type Collector struct {
	repo    repository.Repository
	clients []switchbot.Client
}

func New(repo repository.Repository, clients []switchbot.Client) *Collector {
	return &Collector{repo: repo, clients: clients}
}

// InitialCollect は初回起動時に全アカウント・全デバイスのデータを収集する
func (c *Collector) InitialCollect() error {
	log.Println("=== 初回データ収集を開始します ===")
	return c.Collect()
}

// Collect は全アカウントのデバイスステータスを収集してDBに保存する
func (c *Collector) Collect() error {
	recordedAt := time.Now()

	var lastErr error
	for _, client := range c.clients {
		if err := c.collectFromAccount(client, recordedAt); err != nil {
			log.Printf("[%s] 収集中にエラーが発生しました: %v", client.AccountName(), err)
			lastErr = err
		}
	}
	return lastErr
}

func (c *Collector) collectFromAccount(client switchbot.Client, recordedAt time.Time) error {
	accountID, err := c.repo.UpsertAccount(client.AccountName(), client.AccountToken(), client.AccountSecret())
	if err != nil {
		return fmt.Errorf("アカウント登録失敗: %v", err)
	}

	deviceList, err := client.GetDevices()
	if err != nil {
		return fmt.Errorf("デバイス一覧取得失敗: %v", err)
	}

	log.Printf("[%s] デバイス数: 通常=%d件, 赤外線リモコン=%d件",
		client.AccountName(), len(deviceList.DeviceList), len(deviceList.InfraredRemoteList))

	for _, dev := range deviceList.DeviceList {
		deviceDBID, err := c.repo.UpsertDevice(accountID, dev, false)
		if err != nil {
			log.Printf("[%s] デバイス登録失敗 (device_id=%s): %v", client.AccountName(), dev.DeviceID, err)
			continue
		}

		// クラウドサービスが無効なデバイスはステータス取得不可
		if !dev.EnableCloudService {
			log.Printf("[%s] スキップ (クラウド無効): %s (%s)", client.AccountName(), dev.DeviceName, dev.DeviceType)
			continue
		}

		status, err := client.GetDeviceStatus(dev.DeviceID)
		if err != nil {
			log.Printf("[%s] ステータス取得失敗 (%s): %v", client.AccountName(), dev.DeviceName, err)
			continue
		}

		if err := c.repo.SaveStatusLog(deviceDBID, status, recordedAt); err != nil {
			log.Printf("[%s] ログ保存失敗 (%s): %v", client.AccountName(), dev.DeviceName, err)
			continue
		}

		log.Printf("[%s] 保存完了: %s (%s)", client.AccountName(), dev.DeviceName, dev.DeviceType)
	}

	// 赤外線リモコン: ステータス取得APIが存在しないためデバイス登録のみ
	for _, dev := range deviceList.InfraredRemoteList {
		if _, err := c.repo.UpsertDevice(accountID, dev, true); err != nil {
			log.Printf("[%s] 赤外線デバイス登録失敗 (device_id=%s): %v", client.AccountName(), dev.DeviceID, err)
		}
	}

	return nil
}
