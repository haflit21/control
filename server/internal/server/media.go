package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"ahomelab.io/control/internal/config"
	"ahomelab.io/control/internal/ha"
	"ahomelab.io/control/internal/journal"
	"ahomelab.io/control/internal/model"
	"ahomelab.io/control/internal/spotify"
)

// Music searches tracks and starts one on a speaker (Spotify Web API, or the
// demo catalogue).
type Music interface {
	Search(ctx context.Context, q string, limit int) ([]model.Track, error)
	Play(ctx context.Context, deviceName, uri string) error
}

// mediaSearch: GET /api/media/search?q=… → {tracks}
func (s *Server) mediaSearch(w http.ResponseWriter, r *http.Request) {
	if s.Music == nil {
		fail(w, http.StatusNotFound, "Spotify n'est pas configuré", "media.spotify dans control.yaml et les secrets SPOTIFY_*")
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if utf8.RuneCountInString(q) > 100 {
		fail(w, http.StatusBadRequest, "Recherche trop longue", "")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	tracks, err := s.Music.Search(ctx, q, 10)
	if err != nil {
		fail(w, musicStatus(err), "Recherche Spotify impossible", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tracks": tracks})
}

func musicStatus(err error) int {
	var se *spotify.Error
	if errors.As(err, &se) && se.Status == http.StatusConflict {
		return http.StatusConflict
	}
	return http.StatusBadGateway
}

// radioCall: media_player.play_media with the stream, its name for the
// speaker's screen (Cast reads extra.title) and a live stream type.
func radioCall(entity string, rd config.Radio) call {
	return call{"media_player", "play_media", target(entity, map[string]any{
		"media_content_id":   rd.URL,
		"media_content_type": "music",
		"extra":              map[string]any{"title": rd.Name, "stream_type": "LIVE", "metadata": map[string]any{"metadataType": 3, "title": rd.Name, "artist": rd.Genre}},
	})}
}

// resumeRadio picks what an idle speaker should play: the radio its last
// session was on, else the last one Control started, else the first configured.
func (s *Server) resumeRadio(entity string, st ha.Entity) (config.Radio, bool) {
	if id, _ := st.Attributes["media_content_id"].(string); id != "" {
		for _, r := range s.Cfg.Media.Radios {
			if r.URL == id {
				return r, true
			}
		}
	}
	if v, ok := s.lastRadio.Load(entity); ok {
		if r, ok := s.Cfg.Radio(v.(string)); ok {
			return r, true
		}
	}
	if len(s.Cfg.Media.Radios) > 0 {
		return s.Cfg.Media.Radios[0], true
	}
	return config.Radio{}, false
}

// playSpotify: action play_spotify {uri, title?, artists?} on a media player.
func (s *Server) playSpotify(w http.ResponseWriter, r *http.Request, id string, rc config.Room, dc config.Device, raw json.RawMessage) {
	var q struct {
		URI     string `json:"uri"`
		Title   string `json:"title"`
		Artists string `json:"artists"`
	}
	je := journal.Entry{Scope: "device", Target: id, Action: "play_spotify", Params: journal.Compact(raw)}
	if len(raw) == 0 || json.Unmarshal(raw, &q) != nil || q.URI == "" {
		fail(w, http.StatusBadRequest, "Titre manquant", "")
		return
	}
	if s.Music == nil {
		fail(w, http.StatusNotFound, "Spotify n'est pas configuré", "")
		return
	}
	speaker := dc.SpotifyDevice
	if speaker == "" {
		speaker = rc.Name
	}
	label := dc.Name + " · Spotify"
	if t := strings.TrimSpace(q.Title); t != "" && utf8.RuneCountInString(t) <= 200 {
		label = dc.Name + " · " + t
	}
	je.Text = label
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err := s.Music.Play(ctx, speaker, q.URI); err != nil {
		je.Error = err.Error()
		s.record(r, je)
		fail(w, musicStatus(err), "Lecture Spotify impossible", err.Error())
		return
	}
	if s.Demo {
		// the demo speaker shows what was asked, like the real one would
		extra := map[string]any{"title": q.Title, "metadata": map[string]any{"title": q.Title, "artist": q.Artists}, "app_name": "Spotify"}
		_ = s.Src.CallService(ctx, "media_player", "play_media", target(dc.Entities["media_player"], map[string]any{"media_content_id": q.URI, "media_content_type": "music", "extra": extra}))
	}
	je.OK = true
	s.record(r, je)
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "message": label})
}
