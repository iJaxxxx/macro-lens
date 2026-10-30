package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	CookieName = "ml_session"
	sessionTTL = 30 * 24 * time.Hour
)

// Sessions 用 HMAC 签名的 Cookie 保存登录态，服务端不用存会话。
type Sessions struct {
	secret []byte
	Now    func() time.Time
}

// LoadSecret 优先用 SESSION_SECRET；没有就在 dataDir 下生成一个并保存，重启后会话依然有效。
func LoadSecret(env, dataDir string) ([]byte, error) {
	if env != "" {
		return []byte(env), nil
	}
	path := filepath.Join(dataDir, "session.key")
	if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
		return hex.DecodeString(strings.TrimSpace(string(b)))
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	return secret, os.WriteFile(path, []byte(hex.EncodeToString(secret)), 0o600)
}

func NewSessions(secret []byte) *Sessions { return &Sessions{secret: secret, Now: time.Now} }

type sessionData struct {
	User
	Exp int64 `json:"exp"`
}

func (s *Sessions) sign(payload string) string {
	m := hmac.New(sha256.New, s.secret)
	m.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

func (s *Sessions) Issue(w http.ResponseWriter, r *http.Request, u User) {
	b, _ := json.Marshal(sessionData{User: u, Exp: s.Now().Add(sessionTTL).Unix()})
	payload := base64.RawURLEncoding.EncodeToString(b)
	http.SetCookie(w, &http.Cookie{
		Name: CookieName, Value: payload + "." + s.sign(payload), Path: "/",
		MaxAge: int(sessionTTL.Seconds()), HttpOnly: true, Secure: isHTTPS(r),
		// Lax 让跨站发起的 POST 带不上 Cookie，防 CSRF
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Sessions) Clear(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: isHTTPS(r), SameSite: http.SameSiteLaxMode})
}

// User 从请求的 Cookie 里取出已登录用户。
func (s *Sessions) User(r *http.Request) (User, error) {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return User{}, errors.New("未登录")
	}
	payload, sig, ok := strings.Cut(c.Value, ".")
	if !ok || !hmac.Equal([]byte(sig), []byte(s.sign(payload))) {
		return User{}, errors.New("会话无效")
	}
	b, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return User{}, errors.New("会话无效")
	}
	var d sessionData
	if err := json.Unmarshal(b, &d); err != nil {
		return User{}, errors.New("会话无效")
	}
	if s.Now().Unix() >= d.Exp {
		return User{}, errors.New("会话已过期")
	}
	return d.User, nil
}

func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}
