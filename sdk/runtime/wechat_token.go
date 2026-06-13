package runtime

import (
	"context"
	"encoding/json"

	"github.com/chanxuehong/wechat/oauth2"

	"github.com/ChenBigdata421/jxt-core/storage"
)

// WechatTokenStore stores and retrieves WeChat OAuth2 tokens via the
// storage.AdapterCache interface. Extracted from the runtime Cache wrapper
// to decouple WeChat concerns from the generic cache layer.
type WechatTokenStore struct {
	store  storage.AdapterCache
	prefix string
	key    string
}

// NewWechatTokenStore creates a WeChat token store backed by the given cache.
// The key defaults to "wx_token_store_key" if empty.
func NewWechatTokenStore(store storage.AdapterCache, prefix, key string) *WechatTokenStore {
	if key == "" {
		key = "wx_token_store_key"
	}
	return &WechatTokenStore{store: store, prefix: prefix, key: key}
}

// Token retrieves the stored WeChat OAuth2 token.
func (s *WechatTokenStore) Token(ctx context.Context) (*oauth2.Token, error) {
	var token oauth2.Token
	str, err := s.store.Get(ctx, s.prefix+s.key)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal([]byte(str), &token)
	if err != nil {
		return nil, err
	}
	return &token, nil
}

// tokenExpiryBufferSeconds is subtracted from ExpiresIn to refresh the token
// before it actually expires. If the remaining lifetime is less than this buffer,
// we clamp to 1 second to avoid a negative TTL (which would evict the token
// immediately in the memory adapter).
const tokenExpiryBufferSeconds = 200

// PutToken stores the WeChat OAuth2 token.
func (s *WechatTokenStore) PutToken(ctx context.Context, token *oauth2.Token) error {
	rb, err := json.Marshal(token)
	if err != nil {
		return err
	}
	expire := int(token.ExpiresIn) - tokenExpiryBufferSeconds
	if expire <= 0 {
		expire = 1
	}
	return s.store.Set(ctx, s.prefix+s.key, string(rb), expire)
}
