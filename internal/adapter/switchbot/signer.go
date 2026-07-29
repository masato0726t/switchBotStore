// Package switchbot は SwitchBot API v1.1 に対する usecase.DeviceGateway の実装。
package switchbot

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"time"
)

// authHeaders は SwitchBot API が要求する認証ヘッダーの値一式。
type authHeaders struct {
	Authorization string
	Sign          string
	Nonce         string
	Timestamp     string
}

// signer は SwitchBot API の認証ヘッダーを生成する。
//
// 署名仕様: Base64( HMAC-SHA256( token + timestamp_ms + nonce, secret ) )
type signer struct {
	now      func() time.Time
	newNonce func() (string, error)
}

func newSigner() *signer {
	return &signer{now: time.Now, newNonce: randomNonce}
}

// sign はランダムな nonce を生成して認証ヘッダーを返す。
func (s *signer) sign(token, secret string) (authHeaders, error) {
	nonce, err := s.newNonce()
	if err != nil {
		return authHeaders{}, err
	}
	return s.signWithNonce(token, secret, nonce), nil
}

// signWithNonce は nonce を指定して認証ヘッダーを生成する（テストで固定するため分離）。
func (s *signer) signWithNonce(token, secret, nonce string) authHeaders {
	ts := strconv.FormatInt(s.now().UnixMilli(), 10)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(token + ts + nonce))

	return authHeaders{
		Authorization: token,
		Sign:          base64.StdEncoding.EncodeToString(mac.Sum(nil)),
		Nonce:         nonce,
		Timestamp:     ts,
	}
}

// randomNonce は UUID 形式のランダム文字列を返す。
func randomNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("nonce の生成に失敗しました: %w", err)
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}
