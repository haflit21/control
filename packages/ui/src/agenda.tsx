import type { CSSProperties, ReactNode } from 'react';
import type { Agenda, CalEvent, Calendar } from '@control/model';
import { Icon, cx } from './primitives';

/* ---------- Agenda : semaine partagée ---------- */

const DAY_NAMES = ['dim.', 'lun.', 'mar.', 'mer.', 'jeu.', 'ven.', 'sam.'];
const DAY_LONG = ['dimanche', 'lundi', 'mardi', 'mercredi', 'jeudi', 'vendredi', 'samedi'];

/** Monday 00:00 of the week containing d (local time). */
export function startOfWeek(d: Date): Date {
  const x = new Date(d.getFullYear(), d.getMonth(), d.getDate());
  const wd = (x.getDay() + 6) % 7; // 0 = Monday
  x.setDate(x.getDate() - wd);
  return x;
}
export function addDays(d: Date, n: number): Date {
  const x = new Date(d);
  x.setDate(x.getDate() + n);
  return x;
}
function sameDay(a: Date, b: Date): boolean {
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
}
function hm(d: Date): string {
  return d.toLocaleTimeString('fr-FR', { hour: '2-digit', minute: '2-digit' });
}
/** Events that touch the day [day, day+1). All-day events end at 00:00 of the next day (exclusive). */
export function eventsOfDay(events: CalEvent[], day: Date): CalEvent[] {
  const d0 = new Date(day.getFullYear(), day.getMonth(), day.getDate()).getTime();
  const d1 = d0 + 86400000;
  return events.filter((e) => {
    const s = new Date(e.start).getTime();
    const en = new Date(e.end).getTime();
    return s < d1 && (en > d0 || (en === s && s >= d0));
  }).sort((a, b) => Number(b.allDay) - Number(a.allDay) || new Date(a.start).getTime() - new Date(b.start).getTime());
}
/** "Semaine du 9 au 15 novembre" */
export function weekLabel(monday: Date): string {
  const sunday = addDays(monday, 6);
  const sameMonth = monday.getMonth() === sunday.getMonth();
  const m1 = monday.toLocaleDateString('fr-FR', { month: 'long' });
  const m2 = sunday.toLocaleDateString('fr-FR', { month: 'long' });
  return sameMonth ? `du ${monday.getDate()} au ${sunday.getDate()} ${m1}` : `du ${monday.getDate()} ${m1} au ${sunday.getDate()} ${m2}`;
}

function calColor(cals: Calendar[], id: string): string | undefined {
  return cals.find((c) => c.id === id)?.color;
}

export function EventItem({ ev, day, cals, now, withDay, compact, onOpen }: { ev: CalEvent; day?: Date; cals: Calendar[]; now: Date; withDay?: boolean; compact?: boolean; onOpen?: (ev: CalEvent) => void }) {
  const start = new Date(ev.start);
  const end = new Date(ev.end);
  const past = end.getTime() <= now.getTime();
  const live = !ev.allDay && start.getTime() <= now.getTime() && end.getTime() > now.getTime();
  const color = calColor(cals, ev.calendar);
  let time: string;
  if (ev.allDay) {
    time = 'Journée';
  } else if (day && !sameDay(start, day)) {
    time = `→ ${hm(end)}`;
  } else if (day && !sameDay(end, day) && end.getTime() > new Date(day.getFullYear(), day.getMonth(), day.getDate() + 1).getTime()) {
    time = `${hm(start)} →`;
  } else {
    time = `${hm(start)} – ${hm(end)}`;
  }
  if (compact && !ev.allDay) time = hm(start);
  const sub = compact ? time : [time, ev.location].filter(Boolean).join(' · ');
  const cls = cx('ev', ev.allDay && '-allday', past && 'is-past', live && 'is-now', ev.repeats && 'is-repeat');
  const style = color ? ({ ['--cal' as string]: color } as CSSProperties) : undefined;
  const inner = (
    <>
      <span className="ev-t">{ev.title}{ev.repeats && <Icon name="repeat" size="s" className="ev-r" label="répété" />}</span>
      {!ev.allDay || ev.location ? <span className="ev-s">{ev.allDay ? ev.location : sub}</span> : null}
      {withDay && <span className="ev-d">{sameDay(start, now) ? 'aujourd’hui' : sameDay(start, addDays(now, 1)) ? 'demain' : start.toLocaleDateString('fr-FR', { weekday: 'short', day: 'numeric' })}</span>}
    </>
  );
  if (onOpen) return <button type="button" className={cls} style={style} title={ev.title} onClick={() => onOpen(ev)} aria-label={`${ev.title}, ${ev.allDay ? 'journée' : sub} · modifier`}>{inner}</button>;
  return <div className={cls} style={style} title={ev.title}>{inner}</div>;
}

/** Seven columns, Monday to Sunday. */
export function WeekView({ agenda, monday, now, onOpen }: { agenda: Agenda; monday: Date; now: Date; onOpen?: (ev: CalEvent) => void }) {
  return (
    <div className="week">
      {Array.from({ length: 7 }, (_, i) => {
        const day = addDays(monday, i);
        const evs = eventsOfDay(agenda.events, day);
        const today = sameDay(day, now);
        const past = !today && day.getTime() < now.getTime();
        return (
          <div key={i} className={cx('day', today && 'is-today', past && 'is-past')}>
            <div className="day-head"><span className="day-name">{DAY_NAMES[day.getDay()]}</span><span className="day-num">{day.getDate()}</span></div>
            {evs.length === 0 ? <span className="day-empty">Rien de prévu</span> : evs.map((e) => <EventItem key={e.id} ev={e} day={day} cals={agenda.calendars} now={now} onOpen={onOpen} />)}
          </div>
        );
      })}
    </div>
  );
}

/** One block per day with events (phone). Days without events are skipped except today. */
export function DayList({ agenda, monday, now, onOpen }: { agenda: Agenda; monday: Date; now: Date; onOpen?: (ev: CalEvent) => void }) {
  const days = Array.from({ length: 7 }, (_, i) => addDays(monday, i));
  return (
    <div className="daylist">
      {days.map((day) => {
        const evs = eventsOfDay(agenda.events, day);
        const today = sameDay(day, now);
        if (evs.length === 0 && !today) return null;
        return (
          <div key={day.toISOString()} className={cx('day', today && 'is-today')}>
            <div className="day-head"><span className="day-num">{day.getDate()}</span><span className="day-name">{today ? 'aujourd’hui' : DAY_LONG[day.getDay()]}</span></div>
            {evs.length === 0 ? <span className="day-empty">Rien de prévu</span> : evs.map((e) => <EventItem key={e.id} ev={e} day={day} cals={agenda.calendars} now={now} onOpen={onOpen} />)}
          </div>
        );
      })}
    </div>
  );
}

/** The next few events from now on (home card). */
export function AgendaGlance({ agenda, now, max = 4 }: { agenda: Agenda; now: Date; max?: number }) {
  const upcoming = agenda.events.filter((e) => new Date(e.end).getTime() > now.getTime()).slice(0, max);
  if (upcoming.length === 0) return <span className="day-empty">Rien de prévu ces prochains jours</span>;
  return <div className="glance">{upcoming.map((e) => <EventItem key={e.id} ev={e} cals={agenda.calendars} now={now} withDay compact />)}</div>;
}

export function CalLegend({ cals }: { cals: Calendar[] }) {
  return <div className="cal-legend">{cals.map((c) => <span key={c.id} className="chip" style={c.color ? ({ ['--cal' as string]: c.color } as CSSProperties) : undefined}><i aria-hidden="true" />{c.name}</span>)}</div>;
}

export function AgendaNav({ onPrev, onNext, onToday, children }: { onPrev: () => void; onNext: () => void; onToday: () => void; children?: ReactNode }) {
  return (
    <div className="agenda-nav">
      <button type="button" onClick={onPrev} aria-label="Semaine précédente"><Icon name="chevron-left" /></button>
      <button type="button" className="-today" onClick={onToday}>Aujourd’hui</button>
      <button type="button" onClick={onNext} aria-label="Semaine suivante"><Icon name="chevron-right" /></button>
      {children}
    </div>
  );
}
