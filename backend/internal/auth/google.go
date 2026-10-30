// Package auth 负责「用 Google 账号登录」：前端用 Google Identity Services 拿到 ID Token，
// 后端校验签名和声明后，签发自己的会话 Cookie。
package auth

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// User 是登录后的身份。Sub 是 Google 账号的唯一 ID，邮箱可能会改，存数据用 Sub。
type User struct {
	Sub     string `json:"sub"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
}

const googleCertsURL = "https://www.googleapis.com/oauth2/v3/certs"

// Verifier 校验 Google 签发的 ID Token。公钥按响应头里的 max-age 缓存。
type Verifier struct {
	ClientID string
	CertsURL string
	HTTP     *http.Client
	Now      func() time.Time

	mu      sync.Mutex
	keys    map[string]*rsa.PublicKey
	expires time.Time
}

func NewVerifier(clientID string) *Verifier {
	return &Verifier{ClientID: clientID, CertsURL: googleCertsURL, HTTP: &http.Client{Timeout: 8 * time.Second}, Now: time.Now}
}

type jwk struct {
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func (v *Verifier) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if k, ok := v.keys[kid]; ok && v.Now().Before(v.expires) {
		return k, nil
	}
	// 没缓存、过期或者遇到新的 kid（Google 轮换了密钥）都重新拉
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.CertsURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := v.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("拉取 Google 公钥失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("拉取 Google 公钥失败: status %d", resp.StatusCode)
	}
	var body struct {
		Keys []jwk `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	keys := make(map[string]*rsa.PublicKey, len(body.Keys))
	for _, k := range body.Keys {
		n, err1 := base64.RawURLEncoding.DecodeString(k.N)
		e, err2 := base64.RawURLEncoding.DecodeString(k.E)
		if err1 != nil || err2 != nil {
			continue
		}
		keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
	}
	v.keys = keys
	v.expires = v.Now().Add(maxAge(resp.Header.Get("Cache-Control")))
	if k, ok := keys[kid]; ok {
		return k, nil
	}
	return nil, errors.New("未知的签名密钥")
}

func maxAge(cc string) time.Duration {
	for _, part := range strings.Split(cc, ",") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(part), "max-age="); ok {
			if n, err := strconv.Atoi(v); err == nil {
				return time.Duration(n) * time.Second
			}
		}
	}
	return time.Hour
}

type claims struct {
	Iss           string `json:"iss"`
	Aud           string `json:"aud"`
	Sub           string `json:"sub"`
	Exp           int64  `json:"exp"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
}

// Verify 校验 RS256 签名、签发方、受众（必须是本站的 Client ID）、过期时间和邮箱验证状态。
func (v *Verifier) Verify(ctx context.Context, token string) (User, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return User{}, errors.New("ID Token 格式错误")
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := decodeSegment(parts[0], &header); err != nil {
		return User{}, err
	}
	if header.Alg != "RS256" {
		return User{}, fmt.Errorf("不支持的签名算法 %q", header.Alg)
	}
	key, err := v.key(ctx, header.Kid)
	if err != nil {
		return User{}, err
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return User{}, errors.New("签名格式错误")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], sig); err != nil {
		return User{}, errors.New("签名无效")
	}

	var c claims
	if err := decodeSegment(parts[1], &c); err != nil {
		return User{}, err
	}
	switch {
	case c.Iss != "accounts.google.com" && c.Iss != "https://accounts.google.com":
		return User{}, errors.New("签发方不是 Google")
	case c.Aud != v.ClientID:
		return User{}, errors.New("ID Token 不是签发给本站的")
	case v.Now().Unix() >= c.Exp:
		return User{}, errors.New("ID Token 已过期")
	case !c.EmailVerified:
		return User{}, errors.New("Google 账号的邮箱未验证")
	}
	return User{Sub: c.Sub, Email: c.Email, Name: c.Name, Picture: c.Picture}, nil
}

func decodeSegment(seg string, v any) error {
	b, err := base64.RawURLEncoding.DecodeString(seg)
	if err != nil {
		return errors.New("ID Token 编码错误")
	}
	return json.Unmarshal(b, v)
}
