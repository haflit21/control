// Small derivations from the model, shared by the pages.
import type { Device, Home, Room } from '@maison/model';
import { HEATING_MODE_LABEL, plural, relative, temp1 } from '@maison/ui';

export const ROOM_ICON: Record<string, string> = { sofa: 'home', kitchen: 'home', door: 'door', desk: 'home', bed: 'home', crib: 'home', home: 'home' };
export const ROOM_AMBIENCE: Record<string, string> = { salon: 'salon', cuisine: 'cuisine', entree: 'exterieur', bureau: 'bureau', chambre: 'chambre', bebe: 'chambre' };

/** Product photo of a device: maison.yaml `image:` first, else a default per kind (served from media/devices). */
const DEVICE_IMAGE: Partial<Record<Device['kind'], string>> = {
  light: 'bulb', light_group: 'bulb', cover: 'blind', plug: 'plug', opening: 'window', heating: 'heating', camera: 'camera', media: 'speaker', ac: 'ac',
};
export function deviceImage(d: Device): string | undefined {
  const isLight = d.kind === 'light' || d.kind === 'light_group';
  if (isLight && !d.state.on && d.imageOff) return d.imageOff;
  if (d.image) return d.image;
  const base = DEVICE_IMAGE[d.kind];
  if (!base) return undefined;
  if (base === 'bulb') return `/media/devices/${d.state.on ? 'bulb-on' : 'bulb-off'}.jpg`;
  return `/media/devices/${base}.jpg`;
}

/** CSS colour of a light that is on: its hue/saturation, or a warm/cool white from the colour temperature.
 *  undefined for plain white (nothing to tint). */
export function lightColor(d: Device): string | undefined {
  if (!d.state.on) return undefined;
  const c = d.state.color;
  if (c && c.s >= 20) return `hsl(${Math.round(c.h)} ${Math.round(c.s)}% 55%)`;
  const k = d.state.colorTemp;
  if (k != null) {
    if (k <= 2400) return 'hsl(28 90% 55%)';
    if (k <= 3000) return undefined; // warm white: the photo already is
    if (k >= 5000) return 'hsl(210 70% 70%)';
  }
  return undefined;
}
/** Photo of a room from maison.yaml `image:` (undefined → drawn ambience). */
export function roomImage(room: Room | undefined): string | undefined {
  return room?.image || undefined;
}

export function byKind(room: Room, ...kinds: Device['kind'][]): Device[] {
  return room.devices.filter((d) => kinds.includes(d.kind));
}
export function primaryHeating(room: Room): Device | undefined {
  return room.devices.find((d) => d.kind === 'heating' && d.primary) ?? room.devices.find((d) => d.kind === 'heating');
}
export function cameras(home: Home): Array<{ room: Room; device: Device }> {
  return home.rooms.flatMap((room) => room.devices.filter((d) => d.kind === 'camera').map((device) => ({ room, device })));
}
export function openings(home: Home): Array<{ room: Room; device: Device }> {
  return home.rooms.flatMap((room) => room.devices.filter((d) => d.kind === 'opening').map((device) => ({ room, device })));
}
export function lightsOn(home: Home): Array<{ room: Room; device: Device }> {
  return home.rooms.flatMap((room) => room.devices.filter((d) => d.kind === 'light' && d.state.on).map((device) => ({ room, device })));
}

/** "2 pièces en chauffe · moyenne 19,8°" */
export function heatingSubtitle(home: Home): string {
  const g = home.globals.heating;
  const parts: string[] = [];
  if (!g.available) return 'Chauffage indisponible';
  parts.push(g.roomsOn === 0 ? 'Chauffage coupé' : plural(g.roomsOn, 'pièce en chauffe', 'pièces en chauffe'));
  if (g.average != null) parts.push(`moyenne ${temp1(g.average)}°`);
  return parts.join(' · ');
}

/** Short status line of a room: "Chauffe · fenêtre ouverte", "Fermé · calme" */
export function roomStatus(room: Room, now: Date): string {
  const s = room.summary;
  const parts: string[] = [];
  if (s.heating) parts.push('Chauffe');
  else if (s.heatingMode === 'off') parts.push('Chauffage à l’arrêt');
  if (s.openingsOpen > 0) {
    const o = room.devices.find((d) => d.kind === 'opening' && d.state.open);
    parts.push(`${o?.name.toLowerCase() ?? 'ouvrant'} ouverte ${o?.state.since ? relative(o.state.since, now, 'depuis') : ''}`.trim());
  } else if (room.devices.some((d) => d.kind === 'opening' && d.available)) parts.push('fermé');
  if (s.lightsOn > 0) parts.push(plural(s.lightsOn, 'lumière', 'lumières'));
  if (s.unavailable > 0) parts.push(plural(s.unavailable, 'appareil injoignable', 'appareils injoignables'));
  const txt = parts.join(' · ');
  return txt ? txt.charAt(0).toUpperCase() + txt.slice(1) : '—';
}

export function heatingModeLabel(mode: string | undefined, until?: string, now?: Date): string {
  if (!mode) return '—';
  const base = HEATING_MODE_LABEL[mode] ?? mode;
  if (mode === 'manual' && until && now) {
    if (until === 'forever') return `${base} · sans fin`;
    return `${base} · jusqu'à ${new Date(until).toLocaleTimeString('fr-FR', { hour: '2-digit', minute: '2-digit' })}`;
  }
  return base;
}

/** Device sub-line for lights / plugs: "Allumée · 60 %", "Éteinte", "Allumée depuis 2 h · 86 W" */
export function deviceSub(d: Device, now: Date): string {
  const s = d.state;
  if (!d.available) return 'Injoignable';
  switch (d.kind) {
    case 'light':
    case 'light_group': {
      if (!s.on) return 'Éteinte';
      const parts = ['Allumée'];
      if (s.brightness != null) parts.push(`${s.brightness} %`);
      return parts.join(' · ');
    }
    case 'plug': {
      const parts = [s.on ? 'Allumée' : 'Éteinte'];
      if (s.on && s.since) parts[0] = `Allumée ${relative(s.since, now, 'depuis')}`;
      if (s.power != null) parts.push(`${Math.round(s.power)} W`);
      return parts.join(' · ');
    }
    case 'cover':
      if (s.moving) return s.moving === 'opening' ? 'En ouverture' : 'En fermeture';
      return s.position == null ? '—' : s.position === 0 ? 'Fermé' : s.position === 100 ? 'Ouvert' : `Ouvert à ${s.position} %`;
    case 'media':
      return s.playing === 'playing' ? `En lecture${s.volume != null ? ` · ${s.volume} %` : ''}` : s.playing === 'paused' ? 'En pause' : 'Rien en lecture';
    case 'opening':
      return s.open ? `Ouverte ${s.since ? relative(s.since, now, 'depuis') : ''}`.trim() : 'Fermée';
    case 'motion':
      return s.motion ? 'Mouvement' : `Calme${s.since ? ` ${relative(s.since, now, 'depuis')}` : ''}`;
    default:
      return '';
  }
}
