package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ahomelab.io/control/internal/journal"
	"ahomelab.io/control/internal/model"
	"ahomelab.io/control/internal/push"
)

// Push notifications: the browser subscribes (service worker), the server
// sends a notification when a new warn/crit alert appears (not batteries) and
// when the alarm is triggered. Subscriptions are only ever those of the
// house's own devices (the API is behind the session cookie).

// pushState keeps which alerts were already notified, so a reconnect to HA
// does not re-notify everything.
type pushState struct {
	seen  map[string]struct{}
	ready bool // false until the first rebuild after start: existing alerts are not notified
	// HA lost: notified only if still down after HALostAfter (a reconnect takes seconds)
	haTimer *time.Timer
}

// HALostAfter: how long HA must stay unreachable before the phone is told.
var HALostAfter = time.Minute

func (s *Server) pushKey(w http.ResponseWriter, r *http.Request) {
	if s.Push == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "notifications désactivées"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"key": s.Push.PublicKey(), "count": s.Push.Count()})
}

func (s *Server) pushSubscribe(w http.ResponseWriter, r *http.Request) {
	if s.Push == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "notifications désactivées"})
		return
	}
	var sub push.Subscription
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&sub); err != nil || !strings.HasPrefix(sub.Endpoint, "https://") || sub.Keys.P256dh == "" || sub.Keys.Auth == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "abonnement invalide"})
		return
	}
	sub.Label = shortUA(r.UserAgent())
	s.Push.Add(sub)
	s.record(r, journal.Entry{Scope: "push", Action: "subscribe", Text: "Notifications activées sur " + sub.Label, OK: true})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "count": s.Push.Count()})
}

func (s *Server) pushUnsubscribe(w http.ResponseWriter, r *http.Request) {
	if s.Push == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "notifications désactivées"})
		return
	}
	var body struct {
		Endpoint string `json:"endpoint"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body); err != nil || body.Endpoint == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "endpoint manquant"})
		return
	}
	s.Push.Remove(body.Endpoint)
	s.record(r, journal.Entry{Scope: "push", Action: "unsubscribe", Text: "Notifications désactivées sur " + shortUA(r.UserAgent()), OK: true})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "count": s.Push.Count()})
}

// pushTest sends a notification to every device (button in the bell menu).
func (s *Server) pushTest(w http.ResponseWriter, r *http.Request) {
	if s.Push == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "notifications désactivées"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	n := s.Push.Broadcast(ctx, push.Message{Title: "Control", Body: "Les notifications fonctionnent sur cet appareil.", Tag: "test", URL: "/", Level: "info"})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "sent": n})
}

// notifyAlerts is called after each rebuild with the previous and current house.
func (s *Server) notifyAlerts(prev, cur model.Home) {
	if s.Push == nil {
		return
	}
	if s.push == nil {
		s.push = &pushState{seen: map[string]struct{}{}}
	}
	st := s.push
	if !st.ready {
		// first pass: remember what is already there, notify nothing
		for _, a := range cur.Alerts {
			st.seen[a.ID] = struct{}{}
		}
		st.ready = true
		return
	}
	var msgs []push.Message
	still := map[string]struct{}{}
	for _, a := range cur.Alerts {
		still[a.ID] = struct{}{}
		if _, ok := st.seen[a.ID]; ok {
			continue
		}
		st.seen[a.ID] = struct{}{}
		if a.Level == "info" || a.Kind == "battery" {
			continue
		}
		m := push.Message{Title: a.Title, Body: a.Detail, Tag: "alert:" + a.ID, Level: a.Level, URL: "/"}
		if a.RoomID != "" {
			m.URL = "/pieces/" + a.RoomID
		}
		if a.Kind == "smoke" || a.Kind == "co" || a.Kind == "offline" || a.Kind == "vigilance" {
			m.URL = "/securite"
		}
		msgs = append(msgs, m)
	}
	// forget resolved alerts so they notify again if they come back
	for id := range st.seen {
		if _, ok := still[id]; !ok {
			delete(st.seen, id)
		}
	}
	// alarm: notify when it goes from quiet to triggered
	if cur.Globals.Alarm.Triggered && !prev.Globals.Alarm.Triggered {
		msgs = append(msgs, push.Message{Title: "Alarme déclenchée", Body: "Un capteur ou une caméra a déclenché l’alarme.", Tag: "alarm", Level: "crit", URL: "/securite"})
	}
	if !cur.Connected && prev.Connected && !cur.Demo && st.haTimer == nil {
		st.haTimer = time.AfterFunc(HALostAfter, func() {
			if s.Src.Connected() {
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			n := s.Push.Broadcast(ctx, push.Message{Title: "Home Assistant injoignable", Body: fmt.Sprintf("Control n’a plus de nouvelles de la maison depuis %s.", HALostAfter.Round(time.Second)), Tag: "ha", Level: "warn", URL: "/"})
			log.Printf("push : « Home Assistant injoignable » envoyé à %d appareil(s)", n)
		})
	}
	if cur.Connected && st.haTimer != nil {
		st.haTimer.Stop()
		st.haTimer = nil
	}
	if len(msgs) == 0 {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		for _, m := range msgs {
			n := s.Push.Broadcast(ctx, m)
			log.Printf("push : « %s » envoyé à %d appareil(s)", m.Title, n)
		}
	}()
}

// shortUA keeps a human label out of a User-Agent ("iPhone · Safari", "Mac · Chrome").
func shortUA(ua string) string {
	dev := "Appareil"
	switch {
	case strings.Contains(ua, "iPhone"):
		dev = "iPhone"
	case strings.Contains(ua, "iPad"):
		dev = "iPad"
	case strings.Contains(ua, "Android"):
		dev = "Android"
	case strings.Contains(ua, "Macintosh"):
		dev = "Mac"
	case strings.Contains(ua, "Windows"):
		dev = "PC"
	case strings.Contains(ua, "Linux"):
		dev = "Linux"
	}
	br := "navigateur"
	switch {
	case strings.Contains(ua, "Firefox/"):
		br = "Firefox"
	case strings.Contains(ua, "Edg/"):
		br = "Edge"
	case strings.Contains(ua, "Chrome/"):
		br = "Chrome"
	case strings.Contains(ua, "Safari/"):
		br = "Safari"
	}
	return dev + " · " + br
}

// apiJournal: GET /api/journal?limit=50 — newest first.
func (s *Server) apiJournal(w http.ResponseWriter, r *http.Request) {
	if s.Journal == nil {
		writeJSON(w, http.StatusOK, map[string]any{"entries": []journal.Entry{}, "enabled": false})
		return
	}
	n := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if k, err := strconv.Atoi(v); err == nil && k > 0 && k <= 500 {
			n = k
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": s.Journal.Recent(n), "enabled": true})
}
