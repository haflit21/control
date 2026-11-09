import { useNavigate } from 'react-router-dom';
import type { Home } from '@maison/model';
import { Pict, Pictv, RoomTile, plural, relative } from '@maison/ui';
import { primaryHeating, ROOM_AMBIENCE, roomImage } from '../lib/derive';

/** /pieces — every room as a photo tile; the room page opens from here (and from the rail / bottom nav). */
export function RoomsPage({ home, now, stale }: { home: Home; now: Date; stale: boolean }) {
  const nav = useNavigate();
  const heating = home.rooms.filter((r) => r.summary.heating).length;
  const lights = home.rooms.reduce((n, r) => n + r.summary.lightsOn, 0);
  const sub = [heating ? plural(heating, 'pièce en chauffe', 'pièces en chauffe') : 'chauffage au repos', lights ? plural(lights, 'lumière allumée', 'lumières allumées') : 'tout est éteint'].join(' · ');
  return (
    <div className="page">
      <header className="dash-top hrow">
        <div className="greet"><h1>Pièces</h1><p>{sub.charAt(0).toUpperCase() + sub.slice(1)}</p></div>
      </header>
      <div className="rooms-grid">
        {home.rooms.map((r) => {
          const h = primaryHeating(r);
          const unavailable = h ? !h.available : r.summary.unavailable === r.devices.length;
          const openDev = r.devices.find((d) => d.kind === 'opening' && d.state.open);
          return (
            <RoomTile key={r.id} name={r.name} temperature={r.summary.temperature} ambience={ROOM_AMBIENCE[r.id]} image={roomImage(r)} stale={stale} unavailable={unavailable}
              pict={<Pict icon={unavailable ? 'wifi-off' : r.summary.heating ? 'flame' : r.summary.lightsOn > 0 ? 'bulb' : 'flame'} size="s" state={unavailable ? 'unknown' : r.summary.heating ? 'heat' : r.summary.lightsOn > 0 ? 'on' : 'off'} />}
              right={openDev ? <Pictv icon="window-open" state="warn">{openDev.state.since ? relative(openDev.state.since, now, '') : 'Ouverte'}</Pictv> : r.summary.lightsOn > 0 ? <Pictv icon="bulb" state="on">{r.summary.lightsOn}</Pictv> : r.devices.some((d) => d.kind === 'opening') ? <Pictv icon="window" /> : undefined}
              onClick={() => nav(`/pieces/${r.id}`)}
            />
          );
        })}
      </div>
    </div>
  );
}
