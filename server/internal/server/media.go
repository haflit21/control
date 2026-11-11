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
	"ahomelab.io/control/internal/mass"
	"ahomelab.io/control/internal/model"
	"ahomelab.io/control/internal/spotify"
)

// Music searches tracks and starts one on a speaker: Music Assistant through
// HA, the Spotify Web API, or the demo catalogue.
type Music interface {
	Search(ctx context.Context, q string, limit int) ([]model.Track, error)
	Play(ctx context.Context, room config.Room, dev config.Device, uri string) error
	// Source names the service for the interface ("Music Assistant", "Spotify").
	Source() string
}

func (s *Server) musicSource() string {
	if s.Music == nil {
		return ""
	}
	return s.Music.Source()
}

// MassMusic plays through Music Assistant: the speaker's MA player is entities.music.
type MassMusic struct{ C *mass.Client }

func (m MassMusic) Search(ctx context.Context, q string, limit int) ([]model.Track, error) {
	return m.C.Search(ctx, q, limit)
}
func (m MassMusic) Play(ctx context.Context, room config.Room, dev config.Device, uri string) error {
	return m.C.Play(ctx, dev.Entities["music"], uri)
}
func (MassMusic) Source() string { return "Music Assistant" }

// SpotifyMusic plays through Spotify Connect: the speaker is found by name.
type SpotifyMusic struct{ C *spotify.Client }

func (m SpotifyMusic) Search(ctx context.Context, q string, limit int) ([]model.Track, error) {
	return m.C.Search(ctx, q, limit)
}
func (m SpotifyMusic) Play(ctx context.Context, room config.Room, dev config.Device, uri string) error {
	name := dev.SpotifyDevice
	if name == "" {
		name = room.Name
	}
	return m.C.Play(ctx, name, uri)
}
func (SpotifyMusic) Source() string { return "Spotify" }

// mediaSearch: GET /api/media/search?q=… → {tracks}
func (s *Server) mediaSearch(w http.ResponseWriter, r *http.Request) {
	if s.Music == nil {
		fail(w, http.StatusNotFound, "Recherche de musique non configurée", "media.music dans control.yaml")
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
		fail(w, musicStatus(err), "Recherche impossible", err.Error())
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
// The content type is the stream's real MIME type: a Cast receiver given the
// generic "music" cannot decode a raw AAC stream.
func radioCall(entity string, rd config.Radio) call {
	return call{"media_player", "play_media", target(entity, map[string]any{
		"media_content_id":   rd.URL,
		"media_content_type": streamType(rd.URL),
		"extra":              map[string]any{"title": rd.Name, "stream_type": "LIVE", "metadata": map[string]any{"metadataType": 3, "title": rd.Name, "artist": rd.Genre}},
	})}
}

func streamType(u string) string {
	p, _, _ := strings.Cut(strings.ToLower(u), "?")
	switch {
	case strings.HasSuffix(p, ".mp3"):
		return "audio/mpeg"
	case strings.HasSuffix(p, ".aac"):
		return "audio/aac"
	case strings.HasSuffix(p, ".ogg"), strings.HasSuffix(p, ".opus"):
		return "audio/ogg"
	case strings.HasSuffix(p, ".m3u8"):
		return "application/vnd.apple.mpegurl"
	}
	return "music"
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

// playTrack: action play_track {uri, title?, artists?} on a media player.
func (s *Server) playTrack(w http.ResponseWriter, r *http.Request, id string, rc config.Room, dc config.Device, raw json.RawMessage) {
	var q struct {
		URI     string `json:"uri"`
		Title   string `json:"title"`
		Artists string `json:"artists"`
	}
	je := journal.Entry{Scope: "device", Target: id, Action: "play_track", Params: journal.Compact(raw)}
	if len(raw) == 0 || json.Unmarshal(raw, &q) != nil || q.URI == "" {
		fail(w, http.StatusBadRequest, "Titre manquant", "")
		return
	}
	if s.Music == nil {
		fail(w, http.StatusNotFound, "Recherche de musique non configurée", "")
		return
	}
	label := dc.Name + " · " + s.Music.Source()
	if t := strings.TrimSpace(q.Title); t != "" && utf8.RuneCountInString(t) <= 200 {
		label = dc.Name + " · " + t
	}
	je.Text = label
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err := s.Music.Play(ctx, rc, dc, q.URI); err != nil {
		je.Error = err.Error()
		s.record(r, je)
		fail(w, musicStatus(err), "Lecture impossible", err.Error())
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
