// Command control serves the house interface: Home Assistant behind, the
// React app in front.
//
//	control serve            start the server (default)
//	control check            load control.yaml, connect to HA, print what is missing
//	control spotify-login    authorize Control on the Spotify account once; prints
//	                         the refresh token on stdout (make spotify-login stores it)
//
// Environment (decrypted from .cloud/secrets/secrets.enc.yaml next to
// control.yaml, or a .env in development): HA_URL, HA_TOKEN,
// CLAUDE_GATEWAY_TOKEN, STT_API_KEY, SPOTIFY_CLIENT_ID, SPOTIFY_CLIENT_SECRET,
// SPOTIFY_REFRESH_TOKEN, CONTROL_CONFIG
// (control.yaml), CONTROL_DEMO=1, CONTROL_LISTEN, CONTROL_MEDIA, CONTROL_ALLOW_FROM (comma
// separated, added to serve.allow_from), CONTROL_TOKEN (login token;
// generated otherwise), CONTROL_STATE_DIR.
package main

import (
	"ahomelab.io/control/internal/air"
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"ahomelab.io/control/internal/agenda"
	"ahomelab.io/control/internal/config"
	"ahomelab.io/control/internal/demo"
	"ahomelab.io/control/internal/favorites"
	"ahomelab.io/control/internal/ha"
	"ahomelab.io/control/internal/journal"
	"ahomelab.io/control/internal/push"
	"ahomelab.io/control/internal/secrets"
	"ahomelab.io/control/internal/server"
	"ahomelab.io/control/internal/voice"
)

func main() {
	log.SetFlags(log.Ltime)
	for _, kv := range os.Environ() { // set by the caller (Makefile, stack): never replaced
		k, _, _ := strings.Cut(kv, "=")
		callerEnv[k] = true
	}
	loadDotEnv(".env")
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	// control.yaml : CONTROL_CONFIG, sinon ./control.yaml, sinon ../control.yaml (lancé depuis server/)
	cfgPath := os.Getenv("CONTROL_CONFIG")
	if cfgPath == "" {
		cfgPath = "control.yaml"
		if _, err := os.Stat(cfgPath); err != nil {
			if _, err := os.Stat(filepath.Join("..", "control.yaml")); err == nil {
				cfgPath = filepath.Join("..", "control.yaml")
			}
		}
	}
	// secrets chiffrés (SOPS) à côté de control.yaml : .cloud/secrets/secrets.enc.yaml,
	// ou CONTROL_SECRETS ; ils priment sur un .env de développement
	loadSecrets(cfgPath)
	// .env à côté de control.yaml aussi (sans écraser ce qui est déjà défini)
	loadDotEnv(filepath.Join(filepath.Dir(cfgPath), ".env"))
	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("configuration : %v", err)
	}
	switch cmd {
	case "serve":
		serve(cfg, cfgPath)
	case "check":
		check(cfg)
	case "spotify-login":
		spotifyLogin()
	default:
		fmt.Fprintf(os.Stderr, "usage : control [serve|check|spotify-login]\n")
		os.Exit(2)
	}
}

func serve(cfg *config.Config, cfgPath string) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	demoMode := os.Getenv("CONTROL_DEMO") == "1"
	var src ha.Source
	if demoMode {
		d := demo.New(cfg)
		go d.Run(ctx)
		src = d
		log.Printf("mode démo : état simulé, rien n'est envoyé à Home Assistant")
	} else {
		url, token := os.Getenv("HA_URL"), os.Getenv("HA_TOKEN")
		if url == "" || token == "" {
			log.Fatalf("HA_URL et HA_TOKEN sont requis (ou CONTROL_DEMO=1)")
		}
		c := ha.NewClient(url, token, cfg.Entities(), cfg.Globals.Weather)
		c.Insecure = os.Getenv("HA_INSECURE") == "1"
		go c.Run(ctx)
		src = c
	}
	token := os.Getenv("CONTROL_TOKEN")
	if token == "" {
		b := make([]byte, 24)
		_, _ = rand.Read(b)
		token = hex.EncodeToString(b)
	}
	srv := server.New(cfg, src, token, demoMode)
	if demoMode {
		srv.Air = demo.Air{}
	} else {
		srv.Air = &air.OpenMeteo{}
	}
	// media_dir from control.yaml is relative to the config file (so `go run` from server/ works);
	// CONTROL_MEDIA is relative to the current directory like any path given on the command line.
	srv.MediaDir = os.Getenv("CONTROL_MEDIA")
	if srv.MediaDir == "" {
		rel := cfg.Serve.MediaDir
		if rel == "" {
			rel = "media"
		}
		if filepath.IsAbs(rel) {
			srv.MediaDir = rel
		} else {
			srv.MediaDir = filepath.Join(filepath.Dir(cfgPath), rel)
		}
	}
	if st, err := os.Stat(srv.MediaDir); err != nil || !st.IsDir() {
		log.Printf("photos : dossier %q introuvable, les pièces s'afficheront sans image", srv.MediaDir)
		srv.MediaDir = ""
	} else {
		log.Printf("photos : %s", srv.MediaDir)
	}
	// notifications push : clés VAPID + abonnements dans state_dir (relatif à control.yaml)
	stateDir := env("CONTROL_STATE_DIR", cfg.Serve.StateDir)
	if !filepath.IsAbs(stateDir) && os.Getenv("CONTROL_STATE_DIR") == "" {
		stateDir = filepath.Join(filepath.Dir(cfgPath), stateDir)
	}
	if ps, err := push.Open(filepath.Join(stateDir, "push"), "mailto:control@localhost"); err != nil {
		log.Printf("notifications : désactivées (%v)", err)
	} else {
		srv.Push = ps
		log.Printf("notifications : %d appareil(s) abonné(s)", ps.Count())
	}
	// agenda : events.json dans state_dir ; en démo, en mémoire avec une semaine type
	if demoMode {
		st, _ := agenda.Open("")
		demo.SeedAgenda(st, cfg.Agenda.CalendarIDs(), time.Now())
		srv.Agenda = st
	} else if st, err := agenda.Open(filepath.Join(stateDir, "agenda")); err != nil {
		log.Printf("agenda : désactivé (%v)", err)
	} else {
		srv.Agenda = st
		log.Printf("agenda : %d événement(s)", st.Count())
	}
	if srv.Agenda != nil {
		srv.Agenda.OnChange = srv.AgendaChanged
	}
	srv.Transcriber, srv.Understander = voiceFrom(cfg)
	srv.Music = musicFrom(cfg, demoMode, src)
	// favoris (titres, albums) : favorites.json dans state_dir ; en démo, en mémoire
	favDir := filepath.Join(stateDir, "favorites")
	if demoMode {
		favDir = ""
	}
	if fs, err := favorites.Open(favDir); err != nil {
		log.Printf("favoris : désactivés (%v)", err)
	} else {
		srv.Favorites = fs
	}
	if js, err := journal.Open(filepath.Join(stateDir, "journal")); err != nil {
		log.Printf("journal : désactivé (%v)", err)
	} else {
		srv.Journal = js
	}
	go srv.Run(ctx)

	listen := env("CONTROL_LISTEN", cfg.Serve.Listen)
	allow := append([]string{}, cfg.Serve.AllowFrom...)
	if v := os.Getenv("CONTROL_ALLOW_FROM"); v != "" {
		allow = append(allow, strings.Split(v, ",")...)
	}
	h, err := server.AllowFrom(srv.Handler(), allow)
	if err != nil {
		log.Fatal(err)
	}
	hs := &http.Server{Addr: listen, Handler: h, ReadHeaderTimeout: 10 * time.Second}
	ln, err := net.Listen("tcp", listen)
	if err != nil {
		log.Fatalf("écoute %s : %v", listen, err)
	}
	host, port, _ := net.SplitHostPort(ln.Addr().String())
	if host == "" || host == "::" || host == "0.0.0.0" {
		host = "localhost"
	}
	log.Printf("control : http://%s/login?token=%s", net.JoinHostPort(host, port), token)
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = hs.Shutdown(sctx)
	}()
	if err := hs.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

// check connects to HA once and lists the configured entities HA does not know.
func check(cfg *config.Config) {
	url, token := os.Getenv("HA_URL"), os.Getenv("HA_TOKEN")
	if url == "" || token == "" {
		log.Fatalf("HA_URL et HA_TOKEN sont requis")
	}
	c := ha.NewClient(url, token, cfg.Entities(), cfg.Globals.Weather)
	c.Insecure = os.Getenv("HA_INSECURE") == "1"
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	go c.Run(ctx)
	ch, stop := c.Subscribe()
	defer stop()
	for !c.Connected() {
		select {
		case <-ctx.Done():
			log.Fatalf("impossible de joindre Home Assistant en 20 s")
		case <-ch:
		}
	}
	time.Sleep(500 * time.Millisecond)
	st := c.Snapshot()
	var missing, unavailable []string
	for _, e := range cfg.Entities() {
		x, ok := st[e]
		switch {
		case !ok:
			missing = append(missing, e)
		case x.State == "unavailable" || x.State == "unknown":
			unavailable = append(unavailable, e)
		}
	}
	fmt.Printf("%d entités configurées, %d connues de Home Assistant\n", len(cfg.Entities()), len(st))
	if len(missing) > 0 {
		fmt.Printf("\nInconnues de HA (à corriger dans control.yaml) :\n")
		for _, e := range missing {
			fmt.Printf("  - %s\n", e)
		}
	}
	if len(unavailable) > 0 {
		fmt.Printf("\nInjoignables en ce moment :\n")
		for _, e := range unavailable {
			fmt.Printf("  - %s\n", e)
		}
	}
	if len(missing) == 0 {
		fmt.Println("\nToutes les entités sont câblées.")
	}

	// what HA has that control.yaml could use, and the services the app calls
	rctx, rcancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer rcancel()
	checkVoice(rctx, cfg)
	checkMedia(rctx, cfg, c)
	if svc, err := c.Services(rctx); err != nil {
		fmt.Printf("\n(liste des services HA indisponible : %v)\n", err)
	} else {
		need := []string{"light.turn_on", "light.turn_off", "climate.set_temperature", "climate.set_hvac_mode", "cover.open_cover", "cover.close_cover", "cover.set_cover_position", "switch.turn_on", "media_player.media_play_pause", "media_player.volume_set", "script.turn_on"}
		if len(cfg.Media.Radios) > 0 {
			need = append(need, "media_player.play_media", "media_player.media_next_track", "media_player.media_previous_track", "media_player.media_seek", "media_player.turn_off")
		}
		if cfg.Media.Music.Provider == "music_assistant" {
			need = append(need, "music_assistant.search", "music_assistant.play_media")
		}
		if cfgUsesTado(cfg) {
			need = append(need, "tado_hijack.set_mode")
		}
		var absent []string
		for _, n := range need {
			if !svc[n] {
				absent = append(absent, n)
			}
		}
		if len(absent) == 0 {
			fmt.Printf("\nServices utilisés par l'app : tous présents (%d).\n", len(need))
		} else {
			fmt.Printf("\nServices absents de HA (les actions correspondantes échoueront) :\n")
			for _, n := range absent {
				fmt.Printf("  - %s\n", n)
			}
		}
	}
}

// cfgUsesTado: heating devices with a Tado Hijack zone (the manual-with-duration
// mode calls tado_hijack.set_mode on it).
func cfgUsesTado(cfg *config.Config) bool {
	for _, r := range cfg.Rooms {
		for _, d := range r.Devices {
			if d.Kind == "heating" && d.Entities["zone_mode"] != "" {
				return true
			}
		}
	}
	return false
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// loadDotEnv reads KEY=VALUE lines (no export, no quotes needed) without
// overriding variables already set.
func loadDotEnv(path string) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), `"'`)
		if os.Getenv(k) == "" {
			_ = os.Setenv(k, v)
		}
	}
}

// loadSecrets decrypts the SOPS file of the configuration into the
// environment (names logged, never values). It replaces what a .env set,
// never what the caller set (CONTROL_TOKEN=dev in the Makefile). Without the file nothing
// happens; a file that cannot be decrypted is reported, not fatal (demo
// mode needs no secret).
var callerEnv = map[string]bool{}

func loadSecrets(cfgPath string) {
	p := os.Getenv("CONTROL_SECRETS")
	if p == "" {
		p = filepath.Join(filepath.Dir(cfgPath), ".cloud", "secrets", "secrets.enc.yaml")
	}
	names, err := secrets.Load(p, os.Getenv("SOPS_BIN"), func(k string) bool { return callerEnv[k] })
	switch {
	case err != nil:
		log.Printf("attention : secrets non déchiffrés (%v)", err)
	case names != nil:
		log.Printf("secrets déchiffrés depuis %s : %s", p, strings.Join(names, ", "))
	}
}

// voiceFrom builds the quick-entry chain from control.yaml (agenda.voice) and .env.
// Swapping Claude for a local model = provider: openai + url + model, nothing else.
func voiceFrom(cfg *config.Config) (voice.Transcriber, voice.Understander) {
	v := cfg.Agenda.Voice
	var tr voice.Transcriber
	var un voice.Understander
	switch v.Understand.Provider {
	case "gateway":
		if os.Getenv("CLAUDE_GATEWAY_TOKEN") == "" {
			log.Printf("agenda : saisie rapide désactivée, CLAUDE_GATEWAY_TOKEN absent des secrets")
		} else {
			un = &voice.Gateway{URL: v.Understand.URL, Token: os.Getenv("CLAUDE_GATEWAY_TOKEN"), Profile: v.Understand.Profile}
			log.Printf("agenda : phrases comprises par Claude via %s (profil %s)", v.Understand.URL, v.Understand.Profile)
		}
	case "anthropic":
		if os.Getenv("ANTHROPIC_API_KEY") == "" {
			log.Printf("agenda : saisie rapide désactivée, ANTHROPIC_API_KEY absente de .env")
		} else {
			model := v.Understand.Model
			if model == "" {
				model = "claude-haiku-5-5"
			}
			un = &voice.Anthropic{APIKey: os.Getenv("ANTHROPIC_API_KEY"), Model: model, URL: v.Understand.URL}
			log.Printf("agenda : phrases comprises par Claude (%s)", model)
		}
	case "openai":
		un = &voice.OpenAIChat{URL: v.Understand.URL, Model: v.Understand.Model, APIKey: os.Getenv("LLM_API_KEY")}
		log.Printf("agenda : phrases comprises par %s sur %s", v.Understand.Model, v.Understand.URL)
	}
	if v.Transcribe.URL != "" {
		model := v.Transcribe.Model
		if model == "" {
			model = "parakeet-tdt-0.6b-v3"
		}
		tr = &voice.OpenAITranscriber{URL: v.Transcribe.URL, Model: model, Language: v.Transcribe.Language, APIKey: os.Getenv("STT_API_KEY")}
		log.Printf("agenda : voix transcrite par %s (%s)", v.Transcribe.URL, model)
	}
	return tr, un
}

// checkVoice says whether the quick entry can work (never prints a key).
func checkVoice(ctx context.Context, cfg *config.Config) {
	v := cfg.Agenda.Voice
	fmt.Printf("\nAgenda : %d calendrier(s) (%s)\n", len(cfg.Agenda.Calendars), strings.Join(cfg.Agenda.CalendarIDs(), ", "))
	switch v.Understand.Provider {
	case "":
		fmt.Println("  saisie rapide : non configurée (agenda.voice.understand)")
	case "gateway":
		if os.Getenv("CLAUDE_GATEWAY_TOKEN") == "" {
			fmt.Println("  saisie rapide : claude-gateway, mais CLAUDE_GATEWAY_TOKEN manque dans les secrets")
		} else {
			fmt.Printf("  saisie rapide : claude-gateway %s, profil %s, jeton présent\n", v.Understand.URL, v.Understand.Profile)
		}
	case "anthropic":
		if os.Getenv("ANTHROPIC_API_KEY") == "" {
			fmt.Println("  saisie rapide : Claude, mais ANTHROPIC_API_KEY manque dans .env")
		} else {
			fmt.Println("  saisie rapide : Claude, clé présente")
		}
	case "openai":
		fmt.Printf("  saisie rapide : %s sur %s\n", v.Understand.Model, v.Understand.URL)
	}
	if v.Transcribe.URL == "" {
		fmt.Println("  micro : non configuré (agenda.voice.transcribe)")
		return
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(v.Transcribe.URL, "/")+"/health", nil)
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		fmt.Printf("  micro : service stt injoignable sur %s (%v)\n", v.Transcribe.URL, err)
		return
	}
	resp.Body.Close()
	fmt.Printf("  micro : service stt sur %s : HTTP %d (200 = modèle prêt)\n", v.Transcribe.URL, resp.StatusCode)
}
