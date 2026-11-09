// Package server is Maison's HTTP side: the JSON API, the SSE stream and
// the static interface (apps/web/dist, embedded at build time).
//
// Security model (same as Sentinel): reachable only from the house network
// and the VPN (AllowFrom), a login token printed at startup exchanged once
// for an HttpOnly SameSite=Strict cookie, and the X-Maison header on every
// mutating call. The HA token never leaves the process.
package server

import (
	"ahomelab.io/maison/internal/air"
	"context"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"ahomelab.io/maison/internal/agenda"
	"ahomelab.io/maison/internal/build"
	"ahomelab.io/maison/internal/config"
	"ahomelab.io/maison/internal/ha"
	"ahomelab.io/maison/internal/journal"
	"ahomelab.io/maison/internal/model"
	"ahomelab.io/maison/internal/push"
	"ahomelab.io/maison/internal/voice"
)

//go:embed all:ui/dist
var uiFS embed.FS

const cookieName = "maison_session"

type Server struct {
	Cfg   *config.Config
	Src   ha.Source
	Token string
	Demo  bool
	Loc   *time.Location
	// Debounce: how long to wait for a burst of HA changes to settle.
	Debounce time.Duration
	// MediaDir: directory of photos served under /media/ (rooms, devices). Empty disables the route.
	MediaDir string
	// Air fetches outdoor air quality (nil disables).
	Air air.Fetcher
	// Push sends web push notifications to subscribed devices (nil disables).
	Push *push.Store
	// Journal records every action (nil disables).
	Journal *journal.Store
	// Agenda is the family calendar (nil disables the Agenda tab's data).
	Agenda *agenda.Store
	// Transcriber / Understander: voice and typed quick entry (nil = not offered).
	Transcriber  voice.Transcriber
	Understander voice.Understander
	agendaKick   chan struct{}

	air    *model.Air
	agenda *model.Agenda
	push   *pushState

	mu      sync.RWMutex
	home    model.Home
	devices map[string]model.Device // last published, by id
	clients map[chan model.Event]struct{}
	ui      fs.FS
}

func New(cfg *config.Config, src ha.Source, token string, demo bool) *Server {
	s := &Server{Cfg: cfg, Src: src, Token: token, Demo: demo, Loc: time.Local, Debounce: 150 * time.Millisecond, clients: map[chan model.Event]struct{}{}}
	if sub, err := fs.Sub(uiFS, "ui/dist"); err == nil {
		s.ui = sub
	}
	s.rebuild(nil)
	return s
}

// Run follows the source and publishes events until ctx ends.
func (s *Server) Run(ctx context.Context) {
	ch, cancel := s.Src.Subscribe()
	defer cancel()
	go s.pollAir(ctx)
	go s.runAgenda(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ch:
			// let the burst settle, then rebuild once
			t := time.NewTimer(s.Debounce)
		drain:
			for {
				select {
				case <-ch:
				case <-t.C:
					break drain
				case <-ctx.Done():
					t.Stop()
					return
				}
			}
			s.rebuild(s.publish)
		}
	}
}

// pollAir refreshes the air quality every 30 min (every minute while it fails).
// Coordinates come from maison.yaml or HA's zone.home.
func (s *Server) pollAir(ctx context.Context) {
	if s.Air == nil || !s.Cfg.Globals.AirEnabled() {
		return
	}
	wait := 2 * time.Second
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		lat, lon := s.Cfg.Globals.Air.Latitude, s.Cfg.Globals.Air.Longitude
		if lat == 0 || lon == 0 {
			if z, ok := s.Src.Snapshot()["zone.home"]; ok {
				lat, _ = z.Attributes["latitude"].(float64)
				lon, _ = z.Attributes["longitude"].(float64)
			}
		}
		if lat == 0 || lon == 0 {
			wait = time.Minute // zone.home not known yet
			continue
		}
		fctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		a, err := s.Air.Fetch(fctx, lat, lon)
		cancel()
		if err != nil {
			log.Printf("qualité de l'air : %v", err)
			s.mu.Lock()
			if s.air != nil {
				s.air.Available = false
			}
			s.mu.Unlock()
			wait = time.Minute
		} else {
			a.Place = s.Cfg.Globals.Air.Place
			s.mu.Lock()
			s.air = &a
			s.mu.Unlock()
			wait = 30 * time.Minute
		}
		s.rebuild(s.publish)
		s.publishHome()
	}
}

// pollAgenda refreshes the shared calendars every 10 min: from the Monday of
// the current week to five weeks later.
// publishHome sends the whole house to every client (after air / agenda changes,
// which the device/summary diff does not carry).
func (s *Server) publishHome() {
	h := s.Home()
	s.publish([]model.Event{{Type: "home", Home: &h, At: time.Now().In(s.Loc)}})
}

// rebuild recomputes the house and hands the changes to emit (nil at start).
func (s *Server) rebuild(emit func([]model.Event)) {
	now := time.Now().In(s.Loc)
	weather := s.Cfg.Globals.Weather
	s.mu.RLock()
	air, agenda := s.air, s.agenda
	s.mu.RUnlock()
	home := build.Build(build.Input{Config: s.Cfg, States: s.Src.Snapshot(), Forecasts: s.Src.Forecasts(weather), Now: now, Connected: s.Src.Connected(), Demo: s.Demo, Air: air, Agenda: agenda})
	s.mu.Lock()
	prev := s.devices
	prevHome := s.home
	s.home = home
	s.devices = map[string]model.Device{}
	for _, r := range home.Rooms {
		for _, d := range r.Devices {
			s.devices[d.ID] = d
		}
	}
	s.mu.Unlock()
	s.notifyAlerts(prevHome, home)
	if emit == nil {
		return
	}
	var events []model.Event
	for id, d := range s.devices {
		if p, ok := prev[id]; !ok || !reflect.DeepEqual(p, d) {
			dd := d
			events = append(events, model.Event{Type: "device", Device: &dd, At: now})
		}
	}
	if prevHome.Connected != home.Connected {
		msg := "Connexion à Home Assistant rétablie"
		if !home.Connected {
			msg = "Connexion à Home Assistant perdue"
		}
		events = append(events, model.Event{Type: "status", Status: &model.Status{Connected: home.Connected, Message: msg}, At: now})
	}
	sum := summaryOf(home)
	if len(events) > 0 || !reflect.DeepEqual(summaryOf(prevHome), sum) {
		events = append(events, model.Event{Type: "summary", Summary: &sum, At: now})
	}
	if len(events) > 0 {
		emit(events)
	}
}

func summaryOf(h model.Home) model.Summary {
	rooms := map[string]model.RoomSummary{}
	for _, r := range h.Rooms {
		rooms[r.ID] = r.Summary
	}
	return model.Summary{Rooms: rooms, Globals: h.Globals, Presence: h.Presence, Alerts: h.Alerts, Weather: h.Weather}
}

func (s *Server) publish(events []model.Event) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for ch := range s.clients {
		for _, e := range events {
			select {
			case ch <- e:
			default:
				// a client that does not drain misses events; it will refetch /api/home on reconnect
			}
		}
	}
}

func (s *Server) Home() model.Home {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.home
}

// Handler builds the mux: UI, API, SSE, cameras.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /login", s.login)
	mux.HandleFunc("GET /api/home", s.auth(s.apiHome))
	mux.HandleFunc("GET /api/events", s.auth(s.events))
	mux.HandleFunc("GET /api/cameras/{id}/snapshot", s.auth(s.cameraSnapshot))
	mux.HandleFunc("GET /api/cameras/{id}/stream", s.auth(s.cameraStream))
	mux.HandleFunc("GET /media/{path...}", s.auth(s.media))
	mux.HandleFunc("POST /api/devices/{id}/actions", s.auth(s.mutating(s.deviceAction)))
	mux.HandleFunc("POST /api/home/heating", s.auth(s.mutating(s.houseHeating)))
	mux.HandleFunc("POST /api/home/lights/off", s.auth(s.mutating(s.lightsOff)))
	mux.HandleFunc("POST /api/security/alarm", s.auth(s.mutating(s.alarm)))
	mux.HandleFunc("POST /api/security/alarm/ack", s.auth(s.mutating(s.alarmAck)))
	mux.HandleFunc("GET /api/journal", s.auth(s.apiJournal))
	mux.HandleFunc("GET /api/agenda/events/{id}", s.auth(s.agendaGet))
	mux.HandleFunc("POST /api/agenda/events", s.auth(s.mutating(s.agendaCreate)))
	mux.HandleFunc("PUT /api/agenda/events/{id}", s.auth(s.mutating(s.agendaUpdate)))
	mux.HandleFunc("DELETE /api/agenda/events/{id}", s.auth(s.mutating(s.agendaDelete)))
	mux.HandleFunc("POST /api/agenda/parse", s.auth(s.mutating(s.agendaParse)))
	mux.HandleFunc("POST /api/agenda/voice", s.auth(s.mutating(s.agendaVoice)))
	mux.HandleFunc("GET /api/push/key", s.auth(s.pushKey))
	mux.HandleFunc("POST /api/push/subscribe", s.auth(s.mutating(s.pushSubscribe)))
	mux.HandleFunc("POST /api/push/unsubscribe", s.auth(s.mutating(s.pushUnsubscribe)))
	mux.HandleFunc("POST /api/push/test", s.auth(s.mutating(s.pushTest)))
	mux.HandleFunc("/", s.static)
	return mux
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if subtle.ConstantTimeCompare([]byte(token), []byte(s.Token)) != 1 {
		http.Error(w, "Maison : lien de connexion invalide (jeton incomplet ou ancien). Reprends la ligne « maison : …/login?token=… » des journaux du conteneur.", http.StatusUnauthorized)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: s.Token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 30 * 24 * 3600})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	// the page navigates itself: a redirect from another app would not carry a SameSite=Strict cookie (Safari)
	_, _ = w.Write([]byte(`<!doctype html><meta charset="utf-8"><meta name="viewport" content="width=device-width"><meta http-equiv="refresh" content="1;url=/"><title>Maison</title>` +
		`<body style="font:16px system-ui;background:#0d1017;color:#f3f0ea;display:grid;place-items:center;height:90vh"><p>Connecté. <a href="/" style="color:#ec9b74">Ouvrir Maison</a></p><script>location.replace("/")</script>`))
}

func (s *Server) authed(r *http.Request) bool {
	c, err := r.Cookie(cookieName)
	return err == nil && subtle.ConstantTimeCompare([]byte(c.Value), []byte(s.Token)) == 1
}

func (s *Server) auth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.authed(r) {
			fail(w, http.StatusUnauthorized, "Non connecté", "ouvre le lien de connexion affiché au démarrage de maison")
			return
		}
		h(w, r)
	}
}

func (s *Server) mutating(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Maison") != "1" {
			fail(w, http.StatusForbidden, "En-tête X-Maison manquant", "")
			return
		}
		if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" {
			fail(w, http.StatusForbidden, "Requête cross-site refusée", "")
			return
		}
		h(w, r)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, msg, detail string) {
	writeJSON(w, code, map[string]string{"error": msg, "detail": detail})
}

func (s *Server) apiHome(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Home())
}

// events is the SSE stream: a "home" event first, then device / summary /
// status events, and a comment every 25 s to keep the connection alive.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		fail(w, http.StatusInternalServerError, "SSE non supporté", "")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	ch := make(chan model.Event, 128)
	s.mu.Lock()
	s.clients[ch] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.clients, ch)
		s.mu.Unlock()
	}()
	home := s.Home()
	writeEvent(w, model.Event{Type: "home", Home: &home, At: time.Now()})
	fl.Flush()
	keep := time.NewTicker(25 * time.Second)
	defer keep.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-keep.C:
			_, _ = fmt.Fprint(w, ": keep-alive\n\n")
			fl.Flush()
		case e := <-ch:
			writeEvent(w, e)
			fl.Flush()
		}
	}
}

func writeEvent(w http.ResponseWriter, e model.Event) {
	b, _ := json.Marshal(e)
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.Type, b)
}

// cameraEntity resolves "<room>.<device>" to its HA camera entity.
func (s *Server) cameraEntity(id string) (string, bool) {
	for _, r := range s.Cfg.Rooms {
		for _, d := range r.Devices {
			if r.ID+"."+d.ID == id && d.Kind == model.KindCamera {
				return d.Entities["camera"], true
			}
		}
	}
	return "", false
}

func (s *Server) cameraSnapshot(w http.ResponseWriter, r *http.Request) {
	entity, ok := s.cameraEntity(r.PathValue("id"))
	if !ok {
		fail(w, http.StatusNotFound, "Caméra inconnue", r.PathValue("id"))
		return
	}
	// demo: the room photo stands in for the camera, so the preview looks like the real thing
	if s.Demo && s.MediaDir != "" {
		if rel, ok := s.demoSnapshot(r.PathValue("id")); ok {
			r2 := r.Clone(r.Context())
			r2.SetPathValue("path", rel)
			w.Header().Set("Cache-Control", "no-store")
			s.media(w, r2)
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	body, ctype, err := s.Src.CameraSnapshot(ctx, entity)
	if err != nil {
		fail(w, http.StatusBadGateway, "Image indisponible", err.Error())
		return
	}
	if ctype == "" {
		ctype = "image/jpeg"
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if strings.HasPrefix(ctype, "image/svg") {
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	}
	_, _ = w.Write(body)
}

// demoSnapshot returns the media_dir-relative photo of the room a demo camera belongs to.
func (s *Server) demoSnapshot(deviceID string) (string, bool) {
	roomID, _, ok := strings.Cut(deviceID, ".")
	if !ok {
		return "", false
	}
	for _, rc := range s.Cfg.Rooms {
		if rc.ID == roomID && rc.Image != "" {
			return rc.Image, true
		}
	}
	return "", false
}

func (s *Server) cameraStream(w http.ResponseWriter, r *http.Request) {
	entity, ok := s.cameraEntity(r.PathValue("id"))
	if !ok {
		fail(w, http.StatusNotFound, "Caméra inconnue", r.PathValue("id"))
		return
	}
	err := s.Src.CameraStream(r.Context(), entity, w, func(ctype string) {
		if ctype == "" {
			ctype = "multipart/x-mixed-replace"
		}
		w.Header().Set("Content-Type", ctype)
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
	})
	if err != nil && r.Context().Err() == nil {
		// headers may already be sent; best effort
		log.Printf("caméra %s : flux interrompu : %v", entity, err)
	}
}

// static serves the embedded interface; unknown paths fall back to
// media serves the photos of rooms and devices from MediaDir (maison.yaml
// media_dir, MAISON_MEDIA). Only plain image files, no directory listing;
// the session is required like for the rest of the UI.
func (s *Server) media(w http.ResponseWriter, r *http.Request) {
	rel := strings.TrimPrefix(path.Clean("/"+r.PathValue("path")), "/")
	if s.MediaDir == "" || rel == "" || rel == "." {
		http.NotFound(w, r)
		return
	}
	switch strings.ToLower(path.Ext(rel)) {
	case ".jpg", ".jpeg", ".png", ".webp", ".avif", ".svg":
	default:
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(filepath.Join(s.MediaDir, filepath.FromSlash(rel)))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if path.Ext(rel) == ".svg" {
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	}
	http.ServeContent(w, r, path.Base(rel), st.ModTime(), f)
}

// index.html (client-side routing). The UI itself needs the session: an
// unauthenticated visitor gets a short page explaining how to log in.
func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	if s.ui == nil {
		http.Error(w, "Maison : interface non construite (pnpm build)", http.StatusServiceUnavailable)
		return
	}
	p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if p == "" {
		p = "index.html"
	}
	// manifest and icons are fetched by iOS without the cookie
	public := p == "manifest.webmanifest" || strings.HasPrefix(p, "icons/") || p == "favicon.svg"
	if !public && !s.authed(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`<!doctype html><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>Maison</title><body style="font:16px system-ui;background:#0d1017;color:#f3f0ea;display:grid;place-items:center;height:90vh;text-align:center"><p>Maison<br><small style="color:#b0b4bf">Ouvre le lien de connexion affiché au démarrage du serveur (…/login?token=…).</small></p>`))
		return
	}
	f, err := s.ui.Open(p)
	if err != nil {
		if strings.Contains(path.Base(p), ".") {
			http.NotFound(w, r)
			return
		}
		p = "index.html"
		f, err = s.ui.Open(p)
		if err != nil {
			http.Error(w, "Maison : interface non construite (pnpm build)", http.StatusServiceUnavailable)
			return
		}
	}
	defer f.Close()
	st, _ := f.Stat()
	if st != nil && st.IsDir() {
		http.NotFound(w, r)
		return
	}
	if p == "index.html" || p == "sw.js" || p == "manifest.webmanifest" {
		w.Header().Set("Cache-Control", "no-store")
	} else if strings.HasPrefix(p, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	if p == "index.html" {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; font-src 'self'; connect-src 'self'; manifest-src 'self'")
	}
	rs, ok := f.(interface {
		Read([]byte) (int, error)
		Seek(int64, int) (int64, error)
	})
	if !ok {
		http.NotFound(w, r)
		return
	}
	http.ServeContent(w, r, p, time.Time{}, rs)
}

// AllowFrom refuses clients outside nets before anything else (login
// included). Needed on a macvlan address where no host firewall applies.
func AllowFrom(h http.Handler, cidrs []string) (http.Handler, error) {
	if len(cidrs) == 0 {
		return h, nil
	}
	var nets []*net.IPNet
	for _, c := range cidrs {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if !strings.Contains(c, "/") {
			if strings.Contains(c, ":") {
				c += "/128"
			} else {
				c += "/32"
			}
		}
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			return nil, fmt.Errorf("serve.allow_from : %q n'est pas un réseau valide", c)
		}
		nets = append(nets, n)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		ip := net.ParseIP(host)
		if err == nil && ip != nil {
			for _, n := range nets {
				if n.Contains(ip) {
					h.ServeHTTP(w, r)
					return
				}
			}
		}
		log.Printf("accès refusé à %s (hors de serve.allow_from)", host)
		http.Error(w, "Maison : cet appareil n'est pas autorisé (serve.allow_from). Adresse vue : "+host, http.StatusForbidden)
	}), nil
}

var ErrNoUI = errors.New("interface non construite")
