import { useState } from 'react';
import { Link, Navigate, useParams } from 'react-router-dom';
import type { Device, Home, Room } from '@control/model';
import { Card, CardHead, Chip, ChipRow, DevCard, Dots, LightPanel, Pair, PhotoButton, Pict, Popover, Row, RoomBanner, Section, Seg, Slider, Status, Stepper, Tag, Thumb, Toggle, Val, AC_MODE_LABEL, plural } from '@control/ui';
import { byKind, deviceImage, deviceSub, heatingModeLabel, lightColor, ROOM_AMBIENCE, roomImage, roomStatus } from '../lib/derive';
import { cx } from '@control/ui';
import { snapshotURL, streamURL, useTick } from '../lib/api';
import { MediaCard } from './MediaCard';
import { useActions, usePending } from '../lib/actions';
import { useConfirm } from '../lib/confirm';

export function RoomPage({ home, now, stale }: { home: Home; now: Date; stale: boolean }) {
  const { id } = useParams();
  const room = home.rooms.find((r) => r.id === id);
  const tick = useTick(10);
  const act = useActions();
  const pending = usePending();
  const [confirmEl, ask] = useConfirm();
  const [openLight, setOpenLight] = useState<string | null>(null);
  if (!room) return <Navigate to="/pieces" replace />;
  const lightOpen = openLight ? room.devices.find((d) => d.id === openLight) : undefined;

  const heating = byKind(room, 'heating');
  const acs = byKind(room, 'ac');
  const lights = byKind(room, 'light', 'light_group');
  const covers = byKind(room, 'cover');
  const plugs = byKind(room, 'plug');
  const medias = byKind(room, 'media');
  const cams = byKind(room, 'camera');
  const sensors = byKind(room, 'opening', 'motion', 'smoke', 'climate_sensor');
  const cam = cams[0];

  return (
    <div className="page -room">
      <h1 className="sr-only">{room.name}</h1>
      <Seg items={[{ id: '_all', label: 'Pièces' }, ...home.rooms.map((r) => ({ id: r.id, label: r.name, badge: r.summary.openingsOpen > 0 && r.summary.heating ? '!' : r.summary.lightsOn > 0 ? String(r.summary.lightsOn) : undefined }))]} active={room.id}
        renderLink={(it, cls, children) => <Link to={it.id === '_all' ? '/pieces' : `/pieces/${it.id}`} className={cls} role="tab" aria-selected={it.id === room.id}>{children}</Link>} />

      <RoomHero room={room} cam={cam} now={now} tick={tick} act={act} pending={!!(cam && pending[cam.id])} />

      <div className="room-main">
      {heating.map((h) => <HeatingCard key={h.id} d={h} room={room} now={now} stale={stale} act={act} pending={!!pending[h.id]} ask={ask} />)}

      {acs.map((ac) => (
        <Card key={ac.id} tight unavailable={!ac.available}>
          <CardHead small pict={deviceImage(ac) ? <Thumb src={deviceImage(ac)!} state={!ac.available ? 'unknown' : ac.state.on ? 'on' : 'off'} fallback={<Pict icon="wind" state={ac.state.on ? 'cool' : 'off'} />} /> : <Pict icon="wind" state={ac.state.on ? 'cool' : 'off'} />} title={ac.name}
            sub={!ac.available ? 'Injoignable' : ac.state.on ? `${AC_MODE_LABEL[ac.state.mode ?? 'off']} · ${ac.state.target != null ? `${ac.state.target.toLocaleString('fr-FR')}°` : ''}${ac.state.power != null ? ` · ${Math.round(ac.state.power)} W` : ''}` : `Arrêt${ac.state.power != null ? ` · ${Math.round(ac.state.power)} W` : ''}`}
            right={<Toggle on={!!ac.state.on} pending={!!pending[ac.id]} disabled={!ac.available} label={ac.state.on ? 'Climatisation allumée' : 'Climatisation éteinte'} onChange={(on) => act.device(ac, { action: 'power', params: { on } }).catch(() => {})} />} />
          {ac.state.on && (
            <div className="hrow">
              <Stepper value={ac.state.target} min={16} max={30} unit="°" label="Consigne de climatisation" pending={!!pending[ac.id]} onCommit={(t) => act.device(ac, { action: 'set_target', params: { temperature: t } }).catch(() => {})} />
              <ChipRow>{ac.state.eco && <Tag tone="cool" icon="leaf">Eco</Tag>}{ac.state.boost && <Tag tone="cool" icon="bolt">Puissance</Tag>}</ChipRow>
            </div>
          )}
        </Card>
      ))}

      {sensors.length > 0 && (
        <Section title="Capteurs">
          <Card tight>
            <div className="list sensor-list">
              {sensors.map((s) => <SensorRow key={s.id} d={s} now={now} />)}
              {heating.filter((h) => h.battery).map((h) => <Row key={h.id + '.bat'} pict={<Pict icon={h.battery === 'low' ? 'battery-low' : 'battery'} size="s" state={h.battery === 'low' ? 'warn' : 'off'} />} name={`Pile · ${h.name.toLowerCase()}`} sub={h.battery === 'low' ? 'À remplacer' : 'OK'} right={<Status small tone={h.battery === 'low' ? 'warn' : 'ok'}>{h.battery === 'low' ? 'Faible' : 'OK'}</Status>} />)}
            </div>
          </Card>
        </Section>
      )}
      </div>
      <div className="room-side">
      {medias.length > 0 && (
        <Section title="Médias">
          <div className="stack">
            {medias.map((m) => <MediaCard key={m.id} d={m} media={home.media} act={act} pending={!!pending[m.id]} />)}
          </div>
        </Section>
      )}

      {lights.length > 0 && (
        <Section title="Lumières" count={room.summary.lightsOn === 0 ? 'tout est éteint' : plural(room.summary.lightsOn, 'allumée', 'allumées')}
          right={<Toggle small on={room.summary.lightsOn > 0} label="Lumières de la pièce" onChange={(on) => { lights.filter((l) => l.available && l.kind === 'light' && !!l.state.on !== on).forEach((l) => act.device(l, { action: 'power', params: { on } }, { quiet: true }).catch(() => {})); }} />}>
          <div className="dev-grid">
            {lights.map((l) => <DevCard key={l.id} title={l.name} sub={deviceSub(l, now)} on={!!l.state.on} offline={!l.available} image={deviceImage(l)} tint={lightColor(l)} object={l.kind === 'light_group' ? 'shade' : 'lamp'}
              pending={!!pending[l.id]} onPower={() => act.device(l, { action: 'power', params: { on: !l.state.on } }, { quiet: true }).catch(() => {})}
              onClick={l.available ? () => setOpenLight(l.id) : undefined} />)}
          </div>
        </Section>
      )}

      {covers.length > 0 && (
        <Section title="Volets" count={covers.every((c) => c.available && c.state.position === 0) ? 'tout est fermé' : undefined}>
          <div className="dev-grid">
            {covers.map((c) => <DevCard key={c.id} title={c.name} sub={deviceSub(c, now)} on={c.available && (c.state.position ?? 0) > 0} offline={!c.available} image={deviceImage(c)} icon="blind" object="shade"
              pending={!!pending[c.id]} onPower={() => act.device(c, { action: (c.state.position ?? 0) > 0 ? 'close' : 'open' }).catch(() => {})} />)}
          </div>
          {covers.some((c) => c.available) && (
            <Card tight style={{ marginTop: 'var(--sp-3)' }}>
              <div className="stack">
                {covers.filter((c) => c.available).map((c) => (
                  <Slider key={c.id} label={c.name} value={c.state.position} ends={['Fermé', 'Ouvert']} pending={!!pending[c.id]} onCommit={(pct) => act.device(c, { action: 'set_position', params: { pct } }).catch(() => {})} />
                ))}
              </div>
            </Card>
          )}
        </Section>
      )}

      {plugs.length > 0 && (
        <Section title="Prises">
          <div className="dev-grid">
            {plugs.map((p) => <DevCard key={p.id} title={p.name} sub={deviceSub(p, now)} on={!!p.state.on} offline={!p.available} image={deviceImage(p)} icon="plug" pending={!!pending[p.id]} onPower={() => act.device(p, { action: 'power', params: { on: !p.state.on } }).catch(() => {})} />)}
          </div>
        </Section>
      )}


      </div>
      {confirmEl}
      {lightOpen && (
        <Popover onClose={() => setOpenLight(null)} label={`Réglages ${lightOpen.name}`}>
          <LightPanel d={lightOpen} pending={!!pending[lightOpen.id]}
            onPower={(on) => act.device(lightOpen, { action: 'power', params: { on } }, { quiet: true }).catch(() => {})}
            onBrightness={(pct) => act.device(lightOpen, { action: 'set_brightness', params: { pct } }, { quiet: true }).catch(() => {})}
            onKelvin={(kelvin) => act.device(lightOpen, { action: 'set_color_temp', params: { kelvin } }, { quiet: true }).catch(() => {})}
            onColor={(hs) => act.device(lightOpen, { action: 'set_color', params: { hs } }, { quiet: true }).catch(() => {})} />
        </Popover>
      )}
    </div>
  );
}

const DURATIONS: Array<{ id: 'next_block' | 60 | 180 | 'forever'; label: string }> = [{ id: 'next_block', label: 'Prochain bloc' }, { id: 60, label: '1 h' }, { id: 180, label: '3 h' }, { id: 'forever', label: 'Sans fin' }];

function HeatingCard({ d, room, now, stale, act, pending, ask }: { d: Device; room: Room; now: Date; stale: boolean; act: ReturnType<typeof useActions>; pending: boolean; ask: ReturnType<typeof useConfirm>[1] }) {
  const s = d.state;
  const open = room.devices.find((x) => x.kind === 'opening' && x.state.open);
  const [duration, setDuration] = useState<'next_block' | 60 | 180 | 'forever'>('next_block');
  const setMode = (mode: 'schedule' | 'manual' | 'off') => {
    const run = () => act.device(d, { action: 'set_mode', params: mode === 'manual' ? { mode, duration } : { mode } }).catch(() => {});
    if (mode === 'off') ask({ icon: 'flame', tone: 'warn', title: `Couper le chauffage ${room.name.toLowerCase() === 'entrée' ? "de l'entrée" : 'du ' + room.name.toLowerCase()} ?`, detail: 'La pièce ne sera plus chauffée tant que le mode reste sur Arrêt.', confirmLabel: 'Couper', danger: true, run });
    else void run();
  };
  return (
    <Card hero unavailable={!d.available} stale={stale}>
      <CardHead pict={deviceImage(d) ? <Thumb src={deviceImage(d)!} state={!d.available ? 'unknown' : s.heating ? 'on' : 'off'} fallback={<Pict icon="flame" state={!d.available ? 'unknown' : s.heating ? 'heat' : 'off'} />} /> : <Pict icon="flame" state={!d.available ? 'unknown' : s.heating ? 'heat' : 'off'} />} title={d.name}
        sub={!d.available ? `Injoignable · dernière valeur ${new Date(d.updatedAt).toLocaleTimeString('fr-FR', { hour: '2-digit', minute: '2-digit' })}` : heatingModeLabel(s.mode, s.modeUntil, now)}
        right={d.available && s.heating ? <Tag tone="heat" icon="flame">Chauffe</Tag> : undefined} />
      <Pair wide current={s.temperature} target={s.target} mode={s.mode} heating={!!s.heating} stale={stale || !d.available} />
      {d.available && (
        <div className="heat-ctl">
          <Stepper value={s.mode === 'off' ? undefined : s.target} fallback={s.temperature != null ? Math.round(s.temperature * 2) / 2 : 19} min={5} max={25} step={0.5} unit="°" heat large label="Consigne" pending={pending} onCommit={(t) => act.device(d, { action: 'set_target', params: { temperature: t } }).catch(() => {})} />
          <div className="stack" style={{ gap: 'var(--sp-2)' }}>
            <Seg items={[{ id: 'schedule', label: 'Planning' }, { id: 'manual', label: 'Manuel' }, { id: 'off', label: 'Arrêt' }]} active={s.mode ?? 'schedule'} onSelect={(m) => setMode(m as 'schedule' | 'manual' | 'off')} />
            {s.mode === 'manual' && <ChipRow>{DURATIONS.map((o) => <button key={String(o.id)} type="button" className={cx('chip', duration === o.id && 'is-on')} onClick={() => { setDuration(o.id); act.device(d, { action: 'set_mode', params: { mode: 'manual', duration: o.id } }, { quiet: true }).catch(() => {}); }}>{o.label}</button>)}</ChipRow>}
          </div>
        </div>
      )}
      {s.heatingPower != null && (
        <div className="dotsline">
          <Dots n={16} on={Math.round((s.heatingPower / 100) * 16)} tone="heat" large label={`Puissance de chauffe ${Math.round(s.heatingPower)} %`} />
          <Val value={s.heatingPower} unit="%" size="meta" decimals={0} />
        </div>
      )}
      <ChipRow>
        {s.humidity != null && <Chip size="s" icon="drop">{Math.round(s.humidity)}&#8239;%</Chip>}
        {s.moldRisk != null && <Chip size="s" tone={s.moldRisk ? 'warn' : 'ok'} icon={s.moldRisk ? 'alert' : 'check'}>{s.moldRisk ? 'Risque de moisissure' : 'Pas de moisissure'}</Chip>}
        {open ? <Chip size="s" tone="warn" icon="window-open">{open.name} ouverte</Chip> : room.devices.some((x) => x.kind === 'opening' && x.available) ? <Chip size="s" icon="window">Fermée</Chip> : null}
        {d.battery === 'low' && <Chip size="s" tone="warn" icon="battery-low">Pile faible</Chip>}
      </ChipRow>
    </Card>
  );
}

function SensorRow({ d, now }: { d: Device; now: Date }) {
  const s = d.state;
  switch (d.kind) {
    case 'opening':
      return <Row pict={deviceImage(d) ? <Thumb src={deviceImage(d)!} size="s" state={!d.available ? 'unknown' : s.open ? 'on' : 'off'} fallback={<Pict icon={s.open ? 'window-open' : 'window'} size="s" state={!d.available ? 'unknown' : s.open ? 'warn' : 'off'} />} /> : <Pict icon={s.open ? 'window-open' : 'window'} size="s" state={!d.available ? 'unknown' : s.open ? 'warn' : 'off'} />} name={d.name} sub={deviceSub(d, now)} right={d.available ? <Status small tone={s.open ? 'warn' : 'ok'}>{s.open ? 'Ouverte' : 'Fermée'}</Status> : <Status small tone="unknown">Injoignable</Status>} />;
    case 'motion':
      return <Row pict={<Pict icon="motion" size="s" state={!d.available ? 'unknown' : s.motion ? 'on' : 'off'} />} name={d.name} sub={deviceSub(d, now)} right={d.available ? <Status small tone={s.motion ? 'accent' : undefined}>{s.motion ? 'Mouvement' : 'Calme'}</Status> : undefined} />;
    case 'smoke': {
      const alarm = s.smoke || s.co;
      return <Row pict={<Pict icon="smoke" size="s" state={!d.available ? 'unknown' : alarm ? 'crit' : 'ok'} />} name={d.name} sub="Fumée et monoxyde de carbone" right={<Status small tone={!d.available ? 'unknown' : alarm ? 'crit' : 'ok'}>{!d.available ? 'Injoignable' : s.smoke ? 'Fumée' : s.co ? 'CO' : 'OK'}</Status>} />;
    }
    case 'climate_sensor':
      return <Row pict={<Pict icon="thermo" size="s" />} name={d.name} right={<Pair current={s.temperature} target={undefined} size="meta" />} />;
    default:
      return null;
  }
}

/**
 * Banner across both columns: the room photo, low, when the camera is off
 * (private mode, unreachable, no camera); the live camera at 16:9 when it is on.
 * The button toggles the private mode (it applies to every camera).
 */
function RoomHero({ room, cam, now, tick, act, pending }: { room: Room; cam?: Device; now: Date; tick: number; act: ReturnType<typeof useActions>; pending: boolean }) {
  const live = !!cam && cam.available && !cam.state.privacy;
  const t = room.summary.temperature;
  const chips = (
    <>
      {cam && !live && <Chip size="s" onPhoto icon={cam.state.privacy ? 'eye-off' : 'wifi-off'}>{cam.state.privacy ? 'Caméra en mode privé' : 'Caméra injoignable'}</Chip>}
      {live && cam.state.motion != null && <Chip size="s" onPhoto icon="motion">{cam.state.motion ? 'Mouvement' : 'Calme'}</Chip>}
      {t != null && <Chip size="s" onPhoto tone={room.summary.heating ? 'heat' : undefined} icon="thermo">{t.toLocaleString('fr-FR')}°</Chip>}
      {room.summary.humidity != null && <Chip size="s" onPhoto icon="drop">{Math.round(room.summary.humidity)}&#8239;%</Chip>}
    </>
  );
  const action = cam && cam.available ? (
    <PhotoButton icon={live ? 'eye-off' : 'camera'} pending={pending} label={live ? 'Activer le mode privé des caméras' : 'Allumer les caméras'}
      onClick={() => act.device(cam, { action: 'set_privacy', params: { on: live } }).catch(() => {})}>
      {live ? 'Mode privé' : 'Allumer la caméra'}
    </PhotoButton>
  ) : undefined;
  return (
    <RoomBanner title={room.name} sub={roomStatus(room, now)} ambience={ROOM_AMBIENCE[room.id]} image={roomImage(room)}
      video={live ? streamURL(cam.id) : undefined} poster={cam ? snapshotURL(cam.id, tick) : undefined} live={live} chips={chips} action={action} />
  );
}
