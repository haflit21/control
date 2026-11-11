import { useEffect } from 'react';
import { Link, Navigate, Route, Routes, useLocation, useNavigate } from 'react-router-dom';
import { IconSprite, Icon, BottomNav, Dot, Pict, Toasts, Popover, AlertRow, Row, Seg, Toggle, relative } from '@control/ui';
import { usePush, sendTest } from './lib/push';
import { setBell, useBell } from './lib/bell';
import { setTheme, useTheme, type ThemeChoice } from './lib/theme';
import { pushToast } from './lib/actions';
import { dismissToast, useToasts } from './lib/actions';
import { useEvents, useHome, useNow, isStale } from './lib/api';
import { HomePage } from './pages/Home';
import { RoomPage } from './pages/Room';
import { RoomsPage } from './pages/Rooms';
import { AgendaPage } from './pages/Agenda';
import { WallPage } from './pages/Wall';
import { SleepPage } from './pages/Sleep';
import { SecurityPage } from './pages/Security';
import { WeatherPage } from './pages/Weather';

const NAV = [
  { id: 'home', label: 'Accueil', icon: 'home', to: '/' },
  { id: 'rooms', label: 'Pièces', icon: 'rooms', to: '/pieces' },
  { id: 'security', label: 'Sécurité', icon: 'shield', to: '/securite' },
  { id: 'agenda', label: 'Agenda', icon: 'calendar', to: '/agenda' },
];
const SLEEP = { id: 'sleep', label: 'Veille', icon: 'power', to: '/veille' };

export function App() {
  const link = useEvents();
  const home = useHome();
  const now = useNow();
  const loc = useLocation();
  const navigate = useNavigate();
  const toasts = useToasts();
  const bell = useBell();
  // plain <a href="/…"> links inside UI components navigate client-side
  useEffect(() => {
    const onClick = (e: MouseEvent) => {
      if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
      const a = (e.target as HTMLElement).closest('a[href]') as HTMLAnchorElement | null;
      if (!a || a.target || !a.getAttribute('href')?.startsWith('/') || a.getAttribute('href')?.startsWith('/api')) return;
      e.preventDefault();
      navigate(a.getAttribute('href')!);
    };
    document.addEventListener('click', onClick);
    return () => document.removeEventListener('click', onClick);
  }, [navigate]);
  const active = loc.pathname.startsWith('/pieces') ? 'rooms' : loc.pathname.startsWith('/securite') ? 'security' : loc.pathname.startsWith('/meteo') ? 'weather' : loc.pathname.startsWith('/agenda') ? 'agenda' : 'home';
  const stale = isStale(home.data, link, now);
  const wall = loc.pathname.startsWith('/mur');
  const sleep = loc.pathname.startsWith('/veille');

  if (sleep) {
    return (
      <>
        <IconSprite />
        {home.data && <main><SleepPage home={home.data} now={now} /></main>}
        {home.isLoading && <Loading />}
        {home.isError && <ErrorState status={home.error.status} message={home.error.body.error} detail={home.error.body.detail} />}
      </>
    );
  }

  if (wall) {
    return (
      <>
        <IconSprite />
        {home.data && <main><WallPage home={home.data} now={now} stale={stale} /></main>}
        {home.isLoading && <Loading />}
        {home.isError && <ErrorState status={home.error.status} message={home.error.body.error} detail={home.error.body.detail} />}
        <Toasts items={toasts} onDismiss={dismissToast} />
      </>
    );
  }

  return (
    <>
      <IconSprite />
      <div className="desk">
        <div className="dash">
          <aside className="rail">
            <nav aria-label="Navigation">
              {NAV.map((n) => <Link key={n.id} to={n.to} className={active === n.id ? 'is-active' : undefined} aria-label={n.label}><Icon name={n.icon} size="l" />{n.id === 'rooms' && home.data && home.data.globals.lightsOn > 0 && <span className="pict-n">{home.data.globals.lightsOn}</span>}</Link>)}
              <Link to="/meteo" className={active === 'weather' ? 'is-active' : undefined} aria-label="Météo"><Icon name="cloud" size="l" /></Link>
            </nav>
            <div className="rail-bot">
              <Link to="/veille" className="pict rail-sleep" aria-label="Mettre en veille" title="Veille"><Icon name="power" /></Link>
              <button type="button" className="pict" style={{ cursor: 'pointer', position: 'relative' }} aria-label={`Alertes : ${home.data?.alerts.length ?? 0}`} onClick={() => setBell(true)}><Icon name="bell" />{!!home.data?.alerts.length && <span className="pict-n">{home.data.alerts.length}</span>}</button>
              {home.data?.presence[0] && <span className={home.data.presence[0].home ? 'who is-home' : 'who is-away'}><span className="avatar" aria-label={home.data.presence[0].name}>{home.data.presence[0].name.charAt(0)}</span></span>}
            </div>
          </aside>
          <main className="dash-main">
            {home.data?.demo && <div className="banner is-demo"><Dot tone="accent" />Mode démo : état simulé, rien n'est envoyé à la maison.</div>}
            {!home.data?.demo && home.data && !home.data.connected && <div className="banner"><Dot tone="warn" live />Connexion à Home Assistant perdue · les valeurs datent de {new Date(home.data.updatedAt).toLocaleTimeString('fr-FR', { hour: '2-digit', minute: '2-digit' })}</div>}
            {link.state === 'lost' && home.data && <div className="banner"><Dot tone="warn" live />Connexion au serveur perdue · nouvelle tentative…</div>}
            {home.isLoading && <Loading />}
            {home.isError && <ErrorState status={home.error.status} message={home.error.body.error} detail={home.error.body.detail} />}
            {home.data && (
              <Routes>
                <Route path="/" element={<HomePage home={home.data} now={now} stale={stale} />} />
                <Route path="/pieces" element={<RoomsPage home={home.data} now={now} stale={stale} />} />
                <Route path="/pieces/:id" element={<RoomPage home={home.data} now={now} stale={stale} />} />
                <Route path="/securite" element={<SecurityPage home={home.data} now={now} stale={stale} />} />
                <Route path="/meteo" element={<WeatherPage home={home.data} now={now} />} />
                <Route path="/agenda" element={<AgendaPage home={home.data} now={now} />} />
                <Route path="/mur" element={<WallPage home={home.data} now={now} stale={stale} />} />
                <Route path="*" element={<Navigate to="/" replace />} />
              </Routes>
            )}
          </main>
        </div>
        <div className="nav-wrap">
          <BottomNav items={[...NAV, SLEEP]} active={active} renderLink={(it, cls, children) => <Link to={[...NAV, SLEEP].find((n) => n.id === it.id)!.to} className={cls}>{children}</Link>} />
          <Toasts items={toasts} onDismiss={dismissToast} />
          {bell && home.data && (
            <Popover onClose={() => setBell(false)} label="Alertes">
              <div className="sheet-head"><Pict icon="bell" size="l" state={home.data.alerts.some((a) => a.level === 'crit') ? 'crit' : home.data.alerts.some((a) => a.level === 'warn') ? 'warn' : 'off'} /><div><div className="sheet-t">Alertes</div><div className="sheet-d">{home.data.alerts.length === 0 ? 'Rien à signaler, tout va bien.' : `${home.data.alerts.length} en cours`}</div></div></div>
              <div className="stack">
                {home.data.alerts.map((a) => <AlertRow key={a.id} alert={a} detail={[a.detail, a.since && a.kind !== 'battery' ? `depuis ${relative(a.since, now, '')}` : ''].filter(Boolean).join(' · ')} onClick={a.roomId ? () => { setBell(false); navigate(`/pieces/${a.roomId}`); } : undefined} />)}
              </div>
              <div className="sheet-sec">Cet appareil</div>
              <PushRow />
              <ThemeRow />
            </Popover>
          )}
        </div>
      </div>
    </>
  );
}

/** « Recevoir les alertes sur cet appareil » : abonnement push de ce navigateur. */
function PushRow() {
  const [state, set, error] = usePush();
  const sub = state === 'unsupported' ? 'Indisponible ici (HTTPS ou app installée requis)'
    : state === 'denied' ? 'Refusées dans le navigateur'
    : error ? error
    : 'Alertes et alarme, même app fermée';
  return (
    <div className="list">
      <Row pict={<Pict icon="bell" size="s" state={state === 'on' ? 'on' : state === 'denied' || state === 'unsupported' ? 'unknown' : 'off'} />} name="Recevoir les alertes ici" sub={sub}
        right={<Toggle small on={state === 'on'} pending={state === 'busy'} disabled={state === 'unsupported' || state === 'denied'} label={state === 'on' ? 'Notifications actives sur cet appareil' : 'Notifications inactives sur cet appareil'} onChange={(on) => { void set(on); }} />} />
      {state === 'on' && <Row pict={<Pict icon="check" size="s" />} name="Envoyer un test" sub="Une notification sur tous les appareils abonnés" onClick={() => { sendTest().then((n) => pushToast({ title: n ? `Test envoyé à ${n} appareil${n > 1 ? 's' : ''}` : 'Aucun appareil abonné', tone: n ? 'ok' : 'error' })).catch(() => pushToast({ title: 'Envoi impossible', tone: 'error' })); }} />}
    </div>
  );
}

const THEMES: Array<{ id: ThemeChoice; label: string }> = [{ id: 'auto', label: 'Auto' }, { id: 'dark', label: 'Sombre' }, { id: 'light', label: 'Clair' }];

/** Apparence de cet appareil : suit le système, ou forcée sombre / claire. */
function ThemeRow() {
  const theme = useTheme();
  return (
    <div className="theme-row">
      <Pict icon="contrast" size="s" />
      <div className="theme-row-t">Apparence<small>{theme === 'auto' ? 'Suit le réglage du système' : theme === 'dark' ? 'Toujours sombre' : 'Toujours clair'}</small></div>
      <Seg items={THEMES} active={theme} onSelect={(id) => setTheme(id as ThemeChoice)} />
    </div>
  );
}

function Loading() {
  return (
    <div className="page" aria-busy="true">
      <span className="skel" style={{ height: 32, width: '50%' }} />
      <span className="skel" style={{ height: 140 }} />
      <span className="skel" style={{ height: 260 }} />
      <span className="skel" style={{ height: 200 }} />
    </div>
  );
}

function ErrorState({ status, message, detail }: { status: number; message: string; detail?: string }) {
  return (
    <div className="state" role="alert">
      <Pict icon={status === 401 ? 'lock' : 'wifi-off'} size="xl" state={status === 401 ? 'night' : 'warn'} />
      <h2>{status === 401 ? 'Non connecté' : 'Impossible de joindre le serveur'}</h2>
      <p>{status === 401 ? 'Ouvre le lien de connexion affiché au démarrage du serveur Control (…/login?token=…).' : `${message}${detail ? ` — ${detail}` : ''}`}</p>
    </div>
  );
}
