package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"ahomelab.io/control/internal/build"
	"ahomelab.io/control/internal/config"
	"ahomelab.io/control/internal/ha"
	"ahomelab.io/control/internal/journal"
	"ahomelab.io/control/internal/mass"
	"ahomelab.io/control/internal/model"
	"ahomelab.io/control/internal/spotify"
)

// Music searches tracks and albums and starts one on a speaker: Music
// Assistant through HA, the Spotify Web API, or the demo catalogue.
type Music interface {
	Search(ctx context.Context, q string, limit int) (model.Results, error)
	// Play starts uri, a track or an album (kind), replacing what the speaker plays.
	Play(ctx context.Context, room config.Room, dev config.Device, uri, kind string) error
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

func (m MassMusic) Search(ctx context.Context, q string, limit int) (model.Results, error) {
	return m.C.Search(ctx, q, limit)
}
func (m MassMusic) Play(ctx context.Context, room config.Room, dev config.Device, uri, kind string) error {
	return m.C.Play(ctx, dev.Entities["music"], uri, kind)
}
func (MassMusic) Source() string { return "Music Assistant" }

// SpotifyMusic plays through Spotify Connect: the speaker is found by name.
type SpotifyMusic struct{ C *spotify.Client }

func (m SpotifyMusic) Search(ctx context.Context, q string, limit int) (model.Results, error) {
	t, err := m.C.Search(ctx, q, limit)
	return model.Results{Tracks: t, Albums: []model.Album{}}, err
}
func (m SpotifyMusic) Play(ctx context.Context, room config.Room, dev config.Device, uri, kind string) error {
	if kind == "album" {
		return &spotify.Error{Status: http.StatusBadRequest, Message: "Albums : passer par Music Assistant"}
	}
	name := dev.SpotifyDevice
	if name == "" {
		name = room.Name
	}
	return m.C.Play(ctx, name, uri)
}
func (SpotifyMusic) Source() string { return "Spotify" }

// mediaSearch: GET /api/media/search?q=… → {tracks, albums}
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
	res, err := s.Music.Search(ctx, q, 10)
	if err != nil {
		fail(w, musicStatus(err), "Recherche impossible", err.Error())
		return
	}
	if res.Tracks == nil {
		res.Tracks = []model.Track{}
	}
	if res.Albums == nil {
		res.Albums = []model.Album{}
	}
	writeJSON(w, http.StatusOK, res)
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

// playMusic: action play_music {uri, kind?: track|album, title?, artists?}
// (play_track: the same, a track) on a media player.
func (s *Server) playMusic(w http.ResponseWriter, r *http.Request, id string, rc config.Room, dc config.Device, action string, raw json.RawMessage) {
	var q struct {
		URI     string `json:"uri"`
		Kind    string `json:"kind"`
		Title   string `json:"title"`
		Artists string `json:"artists"`
	}
	je := journal.Entry{Scope: "device", Target: id, Action: action, Params: journal.Compact(raw)}
	if len(raw) == 0 || json.Unmarshal(raw, &q) != nil || q.URI == "" {
		fail(w, http.StatusBadRequest, "Titre manquant", "")
		return
	}
	if q.Kind != "album" {
		q.Kind = "track"
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
	if err := s.Music.Play(ctx, rc, dc, q.URI, q.Kind); err != nil {
		je.Error = err.Error()
		s.record(r, je)
		fail(w, musicStatus(err), "Lecture impossible", err.Error())
		return
	}
	je.OK = true
	s.record(r, je)
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "message": label})
}

// favorites: GET /api/media/favorites → {tracks, albums}
func (s *Server) favoritesList(w http.ResponseWriter, r *http.Request) {
	if s.Favorites == nil {
		writeJSON(w, http.StatusOK, model.Results{Tracks: []model.Track{}, Albums: []model.Album{}})
		return
	}
	writeJSON(w, http.StatusOK, s.Favorites.List())
}

// favoritesSet: POST /api/media/favorites {kind: track|album, item, on} → {tracks, albums}
func (s *Server) favoritesSet(w http.ResponseWriter, r *http.Request) {
	if s.Favorites == nil {
		fail(w, http.StatusNotFound, "Favoris indisponibles", "state_dir")
		return
	}
	var q struct {
		Kind string          `json:"kind"`
		Item json.RawMessage `json:"item"`
		On   bool            `json:"on"`
	}
	if !s.decode(w, r, &q) {
		return
	}
	var (
		res model.Results
		err error
	)
	switch q.Kind {
	case "track":
		var t model.Track
		if err = json.Unmarshal(q.Item, &t); err == nil {
			res, err = s.Favorites.SetTrack(t, q.On)
		}
	case "album":
		var a model.Album
		if err = json.Unmarshal(q.Item, &a); err == nil {
			res, err = s.Favorites.SetAlbum(a, q.On)
		}
	default:
		err = errors.New("kind : track ou album")
	}
	if err != nil {
		fail(w, http.StatusBadRequest, "Favori refusé", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// haImage is what the HA client offers to fetch an image HA serves (artwork).
type haImage interface {
	Image(ctx context.Context, path string) ([]byte, string, error)
}

// mediaCover: GET /api/media/{id}/cover, the artwork of what a speaker plays,
// fetched from HA (the speaker's picture proxy) with Control's token.
func (s *Server) mediaCover(w http.ResponseWriter, r *http.Request) {
	_, dc, ok := s.findDevice(r.PathValue("id"))
	if !ok || dc.Kind != model.KindMedia {
		fail(w, http.StatusNotFound, "Enceinte inconnue", r.PathValue("id"))
		return
	}
	e := s.Src.Snapshot()[build.ActiveMedia(dc, s.Src.Snapshot())]
	pic, _ := e.Attributes["entity_picture"].(string)
	if !strings.HasPrefix(pic, "/api/") {
		pic, _ = e.Attributes["entity_picture_local"].(string)
	}
	hi, ok := s.Src.(haImage)
	if !ok || !strings.HasPrefix(pic, "/api/") {
		fail(w, http.StatusNotFound, "Pas de pochette", "")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	body, ctype, err := hi.Image(ctx, pic)
	if err != nil {
		fail(w, http.StatusBadGateway, "Pochette indisponible", err.Error())
		return
	}
	if !strings.HasPrefix(ctype, "image/") || strings.HasPrefix(ctype, "image/svg") {
		fail(w, http.StatusBadGateway, "Pochette indisponible", ctype)
		return
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", "private, max-age=86400") // the URL changes with the track (?v=)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(body)
}
