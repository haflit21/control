// The event form: create or edit, with a quick-entry bar on top that turns a
// typed or spoken sentence into the pre-filled form. Nothing is saved until
// « Enregistrer ».
import { useCallback, useEffect, useId, useRef, useState, type FormEvent } from 'react';
import { createPortal } from 'react-dom';
import type { Agenda, AgendaEvent, EventDraft, RepeatFreq } from '@maison/model';
import { Icon, Pict, Toggle, cx } from '@maison/ui';
import { HttpError } from '../lib/api';
import { pushToast } from '../lib/actions';
import { createEvent, deleteEvent, getEvent, isoDay, micSupported, parseText, record, sendVoice, updateEvent, type Recording } from '../lib/agenda';

export type SheetOpen =
  | { mode: 'new'; date?: string; recording?: Recording } // recording: started by the tap on the page's mic
  | { mode: 'edit'; id: string };

type Phase = 'form' | 'loading' | 'listening' | 'thinking';

type Fields = {
  title: string; calendar: string; location: string; allDay: boolean;
  date: string; endDate: string; start: string; end: string;
  freq: RepeatFreq | ''; until: string;
};

const REPEATS: Array<{ id: RepeatFreq | ''; label: string }> = [
  { id: '', label: 'Jamais' }, { id: 'weekly', label: 'Chaque semaine' }, { id: 'biweekly', label: 'Toutes les 2 semaines' },
  { id: 'monthly', label: 'Chaque mois' }, { id: 'yearly', label: 'Chaque année' },
];

function addHour(hhmm: string, h = 1): string {
  const [a, b] = hhmm.split(':').map(Number);
  const m = ((a * 60 + b + h * 60) % (24 * 60) + 24 * 60) % (24 * 60);
  return `${String(Math.floor(m / 60)).padStart(2, '0')}:${String(m % 60).padStart(2, '0')}`;
}

function blank(cal: string, date?: string): Fields {
  const now = new Date();
  const start = `${String(Math.min(23, now.getHours() + 1)).padStart(2, '0')}:00`;
  return { title: '', calendar: cal, location: '', allDay: false, date: date ?? isoDay(now), endDate: '', start, end: addHour(start), freq: '', until: '' };
}

function fromEvent(e: EventDraft | AgendaEvent, base: Fields): Fields {
  const start = e.start || base.start;
  return {
    title: e.title ?? '', calendar: e.calendar || base.calendar, location: e.location ?? '', allDay: !!e.allDay,
    date: e.date || base.date, endDate: e.endDate ?? '', start, end: e.end || addHour(start),
    freq: e.repeat?.freq ?? '', until: e.repeat?.until ?? '',
  };
}

function toEvent(f: Fields): AgendaEvent {
  return {
    title: f.title.trim(), calendar: f.calendar, location: f.location.trim() || undefined, allDay: f.allDay, date: f.date,
    endDate: f.allDay && f.endDate && f.endDate > f.date ? f.endDate : undefined,
    start: f.allDay ? undefined : f.start, end: f.allDay ? undefined : f.end,
    repeat: f.freq ? { freq: f.freq, until: f.until || undefined } : undefined,
  };
}

function errText(e: unknown): string {
  if (e instanceof HttpError) return [e.body.error, e.body.detail].filter(Boolean).join(' · ');
  if (e instanceof DOMException && (e.name === 'NotAllowedError' || e.name === 'SecurityError')) return 'Micro refusé : autorise-le dans les réglages du navigateur pour cette adresse.';
  if (e instanceof DOMException && e.name === 'NotFoundError') return 'Aucun micro trouvé sur cet appareil.';
  return e instanceof Error ? e.message : 'Erreur';
}

export function EventSheet({ agenda, open, onClose }: { agenda: Agenda; open: SheetOpen; onClose: () => void }) {
  const cals = agenda.calendars;
  const base = blank(cals[0]?.id ?? 'famille', open.mode === 'new' ? open.date : undefined);
  const [f, setF] = useState<Fields>(base);
  const [phase, setPhase] = useState<Phase>(open.mode === 'edit' ? 'loading' : open.recording ? 'listening' : 'form');
  const [err, setErr] = useState('');
  const [heard, setHeard] = useState('');
  const [warning, setWarning] = useState('');
  const [quick, setQuick] = useState('');
  const [busy, setBusy] = useState(false);
  const [confirmDel, setConfirmDel] = useState(false);
  const [level, setLevel] = useState(0);
  const [elapsed, setElapsed] = useState(0);
  const [series, setSeries] = useState(false);
  const recRef = useRef<Recording | null>(null);
  const titleRef = useRef<HTMLInputElement>(null);
  const id = useId();
  const set = <K extends keyof Fields>(k: K, v: Fields[K]) => setF((p) => ({ ...p, [k]: v }));

  const apply = useCallback((d: EventDraft, w?: string) => {
    setF((p) => fromEvent(d, p));
    setHeard(d.transcript ?? '');
    setWarning(w ?? '');
    setPhase('form');
    setTimeout(() => titleRef.current?.focus(), 50);
  }, []);

  // listen to a recording until it ends, then send it
  const follow = useCallback(async (r: Recording) => {
    recRef.current = r;
    setErr(''); setLevel(0); setElapsed(0); setPhase('listening');
    let blob: Blob;
    try {
      blob = await r.done;
    } catch (e) {
      recRef.current = null;
      setPhase('form');
      if ((e as Error).message !== 'annulé') setErr(errText(e));
      return;
    }
    recRef.current = null;
    setPhase('thinking');
    try {
      const res = await sendVoice(blob);
      apply(res.draft, res.warning);
    } catch (e) {
      setPhase('form');
      setErr(errText(e));
    }
  }, [apply]);

  const listen = useCallback(async () => {
    setErr('');
    try {
      const r = await record({ onLevel: setLevel, onTick: setElapsed });
      void follow(r);
    } catch (e) {
      setPhase('form');
      setErr(errText(e));
    }
  }, [follow]);

  // a recording started by the page's mic button (inside the tap, for Safari)
  useEffect(() => {
    if (open.mode === 'new' && open.recording) {
      open.recording.watch(setLevel, setElapsed);
      void follow(open.recording);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // edit: load the event
  useEffect(() => {
    if (open.mode !== 'edit') return;
    let live = true;
    getEvent(open.id).then((e) => {
      if (!live) return;
      setF(fromEvent(e, blank(cals[0]?.id ?? 'famille')));
      setSeries(!!e.repeat);
      setPhase('form');
    }).catch((e) => { if (live) { setErr(errText(e)); setPhase('form'); } });
    return () => { live = false; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // stop the mic if the sheet goes away mid-recording
  useEffect(() => () => recRef.current?.cancel(), []);

  const close = useCallback(() => { if (!busy) { recRef.current?.cancel(); onClose(); } }, [busy, onClose]);
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') close(); };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [close]);

  const understandText = async () => {
    const text = quick.trim();
    if (!text) return;
    setErr(''); setPhase('thinking');
    try {
      const res = await parseText(text);
      apply(res.draft, res.warning);
      setQuick('');
    } catch (e) {
      setPhase('form');
      setErr(errText(e));
    }
  };

  const save = async (ev: FormEvent) => {
    ev.preventDefault();
    if (!f.title.trim()) { setErr('Donne un titre à l’événement.'); titleRef.current?.focus(); return; }
    setBusy(true); setErr('');
    try {
      const res = open.mode === 'edit' ? await updateEvent(open.id, toEvent(f)) : await createEvent(toEvent(f));
      pushToast({ title: res.message ?? 'Enregistré', tone: 'ok' });
      onClose();
    } catch (e) {
      setErr(errText(e));
    } finally {
      setBusy(false);
    }
  };

  const remove = async () => {
    if (open.mode !== 'edit') return;
    if (!confirmDel) { setConfirmDel(true); setTimeout(() => setConfirmDel(false), 5000); return; }
    setBusy(true);
    try {
      const res = await deleteEvent(open.id);
      pushToast({ title: res.message ?? 'Supprimé', tone: 'ok' });
      onClose();
    } catch (e) {
      setErr(errText(e));
      setBusy(false);
    }
  };

  const canType = agenda.input.text;
  const canTalk = agenda.input.audio && micSupported();
  const micWhy = !agenda.input.audio ? '' : !window.isSecureContext ? 'Le micro demande une adresse en HTTPS (ou l’app installée).' : '';
  const sec = Math.floor(elapsed / 1000);

  const node = (
    <div className="scrim" onClick={close} role="presentation">
      <div className="sheet -form" role="dialog" aria-modal="true" aria-labelledby={`${id}-t`} onClick={(e) => e.stopPropagation()}>
        <div className="sheet-head">
          <Pict icon="calendar" size="l" state="on" />
          <div><div className="sheet-t" id={`${id}-t`}>{open.mode === 'edit' ? 'Modifier l’événement' : 'Nouvel événement'}</div>
            {series && <div className="sheet-d">Événement répété : les changements valent pour toute la série.</div>}
          </div>
        </div>

        {phase === 'listening' && (
          <div className="listen" aria-live="polite">
            <button type="button" className="mic -l is-rec" style={{ ['--lvl' as string]: level.toFixed(2) }} onClick={() => recRef.current?.stop()} aria-label="Terminer l’enregistrement"><Icon name="stop" /></button>
            <div className="listen-t">Je t’écoute…</div>
            <div className="listen-s">Dis l’événement puis fais une pause · {sec} s</div>
            <button type="button" className="btn -ghost" onClick={() => recRef.current?.cancel()}>Annuler</button>
          </div>
        )}
        {(phase === 'thinking' || phase === 'loading') && (
          <div className="listen" aria-live="polite" aria-busy="true">
            <span className="thinking" aria-hidden="true"><i /><i /><i /></span>
            <div className="listen-s">{phase === 'loading' ? 'Chargement…' : 'Je comprends ta phrase…'}</div>
          </div>
        )}

        {phase === 'form' && (
          <form className="form" onSubmit={save} noValidate>
            {open.mode === 'new' && (canType || canTalk) && (
              <div className="field">
                <label className="field-l" htmlFor={`${id}-q`}>Saisie rapide</label>
                <div className="quick">
                  <input id={`${id}-q`} className="input" value={quick} onChange={(e) => setQuick(e.target.value)} placeholder="« Dentiste jeudi 10h »"
                    onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); void understandText(); } }} disabled={!canType} enterKeyHint="go" autoComplete="off" />
                  {quick.trim() ? (
                    <button type="button" className="mic" onClick={() => void understandText()} aria-label="Comprendre la phrase"><Icon name="arrow-up" /></button>
                  ) : (
                    <button type="button" className="mic" onClick={() => void listen()} disabled={!canTalk} aria-label="Dicter l’événement" title={micWhy || undefined}><Icon name="mic" /></button>
                  )}
                </div>
                {micWhy && <span className="form-note">{micWhy}</span>}
              </div>
            )}
            {heard && <div className="heard"><Icon name="mic" size="s" /><span>Compris : <q>{heard}</q> — vérifie avant d’enregistrer.</span></div>}
            {warning && <div className="form-note" role="status">{warning}</div>}

            <div className="field">
              <label className="field-l" htmlFor={`${id}-title`}>Titre</label>
              <input ref={titleRef} id={`${id}-title`} className="input" value={f.title} onChange={(e) => set('title', e.target.value)} placeholder="Dentiste · Marion" maxLength={120} autoComplete="off" aria-invalid={!!err && !f.title.trim()} />
            </div>

            {cals.length > 1 && (
              <div className="field" role="group" aria-labelledby={`${id}-cal`}>
                <span className="field-l" id={`${id}-cal`}>Calendrier</span>
                <div className="cal-pick">
                  {cals.map((c) => (
                    <button key={c.id} type="button" aria-pressed={f.calendar === c.id} onClick={() => set('calendar', c.id)} style={c.color ? ({ ['--cal' as string]: c.color }) : undefined}><i aria-hidden="true" />{c.name}</button>
                  ))}
                </div>
              </div>
            )}

            <div className="field-inline">
              <span className="field-l" id={`${id}-ad`}>Toute la journée</span>
              <Toggle on={f.allDay} label={f.allDay ? 'Toute la journée' : 'À une heure précise'} onChange={(v) => set('allDay', v)} />
            </div>

            <div className={cx('field-row', !f.allDay && '-when')}>
              <div className="field">
                <label className="field-l" htmlFor={`${id}-date`}>{f.allDay ? 'Du' : 'Le'}</label>
                <input id={`${id}-date`} type="date" className="input" value={f.date} onChange={(e) => e.target.value && set('date', e.target.value)} required />
              </div>
              {f.allDay ? (
                <div className="field">
                  <label className="field-l" htmlFor={`${id}-ed`}>Au (facultatif)</label>
                  <input id={`${id}-ed`} type="date" className="input" value={f.endDate} min={f.date} onChange={(e) => set('endDate', e.target.value)} />
                </div>
              ) : (
                <>
                  <div className="field">
                    <label className="field-l" htmlFor={`${id}-s`}>De</label>
                    <input id={`${id}-s`} type="time" className="input" value={f.start} step={300} onChange={(e) => {
                      const v = e.target.value;
                      if (!v) return;
                      setF((p) => ({ ...p, start: v, end: p.end > v ? p.end : addHour(v) }));
                    }} required />
                  </div>
                  <div className="field">
                    <label className="field-l" htmlFor={`${id}-e`}>À</label>
                    <input id={`${id}-e`} type="time" className="input" value={f.end} step={300} onChange={(e) => e.target.value && set('end', e.target.value)} required />
                  </div>
                </>
              )}
            </div>

            <div className="field">
              <label className="field-l" htmlFor={`${id}-loc`}>Lieu (facultatif)</label>
              <input id={`${id}-loc`} className="input" value={f.location} onChange={(e) => set('location', e.target.value)} placeholder="Cabinet Dr Martin" maxLength={120} autoComplete="off" />
            </div>

            <div className="field" role="group" aria-labelledby={`${id}-rep`}>
              <span className="field-l" id={`${id}-rep`}>Répéter</span>
              <div className="seg">
                {REPEATS.map((r) => <button key={r.id || 'none'} type="button" aria-pressed={f.freq === r.id} className={cx(f.freq === r.id && 'is-active')} onClick={() => set('freq', r.id)}>{r.label}</button>)}
              </div>
            </div>
            {f.freq && (
              <div className="field">
                <label className="field-l" htmlFor={`${id}-until`}>Jusqu’au (facultatif)</label>
                <input id={`${id}-until`} type="date" className="input" value={f.until} min={f.date} onChange={(e) => set('until', e.target.value)} />
              </div>
            )}

            {err && <div className="form-err" role="alert">{err}</div>}

            <div className="form-acts">
              {open.mode === 'edit' ? (
                <button type="button" className={cx('btn', confirmDel ? '-danger' : '-icon')} onClick={() => void remove()} disabled={busy} aria-label={confirmDel ? 'Confirmer la suppression' : series ? 'Supprimer toute la série' : 'Supprimer'}>
                  <Icon name="trash" />{confirmDel && (series ? 'Toute la série ?' : 'Supprimer ?')}
                </button>
              ) : <span />}
              <button type="button" className="btn" onClick={close} disabled={busy}>Annuler</button>
              <button type="submit" className="btn -primary" disabled={busy}>{busy ? '…' : 'Enregistrer'}</button>
            </div>
          </form>
        )}
        {phase !== 'form' && err && <div className="form-err" role="alert">{err}</div>}
      </div>
    </div>
  );
  return createPortal(node, document.body);
}
