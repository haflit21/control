import { useEffect, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import type { Home } from '@control/model';
import { AgendaGlance, AirChip, Card, CardHead, Icon, Pict, WxPill, cx } from '@control/ui';
import { cameraSupported, faceOnWake, recognizeOnWake, sleepProfile } from '../lib/profile';

/**
 * /veille — sleep screen: big clock, date, weather and air, the next days.
 * Reached from the power button of the menu; a touch or a swipe anywhere
 * brings the dashboard back. The screen stays awake (wake lock), dimmed at night.
 * Going to sleep forgets who was there; on a device that opted in (« Reconnaître
 * au réveil »), the touch that wakes it also takes one photo to greet the person.
 */
export function SleepPage({ home, now }: { home: Home; now: Date }) {
  const nav = useNavigate();
  const [clock, setClock] = useState(() => new Date());
  useEffect(() => { const t = setInterval(() => setClock(new Date()), 1000); return () => clearInterval(t); }, []);
  useEffect(() => {
    let lock: { release: () => Promise<void> } | undefined;
    const req = async () => { try { lock = await (navigator as Navigator & { wakeLock?: { request: (t: 'screen') => Promise<{ release: () => Promise<void> }> } }).wakeLock?.request('screen'); } catch { /* not allowed */ } };
    void req();
    const onVis = () => { if (document.visibilityState === 'visible') void req(); };
    document.addEventListener('visibilitychange', onVis);
    return () => { document.removeEventListener('visibilitychange', onVis); void lock?.release(); };
  }, []);
  useEffect(() => { sleepProfile(); }, []);
  const left = useRef(false);
  const wake = () => {
    if (left.current) return;
    left.current = true;
    // the camera opens inside the touch (Safari), the dashboard shows at once
    if (home.face && faceOnWake() && cameraSupported()) void recognizeOnWake();
    nav('/');
  };
  const start = useRef<{ x: number; y: number } | null>(null);
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape' || e.key === 'Enter' || e.key === ' ') wake(); };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });
  const night = home.weather?.night ?? (clock.getHours() >= 22 || clock.getHours() < 7);
  const hh = clock.getHours().toString().padStart(2, '0');
  const mm = clock.getMinutes().toString().padStart(2, '0');
  const date = clock.toLocaleDateString('fr-FR', { weekday: 'long', day: 'numeric', month: 'long' });

  return (
    <div className={cx('sleep', night && 'is-night')} role="button" tabIndex={0} aria-label="Veille : toucher pour revenir au tableau de bord"
      onClick={wake}
      onPointerDown={(e) => { start.current = { x: e.clientX, y: e.clientY }; }}
      onPointerUp={(e) => { const s = start.current; start.current = null; if (s && Math.hypot(e.clientX - s.x, e.clientY - s.y) > 40) wake(); }}>
      <h1 className="sr-only">Veille</h1>
      <div className="sleep-top" aria-hidden="true">
        <AirChip air={home.air} />
        {home.weather && <WxPill w={home.weather} now={now} />}
      </div>
      <div className="sleep-clock">
        <div className="sleep-time" aria-label={`${hh} h ${mm}`}>{hh}<span>:</span>{mm}</div>
        <div className="sleep-date">{date.charAt(0).toUpperCase() + date.slice(1)}</div>
      </div>
      {home.agenda?.available && (
        <Card tight className="sleep-agenda">
          <CardHead small pict={<Pict icon="calendar" />} title="Agenda" sub="Les prochains jours" />
          <AgendaGlance agenda={home.agenda} now={now} max={5} />
        </Card>
      )}
      <div className="sleep-hint" aria-hidden="true"><Icon name="chevron" size="l" />Toucher pour revenir</div>
    </div>
  );
}
