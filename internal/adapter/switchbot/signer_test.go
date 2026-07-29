package switchbot

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSigner_署名が仕様どおり生成される(t *testing.T) {
	const (
		token  = "mytoken"
		secret = "mysecret"
		nonce  = "fixed-nonce"
	)
	fixed := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	s := &signer{now: func() time.Time { return fixed }}
	got := s.signWithNonce(token, secret, nonce)

	ts := strconv.FormatInt(fixed.UnixMilli(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(token + ts + nonce))
	want := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	assert.Equal(t, want, got.Sign)
	assert.Equal(t, token, got.Authorization)
	assert.Equal(t, nonce, got.Nonce)
	assert.Equal(t, ts, got.Timestamp)
}

func TestSigner_必須項目が空でない(t *testing.T) {
	s := newSigner()
	got, err := s.sign("tok", "sec")

	require.NoError(t, err)
	assert.NotEmpty(t, got.Authorization)
	assert.NotEmpty(t, got.Sign)
	assert.NotEmpty(t, got.Nonce)
	assert.NotEmpty(t, got.Timestamp)
}

func TestSigner_nonceは毎回変わる(t *testing.T) {
	s := newSigner()

	first, err := s.sign("tok", "sec")
	require.NoError(t, err)
	second, err := s.sign("tok", "sec")
	require.NoError(t, err)

	assert.NotEqual(t, first.Nonce, second.Nonce)
}
