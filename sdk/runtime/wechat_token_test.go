package runtime

import (
	"context"
	"testing"

	"github.com/chanxuehong/wechat/oauth2"
	"github.com/stretchr/testify/assert"

	"github.com/ChenBigdata421/jxt-core/storage/cache"
)

func TestWechatTokenStore_RoundTrip(t *testing.T) {
	store := cache.NewMemory()
	ctx := context.Background()
	ts := NewWechatTokenStore(store, "", "test_key")

	tok := &oauth2.Token{AccessToken: "abc123", ExpiresIn: 3600}
	err := ts.PutToken(ctx, tok)
	assert.NoError(t, err)

	got, err := ts.Token(ctx)
	assert.NoError(t, err)
	assert.Equal(t, "abc123", got.AccessToken)
}

func TestWechatTokenStore_DefaultKey(t *testing.T) {
	store := cache.NewMemory()
	ts := NewWechatTokenStore(store, "", "")
	assert.Equal(t, "wx_token_store_key", ts.key)

	// Verify the default key works with PutToken.
	tok := &oauth2.Token{AccessToken: "default", ExpiresIn: 3600}
	err := ts.PutToken(context.Background(), tok)
	assert.NoError(t, err)
}

func TestWechatTokenStore_MissingKey(t *testing.T) {
	store := cache.NewMemory()
	ts := NewWechatTokenStore(store, "", "missing")
	_, err := ts.Token(context.Background())
	assert.Error(t, err, "Token() should return error for missing key")
}

func TestWechatTokenStore_ShortExpiry(t *testing.T) {
	store := cache.NewMemory()
	ts := NewWechatTokenStore(store, "", "short")

	// ExpiresIn < tokenExpiryBufferSeconds — should clamp to 1s, not go negative.
	tok := &oauth2.Token{AccessToken: "shortlived", ExpiresIn: 50}
	err := ts.PutToken(context.Background(), tok)
	assert.NoError(t, err)

	// Token should be immediately retrievable (not evicted by negative TTL).
	got, err := ts.Token(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "shortlived", got.AccessToken)
}
