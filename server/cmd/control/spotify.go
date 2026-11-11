package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"ahomelab.io/control/internal/config"
	"ahomelab.io/control/internal/demo"
	"ahomelab.io/control/internal/server"
	"ahomelab.io/control/internal/spotify"
)

// musicFrom: Spotify when media.spotify.enabled and the three secrets are
// there; the demo catalogue in demo mode.
func musicFrom(cfg *config.Config, demoMode bool) server.Music {
	if !cfg.Media.Spotify.Enabled {
		return nil
	}
	if demoMode {
		log.Printf("musique : catalogue de démo (Spotify simulé)")
		return demo.Music{}
	}
	id, secret, rt := os.Getenv("SPOTIFY_CLIENT_ID"), os.Getenv("SPOTIFY_CLIENT_SECRET"), os.Getenv("SPOTIFY_REFRESH_TOKEN")
	if id == "" || secret == "" || rt == "" {
		log.Printf("musique : Spotify désactivé (SPOTIFY_CLIENT_ID, SPOTIFY_CLIENT_SECRET et SPOTIFY_REFRESH_TOKEN requis ; make spotify-login)")
		return nil
	}
	log.Printf("musique : Spotify (marché %s)", cfg.Media.Spotify.Market)
	return &spotify.Client{ClientID: id, ClientSecret: secret, RefreshToken: rt, Market: cfg.Media.Spotify.Market,
		OnRefreshToken: func(string) {
			log.Printf("musique : Spotify a renouvelé le jeton de renouvellement ; s'il expire au redémarrage, relancer make spotify-login")
		}}
}

// checkMedia says whether the radios and Spotify can work (never prints a secret).
func checkMedia(ctx context.Context, cfg *config.Config) {
	if len(cfg.Media.Radios) > 0 {
		fmt.Printf("\nRadios : %d configurées (lancées par media_player.play_media).\n", len(cfg.Media.Radios))
		hc := &http.Client{Timeout: 6 * time.Second}
		for _, r := range cfg.Media.Radios {
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, r.URL, nil)
			req.Header.Set("Range", "bytes=0-1023")
			res, err := hc.Do(req)
			if err != nil {
				fmt.Printf("  - %s : injoignable (%v)\n", r.Name, err)
				continue
			}
			_ = res.Body.Close()
			if res.StatusCode >= 400 {
				fmt.Printf("  - %s : HTTP %d\n", r.Name, res.StatusCode)
			} else {
				fmt.Printf("  - %s : OK (%s)\n", r.Name, res.Header.Get("Content-Type"))
			}
		}
	}
	if !cfg.Media.Spotify.Enabled {
		return
	}
	m := musicFrom(cfg, false)
	c, ok := m.(*spotify.Client)
	if !ok {
		fmt.Printf("\nSpotify : secrets manquants (SPOTIFY_CLIENT_ID, SPOTIFY_CLIENT_SECRET, SPOTIFY_REFRESH_TOKEN) ; make spotify-login.\n")
		return
	}
	devs, err := c.Devices(ctx)
	if err != nil {
		fmt.Printf("\nSpotify : %v\n", err)
		return
	}
	fmt.Printf("\nSpotify : connecté, %d enceinte(s) visible(s).\n", len(devs))
	seen := map[string]bool{}
	for _, d := range devs {
		seen[spotify.Fold(d.Name)] = true
		fmt.Printf("  - %s (%s)\n", d.Name, d.Type)
	}
	for _, r := range cfg.Rooms {
		for _, d := range r.Devices {
			if d.Entities["media_player"] == "" {
				continue
			}
			name := d.SpotifyDevice
			if name == "" {
				name = r.Name
			}
			if !seen[spotify.Fold(name)] {
				fmt.Printf("  ! « %s » (%s) n'est pas visible : lancer Spotify une fois sur cette enceinte, ou régler spotify_device.\n", name, r.Name)
			}
		}
	}
}

// spotifyLogin runs the authorization once: the owner opens the printed
// link, allows Control, Spotify redirects to 127.0.0.1:8888 and the refresh
// token is printed on stdout only (everything else goes to stderr).
func spotifyLogin() {
	id, secret := os.Getenv("SPOTIFY_CLIENT_ID"), os.Getenv("SPOTIFY_CLIENT_SECRET")
	if id == "" || secret == "" {
		log.Fatalf("SPOTIFY_CLIENT_ID et SPOTIFY_CLIENT_SECRET sont requis (make secret-set NAME=…)")
	}
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	state := hex.EncodeToString(b)
	ln, err := net.Listen("tcp", "127.0.0.1:8888")
	if err != nil {
		log.Fatalf("écoute 127.0.0.1:8888 : %v", err)
	}
	type result struct {
		token string
		err   error
	}
	done := make(chan result, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("state") != state {
			http.Error(w, "état inattendu", http.StatusBadRequest)
			return
		}
		if e := q.Get("error"); e != "" {
			http.Error(w, "Autorisation refusée : "+e, http.StatusForbidden)
			done <- result{err: errors.New("autorisation refusée : " + e)}
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		tok, err := spotify.Exchange(ctx, nil, "", id, secret, q.Get("code"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			done <- result{err: err}
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, "<!doctype html><meta charset=utf-8><title>Control</title><p style=\"font:16px system-ui;padding:2em\">Control est autorisé sur Spotify. Tu peux fermer cette page.</p>")
		done <- result{token: tok}
	})
	hs := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = hs.Serve(ln) }()
	fmt.Fprintf(os.Stderr, "Ouvre ce lien sur ce Mac, connecte-toi au compte Spotify Premium et autorise Control :\n\n  %s\n\n(en attente, 5 min au plus)\n", spotify.AuthorizeURL(id, state))
	var res result
	select {
	case res = <-done:
	case <-time.After(5 * time.Minute):
		res.err = errors.New("délai dépassé")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = hs.Shutdown(ctx)
	if res.err != nil {
		log.Fatalf("spotify : %v", res.err)
	}
	fmt.Print(res.token)
}
