// Package push sends Web Push notifications (RFC 8030 + RFC 8291 aes128gcm
// payload encryption + RFC 8292 VAPID) with the standard library only.
// Subscriptions and the VAPID key pair live as JSON files in the state dir.
package push

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Subscription is what the browser's PushManager gives us.
type Subscription struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
	// ours
	Label   string    `json:"label,omitempty"` // user agent summary
	AddedAt time.Time `json:"addedAt"`
}

// Store keeps the VAPID keys and the subscriptions on disk.
type Store struct {
	dir  string
	mu   sync.Mutex
	subs map[string]Subscription // by endpoint
	priv *ecdsa.PrivateKey
	// Subject is the VAPID "sub" claim (mailto: or https:).
	Subject string
	HTTP    *http.Client
}

func Open(dir, subject string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, subs: map[string]Subscription{}, Subject: subject, HTTP: &http.Client{Timeout: 15 * time.Second}}
	if err := s.loadKey(); err != nil {
		return nil, err
	}
	s.loadSubs()
	return s, nil
}

func (s *Store) loadKey() error {
	p := filepath.Join(s.dir, "vapid.json")
	var saved struct{ D string }
	if b, err := os.ReadFile(p); err == nil && json.Unmarshal(b, &saved) == nil && saved.D != "" {
		d, err := b64d(saved.D)
		if err == nil {
			k := new(ecdsa.PrivateKey)
			k.Curve = elliptic.P256()
			k.D = new(big.Int).SetBytes(d)
			k.PublicKey.X, k.PublicKey.Y = k.Curve.ScalarBaseMult(d)
			s.priv = k
			return nil
		}
	}
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	s.priv = k
	b, _ := json.Marshal(struct{ D string }{b64(k.D.FillBytes(make([]byte, 32)))})
	return os.WriteFile(p, b, 0o600)
}

func (s *Store) loadSubs() {
	b, err := os.ReadFile(filepath.Join(s.dir, "push.json"))
	if err != nil {
		return
	}
	var list []Subscription
	if json.Unmarshal(b, &list) == nil {
		for _, sub := range list {
			s.subs[sub.Endpoint] = sub
		}
	}
}

func (s *Store) saveSubs() {
	list := make([]Subscription, 0, len(s.subs))
	for _, sub := range s.subs {
		list = append(list, sub)
	}
	b, _ := json.MarshalIndent(list, "", " ")
	_ = os.WriteFile(filepath.Join(s.dir, "push.json"), b, 0o600)
}

// PublicKey is the VAPID public key in the form PushManager.subscribe wants (base64url, uncompressed point).
func (s *Store) PublicKey() string {
	return b64(elliptic.Marshal(elliptic.P256(), s.priv.PublicKey.X, s.priv.PublicKey.Y)) //nolint:staticcheck // the uncompressed point is what Web Push wants
}

func (s *Store) Add(sub Subscription) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sub.AddedAt.IsZero() {
		sub.AddedAt = time.Now()
	}
	s.subs[sub.Endpoint] = sub
	s.saveSubs()
}

func (s *Store) Remove(endpoint string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.subs, endpoint)
	s.saveSubs()
}

func (s *Store) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.subs)
}

func (s *Store) List() []Subscription {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Subscription, 0, len(s.subs))
	for _, sub := range s.subs {
		out = append(out, sub)
	}
	return out
}

// Message is what the service worker shows.
type Message struct {
	Title string `json:"title"`
	Body  string `json:"body,omitempty"`
	Tag   string `json:"tag,omitempty"` // same tag replaces the previous notification
	URL   string `json:"url,omitempty"` // opened on click
	Level string `json:"level,omitempty"`
}

// Broadcast sends the message to every subscription; dead ones (404/410) are dropped.
// Returns the number delivered.
func (s *Store) Broadcast(ctx context.Context, m Message) int {
	payload, _ := json.Marshal(m)
	sent := 0
	for _, sub := range s.List() {
		code, err := s.Send(ctx, sub, payload, 24*time.Hour)
		switch {
		case err == nil && code/100 == 2:
			sent++
		case code == 404 || code == 410:
			s.Remove(sub.Endpoint)
		}
	}
	return sent
}

// Send encrypts the payload for one subscription and POSTs it. Returns the push service's status.
func (s *Store) Send(ctx context.Context, sub Subscription, payload []byte, ttl time.Duration) (int, error) {
	body, err := Encrypt(sub, payload)
	if err != nil {
		return 0, err
	}
	u, err := url.Parse(sub.Endpoint)
	if err != nil {
		return 0, err
	}
	jwt, err := s.vapid(u.Scheme + "://" + u.Host)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", sub.Endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("TTL", fmt.Sprint(int(ttl.Seconds())))
	req.Header.Set("Urgency", "high")
	req.Header.Set("Authorization", "vapid t="+jwt+", k="+s.PublicKey())
	resp, err := s.HTTP.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode/100 != 2 {
		return resp.StatusCode, fmt.Errorf("push : HTTP %d", resp.StatusCode)
	}
	return resp.StatusCode, nil
}

// vapid builds the ES256 JWT for an audience (the push service origin).
func (s *Store) vapid(aud string) (string, error) {
	hdr := b64([]byte(`{"typ":"JWT","alg":"ES256"}`))
	claims, _ := json.Marshal(map[string]any{"aud": aud, "exp": time.Now().Add(12 * time.Hour).Unix(), "sub": s.Subject})
	signing := hdr + "." + b64(claims)
	h := sha256.Sum256([]byte(signing))
	r, sig, err := ecdsa.Sign(rand.Reader, s.priv, h[:])
	if err != nil {
		return "", err
	}
	out := make([]byte, 64)
	r.FillBytes(out[:32])
	sig.FillBytes(out[32:])
	return signing + "." + b64(out), nil
}

// Encrypt implements RFC 8291 (aes128gcm, one record) for a subscription.
func Encrypt(sub Subscription, plaintext []byte) ([]byte, error) {
	uaPub, err := b64d(sub.Keys.P256dh)
	if err != nil {
		return nil, fmt.Errorf("p256dh : %w", err)
	}
	auth, err := b64d(sub.Keys.Auth)
	if err != nil {
		return nil, fmt.Errorf("auth : %w", err)
	}
	if len(auth) != 16 {
		return nil, errors.New("auth : 16 octets attendus")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	asPriv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return encryptWith(uaPub, auth, salt, asPriv, plaintext)
}

func encryptWith(uaPub, auth, salt []byte, asPriv *ecdh.PrivateKey, plaintext []byte) ([]byte, error) {
	uaKey, err := ecdh.P256().NewPublicKey(uaPub)
	if err != nil {
		return nil, fmt.Errorf("p256dh : %w", err)
	}
	shared, err := asPriv.ECDH(uaKey)
	if err != nil {
		return nil, err
	}
	asPub := asPriv.PublicKey().Bytes()
	// IKM = HKDF(auth, shared, "WebPush: info" || ua_public || as_public, 32)
	info := append(append([]byte("WebPush: info\x00"), uaPub...), asPub...)
	ikm := hkdf(auth, shared, info, 32)
	key := hkdf(salt, ikm, []byte("Content-Encoding: aes128gcm\x00"), 16)
	nonce := hkdf(salt, ikm, []byte("Content-Encoding: nonce\x00"), 12)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	// one record: plaintext || 0x02 (last record delimiter)
	record := append(append([]byte{}, plaintext...), 0x02)
	ct := gcm.Seal(nil, nonce, record, nil)
	// header: salt(16) | rs(4) | idlen(1) | keyid(65) — rs 4096 as in the RFC vector (one record, payload ≤ ~4 ko)
	const rs = uint32(4096)
	if len(ct) > int(rs) {
		return nil, errors.New("push : charge utile trop longue")
	}
	hdr := make([]byte, 0, 16+4+1+len(asPub))
	hdr = append(hdr, salt...)
	hdr = binary.BigEndian.AppendUint32(hdr, rs)
	hdr = append(hdr, byte(len(asPub)))
	hdr = append(hdr, asPub...)
	return append(hdr, ct...), nil
}

// hkdf is RFC 5869 with SHA-256, extract + one expand round (length ≤ 32).
func hkdf(salt, ikm, info []byte, n int) []byte {
	ext := hmac.New(sha256.New, salt)
	ext.Write(ikm)
	prk := ext.Sum(nil)
	exp := hmac.New(sha256.New, prk)
	exp.Write(info)
	exp.Write([]byte{1})
	return exp.Sum(nil)[:n]
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func b64d(s string) ([]byte, error) {
	if b, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	if b, err := base64.URLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.StdEncoding.DecodeString(s)
}
