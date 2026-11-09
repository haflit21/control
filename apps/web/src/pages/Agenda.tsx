import { useState } from 'react';
import type { CalEvent, Home } from '@maison/model';
import { AgendaNav, CalLegend, DayList, Icon, Pict, WeekView, addDays, startOfWeek, weekLabel } from '@maison/ui';
import { useMedia } from '../lib/api';
import { pushToast } from '../lib/actions';
import { micSupported, record } from '../lib/agenda';
import { EventSheet, type SheetOpen } from './EventSheet';

export function AgendaPage({ home, now }: { home: Home; now: Date }) {
  const [offset, setOffset] = useState(0); // weeks from the current one
  const [sheet, setSheet] = useState<SheetOpen | null>(null);
  const monday = addDays(startOfWeek(now), offset * 7);
  const ag = home.agenda;
  const wide = useMedia('(min-width: 861px)');
  const beyond = ag ? monday.getTime() < new Date(ag.from).getTime() || addDays(monday, 7).getTime() > new Date(ag.to).getTime() : false;
  const open = (ev: CalEvent) => setSheet({ mode: 'edit', id: ev.eventId });

  // the mic starts recording inside the tap (Safari), then the sheet shows it
  const talk = async () => {
    if (!micSupported()) { setSheet({ mode: 'new' }); return; }
    try {
      const recording = await record();
      setSheet({ mode: 'new', recording });
    } catch (e) {
      const denied = e instanceof DOMException && (e.name === 'NotAllowedError' || e.name === 'SecurityError');
      pushToast({ title: denied ? 'Micro refusé' : 'Micro indisponible', detail: denied ? 'Autorise le micro pour cette adresse dans les réglages du navigateur.' : (e as Error).message, tone: 'error' });
    }
  };

  return (
    <div className="page agenda">
      <header className="dash-top hrow agenda-head">
        <div className="greet"><h1>Agenda</h1><p>Semaine {weekLabel(monday)}</p></div>
        <div className="agenda-tools">
          <AgendaNav onPrev={() => setOffset((o) => o - 1)} onNext={() => setOffset((o) => o + 1)} onToday={() => setOffset(0)} />
          {ag && <button type="button" className="btn -primary" onClick={() => setSheet({ mode: 'new' })}><Icon name="plus" />Ajouter</button>}
          {ag?.input.audio && <button type="button" className="mic" onClick={() => void talk()} aria-label="Dicter un événement" title={micSupported() ? 'Dicter un événement' : 'Le micro demande HTTPS'}><Icon name="mic" /></button>}
        </div>
      </header>

      {!ag && (
        <div className="agenda-state"><Pict icon="calendar" size="l" />Agenda indisponible<br /><small>Le serveur n’a pas pu ouvrir son dossier d’état.</small></div>
      )}
      {ag && (
        <>
          {ag.calendars.length > 1 && <CalLegend cals={ag.calendars} />}
          {beyond && <div className="agenda-state" style={{ padding: 'var(--sp-3)' }}><Icon name="clock" /> Cette semaine est hors de la fenêtre affichée ({new Date(ag.from).toLocaleDateString('fr-FR')} → {new Date(ag.to).toLocaleDateString('fr-FR')}).</div>}
          {wide ? (
            <>
              <WeekView agenda={ag} monday={monday} now={now} onOpen={open} />
              <div className="agenda-title" style={{ fontSize: 'var(--fs-l)', marginTop: 'var(--sp-2)' }}>Semaine suivante<small>{weekLabel(addDays(monday, 7))}</small></div>
              <WeekView agenda={ag} monday={addDays(monday, 7)} now={now} onOpen={open} />
            </>
          ) : <DayList agenda={ag} monday={monday} now={now} onOpen={open} />}
          {sheet && <EventSheet agenda={ag} open={sheet} onClose={() => setSheet(null)} />}
        </>
      )}
    </div>
  );
}
