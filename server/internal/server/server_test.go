package server

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ahomelab.io/maison/internal/config"
	"ahomelab.io/maison/internal/demo"
	"ahomelab.io/maison/internal/ha"
	"ahomelab.io/maison/internal/model"
	"ahomelab.io/maison/internal/push"
)

func newTestServer(t *testing.T) (*Server, *demo.Source) {
	t.Helper()
	cfg, err := config.Load("../../../maison.yaml")
	if err != nil {
		t.Fatal(err)
	}
	src := demo.New(cfg)
	s := New(cfg, src, strings.Repeat("a", 48), true)
	s.Debounce = 10 * time.Millisecond
	return s, src
}

func TestLoginAndAuth(t *testing.T) {
	s, _ := newTestServer(t)
	h := s.Handler()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/home", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("sans cookie : %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/login?token=faux", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("faux jeton : %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/login?token="+s.Token+"%0A", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("login : %d %s", w.Code, w.Body)
	}
	c := w.Result().Cookies()
	if len(c) != 1 || c[0].Name != cookieName || !c[0].HttpOnly || c[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie : %+v", c)
	}
	req := httptest.NewRequest("GET", "/api/home", nil)
	req.AddCookie(c[0])
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("home : %d", w.Code)
	}
	var home model.Home
	if err := json.NewDecoder(w.Body).Decode(&home); err != nil {
		t.Fatal(err)
	}
	if len(home.Rooms) != 6 || !home.Demo {
		t.Fatalf("home : %d pièces, demo=%v", len(home.Rooms), home.Demo)
	}
	// mutations: not yet, and never without the header
	req = httptest.NewRequest("POST", "/api/home/heating", strings.NewReader(`{"mode":"off"}`))
	req.AddCookie(c[0])
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("sans X-Maison : %d", w.Code)
	}
	req.Header.Set("X-Maison", "1")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("chauffage off : %d %s", w.Code, w.Body)
	}
}

func TestActions(t *testing.T) {
	s, src := newTestServer(t)
	h := s.Handler()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/login?token="+s.Token, nil))
	c := w.Result().Cookies()[0]
	post := func(path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", path, strings.NewReader(body))
		req.AddCookie(c)
		req.Header.Set("X-Maison", "1")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	cases := []struct {
		path, body string
		want       int
	}{
		{"/api/devices/bureau.ampoule/actions", `{"action":"power","params":{"on":true}}`, 202},
		{"/api/devices/bureau.ampoule/actions", `{"action":"set_brightness","params":{"pct":40}}`, 202},
		{"/api/devices/bureau.hue1/actions", `{"action":"set_color","params":{"hs":{"h":200,"s":80}}}`, 202},
		{"/api/devices/bureau.volet/actions", `{"action":"set_position","params":{"pct":30}}`, 202},
		{"/api/devices/bureau.store/actions", `{"action":"open"}`, 502}, // unavailable in the demo
		{"/api/devices/salon.chauffage/actions", `{"action":"set_target","params":{"temperature":22}}`, 202},
		{"/api/devices/salon.chauffage/actions", `{"action":"set_mode","params":{"mode":"schedule"}}`, 202},
		{"/api/devices/salon.chauffage/actions", `{"action":"set_mode","params":{"mode":"manual","duration":60}}`, 202},
		{"/api/devices/salon.chauffage/actions", `{"action":"set_mode","params":{"mode":"bizarre"}}`, 400},
		{"/api/devices/salon.camera/actions", `{"action":"set_privacy","params":{"on":true}}`, 202},
		{"/api/devices/salon.camera/actions", `{"action":"power","params":{"on":true}}`, 400},
		{"/api/devices/nope.x/actions", `{"action":"power"}`, 404},
		{"/api/home/heating", `{"mode":"boost"}`, 202},
		{"/api/home/lights/off", `{}`, 202},
		{"/api/security/alarm", `{"armed":false}`, 202},
		{"/api/security/alarm/ack", `{}`, 202},
	}
	for _, tc := range cases {
		if w := post(tc.path, tc.body); w.Code != tc.want {
			t.Errorf("%s %s : %d, attendu %d (%s)", tc.path, tc.body, w.Code, tc.want, w.Body)
		}
	}
	// the demo applies the calls after ~350 ms
	time.Sleep(600 * time.Millisecond)
	st := src.Snapshot()
	if e := st["light.ampoule_4"]; e.State != "off" { // lights/off came last
		t.Errorf("ampoule : %s", e.State)
	}
	if e := st["input_boolean.alarm"]; e.State != "off" {
		t.Errorf("alarme : %s", e.State)
	}
	if e := st["binary_sensor.camera_sensor_private"]; e.State != "on" {
		t.Errorf("mode privé : %s", e.State)
	}
	if t0, _ := st["climate.tado_smart_thermostat_ru0044248832"].Attributes["temperature"].(float64); t0 != 25 { // boost
		t.Errorf("boost : %v", t0)
	}
}

func TestEventsStream(t *testing.T) {
	s, src := newTestServer(t)
	ctx, cancel := contextWithTimeout(t, 3*time.Second)
	defer cancel()
	go s.Run(ctx)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/api/events", nil)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: s.Token})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("type : %s", resp.Header.Get("Content-Type"))
	}
	r := bufio.NewReader(resp.Body)
	ev, data := readEvent(t, r)
	if ev != "home" || !strings.Contains(data, `"rooms"`) {
		t.Fatalf("premier événement : %s", ev)
	}
	// change a light in "HA" → a device event then a summary event
	st := src.Snapshot()
	e := st["light.ampoule_4"]
	e.State = "on"
	e.Attributes["brightness"] = 255.0
	e.LastChanged, e.LastUpdated = time.Now(), time.Now()
	src.Set(e)
	seen := map[string]string{}
	for i := 0; i < 2; i++ {
		ev, data = readEvent(t, r)
		seen[ev] = data
	}
	if !strings.Contains(seen["device"], `"bureau.ampoule"`) || !strings.Contains(seen["device"], `"brightness":100`) {
		t.Fatalf("événement device : %s", seen["device"])
	}
	if !strings.Contains(seen["summary"], `"lightsOn":3`) {
		t.Fatalf("événement summary : %s", seen["summary"])
	}
}

func readEvent(t *testing.T, r *bufio.Reader) (string, string) {
	t.Helper()
	var ev, data string
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		line = strings.TrimRight(line, "\n")
		switch {
		case strings.HasPrefix(line, "event: "):
			ev = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			data = strings.TrimPrefix(line, "data: ")
		case line == "" && ev != "":
			return ev, data
		}
	}
}

func TestAllowFrom(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	h, err := AllowFrom(ok, []string{"192.168.30.0/24", "127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	for addr, want := range map[string]int{"192.168.30.12:5000": 200, "127.0.0.1:1": 200, "10.0.0.1:1": 403} {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = addr
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("%s : %d", addr, w.Code)
		}
	}
	if _, err := AllowFrom(ok, []string{"pas-un-réseau"}); err == nil {
		t.Fatal("réseau invalide accepté")
	}
}

func TestCameraSnapshotDemo(t *testing.T) {
	s, _ := newTestServer(t)
	req := httptest.NewRequest("GET", "/api/cameras/salon.camera/snapshot", nil)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: s.Token})
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "image/") {
		t.Fatalf("snapshot : %d %s", w.Code, w.Header().Get("Content-Type"))
	}
	req = httptest.NewRequest("GET", "/api/cameras/salon.chauffage/snapshot", nil)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: s.Token})
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != 404 {
		t.Fatalf("pas une caméra : %d", w.Code)
	}
}

var _ ha.Source = (*demo.Source)(nil)

func TestMedia(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "rooms"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rooms", "salon.jpg"), []byte("\xff\xd8\xff"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, _ := newTestServer(t)
	s.MediaDir = dir
	h := s.Handler()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/login?token="+s.Token, nil))
	c := w.Result().Cookies()[0]
	for _, tc := range []struct {
		path string
		want int
	}{
		{"/media/rooms/salon.jpg", http.StatusOK},
		{"/media/rooms/x/../salon.jpg", http.StatusOK}, // cleaned by the mux, stays inside
		{"/media/notes.txt", http.StatusNotFound},      // not an image
		{"/media/rooms/", http.StatusNotFound},
		{"/media/missing.jpg", http.StatusNotFound},
	} {
		req := httptest.NewRequest("GET", tc.path, nil)
		req.AddCookie(c)
		w = httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != tc.want && !(tc.want == http.StatusOK && w.Code == http.StatusMovedPermanently) {
			t.Errorf("%s : %d, attendu %d", tc.path, w.Code, tc.want)
		}
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/media/rooms/salon.jpg", nil))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("sans session : %d", w.Code)
	}
}

func TestPush(t *testing.T) {
	// a fake push service: "/dead" answers 410, everything else 201
	var got []string
	ps := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/dead") {
			w.WriteHeader(410)
			return
		}
		got = append(got, r.URL.Path)
		w.WriteHeader(201)
	}))
	defer ps.Close()
	// the browser side of a subscription (any P-256 point + 16-byte auth is enough to encrypt)
	mk := func(endpoint string) string {
		return `{"endpoint":"` + endpoint + `","keys":{"p256dh":"BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4","auth":"BTBZMqHH6r4Tts7J_aSIgg"}}`
	}

	s, _ := newTestServer(t)
	h := s.Handler()
	call := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.AddCookie(&http.Cookie{Name: cookieName, Value: s.Token})
		req.Header.Set("X-Maison", "1")
		req.Header.Set("User-Agent", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 Version/17.0 Mobile/15E148 Safari/604.1")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	if w := call("GET", "/api/push/key", ""); w.Code != 404 {
		t.Fatalf("sans store : %d", w.Code)
	}
	st, err := push.Open(t.TempDir(), "mailto:test@maison")
	if err != nil {
		t.Fatal(err)
	}
	s.Push = st
	var key struct{ Key string }
	if w := call("GET", "/api/push/key", ""); w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &key) != nil || len(key.Key) != 87 {
		t.Fatalf("clé : %d %s", w.Code, w.Body)
	}
	if w := call("POST", "/api/push/subscribe", `{"endpoint":"http://insecure","keys":{}}`); w.Code != 400 {
		t.Fatalf("abonnement invalide accepté : %d", w.Code)
	}
	if w := call("POST", "/api/push/subscribe", mk("https://push.example/a")); w.Code != 200 || st.Count() != 1 || st.List()[0].Label != "iPhone · Safari" {
		t.Fatalf("subscribe : %d %s (%+v)", w.Code, w.Body, st.List())
	}
	if w := call("POST", "/api/push/unsubscribe", `{"endpoint":"https://push.example/a"}`); w.Code != 200 || st.Count() != 0 {
		t.Fatalf("unsubscribe : %d count=%d", w.Code, st.Count())
	}
	// httptest is http:// — the handler requires https; add the fake ones directly
	var a, dead push.Subscription
	json.Unmarshal([]byte(mk(ps.URL+"/a")), &a)
	json.Unmarshal([]byte(mk(ps.URL+"/dead")), &dead)
	st.Add(a)
	st.Add(dead)
	if l := st.List(); len(l) != 2 {
		t.Fatalf("liste : %d", len(l))
	}
	if w := call("POST", "/api/push/test", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"sent":1`) {
		t.Fatalf("test : %d %s", w.Code, w.Body)
	}
	if st.Count() != 1 {
		t.Fatalf("abonnement mort conservé : %d", st.Count())
	}
	if w := call("POST", "/api/push/unsubscribe", `{"endpoint":"`+ps.URL+`/a"}`); w.Code != 200 || st.Count() != 0 {
		t.Fatalf("unsubscribe : %d count=%d", w.Code, st.Count())
	}
	st.Add(a)
	got = nil

	// notifyAlerts: the first pass only records, then new warn/crit alerts notify, info and batteries do not
	base := model.Home{Alerts: []model.Alert{{ID: "old", Level: "crit", Kind: "smoke", Title: "Déjà là"}}}
	s.push = nil
	s.notifyAlerts(model.Home{}, base)
	cur := model.Home{Alerts: append(base.Alerts,
		model.Alert{ID: "w", Level: "warn", Kind: "window_heating", Title: "Fenêtre ouverte", RoomID: "salon"},
		model.Alert{ID: "b", Level: "warn", Kind: "battery", Title: "Pile"},
		model.Alert{ID: "i", Level: "info", Kind: "unavailable", Title: "Info"})}
	s.notifyAlerts(base, cur)
	// alarm trigger
	trig := cur
	trig.Globals.Alarm.Triggered = true
	s.notifyAlerts(cur, trig)
	deadline := time.Now().Add(3 * time.Second)
	for len(got) < 2 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if len(got) != 2 {
		t.Fatalf("notifications envoyées : %d (attendu 2 : fenêtre + alarme)", len(got))
	}
	// resolved then back: notifies again
	s.notifyAlerts(trig, base)
	s.notifyAlerts(base, cur)
	deadline = time.Now().Add(3 * time.Second)
	for len(got) < 3 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if len(got) != 3 {
		t.Fatalf("après retour de l'alerte : %d", len(got))
	}
	if shortUA("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/120.0 Safari/537.36") != "Mac · Chrome" {
		t.Fatal("shortUA")
	}
}
