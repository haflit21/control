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

func TestCalendarEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/calendars/calendar.famille" || r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("requête : %s %s", r.URL.Path, r.Header.Get("Authorization"))
		}
		if r.URL.Query().Get("start") == "" || r.URL.Query().Get("end") == "" {
			t.Errorf("start/end manquants : %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`[{"uid":"a","summary":"Pédiatre","location":"Lyon","start":{"dateTime":"2026-11-12T09:30:00+01:00"},"end":{"dateTime":"2026-11-12T10:00:00+01:00"}},{"uid":"b","summary":"Anniversaire","start":{"date":"2026-11-13"},"end":{"date":"2026-11-14"}},{"summary":"cassé","start":{}}]`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "tok", []string{"calendar.famille"}, "")
	loc, _ := time.LoadLocation("Europe/Paris")
	from := time.Date(2026, 11, 9, 0, 0, 0, 0, loc)
	evs, err := c.CalendarEvents(context.Background(), "calendar.famille", from, from.AddDate(0, 0, 7))
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 2 {
		t.Fatalf("%d événements : %+v", len(evs), evs)
	}
	if evs[0].Summary != "Pédiatre" || evs[0].AllDay || evs[0].Start.Hour() != 9 || evs[0].Start.Location() != loc {
		t.Errorf("événement 1 : %+v", evs[0])
	}
	if !evs[1].AllDay || evs[1].Start.Day() != 13 || evs[1].End.Day() != 14 {
		t.Errorf("événement 2 : %+v", evs[1])
	}
}
