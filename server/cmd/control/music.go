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
	"ahomelab.io/control/internal/ha"
	"ahomelab.io/control/internal/mass"
	"ahomelab.io/control/internal/model"
	"ahomelab.io/control/internal/server"
	"ahomelab.io/control/internal/spotify"
)

// musicFrom: the music search of the Musique tab, by media.music.provider —
// Music Assistant through HA (needs the real HA client), the Spotify Web API
// (needs the SPOTIFY_* secrets), or the demo catalogue in demo mode.
func musicFrom(cfg *config.Config, demoMode bool, src ha.Source) server.Music {
	p := cfg.Media.Music.Provider
	if p == "" {
		return nil
	}
	if demoMode {
		log.Printf("musique : catalogue de démo (%s simulé)", p)
		d, _ := src.(*demo.Source)
		return demo.Music{Src: d}
	}
	switch p {
	case "music_assistant":
		c, ok := src.(*ha.Client)
		if !ok {
			return nil
		}
		log.Printf("musique : Music Assistant par Home Assistant")
		// a Music Assistant player to browse albums with: the first speaker's
		player := ""
		for _, r := range cfg.Rooms {
			for _, d := range r.Devices {
				if player == "" && d.Entities["music"] != "" {
					player = d.Entities["music"]
				}
			}
		}
		return server.MassMusic{C: &mass.Client{HA: c, EntryID: cfg.Media.Music.ConfigEntryID}, Player: player}
	case "spotify":
		id, secret, rt := os.Getenv("SPOTIFY_CLIENT_ID"), os.Getenv("SPOTIFY_CLIENT_SECRET"), os.Getenv("SPOTIFY_REFRESH_TOKEN")
		if id == "" || secret == "" || rt == "" {
			log.Printf("musique : Spotify désactivé (SPOTIFY_CLIENT_ID, SPOTIFY_CLIENT_SECRET et SPOTIFY_REFRESH_TOKEN requis ; make spotify-login)")
			return nil
		}
		log.Printf("musique : Spotify (marché %s)", cfg.Media.Music.Market)
		return server.SpotifyMusic{C: &spotify.Client{ClientID: id, ClientSecret: secret, RefreshToken: rt, Market: cfg.Media.Music.Market,
			OnRefreshToken: func(string) {
				log.Printf("musique : Spotify a renouvelé le jeton de renouvellement ; s'il expire au redémarrage, relancer make spotify-login")
			}}}
	}
	return nil
}

// checkMedia says whether the radios and the music search can work (never prints a secret).
func checkMedia(ctx context.Context, cfg *config.Config, c *ha.Client) {
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
	speakers := func(each func(room config.Room, d config.Device)) {
		for _, r := range cfg.Rooms {
			for _, d := range r.Devices {
				if d.Kind == model.KindMedia {
					each(r, d)
				}
			}
		}
	}
	switch cfg.Media.Music.Provider {
	case "music_assistant":
		id, err := c.ConfigEntryID(ctx, "music_assistant")
		if err != nil {
			fmt.Printf("\nMusic Assistant : %v. Installer le module Music Assistant dans Home Assistant puis ajouter l'intégration.\n", err)
			return
		}
		fmt.Printf("\nMusic Assistant : intégration chargée (%s).\n", id)
		players, err := c.StatesWith(ctx, "mass_player_type")
		if err != nil {
			fmt.Printf("  (lecteurs indisponibles : %v)\n", err)
			return
		}
		fmt.Printf("  Lecteurs Music Assistant :\n")
		for e, name := range players {
			fmt.Printf("  - %s (%s)\n", e, name)
		}
		speakers(func(r config.Room, d config.Device) {
			m := d.Entities["music"]
			if m == "" {
				fmt.Printf("  ! %s · %s : pas de entities.music (le lecteur Music Assistant de l'enceinte)\n", r.Name, d.Name)
			} else if _, ok := players[m]; !ok {
				fmt.Printf("  ! %s · %s : %s n'est pas un lecteur Music Assistant\n", r.Name, d.Name, m)
			}
		})
	case "spotify":
		m, ok := musicFrom(cfg, false, nil).(server.SpotifyMusic)
		if !ok {
			fmt.Printf("\nSpotify : secrets manquants (SPOTIFY_CLIENT_ID, SPOTIFY_CLIENT_SECRET, SPOTIFY_REFRESH_TOKEN) ; make spotify-login.\n")
			return
		}
		devs, err := m.C.Devices(ctx)
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
		speakers(func(r config.Room, d config.Device) {
			name := d.SpotifyDevice
			if name == "" {
				name = r.Name
			}
			if !seen[spotify.Fold(name)] {
				fmt.Printf("  ! « %s » (%s) n'est pas visible : lancer Spotify une fois sur cette enceinte, ou régler spotify_device.\n", name, r.Name)
			}
		})
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
