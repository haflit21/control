import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import type { Home } from '@control/model';
import { AgendaGlance, AlertRow, Card, CardHead, Chip, Hero, Icon, Pair, Pict, Row, Status, Thumb, Toggle, WhoPill, WxPill, cx, plural, relative, HEATING_MODE_LABEL } from '@control/ui';
import { cameras, deviceImage, heatingSubtitle, lightColor, lightsOn, openings, primaryHeating, ROOM_AMBIENCE, roomImage, roomStatus } from '../lib/derive';
import { snapshotURL, useTick } from '../lib/api';
import { useActions, usePending } from '../lib/actions';
import { useConfirm } from '../lib/confirm';

/** /mur — the wall tablet: no chrome, big clock, cameras in rotation, wide touch targets. */
export function WallPage({ home, now, stale }: { home: Home; now: Date; stale: boolean }) {
  const nav = useNavigate();
  const tick = useTick(10);
  const act = useActions();
  const pending = usePending();
  const [confirmEl, ask] = useConfirm();
  const g = home.globals;
  const cams = cameras(home).filter((c) => c.device.available && !c.device.state.privacy);
  const [camIdx, setCamIdx] = useState(0);
  useEffect(() => {
    if (cams.length < 2) return;
    const t = setInterval(() => setCamIdx((i) => (i + 1) % cams.length), 20_000);
    return () => clearInterval(t);
  }, [cams.length]);
  const cam = cams[camIdx % Math.max(1, cams.length)];
  const [clock, setClock] = useState(() => new Date());
  useEffect(() => { const t = setInterval(() => setClock(new Date()), 1000); return () => clearInterval(t); }, []);
  // keep the screen awake while the page is shown
  useEffect(() => {
    let lock: { release: () => Promise<void> } | undefined;
    const req = async () => { try { lock = await (navigator as Navigator & { wakeLock?: { request: (t: 'screen') => Promise<{ release: () => Promise<void> }> } }).wakeLock?.request('screen'); } catch { /* not allowed */ } };
    void req();
    const onVis = () => { if (document.visibilityState === 'visible') void req(); };
    document.addEventListener('visibilitychange', onVis);
    return () => { document.removeEventListener('visibilitychange', onVis); void lock?.release(); };
  }, []);
  const night = home.weather?.night ?? (clock.getHours() >= 23 || clock.getHours() < 6);
  const lit = lightsOn(home);
  const opens = openings(home);
  const hh = clock.getHours().toString().padStart(2, '0');
  const mm = clock.getMinutes().toString().padStart(2, '0');

  const setAlarm = (armed: boolean) => {
    const run = () => act.house('/api/security/alarm', { armed }, (h) => ({ ...h, globals: { ...h.globals, alarm: { ...h.globals.alarm, armed } } }));
    if (!armed) ask({ icon: 'shield', tone: 'warn', title: 'Désarmer l’alarme ?', detail: 'Les capteurs et les caméras ne déclencheront plus rien jusqu’au réarmement.', confirmLabel: 'Désarmer', danger: true, run });
    else void run();
  };
  const allOff = () => ask({ icon: 'bulb', tone: 'off', title: 'Éteindre toutes les lumières ?', detail: plural(g.lightsOn, 'lampe allumée', 'lampes allumées') + '.', confirmLabel: 'Tout éteindre', run: () => act.house('/api/home/lights/off', {}, (h) => ({ ...h, globals: { ...h.globals, lightsOn: 0 } })) });

  return (
    <div className={cx('wall', night && 'is-night')}>
      <h1 className="sr-only">Control · tableau mural</h1>
      <header className="wall-top">
        <div>
          <div className="wall-clock" aria-label={`${hh} h ${mm}`}>{hh}<small>:</small>{mm}</div>
          <div className="wall-date">{clock.toLocaleDateString('fr-FR', { weekday: 'long', day: 'numeric', month: 'long' })}</div>
        </div>
        <div className="wall-alerts">
          {home.alerts.slice(0, 2).map((a) => <AlertRow key={a.id} alert={a} detail={[a.detail, a.since && a.kind !== 'battery' ? `depuis ${relative(a.since, now, '')}` : ''].filter(Boolean).join(' · ')} onClick={a.roomId ? () => nav(`/pieces/${a.roomId}`) : undefined} />)}
        </div>
        <div className="wall-tools">
          {home.weather && <WxPill w={home.weather} now={now} onClick={() => nav('/meteo')} href="/meteo" />}
          {home.presence.length > 0 && <WhoPill people={home.presence} />}
          <button type="button" className="pict -l" aria-label="Ouvrir l’application" onClick={() => nav('/')} style={{ cursor: 'pointer' }}><Icon name="expand" /></button>
        </div>
      </header>

      <div className="wall-grid">
        <Card className="wall-heat">
          <CardHead pict={<Pict icon="flame" state={g.heating.roomsOn > 0 ? 'heat' : 'off'} />} title="Chauffage" sub={heatingSubtitle(home)} right={g.heating.mode && g.heating.mode !== 'mixed' ? <Status small tone={g.heating.mode === 'off' ? undefined : 'ok'}>{HEATING_MODE_LABEL[g.heating.mode] ?? g.heating.mode}</Status> : undefined} />
          <div className="list">
            {home.rooms.map((r) => {
              const h = primaryHeating(r);
              if (!h) return null;
              return <Row key={r.id} onClick={() => nav(`/pieces/${r.id}`)} pict={<Pict icon="flame" size="s" state={!h.available ? 'unknown' : r.summary.heating ? 'heat' : 'off'} />} name={r.name} sub={r.summary.openingsOpen > 0 && r.summary.heating ? <span style={{ color: 'var(--warn-fg)' }}>Fenêtre ouverte</span> : undefined} right={<Pair current={h.state.temperature} target={h.state.target} mode={h.state.mode} heating={r.summary.heating} stale={stale || !h.available} />} />;
            })}
          </div>
          <div className="card-foot" style={{ justifyContent: 'space-between' }}>
            <span>Alarme</span>
            <span className="hrow" style={{ gap: 'var(--sp-3)' }}><span className="muted">{g.alarm.armed ? 'Armée' : 'Désarmée'}</span>{g.alarm.available && <Toggle on={g.alarm.armed} label={g.alarm.armed ? 'Désarmer' : 'Armer'} onChange={setAlarm} />}</span>
          </div>
        </Card>

        <div className="wall-cam">
          {cam ? (
            <Hero cinema image={snapshotURL(cam.device.id, tick)} ambience={ROOM_AMBIENCE[cam.room.id]} live
              chips={<>
                {cam.room.summary.temperature != null && <Chip size="s" onPhoto tone={cam.room.summary.heating ? 'heat' : undefined} icon="thermo">{cam.room.summary.temperature.toLocaleString('fr-FR')}°</Chip>}
                {cam.device.state.motion && <Chip size="s" onPhoto icon="motion">Mouvement</Chip>}
              </>}
              pict="camera" title={cam.room.name} sub={roomStatus(cam.room, now)} onClick={() => nav(`/pieces/${cam.room.id}`)}
              overlay={cams.length > 1 ? <span className="wall-dots" aria-hidden="true" style={{ position: 'absolute', top: 22, left: 92 }}>{cams.map((c, i) => <i key={c.device.id} className={cx(i === camIdx % cams.length && 'is-on')} />)}</span> : undefined} />
          ) : (
            <Hero cinema image={roomImage(home.rooms[0])} ambience={ROOM_AMBIENCE[home.rooms[0]?.id]} pict="home" title={home.name} sub={g.camerasPrivate ? 'Caméras en mode privé' : 'Aucune caméra'} state={g.camerasPrivate ? 'private' : undefined} />
          )}
        </div>

        <div className="wall-lights">
          {lit.length === 0 && <span className="wall-light -off"><Pict icon="bulb" size="s" />Tout est éteint</span>}
          {lit.map(({ room, device }) => {
            const img = deviceImage(device); const tint = lightColor(device);
            return (
              <button key={device.id} type="button" className={cx('wall-light', !!pending[device.id] && 'is-pending')} onClick={() => act.device(device, { action: 'power', params: { on: false } }, { quiet: true }).catch(() => {})} aria-label={`Éteindre ${device.name}`}>
                {img ? <Thumb src={img} state="on" tint={tint} fallback={<Pict icon="lamp" size="s" state="lit" />} /> : <Pict icon="lamp" size="s" state="lit" />}
                <span>{device.name} <small>· {room.name}{device.state.brightness != null ? ` · ${Math.round(device.state.brightness)} %` : ''}</small></span>
              </button>
            );
          })}
          {lit.length > 1 && <button type="button" className="wall-light" onClick={allOff}><Pict icon="power" size="s" />Tout éteindre</button>}
        </div>

        <Card tight className="wall-agenda">
          <CardHead small pict={<Pict icon="calendar" />} title="Agenda" sub={home.agenda?.available ? 'Les prochains jours' : 'Pas de calendrier'} />
          {home.agenda?.available && <AgendaGlance agenda={home.agenda} now={now} max={6} />}
        </Card>

        <Card tight className="wall-opens">
          <CardHead small pict={<Pict icon="window" state={g.openingsOpen > 0 ? 'warn' : 'off'} />} title="Ouvrants" sub={g.openingsOpen === 0 ? 'Tout est fermé' : plural(g.openingsOpen, 'ouvert', 'ouverts')} />
          <div className="list">
            {opens.filter(({ device: d }) => d.state.open || !d.available).slice(0, 4).map(({ room, device: d }) => <Row key={d.id} pict={<Pict icon={d.state.open ? 'window-open' : 'window'} size="s" state={!d.available ? 'unknown' : d.state.open ? 'warn' : 'off'} />} name={`${d.name} · ${room.name.toLowerCase()}`} sub={!d.available ? 'Injoignable' : d.state.since ? `Ouverte ${relative(d.state.since, now, 'depuis')}` : 'Ouverte'} right={<Status small tone={!d.available ? 'unknown' : 'warn'}>{!d.available ? '—' : 'Ouverte'}</Status>} />)}
            {opens.every(({ device: d }) => d.available && !d.state.open) && <span className="day-empty">Toutes les fenêtres sont fermées</span>}
          </div>
        </Card>
      </div>
      {confirmEl}
    </div>
  );
}
