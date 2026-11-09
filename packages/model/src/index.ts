// Mirror of server/internal/model/model.go (JSON field names). Keep both in
// sync by hand; the Go test TestModelFields checks the field list against
// this file.

export type DeviceKind =
  | 'heating' | 'ac' | 'light' | 'light_group' | 'cover' | 'plug' | 'opening'
  | 'camera' | 'media' | 'smoke' | 'motion' | 'climate_sensor';

export type HeatingMode = 'schedule' | 'manual' | 'off';
export type ACMode = 'cool' | 'heat' | 'fan' | 'auto' | 'dry' | 'off';
export type Playing = 'playing' | 'paused' | 'idle' | 'off';

export interface HS { h: number; s: number }

export interface State {
  on?: boolean;
  temperature?: number;
  target?: number;
  humidity?: number;
  heating?: boolean;
  heatingPower?: number;
  mode?: HeatingMode | ACMode | string;
  modeUntil?: string;
  moldRisk?: boolean;
  eco?: boolean;
  boost?: boolean;
  brightness?: number;
  colorTemp?: number;
  color?: HS;
  position?: number;
  moving?: 'opening' | 'closing' | '';
  power?: number;
  open?: boolean;
  motion?: boolean;
  since?: string;
  playing?: Playing;
  volume?: number;
  title?: string;
  artist?: string;
  privacy?: boolean;
  smoke?: boolean;
  co?: boolean;
}

export interface Device {
  id: string;
  roomId: string;
  kind: DeviceKind;
  name: string;
  state: State;
  capabilities: string[];
  available: boolean;
  updatedAt: string;
  battery?: 'ok' | 'low';
  primary?: boolean;
  /** URL path of a product photo (/media/...), set from maison.yaml; the UI has a default per kind */
  image?: string;
  /** For lights: photo when off (image is then the "on" photo) */
  imageOff?: string;
}

export type AlertLevel = 'info' | 'warn' | 'crit';
export type AlertKind = 'window_heating' | 'battery' | 'unavailable' | 'smoke' | 'co' | 'offline' | 'vigilance';

export interface Alert {
  id: string;
  level: AlertLevel;
  kind: AlertKind | string;
  title: string;
  detail?: string;
  roomId?: string;
  deviceId?: string;
  since: string;
}

export interface RoomSummary {
  temperature?: number;
  humidity?: number;
  heating: boolean;
  target?: number;
  heatingMode?: string;
  lightsOn: number;
  openingsOpen: number;
  unavailable: number;
  alerts: Alert[];
}

export interface Room {
  id: string;
  name: string;
  icon: string;
  /** URL path of the room photo (/media/...), optional */
  image?: string;
  devices: Device[];
  summary: RoomSummary;
}

export interface Person { id: string; name: string; home: boolean }

export interface HeatingGlobal {
  average?: number;
  humidity?: number;
  roomsOn: number;
  mode?: 'schedule' | 'manual' | 'off' | 'mixed' | string;
  presence?: 'home' | 'away' | 'auto';
  apiQuota?: number;
  available: boolean;
}

export interface Alarm {
  armed: boolean;
  triggered: boolean;
  acked: boolean;
  unifiState?: string;
  available: boolean;
}

export interface Globals {
  heating: HeatingGlobal;
  alarm: Alarm;
  lightsOn: number;
  openingsOpen: number;
  camerasPrivate?: boolean;
}

export type Condition =
  | 'sunny' | 'clear-night' | 'partlycloudy' | 'cloudy' | 'fog' | 'rainy' | 'pouring'
  | 'lightning' | 'lightning-rainy' | 'snowy' | 'snowy-rainy' | 'hail' | 'windy' | 'windy-variant' | 'exceptional';

export interface ForecastHour {
  time: string;
  condition: Condition | string;
  temperature?: number;
  precipitation?: number;
  precipProb?: number;
  windSpeed?: number;
}

export interface ForecastDay {
  date: string;
  condition: Condition | string;
  max?: number;
  min?: number;
  precipitation?: number;
  precipProb?: number;
  windSpeed?: number;
  windBearing?: number;
  humidity?: number;
  uv?: number;
}

export interface Weather {
  available: boolean;
  condition?: Condition | string;
  night: boolean;
  temperature?: number;
  feels?: number;
  humidity?: number;
  pressure?: number;
  windSpeed?: number;
  windBearing?: number;
  windGust?: number;
  uv?: number;
  visibility?: number;
  cloudCover?: number;
  dewPoint?: number;
  min?: number;
  max?: number;
  sunrise?: string;
  sunset?: string;
  hourly: ForecastHour[];
  daily: ForecastDay[];
  place?: string;
  updatedAt: string;
}

/** Outdoor air quality: ATMO-style index 1 (good) … 6 (extremely poor). */
export interface Air {
  available: boolean;
  index: number; // 0 = unknown
  label: string;
  dominant?: PollutantKey;
  tomorrow?: number;
  pollutants: Pollutant[];
  pollens: Pollen[];
  place?: string;
  source: string;
  updatedAt: string;
}
export type PollutantKey = 'pm25' | 'pm10' | 'no2' | 'o3' | 'so2';
export interface Pollutant { key: PollutantKey; index: number; value: number }
export type PollenKey = 'grass' | 'birch' | 'alder' | 'mugwort' | 'olive' | 'ragweed';
export interface Pollen { key: PollenKey; level: number; value: number }

/** Shared calendar: events of the coming weeks from HA calendar entities. */
export interface Agenda {
  available: boolean;
  calendars: Calendar[];
  events: CalEvent[];
  from: string;
  to: string;
  updatedAt: string;
}
export interface Calendar { id: string; name: string; color?: string }
export interface CalEvent { id: string; calendar: string; title: string; start: string; end: string; allDay: boolean; location?: string }

export interface Home {
  name: string;
  rooms: Room[];
  globals: Globals;
  weather?: Weather;
  air?: Air;
  agenda?: Agenda;
  presence: Person[];
  alerts: Alert[];
  connected: boolean;
  demo: boolean;
  updatedAt: string;
}

export interface Summary {
  rooms: Record<string, RoomSummary>;
  globals: Globals;
  presence: Person[];
  alerts: Alert[];
  weather?: Weather;
}

export interface Status { connected: boolean; message?: string }

export type Event =
  | { type: 'home'; home: Home; at: string }
  | { type: 'device'; device: Device; at: string }
  | { type: 'summary'; summary: Summary; at: string }
  | { type: 'status'; status: Status; at: string };

// Actions (étape 2) — the shapes the server will accept.
export type DeviceAction =
  | { action: 'set_target'; params: { temperature: number } }
  | { action: 'set_mode'; params: { mode: HeatingMode; duration?: 'next_block' | 60 | 180 | 'forever' } }
  | { action: 'power'; params: { on: boolean } }
  | { action: 'set_brightness'; params: { pct: number } }
  | { action: 'set_color_temp'; params: { kelvin: number } }
  | { action: 'set_color'; params: { hs: HS } }
  | { action: 'open' | 'close' | 'stop'; params?: undefined }
  | { action: 'set_position'; params: { pct: number } }
  | { action: 'play' | 'pause'; params?: undefined }
  | { action: 'set_volume'; params: { pct: number } }
  | { action: 'set_privacy'; params: { on: boolean } }
  | { action: 'set_eco' | 'set_boost'; params: { on: boolean } };

export type HouseHeatingMode = 'schedule' | 'night' | 'off' | 'boost';

export interface ApiError { error: string; detail?: string }

/** Finds a device by id in a Home. */
export function findDevice(home: Home, id: string): Device | undefined {
  for (const r of home.rooms) for (const d of r.devices) if (d.id === id) return d;
  return undefined;
}

/** Applies one SSE event to a Home (immutably). */
export function applyEvent(home: Home, ev: Event): Home {
  switch (ev.type) {
    case 'home':
      return ev.home;
    case 'device': {
      const d = ev.device;
      return {
        ...home,
        rooms: home.rooms.map((r) => (r.id === d.roomId ? { ...r, devices: r.devices.map((x) => (x.id === d.id ? d : x)) } : r)),
        updatedAt: ev.at,
      };
    }
    case 'summary': {
      const s = ev.summary;
      return {
        ...home,
        rooms: home.rooms.map((r) => (s.rooms[r.id] ? { ...r, summary: s.rooms[r.id] } : r)),
        globals: s.globals,
        presence: s.presence,
        alerts: s.alerts,
        weather: s.weather ?? home.weather,
        updatedAt: ev.at,
      };
    }
    case 'status':
      return { ...home, connected: ev.status.connected, updatedAt: ev.at };
  }
}

/** One line of the server's action journal (who did what, when). */
export type JournalEntry = {
  at: string;
  user: string;
  via?: string;
  scope: 'device' | 'house' | 'security' | 'push';
  target?: string;
  action: string;
  params?: string;
  text: string;
  ok: boolean;
  error?: string;
};
