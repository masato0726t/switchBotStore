-- +goose Up
-- デバイス一覧を API から最後に取得した日時。デバイス構成はめったに変わらない
-- ため、この時刻が新しいうちは devices テーブルの内容を再利用し、
-- /v1.1/devices の呼び出しを省く（SwitchBot API のレート制限対策）。
ALTER TABLE api_accounts
    ADD COLUMN devices_synced_at DATETIME NULL
    COMMENT 'デバイス一覧をAPIから最後に取得した日時';

-- +goose Down
ALTER TABLE api_accounts DROP COLUMN devices_synced_at;
