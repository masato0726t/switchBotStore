-- +goose Up
-- SwitchBot API がレート制限 (HTTP 429) で応答を拒んだときの待機状態。
-- 収集は1分ごとに別プロセスとして起動するため、待機の状態をプロセス内に
-- 持てない。アカウント単位でここに記録し、次の起動が読み直して、期限が
-- 来るまで API を呼ばずに見送る。
ALTER TABLE api_accounts
    ADD COLUMN rate_limited_until DATETIME NULL
        COMMENT 'レート制限で待機する期限。NULLなら待機していない',
    ADD COLUMN rate_limited_since DATETIME NULL
        COMMENT '連続した429のうち最初のものを受けた日時。NULLなら待機していない',
    ADD COLUMN rate_limit_failures INT NOT NULL DEFAULT 0
        COMMENT '連続して429を受けた回数。待機時間の算出に使う';

-- +goose Down
ALTER TABLE api_accounts
    DROP COLUMN rate_limited_until,
    DROP COLUMN rate_limited_since,
    DROP COLUMN rate_limit_failures;
