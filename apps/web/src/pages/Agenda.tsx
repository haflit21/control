import { useState } from 'react';
import type { Home } from '@maison/model';
import { AgendaNav, CalLegend, DayList, Icon, Pict, WeekView, addDays, relative, startOfWeek, weekLabel } from '@maison/ui';
import { useMedia } from '../lib/api';

export function AgendaPage({ home, now }: { home: Home; now: Date }) {
  const [offset, setOffset] = useState(0); // weeks from the current one
  const monday = addDays(startOfWeek(now), offset * 7);
  const ag = home.agenda;
  const wide = useMedia('(min-width: 861px)');
  const beyond = ag ? monday.getTime() < new Date(ag.from).getTime() || addDays(monday, 7).getTime() > new Date(ag.to).getTime() : false;

  return (
    <div className="page agenda">
      <header className="dash-top hrow agenda-head">
        <div className="greet"><h1>Agenda</h1><p>Semaine {weekLabel(monday)}{ag && <> · <span style={{ color: 'var(--fg-faint)' }}>mis à jour {relative(ag.updatedAt, now, '')}</span></>}</p></div>
        <AgendaNav onPrev={() => setOffset((o) => o - 1)} onNext={() => setOffset((o) => o + 1)} onToday={() => setOffset(0)} />
      </header>

      {!ag && (
        <div className="agenda-state"><Pict icon="calendar" size="l" />Aucun calendrier configuré<br /><small>Ajoute tes entités <code>calendar.*</code> dans <code>maison.yaml</code>, section <code>calendars</code>.</small></div>
      )}
      {ag && !ag.available && (
        <div className="agenda-state"><Pict icon="wifi-off" size="l" state="unknown" />Calendriers injoignables<br /><small>Home Assistant n’a pas répondu pour {ag.calendars.map((c) => c.name).join(', ')}.</small></div>
      )}
      {ag && ag.available && (
        <>
          {ag.calendars.length > 1 && <CalLegend cals={ag.calendars} />}
          {beyond && <div className="agenda-state" style={{ padding: 'var(--sp-3)' }}><Icon name="clock" /> Cette semaine est hors de la fenêtre chargée ({new Date(ag.from).toLocaleDateString('fr-FR')} → {new Date(ag.to).toLocaleDateString('fr-FR')}).</div>}
          {wide ? (
            <>
              <WeekView agenda={ag} monday={monday} now={now} />
              <div className="agenda-title" style={{ fontSize: 'var(--fs-l)', marginTop: 'var(--sp-2)' }}>Semaine suivante<small>{weekLabel(addDays(monday, 7))}</small></div>
              <WeekView agenda={ag} monday={addDays(monday, 7)} now={now} />
            </>
          ) : <DayList agenda={ag} monday={monday} now={now} />}
        </>
      )}
    </div>
  );
}
