package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func b64(v any) string {
	b, _ := json.Marshal(v)
	return base64.RawURLEncoding.EncodeToString(b)
}

func sign(t *testing.T, key *rsa.PrivateKey, kid string, c map[string]any) string {
	t.Helper()
	unsigned := b64(map[string]string{"alg": "RS256", "kid": kid}) + "." + b64(c)
	digest := sha256.Sum256([]byte(unsigned))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func TestVerify(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=600")
		json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kid": "k1",
			"n":   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
	}))
	defer srv.Close()

	now := time.Unix(1_800_000_000, 0)
	v := &Verifier{ClientID: "cid.apps.googleusercontent.com", CertsURL: srv.URL, HTTP: srv.Client(), Now: func() time.Time { return now }}
	good := map[string]any{"iss": "https://accounts.google.com", "aud": v.ClientID, "sub": "123", "exp": now.Unix() + 60,
		"email": "a@gmail.com", "email_verified": true, "name": "A"}

	u, err := v.Verify(context.Background(), sign(t, key, "k1", good))
	if err != nil || u.Sub != "123" || u.Email != "a@gmail.com" {
		t.Fatalf("user = %+v, err = %v", u, err)
	}

	for name, mutate := range map[string]func(map[string]any){
		"wrong aud":  func(c map[string]any) { c["aud"] = "other" },
		"wrong iss":  func(c map[string]any) { c["iss"] = "evil.com" },
		"expired":    func(c map[string]any) { c["exp"] = now.Unix() - 1 },
		"unverified": func(c map[string]any) { c["email_verified"] = false },
	} {
		c := map[string]any{}
		for k, val := range good {
			c[k] = val
		}
		mutate(c)
		if _, err := v.Verify(context.Background(), sign(t, key, "k1", c)); err == nil {
			t.Errorf("%s: want error", name)
		}
	}

	// 用别的私钥签名
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	if _, err := v.Verify(context.Background(), sign(t, other, "k1", good)); err == nil {
		t.Error("forged signature accepted")
	}
	if _, err := v.Verify(context.Background(), sign(t, key, "unknown", good)); err == nil {
		t.Error("unknown kid accepted")
	}
}

func TestSessions(t *testing.T) {
	s := NewSessions([]byte("secret"))
	rec := httptest.NewRecorder()
	s.Issue(rec, httptest.NewRequest(http.MethodPost, "/", nil), User{Sub: "1", Email: "a@gmail.com"})
	cookie := rec.Result().Cookies()[0]
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("cookie flags = %+v", cookie)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(cookie)
	if u, err := s.User(req); err != nil || u.Email != "a@gmail.com" {
		t.Fatalf("user = %+v, err = %v", u, err)
	}

	// 篡改载荷后签名对不上
	payload, sig, _ := strings.Cut(cookie.Value, ".")
	forged := b64(sessionData{User: User{Sub: "2"}, Exp: time.Now().Add(time.Hour).Unix()})
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: forged + "." + sig})
	if _, err := s.User(req); err == nil {
		t.Error("tampered session accepted")
	}

	// 过期
	s.Now = func() time.Time { return time.Now().Add(sessionTTL + time.Hour) }
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: payload + "." + sig})
	if _, err := s.User(req); err == nil {
		t.Error("expired session accepted")
	}
}
