// Package model is the house as the interface sees it: rooms and devices
// with normalised states. Nothing here knows Home Assistant entity ids;
// the mapping lives in internal/build from control.yaml.
//
// packages/model/src/index.ts mirrors these types by hand; model_test.go
// checks that the JSON field names match.
package model

import "time"

type DeviceKind string

const (
	KindHeating       DeviceKind = "heating"
	KindAC            DeviceKind = "ac"
	KindLight         DeviceKind = "light"
	KindLightGroup    DeviceKind = "light_group"
	KindCover         DeviceKind = "cover"
	KindPlug          DeviceKind = "plug"
	KindOpening       DeviceKind = "opening"
	KindCamera        DeviceKind = "camera"
	KindMedia         DeviceKind = "media"
	KindSmoke         DeviceKind = "smoke"
	KindMotion        DeviceKind = "motion"
	KindClimateSensor DeviceKind = "climate_sensor"
)

// State holds every normalised value a device can expose. Fields are
// optional; a kind fills only the ones that make sense for it (see the
// table in packages/model). Values never carry HA's raw strings.
type State struct {
	On           *bool    `json:"on,omitempty"`           // light, light_group, plug, ac, media power
	Temperature  *float64 `json:"temperature,omitempty"`  // °C, 1 decimal
	Target       *float64 `json:"target,omitempty"`       // °C setpoint, 0.5 steps
	Humidity     *float64 `json:"humidity,omitempty"`     // %
	Heating      *bool    `json:"heating,omitempty"`      // heat demand right now
	HeatingPower *float64 `json:"heatingPower,omitempty"` // % (tado)
	Mode         string   `json:"mode,omitempty"`         // heating: schedule|manual|off ; ac: cool|heat|fan|auto|dry|off
	ModeUntil    string   `json:"modeUntil,omitempty"`    // heating manual: RFC3339 end, or "forever"
	MoldRisk     *bool    `json:"moldRisk,omitempty"`     // tado mould risk
	Eco          *bool    `json:"eco,omitempty"`          // ac
	Boost        *bool    `json:"boost,omitempty"`        // ac high power
	Brightness   *int     `json:"brightness,omitempty"`   // 0-100
	ColorTemp    *int     `json:"colorTemp,omitempty"`    // kelvin
	Color        *HS      `json:"color,omitempty"`        // hue/saturation
	Position     *int     `json:"position,omitempty"`     // cover 0 closed - 100 open
	Moving       string   `json:"moving,omitempty"`       // cover: opening|closing|""
	Power        *float64 `json:"power,omitempty"`        // W
	Open         *bool    `json:"open,omitempty"`         // opening
	Motion       *bool    `json:"motion,omitempty"`       // motion, camera
	Since        string   `json:"since,omitempty"`        // RFC3339 of the last change of the main value
	Playing      string   `json:"playing,omitempty"`      // media: playing|paused|idle|off
	Volume       *int     `json:"volume,omitempty"`       // 0-100
	Title        string   `json:"title,omitempty"`        // media
	Artist       string   `json:"artist,omitempty"`       // media
	Album        string   `json:"album,omitempty"`        // media
	App          string   `json:"app,omitempty"`          // media: the app playing (Spotify, Radio…)
	Radio        string   `json:"radio,omitempty"`        // media: id of the configured radio being played
	URI          string   `json:"uri,omitempty"`          // media: what plays (Music Assistant URI, stream URL)
	Elapsed      *float64 `json:"elapsed,omitempty"`      // media: seconds played at ElapsedAt
	ElapsedAt    string   `json:"elapsedAt,omitempty"`    // media: RFC3339
	Duration     *float64 `json:"duration,omitempty"`     // media: seconds (not for a live stream)
	Cover        string   `json:"cover,omitempty"`        // media: artwork URL (https or /api/media/{id}/cover)
	Privacy      *bool    `json:"privacy,omitempty"`      // camera private mode
	Smoke        *bool    `json:"smoke,omitempty"`        // smoke detector
	CO           *bool    `json:"co,omitempty"`           // smoke detector
}

type HS struct {
	H float64 `json:"h"`
	S float64 `json:"s"`
}

type Device struct {
	ID           string     `json:"id"` // "<room>.<device>", e.g. salon.chauffage
	RoomID       string     `json:"roomId"`
	Kind         DeviceKind `json:"kind"`
	Name         string     `json:"name"`
	State        State      `json:"state"`
	Capabilities []string   `json:"capabilities"`
	Available    bool       `json:"available"`
	UpdatedAt    time.Time  `json:"updatedAt"`
	Battery      string     `json:"battery,omitempty"`  // "ok" | "low"
	Primary      bool       `json:"primary,omitempty"`  // the room's main climate device
	Image        string     `json:"image,omitempty"`    // URL path of a product photo (/media/...), optional
	ImageOff     string     `json:"imageOff,omitempty"` // lights: photo when off
}

type RoomSummary struct {
	Temperature  *float64 `json:"temperature,omitempty"`
	Humidity     *float64 `json:"humidity,omitempty"`
	Heating      bool     `json:"heating"`
	Target       *float64 `json:"target,omitempty"`
	HeatingMode  string   `json:"heatingMode,omitempty"`
	LightsOn     int      `json:"lightsOn"`
	OpeningsOpen int      `json:"openingsOpen"`
	Unavailable  int      `json:"unavailable"`
	Alerts       []Alert  `json:"alerts"`
}

type Room struct {
	ID      string      `json:"id"`
	Name    string      `json:"name"`
	Icon    string      `json:"icon"`
	Image   string      `json:"image,omitempty"` // URL path of the room photo (/media/...), optional
	Devices []Device    `json:"devices"`
	Summary RoomSummary `json:"summary"`
}

type Alert struct {
	ID       string    `json:"id"`    // stable: "<room>.<device>.<kind>"
	Level    string    `json:"level"` // info | warn | crit
	Kind     string    `json:"kind"`  // window_heating | battery | unavailable | smoke | co | offline | vigilance
	Title    string    `json:"title"`
	Detail   string    `json:"detail,omitempty"`
	RoomID   string    `json:"roomId,omitempty"`
	DeviceID string    `json:"deviceId,omitempty"`
	Since    time.Time `json:"since"`
}

type Person struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Home bool   `json:"home"`
}

type HeatingGlobal struct {
	Average  *float64 `json:"average,omitempty"`  // sensor.day_area_temperature
	Humidity *float64 `json:"humidity,omitempty"` // sensor.day_area_humidity
	RoomsOn  int      `json:"roomsOn"`            // rooms heating right now
	Mode     string   `json:"mode,omitempty"`     // schedule | manual | off | mixed (from the zones)
	// Preset: the house mode last chosen in Control (schedule | night | boost | off), while the zones still follow it
	Preset    string `json:"preset,omitempty"`
	Presence  string `json:"presence,omitempty"` // tado presence: home | away | auto
	APIQuota  *int   `json:"apiQuota,omitempty"` // tado remaining calls
	Available bool   `json:"available"`
}

type Alarm struct {
	Armed      bool   `json:"armed"`
	Triggered  bool   `json:"triggered"`
	Acked      bool   `json:"acked"`
	UnifiState string `json:"unifiState,omitempty"` // read-only
	Available  bool   `json:"available"`
}

type Globals struct {
	Heating        HeatingGlobal `json:"heating"`
	Alarm          Alarm         `json:"alarm"`
	LightsOn       int           `json:"lightsOn"`
	OpeningsOpen   int           `json:"openingsOpen"`
	CamerasPrivate *bool         `json:"camerasPrivate,omitempty"`
}

type Weather struct {
	Available   bool           `json:"available"`
	Condition   string         `json:"condition,omitempty"` // HA condition (sunny, rainy, ...)
	Night       bool           `json:"night"`
	Temperature *float64       `json:"temperature,omitempty"`
	Feels       *float64       `json:"feels,omitempty"`
	Humidity    *float64       `json:"humidity,omitempty"`
	Pressure    *float64       `json:"pressure,omitempty"`
	WindSpeed   *float64       `json:"windSpeed,omitempty"`
	WindBearing *float64       `json:"windBearing,omitempty"`
	WindGust    *float64       `json:"windGust,omitempty"`
	UV          *float64       `json:"uv,omitempty"`
	Visibility  *float64       `json:"visibility,omitempty"`
	CloudCover  *float64       `json:"cloudCover,omitempty"`
	DewPoint    *float64       `json:"dewPoint,omitempty"`
	Min         *float64       `json:"min,omitempty"` // today's forecast low
	Max         *float64       `json:"max,omitempty"`
	Sunrise     string         `json:"sunrise,omitempty"` // RFC3339
	Sunset      string         `json:"sunset,omitempty"`
	Hourly      []ForecastHour `json:"hourly"`
	Daily       []ForecastDay  `json:"daily"`
	Place       string         `json:"place,omitempty"`
	UpdatedAt   time.Time      `json:"updatedAt"`
}

type ForecastHour struct {
	Time          string   `json:"time"` // RFC3339
	Condition     string   `json:"condition"`
	Temperature   *float64 `json:"temperature,omitempty"`
	Precipitation *float64 `json:"precipitation,omitempty"` // mm
	PrecipProb    *float64 `json:"precipProb,omitempty"`    // %
	WindSpeed     *float64 `json:"windSpeed,omitempty"`
}

type ForecastDay struct {
	Date          string   `json:"date"` // RFC3339 (midnight local)
	Condition     string   `json:"condition"`
	Max           *float64 `json:"max,omitempty"`
	Min           *float64 `json:"min,omitempty"`
	Precipitation *float64 `json:"precipitation,omitempty"`
	PrecipProb    *float64 `json:"precipProb,omitempty"`
	WindSpeed     *float64 `json:"windSpeed,omitempty"`
	WindBearing   *float64 `json:"windBearing,omitempty"`
	Humidity      *float64 `json:"humidity,omitempty"`
	UV            *float64 `json:"uv,omitempty"`
}

// Air is the outdoor air quality: ATMO-style index (1 good … 6 extremely poor),
// per-pollutant sub-indices and concentrations, pollen levels (0–6).
type Air struct {
	Available  bool        `json:"available"`
	Index      int         `json:"index"` // 0 = unknown
	Label      string      `json:"label"`
	Dominant   string      `json:"dominant,omitempty"` // pollutant key
	Tomorrow   int         `json:"tomorrow,omitempty"` // index forecast for tomorrow, 0 = unknown
	Pollutants []Pollutant `json:"pollutants"`
	Pollens    []Pollen    `json:"pollens"`
	Place      string      `json:"place,omitempty"`
	Source     string      `json:"source"`
	UpdatedAt  time.Time   `json:"updatedAt"`
}

type Pollutant struct {
	Key   string  `json:"key"`   // pm25 | pm10 | no2 | o3 | so2
	Index int     `json:"index"` // 1–6
	Value float64 `json:"value"` // µg/m³
}

type Pollen struct {
	Key   string  `json:"key"`   // grass | birch | alder | mugwort | olive | ragweed
	Level int     `json:"level"` // 0–6
	Value float64 `json:"value"` // grains/m³
}

// Agenda is the family calendar kept by the app: the occurrences of the weeks
// shown (current week → +5 weeks), and what the quick entry can do.
type Agenda struct {
	Available bool       `json:"available"`
	Calendars []Calendar `json:"calendars"`
	Events    []CalEvent `json:"events"`
	From      time.Time  `json:"from"`
	To        time.Time  `json:"to"`
	UpdatedAt time.Time  `json:"updatedAt"`
	// Input: quick entry by typed sentence (Text) and by voice (Audio).
	Input AgendaInput `json:"input"`
}

type AgendaInput struct {
	Text  bool `json:"text"`
	Audio bool `json:"audio"`
}

type Calendar struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
	Owner string `json:"owner,omitempty"` // member id of a personal calendar; empty = shared
}

// Member is one person of the household (control.yaml people:), with a
// personal calendar of the same id.
type Member struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
}

type CalEvent struct {
	ID       string    `json:"id"`      // occurrence: "<eventId>@<date>"
	EventID  string    `json:"eventId"` // the event to edit (the whole series when it repeats)
	Repeats  bool      `json:"repeats,omitempty"`
	Calendar string    `json:"calendar"`
	Title    string    `json:"title"`
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
	AllDay   bool      `json:"allDay"`
	Location string    `json:"location,omitempty"`
}

type Home struct {
	Name      string    `json:"name"`
	Rooms     []Room    `json:"rooms"`
	Globals   Globals   `json:"globals"`
	Weather   *Weather  `json:"weather,omitempty"`
	Air       *Air      `json:"air,omitempty"`
	Agenda    *Agenda   `json:"agenda,omitempty"`
	Media     *Media    `json:"media,omitempty"`
	Presence  []Person  `json:"presence"`
	People    []Member  `json:"people"` // the household (personal calendars, face recognition)
	Face      bool      `json:"face"`   // face recognition configured: a device may recognise who wakes it
	Alerts    []Alert   `json:"alerts"`
	Connected bool      `json:"connected"` // server ↔ Home Assistant
	Demo      bool      `json:"demo"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Event is one SSE message. Exactly one of the payload fields is set.
type Event struct {
	Type    string    `json:"type"` // home | device | summary | alert | status
	Device  *Device   `json:"device,omitempty"`
	Summary *Summary  `json:"summary,omitempty"`
	Home    *Home     `json:"home,omitempty"`
	Status  *Status   `json:"status,omitempty"`
	At      time.Time `json:"at"`
}

// Summary is everything that is derived from several devices: room
// summaries, globals, presence, alerts, weather.
type Summary struct {
	Rooms    map[string]RoomSummary `json:"rooms"`
	Globals  Globals                `json:"globals"`
	Presence []Person               `json:"presence"`
	Alerts   []Alert                `json:"alerts"`
	Weather  *Weather               `json:"weather,omitempty"`
}

type Status struct {
	Connected bool   `json:"connected"`
	Message   string `json:"message,omitempty"`
}

func F(v float64) *float64 { return &v }
func B(v bool) *bool       { return &v }
func I(v int) *int         { return &v }

// Media is what the speakers can play: the configured radios, and whether a
// music search is offered (and through what: "Music Assistant", "Spotify").
type Media struct {
	Radios      []Radio `json:"radios"`
	Music       bool    `json:"music"`
	MusicSource string  `json:"musicSource,omitempty"`
}

type Radio struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Genre string `json:"genre,omitempty"`
	Short string `json:"short"`
	Color string `json:"color,omitempty"`
	Logo  string `json:"logo,omitempty"` // /radios/….webp
}

// Track is one music search result (Music Assistant or Spotify).
type Track struct {
	URI      string `json:"uri"`
	Title    string `json:"title"`
	Artists  string `json:"artists"`
	Album    string `json:"album,omitempty"`
	Duration int    `json:"durationMs"`
	Cover    string `json:"cover,omitempty"` // https://i.scdn.co/…
}

// Album is one album search result; playing it replaces the queue with its tracks.
type Album struct {
	URI     string `json:"uri"`
	Title   string `json:"title"`
	Artists string `json:"artists"`
	Year    int    `json:"year,omitempty"`
	Cover   string `json:"cover,omitempty"`
}

// Results: a music search (or the favourites), tracks and albums.
type Results struct {
	Tracks []Track `json:"tracks"`
	Albums []Album `json:"albums"`
}
