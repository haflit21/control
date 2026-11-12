import { Link, useNavigate } from 'react-router-dom';
import { setBell } from '../lib/bell';
import { setSettings } from '../lib/settings';
import type { Home } from '@control/model';
import { AgendaGlance, AirChip, AlertRow, Icon, Card, CardHead, ChipRow, Pair, Pict, Row, Status, TimeAgo, Toggle, WxPill, Val, cx, dateLong, greeting, plural, relative, HEATING_MODE_LABEL } from '@control/ui';
import { cameras, heatingSubtitle, openings, primaryHeating } from '../lib/derive';
import { useActions, usePending } from '../lib/actions';
import { useConfirm } from '../lib/confirm';
import { agendaFor, clearProfile, profilePerson, useProfile } from '../lib/profile';

const HOUSE_MODE_ICON = { schedule: 'calendar', night: 'moon', boost: 'rocket' } as const;

export function HomePage({ home, now, stale }: { home: Home; now: Date; stale: boolean }) {
  const nav = useNavigate();
  const act = useActions();
  const pending = usePending();
  const [confirmEl, ask] = useConfirm();
  const g = home.globals;
  const setHouseHeating = (mode: 'schedule' | 'night' | 'off' | 'boost') => {
    // the zones show the new mode at once (chip, target colours); HA confirms the values a little later
    const zone = mode === 'schedule' ? 'schedule' : mode === 'off' ? 'off' : 'manual';
    const run = () => act.house('/api/home/heating', { mode }, (h) => ({
      ...h,
      globals: { ...h.globals, heating: { ...h.globals.heating, mode: zone, preset: mode } },
      // night: which rooms heat comes from the server (night_rooms), at once
      rooms: mode === 'night' ? h.rooms : h.rooms.map((r) => ({ ...r, devices: r.devices.map((d) => (d.kind === 'heating' && d.available ? { ...d, state: { ...d.state, mode: zone, ...(mode === 'off' ? { target: undefined, heating: false } : {}) } } : d)) })),
    }));
    if (mode === 'off') ask({ icon: 'power', tone: 'warn', title: 'Couper tout le chauffage ?', detail: 'Toutes les pièces passent à l’arrêt jusqu’à ce que tu relances le planning.', confirmLabel: 'Couper', danger: true, run });
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
  const opens = openings(home);
  const heatOff = g.heating.mode === 'off';
  const preset = g.heating.preset ?? (g.heating.mode === 'schedule' ? 'schedule' : undefined);
  // who woke the screen (recognised at the wall tablet): greeted, and the agenda is theirs
  const profile = useProfile();
  const me = profilePerson(profile);
  const myAgenda = home.agenda && me ? agendaFor(home.agenda, me.id) : home.agenda;

  return (
    <div className="page">
      <header className="dash-top hrow">
        <div className={cx('greet', profile.state === 'looking' && 'is-looking')}>
          <h1>{greeting(now)}{me ? ` ${me.name}` : ''}</h1>
          <p>
            {dateLong(now)} - {now.toLocaleTimeString('fr-FR', { hour: '2-digit', minute: '2-digit' })}
            {me && <> · <button type="button" className="me-not" onClick={clearProfile}>Pas {me.name} ?</button></>}
          </p>
        </div>
        <div className="dash-tools">
          <AirChip air={home.air} />
          {home.weather && <WxPill w={home.weather} now={now} href="/meteo" onClick={() => nav('/meteo')} />}
          <button type="button" className="pict bell-phone" aria-label="Réglages" onClick={() => setSettings(true)}><Icon name="settings" /></button>
          <button type="button" className="pict bell-phone" aria-label={`Alertes : ${home.alerts.length}`} onClick={() => setBell(true)}><Icon name="bell" />{home.alerts.length > 0 && <span className="pict-n">{home.alerts.length}</span>}</button>
        </div>
      </header>

      {home.alerts.length > 0 && (
        <div className="dash-alerts stack">
          {home.alerts.map((a) => <AlertRow key={a.id} alert={a} detail={[a.detail, a.since && a.kind !== 'battery' ? `depuis ${rel(a.since, now)}` : ''].filter(Boolean).join(' · ')} href={a.roomId ? `/pieces/${a.roomId}` : undefined} />)}
        </div>
      )}

      <div className="dash-grid home-grid">
        <Card className="dash-heat">
          <CardHead pict={g.heating.available
            ? <button type="button" className={cx('pict-btn', heatOff && 'is-off')} aria-pressed={!heatOff} aria-label={heatOff ? 'Relancer le chauffage (planning)' : 'Couper tout le chauffage'} title={heatOff ? 'Relancer le planning' : 'Couper le chauffage'} onClick={() => setHouseHeating(heatOff ? 'schedule' : 'off')}><Pict icon="power" state={heatOff ? 'off' : g.heating.roomsOn > 0 ? 'heat' : 'on'} /></button>
            : <Pict icon="flame" state="off" />} title="Chauffage" sub={heatingSubtitle(home)} />
          {g.heating.available && (
            <ChipRow>
              {(['schedule', 'night', 'boost'] as const).map((m) => <button key={m} type="button" className={cx('chip -s -mode', `-${m}`, preset === m && 'is-on')} aria-pressed={preset === m} onClick={() => setHouseHeating(m)}><Icon name={HOUSE_MODE_ICON[m]} />{HEATING_MODE_LABEL[m]}</button>)}
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
                  right={<Pair current={h.state.temperature} target={h.state.target} mode={h.state.mode} heating={r.summary.heating} stale={stale || !h.available}
                    tone={h.state.mode === 'schedule' ? 'schedule' : h.state.mode === 'manual' && preset === 'night' ? 'night' : undefined} />}
                />
              );
            })}
          </div>
          {g.heating.average != null && (
            <div className="heat-avg">
              <div className="hrow"><span className="muted" style={{ fontSize: 'var(--fs-s)' }}>Moyenne de la maison</span><Val value={g.heating.average} unit="°" size="s" stale={stale} /></div>
              {g.heating.humidity != null && <div className="hrow"><span className="muted" style={{ fontSize: 'var(--fs-s)' }}>Humidité</span><Val value={g.heating.humidity} unit="%" size="s" decimals={0} stale={stale} /></div>}
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
          {/* on: privacy off and recording; off: privacy on, no recording. The tile opens Sécurité. */}
          <Link to="/securite" className="dev-cams" style={{ textDecoration: 'none', display: 'contents' }}>
            <Card tight className="dev-cams">
              <div className="card-head"><Pict icon="camera" state={g.camerasPrivate ? 'off' : cams.length ? 'lit' : 'off'} />{cams.length > 0 && <Toggle on={!g.camerasPrivate} pending={!!firstCam && !!pending[firstCam.id]} label={g.camerasPrivate ? 'Caméras coupées : les activer' : 'Caméras actives : les couper'} onChange={(on) => setPrivacy(!on)} />}</div>
              <div style={{ display: 'grid', gap: 2 }}><span className="card-title -s">Caméras</span><span className="card-sub">{cams.length === 0 ? 'Aucune' : g.camerasPrivate ? 'Coupées · mode privé' : `${plural(cams.filter((c) => c.device.available).length, 'en direct', 'en direct')} · enregistrement`}</span></div>
            </Card>
          </Link>
        </div>

        {home.agenda && (
          <Card tight className="dash-agenda is-link" as="a" href="/agenda">
            <CardHead small pict={me ? <span className="avatar -s me-avatar" style={me.color ? { background: me.color, borderColor: me.color } : undefined} aria-hidden="true">{me.name.charAt(0)}</span> : <Pict icon="calendar" />}
              title={me ? 'Ton agenda' : 'Agenda'} sub={!home.agenda.available ? 'Calendriers injoignables' : me ? 'Le tien et les calendriers communs' : 'Les prochains jours'} right={<span className="sec-link">Semaine</span>} />
            {home.agenda.available && myAgenda && <AgendaGlance agenda={myAgenda} now={now} max={5} />}
          </Card>
        )}

        {confirmEl}
        <div className="dash-col dash-side">
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
