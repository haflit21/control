import { Link, useNavigate } from 'react-router-dom';
import { setBell } from '../lib/bell';
import type { Home } from '@control/model';
import { AgendaGlance, AirChip, AlertRow, Icon, Card, CardHead, Chip, ChipRow, Hero, Pair, Pict, Row, Status, TimeAgo, Toggle, Thumb, WhoPill, WxPill, Dots, Val, cx, dateLong, greeting, plural, temp1, relative, HEATING_MODE_LABEL } from '@control/ui';
import { cameras, heatingSubtitle, lightsOn, openings, primaryHeating, ROOM_AMBIENCE, roomStatus, roomImage, deviceSub, deviceImage, lightColor } from '../lib/derive';
import { snapshotURL, useTick } from '../lib/api';
import { useActions, usePending } from '../lib/actions';
import { useConfirm } from '../lib/confirm';

export function HomePage({ home, now, stale }: { home: Home; now: Date; stale: boolean }) {
  const nav = useNavigate();
  const tick = useTick(10);
  const act = useActions();
  const pending = usePending();
  const [confirmEl, ask] = useConfirm();
  const g = home.globals;
  const setHouseHeating = (mode: 'schedule' | 'night' | 'off' | 'boost') => {
    const run = () => act.house('/api/home/heating', { mode }, (h) => ({ ...h, globals: { ...h.globals, heating: { ...h.globals.heating, mode: mode === 'boost' ? h.globals.heating.mode : mode === 'night' ? 'manual' : mode } } }));
    if (mode === 'off') ask({ icon: 'flame', tone: 'warn', title: 'Couper tout le chauffage ?', detail: 'Toutes les pièces passent à l’arrêt jusqu’à ce que tu relances le planning.', confirmLabel: 'Couper', danger: true, run });
    else if (mode === 'boost') ask({ icon: 'bolt', tone: 'heat', title: 'Lancer un boost ?', detail: 'Toutes les pièces chauffent à fond pendant une demi-heure, puis reviennent au planning.', confirmLabel: 'Booster', run });
    else void run();
  };
  const setAlarm = (armed: boolean) => {
    const run = () => act.house('/api/security/alarm', { armed }, (h) => ({ ...h, globals: { ...h.globals, alarm: { ...h.globals.alarm, armed } } }));
    if (!armed) ask({ icon: 'shield', tone: 'warn', title: 'Désarmer l’alarme ?', detail: 'Les capteurs et les caméras ne déclencheront plus rien jusqu’au réarmement.', confirmLabel: 'Désarmer', danger: true, run });
    else void run();
  };
  const allLightsOff = () => ask({ icon: 'bulb', tone: 'off', title: 'Éteindre toutes les lumières ?', detail: plural(g.lightsOn, 'lampe allumée', 'lampes allumées') + ' dans la maison.', confirmLabel: 'Tout éteindre', run: () => act.house('/api/home/lights/off', {}, (h) => ({ ...h, rooms: h.rooms.map((r) => ({ ...r, devices: r.devices.map((d) => (d.kind === 'light' || d.kind === 'light_group' ? { ...d, state: { ...d.state, on: false } } : d)), summary: { ...r.summary, lightsOn: 0 } })), globals: { ...h.globals, lightsOn: 0 } })) });
  const firstCam = cameras(home)[0]?.device;
  const setPrivacy = (on: boolean) => { if (firstCam) act.device(firstCam, { action: 'set_privacy', params: { on } }).catch(() => {}); };
  const cams = cameras(home);
  const mainCam = cams.find((c) => c.room.id === 'salon') ?? cams[0];
  const camRoom = mainCam?.room;
  const lit = lightsOn(home);
  const opens = openings(home);
  const heatingDevices = home.rooms.map(primaryHeating).filter(Boolean);
  const power = heatingDevices.reduce<{ sum: number; n: number }>((acc, h) => (h?.state.heatingPower != null ? { sum: acc.sum + h.state.heatingPower, n: acc.n + 1 } : acc), { sum: 0, n: 0 });
  const avgPower = power.n ? Math.round(power.sum / power.n) : undefined;

  return (
    <div className="page">
      <header className="dash-top hrow">
        <div className="greet"><h1>{greeting(now)} {home.presence.find((p) => p.home)?.name ?? ''}</h1><p>{dateLong(now)}</p></div>
        <div className="dash-tools">
          <AirChip air={home.air} />
          {home.weather && <WxPill w={home.weather} now={now} href="/meteo" onClick={() => nav('/meteo')} />}
          {home.presence.length > 0 && <WhoPill people={home.presence} />}
          <button type="button" className="pict bell-phone" aria-label={`Alertes : ${home.alerts.length}`} onClick={() => setBell(true)}><Icon name="bell" />{home.alerts.length > 0 && <span className="pict-n">{home.alerts.length}</span>}</button>
        </div>
      </header>

      {home.alerts.length > 0 && (
        <div className="dash-alerts stack">
          {home.alerts.map((a) => <AlertRow key={a.id} alert={a} detail={[a.detail, a.since && a.kind !== 'battery' ? `depuis ${rel(a.since, now)}` : ''].filter(Boolean).join(' · ')} href={a.roomId ? `/pieces/${a.roomId}` : undefined} />)}
        </div>
      )}

      <div className="dash-grid home-grid">
        {mainCam && camRoom && (
          <Hero className="dash-hero" cinema
            image={mainCam.device.available && !mainCam.device.state.privacy ? snapshotURL(mainCam.device.id, tick) : roomImage(camRoom)}
            ambience={ROOM_AMBIENCE[camRoom.id]}
            live={mainCam.device.available && !mainCam.device.state.privacy}
            state={mainCam.device.state.privacy ? 'private' : !mainCam.device.available ? 'offline' : undefined}
            center={mainCam.device.state.privacy ? { icon: 'eye-off', text: 'Mode privé' } : !mainCam.device.available ? { icon: 'wifi-off', text: 'Caméra injoignable' } : undefined}
            chips={<>
              {camRoom.summary.temperature != null && <Chip size="s" onPhoto tone={camRoom.summary.heating ? 'heat' : undefined} icon="thermo">{temp1(camRoom.summary.temperature)}°</Chip>}
              {camRoom.summary.humidity != null && <Chip size="s" onPhoto icon="drop">{Math.round(camRoom.summary.humidity)}&#8239;%</Chip>}
              {mainCam.device.state.motion && <Chip size="s" onPhoto icon="motion">Mouvement</Chip>}
            </>}
            pict="camera"
            title={camRoom.name}
            sub={roomStatus(camRoom, now)}
            onClick={() => nav(`/pieces/${camRoom.id}`)}
          />
        )}

        <Card className="dash-heat">
          <CardHead pict={<Pict icon="flame" state={g.heating.roomsOn > 0 ? 'heat' : 'off'} />} title="Chauffage" sub={heatingSubtitle(home)} />
          {g.heating.available && (
            <ChipRow>
              {(['schedule', 'night', 'off', 'boost'] as const).map((m) => <button key={m} type="button" className={cx('chip', g.heating.mode === m && 'is-on')} onClick={() => setHouseHeating(m)}>{HEATING_MODE_LABEL[m]}</button>)}
            </ChipRow>
          )}
          <div className="list">
            {home.rooms.map((r) => {
              const h = primaryHeating(r);
              if (!h) return null;
              const open = r.summary.openingsOpen > 0 && r.summary.heating;
              return (
                <Row key={r.id} href={`/pieces/${r.id}`}
                  pict={<Pict icon="flame" size="s" state={!h.available ? 'unknown' : r.summary.heating ? 'heat' : 'off'} />}
                  name={r.name}
                  sub={!h.available ? 'Injoignable' : open ? <span style={{ color: 'var(--warn-fg)' }}>Fenêtre ouverte</span> : h.state.mode === 'manual' && h.state.modeUntil ? `Manuel · jusqu'à ${new Date(h.state.modeUntil).toLocaleTimeString('fr-FR', { hour: '2-digit', minute: '2-digit' })}` : undefined}
                  right={<Pair current={h.state.temperature} target={h.state.target} mode={h.state.mode} heating={r.summary.heating} stale={stale || !h.available} />}
                />
              );
            })}
          </div>
          {g.heating.average != null && (
            <div className="heat-avg">
              <div className="hrow"><span className="muted" style={{ fontSize: 'var(--fs-s)' }}>Moyenne de la maison</span><Val value={g.heating.average} unit="°" size="s" stale={stale} /></div>
              {avgPower != null && <div className="dotsline"><Dots n={14} on={Math.round((avgPower / 100) * 14)} tone="heat" label={`Puissance de chauffe ${avgPower} %`} /><Val value={avgPower} unit="%" size="meta" decimals={0} /></div>}
            </div>
          )}
          {g.heating.presence && <div className="card-foot"><span>Présence tado · <b style={{ color: 'var(--fg)', fontWeight: 500 }}>{{ home: 'Maison', away: 'Absent', auto: 'Auto' }[g.heating.presence]}</b></span><TimeAgo iso={home.updatedAt} now={now} /></div>}
        </Card>

        <div className="dash-devs">
          <Link to="/securite" style={{ textDecoration: 'none', display: 'contents' }}>
            <Card tight>
              <div className="card-head"><Pict icon={g.alarm.armed ? 'shield-check' : 'shield'} state={g.alarm.triggered ? 'crit' : g.alarm.armed ? 'ok' : 'off'} />{g.alarm.available && <Toggle on={g.alarm.armed} label={g.alarm.armed ? 'Alarme armée' : 'Alarme désarmée'} onChange={setAlarm} />}</div>
              <div style={{ display: 'grid', gap: 2 }}><span className="card-title -s">Alarme</span><span className="card-sub">{g.alarm.triggered ? 'Déclenchée' : g.alarm.armed ? 'Armée' : 'Désarmée'} · {g.openingsOpen === 0 ? 'tout est fermé' : plural(g.openingsOpen, 'ouvrant ouvert', 'ouvrants ouverts')}</span></div>
            </Card>
          </Link>
          <Card tight>
            <div className="card-head"><Pict icon="bulb" state={g.lightsOn > 0 ? 'on' : 'off'} /><Toggle on={g.lightsOn > 0} label={g.lightsOn > 0 ? 'Lumières allumées' : 'Tout est éteint'} disabled={g.lightsOn === 0} onChange={() => allLightsOff()} /></div>
            <div style={{ display: 'grid', gap: 2 }}><span className="card-title -s">Lumières</span><span className="card-sub">{g.lightsOn === 0 ? 'Tout est éteint' : plural(g.lightsOn, 'allumée', 'allumées')}</span></div>
          </Card>
          <Card tight>
            <div className="card-head"><Pict icon="camera" state={g.camerasPrivate ? 'off' : cams.length ? 'lit' : 'off'} />{cams.length > 0 && <Toggle on={!!g.camerasPrivate} pending={!!firstCam && !!pending[firstCam.id]} label={g.camerasPrivate ? 'Mode privé actif' : 'Caméras actives'} onChange={setPrivacy} />}</div>
            <div style={{ display: 'grid', gap: 2 }}><span className="card-title -s">Caméras</span><span className="card-sub">{cams.length === 0 ? 'Aucune' : g.camerasPrivate ? 'Mode privé' : plural(cams.filter((c) => c.device.available).length, 'en direct', 'en direct')}</span></div>
          </Card>
        </div>

        {home.agenda && (
          <Card tight className="dash-agenda">
            <CardHead small pict={<Pict icon="calendar" />} title="Agenda" sub={home.agenda.available ? 'Les prochains jours' : 'Calendriers injoignables'} right={<Link className="sec-link" to="/agenda">Semaine</Link>} />
            {home.agenda.available && <AgendaGlance agenda={home.agenda} now={now} max={5} />}
          </Card>
        )}

        {confirmEl}
        <div className="dash-col dash-side">
          <Card tight>
            <CardHead small pict={<Pict icon="bulb" state={lit.length ? 'on' : 'off'} />} title="Allumées" sub={lit.length === 0 ? 'Tout est éteint' : plural(lit.length, 'lumière', 'lumières')} />
            {lit.length > 0 && (
              <div className="list">
                {lit.slice(0, 6).map(({ room, device }) => {
                  const img = deviceImage(device); const tint = lightColor(device);
                  return <Row key={device.id} href={`/pieces/${room.id}`} pict={img ? <Thumb src={img} size="s" state="on" tint={tint} fallback={<Pict icon="lamp" size="s" state="lit" />} /> : <Pict icon="lamp" size="s" state="lit" />} name={device.name} sub={`${room.name} · ${deviceSub(device, now)}`} right={<Toggle small on pending={!!pending[device.id]} label={`Éteindre ${device.name}`} onChange={() => act.device(device, { action: 'power', params: { on: false } }, { quiet: true }).catch(() => {})} />} />;
                })}
              </div>
            )}
          </Card>
          <Card tight>
            <CardHead small pict={<Pict icon="window" state={g.openingsOpen > 0 ? 'warn' : 'off'} />} title="Ouvrants" sub={g.openingsOpen === 0 ? 'Tout est fermé' : plural(g.openingsOpen, 'ouvert', 'ouverts')} />
            <div className="list">{opens.map(({ room, device: d }) => <Row key={d.id} href={`/pieces/${room.id}`} pict={<Pict icon={d.state.open ? 'window-open' : 'window'} size="s" state={!d.available ? 'unknown' : d.state.open ? 'warn' : 'off'} />} name={`${d.name} · ${room.name.toLowerCase()}`} sub={!d.available ? `Injoignable ${relative(d.updatedAt, now, 'depuis')}` : d.state.open && d.state.since ? `Ouverte ${relative(d.state.since, now, 'depuis')}` : 'Fermée'} right={d.available ? <Status small tone={d.state.open ? 'warn' : 'ok'}>{d.state.open ? 'Ouverte' : 'Fermée'}</Status> : <Status small tone="unknown">—</Status>} />)}</div>
          </Card>
        </div>
      </div>
    </div>
  );
}

function rel(iso: string, now: Date): string {
  const s = (now.getTime() - new Date(iso).getTime()) / 1000;
  if (s < 60) return `${Math.round(s)} s`;
  if (s < 3600) return `${Math.round(s / 60)} min`;
  if (s < 86400) return `${Math.round(s / 3600)} h`;
  return `${Math.round(s / 86400)} j`;
}
