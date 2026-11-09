package ha

import (
	"bufio"
	"context"
	"crypto/sha1" //nolint:gosec // RFC 6455 handshake
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeHA speaks just enough of the HA WebSocket protocol: auth, get_states,
// subscribe_events, weather/subscribe_forecast, ping, and lets the test
// push a state_changed event.
type fakeHA struct {
	t      *testing.T
	token  string
	states []Entity
	push   chan Entity
	kick   chan struct{} // closes the current connection (simulates a network drop)
}

func (f *fakeHA) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/websocket" {
		http.NotFound(w, r)
		return
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		f.t.Fatal("no hijack")
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	sum := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11")) //nolint:gosec
	conn, rw, err := hj.Hijack()
	if err != nil {
		f.t.Fatal(err)
	}
	defer conn.Close()
	_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " + base64.StdEncoding.EncodeToString(sum[:]) + "\r\n\r\n")
	_ = rw.Flush()
	send := func(v any) {
		b, _ := json.Marshal(v)
		writeServerFrame(conn, b)
	}
	send(map[string]any{"type": "auth_required", "ha_version": "2026.11.0"})
	var subID, hourlyID, dailyID int
	msgs := make(chan map[string]any)
	go func() {
		br := bufio.NewReader(rw)
		for {
			b, err := readClientFrame(br, conn)
			if err != nil {
				close(msgs)
				return
			}
			var m map[string]any
			_ = json.Unmarshal(b, &m)
			msgs <- m
		}
	}()
	for {
		select {
		case <-f.kick:
			return
		case e := <-f.push:
			send(map[string]any{"id": subID, "type": "event", "event": map[string]any{"event_type": "state_changed", "data": map[string]any{"entity_id": e.EntityID, "new_state": e}}})
		case m, ok := <-msgs:
			if !ok {
				return
			}
			id, _ := m["id"].(float64)
			switch m["type"] {
			case "auth":
				if m["access_token"] != f.token {
					send(map[string]any{"type": "auth_invalid", "message": "Invalid access token"})
					return
				}
				send(map[string]any{"type": "auth_ok", "ha_version": "2026.11.0"})
			case "get_states":
				send(map[string]any{"id": int(id), "type": "result", "success": true, "result": f.states})
			case "subscribe_events":
				subID = int(id)
				send(map[string]any{"id": int(id), "type": "result", "success": true, "result": nil})
			case "weather/subscribe_forecast":
				send(map[string]any{"id": int(id), "type": "result", "success": true, "result": nil})
				if m["forecast_type"] == "hourly" {
					hourlyID = int(id)
				} else {
					dailyID = int(id)
				}
				fid := hourlyID
				if m["forecast_type"] == "daily" {
					fid = dailyID
				}
				send(map[string]any{"id": fid, "type": "event", "event": map[string]any{"type": m["forecast_type"], "forecast": []map[string]any{{"datetime": "2026-11-08T18:00:00+01:00", "condition": "rainy", "temperature": 8.0, "templow": 5.0}}}})
			case "ping":
				send(map[string]any{"id": int(id), "type": "pong"})
			case "call_service":
				send(map[string]any{"id": int(id), "type": "result", "success": true, "result": map[string]any{"context": map[string]any{}}})
			}
		}
	}
}

func writeServerFrame(c net.Conn, p []byte) {
	h := []byte{0x81}
	switch n := len(p); {
	case n < 126:
		h = append(h, byte(n))
	case n <= 0xFFFF:
		h = append(h, 126, byte(n>>8), byte(n))
	default:
		h = append(h, 127)
		h = binary.BigEndian.AppendUint64(h, uint64(n))
	}
	_, _ = c.Write(append(h, p...))
}

func readClientFrame(r *bufio.Reader, c net.Conn) ([]byte, error) {
	var h [2]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return nil, err
	}
	op := h[0] & 0x0F
	n := uint64(h[1] & 0x7F)
	switch n {
	case 126:
		var b [2]byte
		_, _ = io.ReadFull(r, b[:])
		n = uint64(binary.BigEndian.Uint16(b[:]))
	case 127:
		var b [8]byte
		_, _ = io.ReadFull(r, b[:])
		n = binary.BigEndian.Uint64(b[:])
	}
	mask := make([]byte, 4)
	_, _ = io.ReadFull(r, mask)
	p := make([]byte, n)
	if _, err := io.ReadFull(r, p); err != nil {
		return nil, err
	}
	for i := range p {
		p[i] ^= mask[i%4]
	}
	if op == 8 {
		return nil, io.EOF
	}
	return p, nil
}

func TestClientSession(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	f := &fakeHA{t: t, token: "secret", push: make(chan Entity, 4), states: []Entity{
		{EntityID: "light.a", State: "off", Attributes: map[string]any{}, LastChanged: now, LastUpdated: now},
		{EntityID: "light.ignored", State: "on", Attributes: map[string]any{}, LastChanged: now, LastUpdated: now},
		{EntityID: "weather.v", State: "cloudy", Attributes: map[string]any{"temperature": 9.0}, LastChanged: now, LastUpdated: now},
	}}
	ts := httptest.NewServer(f)
	defer ts.Close()
	c := NewClient(ts.URL, "secret", []string{"light.a", "weather.v"}, "weather.v")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go c.Run(ctx)
	ch, stop := c.Subscribe()
	defer stop()
	deadline := time.After(4 * time.Second)
	for !c.Connected() || len(c.Forecasts("weather.v")) < 2 {
		select {
		case <-deadline:
			t.Fatalf("pas connecté / prévisions incomplètes : connected=%v forecasts=%v", c.Connected(), c.Forecasts("weather.v"))
		case <-ch:
		}
	}
	st := c.Snapshot()
	if len(st) != 2 || st["light.a"].State != "off" {
		t.Fatalf("états filtrés : %v", c.IDs())
	}
	f.push <- Entity{EntityID: "light.a", State: "on", Attributes: map[string]any{"brightness": 255.0}, LastChanged: now, LastUpdated: now}
	for st = c.Snapshot(); st["light.a"].State != "on"; st = c.Snapshot() {
		select {
		case <-deadline:
			t.Fatal("state_changed non reçu")
		case <-ch:
		}
	}
	if err := c.CallService(ctx, "light", "turn_off", map[string]any{"entity_id": "light.a"}); err != nil {
		t.Fatalf("call_service : %v", err)
	}
}

func TestClientBadToken(t *testing.T) {
	f := &fakeHA{t: t, token: "secret", push: make(chan Entity)}
	ts := httptest.NewServer(f)
	defer ts.Close()
	c := NewClient(ts.URL, "wrong", nil, "")
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	err := c.session(ctx)
	if err == nil || !strings.Contains(err.Error(), "refusée") {
		t.Fatalf("erreur d'authentification attendue, %v", err)
	}
}

func TestEntityIDsAndServices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/states":
			_, _ = w.Write([]byte(`[{"entity_id":"calendar.maison","attributes":{"friendly_name":"Maison"}},{"entity_id":"light.x","attributes":{}}]`))
		case "/api/services":
			_, _ = w.Write([]byte(`[{"domain":"light","services":{"turn_on":{},"turn_off":{}}},{"domain":"tado","services":{"set_climate_timer":{}}}]`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "tok", nil, "")
	ids, err := c.EntityIDs(context.Background())
	if err != nil || len(ids) != 2 || ids["calendar.maison"] != "Maison" {
		t.Fatalf("états : %v %v", ids, err)
	}
	svc, err := c.Services(context.Background())
	if err != nil || !svc["light.turn_off"] || !svc["tado.set_climate_timer"] || svc["cover.open_cover"] {
		t.Fatalf("services : %v %v", svc, err)
	}
}

// The handshake deadline must not stay on the connection: with a 300 ms dial
// timeout, the session must still be the same one and receive events 1 s later.
// (Bug seen on the real HA: "read … i/o timeout" 15 s after each connection.)
func TestSessionOutlivesDialTimeout(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	f := &fakeHA{t: t, token: "secret", push: make(chan Entity, 4), states: []Entity{{EntityID: "light.a", State: "off", Attributes: map[string]any{}, LastChanged: now, LastUpdated: now}}}
	ts := httptest.NewServer(f)
	defer ts.Close()
	c := NewClient(ts.URL, "secret", []string{"light.a"}, "")
	c.DialTimeout = 300 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go c.Run(ctx)
	ch, stop := c.Subscribe()
	defer stop()
	for !c.Connected() {
		select {
		case <-ctx.Done():
			t.Fatal("pas connecté")
		case <-ch:
		}
	}
	drops := 0
	go func() {
		for range ch {
			if !c.Connected() {
				drops++
			}
		}
	}()
	time.Sleep(time.Second)
	f.push <- Entity{EntityID: "light.a", State: "on", Attributes: map[string]any{}, LastChanged: now, LastUpdated: now}
	deadline := time.Now().Add(2 * time.Second)
	for c.Snapshot()["light.a"].State != "on" {
		if time.Now().After(deadline) {
			t.Fatal("événement non reçu après le délai de connexion : la connexion a expiré")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if drops > 0 {
		t.Fatalf("%d déconnexion(s) pendant la session", drops)
	}
}

// A tap during a reconnection waits for HA instead of failing.
func TestCallServiceAcrossReconnect(t *testing.T) {
	f := &fakeHA{t: t, token: "secret", push: make(chan Entity), kick: make(chan struct{}, 1)}
	ts := httptest.NewServer(f)
	defer ts.Close()
	c := NewClient(ts.URL, "secret", nil, "")
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	go c.Run(ctx)
	for !c.Connected() {
		select {
		case <-ctx.Done():
			t.Fatal("pas connecté")
		case <-time.After(20 * time.Millisecond):
		}
	}
	f.kick <- struct{}{} // the server drops the connection
	for c.Connected() {
		time.Sleep(10 * time.Millisecond)
	}
	if err := c.CallService(ctx, "light", "turn_on", map[string]any{"entity_id": "light.a"}); err != nil {
		t.Fatalf("commande pendant la reconnexion : %v", err)
	}
}
