import type { Home } from '@maison/model';
import { Card, CardHead, Chip, Hero, Pict, Row, Status, Tag, Toggle, Who, plural, relative } from '@maison/ui';
import type { IconName } from '@maison/ui';
import { cameras, openings, ROOM_AMBIENCE, roomImage } from '../lib/derive';
import { snapshotURL, useJournal, useTick } from '../lib/api';
import { useState } from 'react';
import type { JournalEntry } from '@maison/model';
import { useActions, usePending } from '../lib/actions';
import { useConfirm } from '../lib/confirm';

export function SecurityPage({ home, now, stale }: { home: Home; now: Date; stale: boolean }) {
  const g = home.globals;
  const tick = useTick(10);
  const cams = cameras(home);
  const opens = openings(home);
  const smokes = home.rooms.flatMap((r) => r.devices.filter((d) => d.kind === 'smoke').map((d) => ({ room: r, d })));
  const motions = home.rooms.flatMap((r) => r.devices.filter((d) => d.kind === 'motion' || (d.kind === 'camera' && d.state.motion != null)).map((d) => ({ room: r, d })));
  const openNow = opens.filter((o) => o.device.state.open);
  const unifi = g.alarm.unifiState;
  const act = useActions();
  const pending = usePending();
  const [confirmEl, ask] = useConfirm();
  const setAlarm = (armed: boolean) => {
    const run = () => act.house('/api/security/alarm', { armed }, (h) => ({ ...h, globals: { ...h.globals, alarm: { ...h.globals.alarm, armed } } }));
    if (!armed) ask({ icon: 'shield', tone: 'warn', title: 'Désarmer l’alarme ?', detail: 'Les capteurs et les caméras ne déclencheront plus rien jusqu’au réarmement.', confirmLabel: 'Désarmer', danger: true, run });
    else void run();
  };
  const firstCam = cams[0]?.device;

  return (
    <div className="page -security">
      <h1 className="sr-only">Sécurité</h1>
      <Card hero stale={stale} crit={g.alarm.triggered} className="sec-alarm">
        <CardHead pict={<Pict icon={g.alarm.armed ? 'shield-check' : 'shield'} size="l" state={g.alarm.triggered ? 'crit' : g.alarm.armed ? 'ok' : 'off'} />} title="Alarme"
          sub={!g.alarm.available ? 'Indisponible' : g.alarm.triggered ? `Déclenchée${g.alarm.acked ? ' · vue' : ''}` : g.alarm.armed ? 'Armée · capteurs et caméras actifs' : 'Désarmée'}
          right={g.alarm.available ? <Toggle on={g.alarm.armed} label={g.alarm.armed ? 'Alarme armée' : 'Alarme désarmée'} onChange={setAlarm} /> : undefined} />
        <div className="list">
          <Row pict={<Pict icon="window" size="s" state={openNow.length ? 'warn' : 'ok'} />} name="Ouvrants" sub={plural(opens.length, 'capteur', 'capteurs')} right={<Status small tone={openNow.length ? 'warn' : 'ok'}>{openNow.length === 0 ? 'Tout fermé' : plural(openNow.length, 'ouvert', 'ouverts')}</Status>} />
          {unifi && <Row pict={<Pict icon="shield" size="s" />} name="Alarme UniFi" sub="Lecture seule" right={<Status small tone={unifi.startsWith('armed') ? 'ok' : unifi === 'triggered' ? 'crit' : undefined}>{{ armed_away: 'Armée (absent)', armed_home: 'Armée (maison)', disarmed: 'Désarmée', triggered: 'Déclenchée', pending: 'En attente', arming: 'Armement' }[unifi] ?? unifi}</Status>} />}
          {g.alarm.triggered && !g.alarm.acked && <Row pict={<Pict icon="bell" size="s" state="crit" />} name="Alerte en cours" sub="Marquer comme vue" right={<button type="button" className="btn -danger" style={{ height: 36 }} onClick={() => act.house('/api/security/alarm/ack', {}, (h) => ({ ...h, globals: { ...h.globals, alarm: { ...h.globals.alarm, acked: true } } }))}>Vu</button>} />}
          {g.camerasPrivate != null && <Row pict={<Pict icon="eye-off" size="s" state={g.camerasPrivate ? 'on' : 'off'} />} name="Mode privé" sub={plural(cams.length, 'caméra', 'caméras')} right={<Toggle small on={g.camerasPrivate} pending={!!firstCam && !!pending[firstCam.id]} label={g.camerasPrivate ? 'Mode privé actif' : 'Mode privé inactif'} onChange={(on) => { if (firstCam) act.device(firstCam, { action: 'set_privacy', params: { on } }).catch(() => {}); }} />} />}
          {smokes.map(({ room, d }) => <Row key={d.id} href={`/pieces/${room.id}`} pict={<Pict icon="smoke" size="s" state={!d.available ? 'unknown' : d.state.smoke || d.state.co ? 'crit' : 'ok'} />} name={d.name} sub={`${room.name} · fumée et CO`} right={<Status small tone={!d.available ? 'unknown' : d.state.smoke || d.state.co ? 'crit' : 'ok'}>{!d.available ? 'Injoignable' : d.state.smoke ? 'Fumée' : d.state.co ? 'CO' : 'OK'}</Status>} />)}
        </div>
        {home.presence.length > 0 && (
          <div className="card-foot" style={{ display: 'flex', gap: 'var(--sp-3)', flexWrap: 'wrap' }}>
            {home.presence.map((p) => <Who key={p.id} person={p} />)}
          </div>
        )}
      </Card>

      <div className="sec-cams">
        {cams.map(({ room, device }) => (
          <Hero key={device.id} wide
            image={device.available && !device.state.privacy ? snapshotURL(device.id, tick) : roomImage(room)}
            ambience={ROOM_AMBIENCE[room.id]}
            live={device.available && !device.state.privacy}
            state={device.state.privacy ? 'private' : !device.available ? 'offline' : undefined}
            center={device.state.privacy ? { icon: 'eye-off', text: 'Mode privé' } : !device.available ? { icon: 'wifi-off', text: 'Injoignable' } : undefined}
            chips={device.state.motion != null ? <Chip size="s" onPhoto icon="motion">{device.state.motion ? 'Mouvement' : 'Calme'}</Chip> : undefined}
            pict="camera" title={room.name} sub={device.available ? relative(device.updatedAt, now) : ''} href={`/pieces/${room.id}`} />
        ))}
      </div>

      <Card tight className="sec-opens">
        <CardHead small pict={<Pict icon="window" state={openNow.length ? 'warn' : 'off'} />} title="Ouvrants" sub={openNow.length === 0 ? 'Tout est fermé' : plural(openNow.length, 'ouvert', 'ouverts')} />
        <div className="list -cols">
          {opens.map(({ room, device: d }) => (
            <Row key={d.id} href={`/pieces/${room.id}`} pict={<Pict icon={d.state.open ? 'window-open' : d.name.toLowerCase().includes('porte') ? 'door' : 'window'} size="s" state={!d.available ? 'unknown' : d.state.open ? 'warn' : 'off'} />} name={`${d.name} · ${room.name.toLowerCase()}`}
              sub={!d.available ? `Injoignable ${relative(d.updatedAt, now, 'depuis')}` : d.state.open && d.state.since ? `Ouverte ${relative(d.state.since, now, 'depuis')}` : 'Fermée'}
              right={d.available ? <Status small tone={d.state.open ? 'warn' : 'ok'}>{d.state.open ? 'Ouverte' : 'Fermée'}</Status> : <Tag tone="unknown" icon="wifi-off">Injoignable</Tag>} />
          ))}
        </div>
      </Card>

      {confirmEl}
      <JournalCard home={home} now={now} />
      <Card tight className="sec-motion">
        <CardHead small pict={<Pict icon="motion" state={motions.some(({ d }) => d.state.motion) ? 'on' : 'off'} />} title="Mouvement" sub={motions.some(({ d }) => d.state.motion) ? 'Du mouvement quelque part' : 'Tout est calme'} />
        <div className="list">
          {motions.map(({ room, d }) => <Row key={d.id} href={`/pieces/${room.id}`} pict={<Pict icon="motion" size="s" state={!d.available ? 'unknown' : d.state.motion ? 'on' : 'off'} />} name={room.name} sub={d.kind === 'camera' ? 'Caméra' : d.name} right={<Status small tone={!d.available ? 'unknown' : d.state.motion ? 'accent' : undefined}>{!d.available ? '—' : d.state.motion ? 'Mouvement' : d.state.since ? `Calme ${relative(d.state.since, now, 'depuis')}` : 'Calme'}</Status>} />)}
          {motions.length === 0 && <span className="day-empty">Aucun détecteur de mouvement</span>}
        </div>
      </Card>
    </div>
  );
}

const SCOPE_ICON: Record<JournalEntry['scope'], IconName> = { device: 'power', house: 'home', security: 'shield', push: 'bell', agenda: 'calendar' };

function journalIcon(e: JournalEntry, home: Home): IconName {
  if (e.scope === 'device' && e.target) {
    const d = home.rooms.flatMap((r) => r.devices).find((x) => x.id === e.target);
    if (d) return ({ light: 'bulb', light_group: 'bulb', cover: 'blind', plug: 'plug', heating: 'flame', ac: 'fan', media: 'speaker', camera: 'camera' } as Record<string, IconName>)[d.kind] ?? 'power';
  }
  if (e.scope === 'house' && e.action === 'heating') return 'flame';
  if (e.scope === 'house' && e.action === 'lights_off') return 'bulb';
  return SCOPE_ICON[e.scope] ?? 'power';
}

function journalWhere(e: JournalEntry, home: Home): string {
  if (e.scope === 'device' && e.target) {
    const room = home.rooms.find((r) => r.devices.some((d) => d.id === e.target));
    if (room) return room.name;
  }
  return { device: 'Appareil', house: 'Maison', security: 'Sécurité', push: 'Notifications', agenda: 'Agenda' }[e.scope];
}

function hhmm(iso: string, now: Date): string {
  const d = new Date(iso);
  const sameDay = d.toDateString() === now.toDateString();
  const t = d.toLocaleTimeString('fr-FR', { hour: '2-digit', minute: '2-digit' });
  if (sameDay) return t;
  const y = new Date(now); y.setDate(y.getDate() - 1);
  if (d.toDateString() === y.toDateString()) return `hier ${t}`;
  return `${d.toLocaleDateString('fr-FR', { day: '2-digit', month: '2-digit' })} ${t}`;
}

/** Who did what, when — the server's action journal (last entries, newest first). */
function JournalCard({ home, now }: { home: Home; now: Date }) {
  const [limit, setLimit] = useState(12);
  const q = useJournal(limit);
  const entries = q.data?.entries ?? [];
  return (
    <Card tight className="sec-journal">
      <CardHead small pict={<Pict icon="clock" />} title="Journal" sub={q.data && !q.data.enabled ? 'Désactivé sur ce serveur' : entries.length === 0 ? 'Aucune action pour l’instant' : `${plural(entries.length, 'dernière action', 'dernières actions')} · qui, quoi, quand`} />
      {entries.length > 0 && (
        <div className="list -cols">
          {entries.map((e, i) => (
            <Row key={`${e.at}-${i}`} pict={<Pict icon={journalIcon(e, home)} size="s" state={e.ok ? undefined : 'crit'} />} name={e.text}
              sub={[journalWhere(e, home), e.user].filter(Boolean).join(' · ') + (e.error ? ` · ${e.error}` : '')}
              right={<span className="jr-time">{hhmm(e.at, now)}</span>} />
          ))}
        </div>
      )}
      {entries.length >= limit && limit < 200 && <div className="jr-more"><button type="button" className="btn" style={{ height: 36 }} onClick={() => setLimit((l) => Math.min(200, l * 3))}>Voir plus</button></div>}
    </Card>
  );
}
