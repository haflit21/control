package push

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// decrypt is the browser side of RFC 8291, used to check our encoding.
func decrypt(t *testing.T, uaPriv *ecdh.PrivateKey, auth, body []byte) []byte {
	t.Helper()
	if len(body) < 21+65 {
		t.Fatalf("body trop court : %d", len(body))
	}
	salt := body[:16]
	rs := binary.BigEndian.Uint32(body[16:20])
	idlen := int(body[20])
	asPub := body[21 : 21+idlen]
	ct := body[21+idlen:]
	if int(rs) < len(ct) {
		t.Fatalf("rs %d < ciphertext %d", rs, len(ct))
	}
	asKey, err := ecdh.P256().NewPublicKey(asPub)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := uaPriv.ECDH(asKey)
	if err != nil {
		t.Fatal(err)
	}
	info := append(append([]byte("WebPush: info\x00"), uaPriv.PublicKey().Bytes()...), asPub...)
	ikm := hkdf(auth, shared, info, 32)
	key := hkdf(salt, ikm, []byte("Content-Encoding: aes128gcm\x00"), 16)
	nonce := hkdf(salt, ikm, []byte("Content-Encoding: nonce\x00"), 12)
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		t.Fatalf("déchiffrement : %v", err)
	}
	if pt[len(pt)-1] != 0x02 {
		t.Fatalf("délimiteur attendu 0x02, reçu %x", pt[len(pt)-1])
	}
	return pt[:len(pt)-1]
}

func newSub(t *testing.T, endpoint string) (Subscription, *ecdh.PrivateKey, []byte) {
	t.Helper()
	ua, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	auth := make([]byte, 16)
	rand.Read(auth)
	var sub Subscription
	sub.Endpoint = endpoint
	sub.Keys.P256dh = base64.RawURLEncoding.EncodeToString(ua.PublicKey().Bytes())
	sub.Keys.Auth = base64.RawURLEncoding.EncodeToString(auth)
	return sub, ua, auth
}

func TestEncryptRoundTrip(t *testing.T) {
	sub, ua, auth := newSub(t, "https://push.example/x")
	body, err := Encrypt(sub, []byte(`{"title":"Control"}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(decrypt(t, ua, auth, body)); got != `{"title":"Control"}` {
		t.Fatalf("got %q", got)
	}
}

// RFC 8291 appendix A test vector.
func TestRFC8291Vector(t *testing.T) {
	d := func(s string) []byte {
		b, err := base64.RawURLEncoding.DecodeString(s)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	uaPub := d("BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4")
	auth := d("BTBZMqHH6r4Tts7J_aSIgg")
	salt := d("DGv6ra1nlYgDCS1FRnbzlw")
	asPrivBytes := d("yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw")
	asPriv, err := ecdh.P256().NewPrivateKey(asPrivBytes)
	if err != nil {
		t.Fatal(err)
	}
	out, err := encryptWith(uaPub, auth, salt, asPriv, []byte("When I grow up, I want to be a watermelon"))
	if err != nil {
		t.Fatal(err)
	}
	want := d("DGv6ra1nlYgDCS1FRnbzlwAAEABBBP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A_yl95bQpu6cVPTpK4Mqgkf1CXztLVBSt2Ks3oZwbuwXPXLWyouBWLVWGNWQexSgSxsj_Qulcy4a-fN")
	if !equalBytes(out[:21+65], want[:21+65]) {
		t.Fatalf("en-tête différent\n got %x\nwant %x", out[:86], want[:86])
	}
	if !equalBytes(out, want) {
		t.Fatalf("chiffré différent (%d vs %d octets)\n got %s\nwant %s", len(out), len(want), base64.RawURLEncoding.EncodeToString(out), base64.RawURLEncoding.EncodeToString(want))
	}
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestStoreBroadcast(t *testing.T) {
	var hits atomic.Int32
	var gotAuth atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/gone") {
			w.WriteHeader(410)
			return
		}
		hits.Add(1)
		gotAuth.Store(r.Header.Get("Authorization"))
		if r.Header.Get("Content-Encoding") != "aes128gcm" || r.Header.Get("TTL") == "" {
			t.Errorf("en-têtes : %v", r.Header)
		}
		b, _ := io.ReadAll(r.Body)
		if len(b) < 86 {
			t.Errorf("corps trop court")
		}
		w.WriteHeader(201)
	}))
	defer srv.Close()

	dir := t.TempDir()
	st, err := Open(dir, "mailto:control@example.test")
	if err != nil {
		t.Fatal(err)
	}
	pub := st.PublicKey()
	if len(pub) != 87 {
		t.Fatalf("clé publique : %d caractères", len(pub))
	}
	// the key survives a restart
	st2, _ := Open(dir, "mailto:control@example.test")
	if st2.PublicKey() != pub {
		t.Fatal("clé VAPID non persistée")
	}

	ok, _, _ := newSub(t, srv.URL+"/ok")
	gone, _, _ := newSub(t, srv.URL+"/gone")
	st.Add(ok)
	st.Add(gone)
	if st.Count() != 2 {
		t.Fatalf("count %d", st.Count())
	}
	st3, _ := Open(dir, "mailto:x@y")
	if st3.Count() != 2 {
		t.Fatal("abonnements non persistés")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	n := st.Broadcast(ctx, Message{Title: "Alarme", Body: "Déclenchée", Tag: "alarm"})
	if n != 1 || hits.Load() != 1 {
		t.Fatalf("delivered %d hits %d", n, hits.Load())
	}
	if st.Count() != 1 {
		t.Fatalf("l'abonnement 410 devrait être supprimé, count=%d", st.Count())
	}
	auth, _ := gotAuth.Load().(string)
	if !strings.HasPrefix(auth, "vapid t=") || !strings.Contains(auth, ", k="+pub) {
		t.Fatalf("Authorization : %q", auth)
	}
	jwt := strings.TrimPrefix(strings.SplitN(auth, ",", 2)[0], "vapid t=")
	if parts := strings.Split(jwt, "."); len(parts) != 3 {
		t.Fatalf("JWT : %q", jwt)
	}
}
