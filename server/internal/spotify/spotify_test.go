package spotify

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// fake is a Spotify that knows one account, two devices and one track.
func fake(t *testing.T, premium bool) (*httptest.Server, *int32, *[]string) {
	t.Helper()
	var refreshes int32
	var played []string
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/token", func(w http.ResponseWriter, r *http.Request) {
		u, p, _ := r.BasicAuth()
		if u != "id" || p != "secret" {
			w.WriteHeader(400)
			_, _ = io.WriteString(w, `{"error":"invalid_client"}`)
			return
		}
		_ = r.ParseForm()
		switch r.Form.Get("grant_type") {
		case "refresh_token":
			if r.Form.Get("refresh_token") != "rt" {
				w.WriteHeader(400)
				_, _ = io.WriteString(w, `{"error":"invalid_grant","error_description":"Refresh token revoked"}`)
				return
			}
			atomic.AddInt32(&refreshes, 1)
			_, _ = io.WriteString(w, `{"access_token":"at","expires_in":3600}`)
		case "authorization_code":
			if r.Form.Get("code") != "c0de" || r.Form.Get("redirect_uri") != RedirectURI {
				w.WriteHeader(400)
				_, _ = io.WriteString(w, `{"error":"invalid_grant"}`)
				return
			}
			_, _ = io.WriteString(w, `{"access_token":"at","expires_in":3600,"refresh_token":"rt"}`)
		}
	})
	auth := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("Authorization") != "Bearer at" {
			w.WriteHeader(401)
			_, _ = io.WriteString(w, `{"error":{"status":401,"message":"Invalid access token"}}`)
			return false
		}
		return true
	}
	mux.HandleFunc("GET /v1/search", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		if r.URL.Query().Get("type") != "track" || r.URL.Query().Get("market") != "FR" {
			w.WriteHeader(400)
			return
		}
		_, _ = io.WriteString(w, `{"tracks":{"items":[
			{"uri":"spotify:track:abc","name":"Get Lucky","duration_ms":369000,"artists":[{"name":"Daft Punk"},{"name":"Pharrell Williams"}],
			 "album":{"name":"Random Access Memories","images":[{"url":"https://i.scdn.co/image/640","width":640},{"url":"https://i.scdn.co/image/300","width":300},{"url":"https://i.scdn.co/image/64","width":64}]}},
			{"uri":"spotify:episode:x","name":"Un podcast","artists":[],"album":{"images":[]}}]}}`)
	})
	mux.HandleFunc("GET /v1/me/player/devices", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		_, _ = io.WriteString(w, `{"devices":[{"id":"d1","name":"iPhone","type":"Smartphone"},{"id":"d2","name":"Salon","type":"CastAudio"}]}`)
	})
	mux.HandleFunc("PUT /v1/me/player/play", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		if !premium {
			w.WriteHeader(403)
			_, _ = io.WriteString(w, `{"error":{"status":403,"message":"Player command failed: Premium required","reason":"PREMIUM_REQUIRED"}}`)
			return
		}
		var b struct{ URIs []string }
		_ = json.NewDecoder(r.Body).Decode(&b)
		played = append(played, r.URL.Query().Get("device_id")+" "+strings.Join(b.URIs, ","))
		w.WriteHeader(204)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &refreshes, &played
}

func client(srv *httptest.Server) *Client {
	return &Client{ClientID: "id", ClientSecret: "secret", RefreshToken: "rt", Market: "FR", API: srv.URL + "/v1", Accounts: srv.URL}
}

func TestSearchAndPlay(t *testing.T) {
	srv, refreshes, played := fake(t, true)
	c := client(srv)
	ctx := context.Background()
	tracks, err := c.Search(ctx, "daft punk", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 1 {
		t.Fatalf("épisode non filtré : %+v", tracks)
	}
	tr := tracks[0]
	if tr.Title != "Get Lucky" || tr.Artists != "Daft Punk, Pharrell Williams" || tr.Album != "Random Access Memories" || tr.Duration != 369000 {
		t.Fatalf("titre mal lu : %+v", tr)
	}
	if tr.Cover != "https://i.scdn.co/image/300" {
		t.Fatalf("pochette : %s, veut la plus petite ≥ 96 px", tr.Cover)
	}
	if err := c.Play(ctx, "  SALON ", tr.URI); err != nil {
		t.Fatal(err)
	}
	if len(*played) != 1 || (*played)[0] != "d2 spotify:track:abc" {
		t.Fatalf("lecture : %v", *played)
	}
	if *refreshes != 1 {
		t.Fatalf("le jeton devrait être réutilisé : %d renouvellements", *refreshes)
	}
}

func TestErrors(t *testing.T) {
	srv, _, _ := fake(t, false)
	c := client(srv)
	ctx := context.Background()
	var se *Error
	if err := c.Play(ctx, "Salon", "spotify:track:abc"); !errors.As(err, &se) || se.Reason != "PREMIUM_REQUIRED" || !strings.Contains(se.Message, "Premium") {
		t.Fatalf("premium : %v", err)
	}
	if err := c.Play(ctx, "Cuisine", "spotify:track:abc"); !errors.As(err, &se) || se.Reason != "DEVICE_NOT_FOUND" || !strings.Contains(se.Message, "iPhone, Salon") {
		t.Fatalf("enceinte absente : %v", err)
	}
	if err := c.Play(ctx, "Salon", "spotify:track:abc?x=1"); err == nil {
		t.Fatal("uri invalide acceptée")
	}
	bad := client(srv)
	bad.RefreshToken = "revoked"
	if _, err := bad.Search(ctx, "x", 1); err == nil || !strings.Contains(err.Error(), "spotify-login") {
		t.Fatalf("jeton révoqué : %v", err)
	}
	if tr, err := c.Search(ctx, "   ", 5); err != nil || len(tr) != 0 {
		t.Fatalf("recherche vide : %v %v", tr, err)
	}
}

func TestExchangeAndFold(t *testing.T) {
	srv, _, _ := fake(t, true)
	rt, err := Exchange(context.Background(), nil, srv.URL, "id", "secret", "c0de")
	if err != nil || rt != "rt" {
		t.Fatalf("échange : %q %v", rt, err)
	}
	if _, err := Exchange(context.Background(), nil, srv.URL, "id", "secret", "faux"); err == nil {
		t.Fatal("code faux accepté")
	}
	if Fold(" Enceinte  du SALON ") != "enceinte du salon" || Fold("Entrée") != "entree" {
		t.Fatalf("fold : %q %q", Fold(" Enceinte  du SALON "), Fold("Entrée"))
	}
	if !strings.Contains(AuthorizeURL("id", "s"), "redirect_uri=http%3A%2F%2F127.0.0.1%3A8888%2Fcallback") {
		t.Fatal("redirect_uri")
	}
}
