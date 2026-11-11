import { Children, type CSSProperties, type ReactNode } from 'react';
import type { Home } from '@control/model';
import { useMedia } from '../lib/api';
import { AirIndex, AirPollens, AirPollutants, Card, CardHead, Compass, InOut, Pict, Pressure, Section, Spark, SunArc, UV, WxDays, WxHero, WxHours, WxStat, cardinal, int, relative, temp0 } from '@control/ui';

export function WeatherPage({ home, now }: { home: Home; now: Date }) {
  const wide3 = useMedia('(min-width: 1280px)');
  const wide2 = useMedia('(min-width: 861px)');
  const cols = wide3 ? 3 : wide2 ? 2 : 1;
  const w = home.weather;
  if (!w) return <div className="state"><Pict icon="cloud" size="xl" /><h2>Pas de météo configurée</h2><p>Ajoute une entité <code>weather</code> dans control.yaml.</p></div>;
  const hours = w.hourly.slice(0, 12);
  const air = home.air;
  return (
    <div className="page -weather">
      <h1 className="sr-only">Météo et qualité de l’air</h1>
      <Columns n={cols}>
      <Card hero><WxHero w={w} now={now} /></Card>

      <Card tight>
        <CardHead small pict={<Pict icon="air" state={air?.available && air.index >= 4 ? 'warn' : 'off'} />} title="Qualité de l’air" sub={air?.available ? `${air.source} · ${relative(air.updatedAt, now, '')}` : undefined} />
        <AirIndex air={air} now={now} />
        {air?.available && air.pollutants.length > 0 && <AirPollutants air={air} />}
      </Card>

      {air?.available && air.pollens.length > 0 && (
        <Card tight>
          <CardHead small pict={<Pict icon="flower" state={air.pollens.some((p) => p.level >= 4) ? 'warn' : 'off'} />} title="Pollens" sub={air.pollens.every((p) => p.level === 0) ? 'Rien dans l’air en ce moment' : undefined} />
          <AirPollens air={air} />
        </Card>
      )}

      {w.available && hours.length > 0 && (
        <Card tight>
          <CardHead small pict={<Pict icon="clock" />} title="Heures à venir" sub="Météo-France" />
          <div className="wx-strip"><WxHours hours={hours} now={now} full night={w.night} /><Spark values={hours.map((h) => h.temperature ?? 0)} tone="muted" /></div>
        </Card>
      )}

      {w.available && w.daily.length > 0 && (
        <Card tight>
          <CardHead small pict={<Pict icon="sun" />} title="Sept jours" />
          <WxDays days={w.daily} now={now} full />
        </Card>
      )}

      {w.available && (
        <Card tight>
          <CardHead small pict={<Pict icon="thermo" />} title="Détails" />
          <div className="wx-grid">
            <WxStat icon="thermo" label="Ressenti" value={w.feels} unit="°" sub={w.feels != null && w.temperature != null && w.feels < w.temperature ? `Le vent refroidit de ${temp0(w.temperature - w.feels)}°` : undefined} />
            <WxStat icon="drop" label="Humidité" value={w.humidity} unit="%" sub={w.dewPoint != null ? `Point de rosée ${temp0(w.dewPoint)}°` : undefined} />
            <WxStat icon="wind" label="Vent" value={w.windSpeed} unit="km/h" sub={w.windBearing != null ? `${cardinal(w.windBearing, true)}${w.windGust != null ? ` · rafales ${int(w.windGust)}` : ''}` : undefined} />
            <WxStat icon="gauge" label="Pression" value={w.pressure} unit="hPa" />
            <WxStat icon="uv" label="Indice UV" value={w.uv} none={w.night ? 'Pas la nuit' : '—'} />
            <WxStat icon="eye" label="Visibilité" value={w.visibility} unit="km" none="Non fournie" />
            <WxStat icon="cloud" label="Nuages" value={w.cloudCover} unit="%" none="Non fournie" />
            {w.daily[0]?.precipitation != null && <WxStat icon="umbrella" label="Pluie aujourd'hui" value={w.daily[0].precipitation} unit="mm" decimals={1} />}
          </div>
        </Card>
      )}

      {w.available && (
        <Card tight>
          <CardHead small pict={<Pict icon="wind" />} title="Vent" />
          <Compass speed={w.windSpeed} bearing={w.windBearing} gust={w.windGust} />
        </Card>
      )}

      {w.available && (
        <Card tight>
          <div className="stack">
            <UV uv={w.night ? undefined : w.uv} />
            <Pressure hpa={w.pressure} trend="flat" />
          </div>
        </Card>
      )}

      {w.sunrise && w.sunset && (
        <Card tight>
          <CardHead small pict={<Pict icon="sunrise" />} title="Soleil" />
          <SunArc sunrise={w.sunrise} sunset={w.sunset} now={now} />
        </Card>
      )}

      {home.globals.heating.average != null && (
        <Card tight>
          <Section title="Dedans / dehors" />
          <InOut inside={home.globals.heating.average} outside={w.temperature} cond={w.condition} night={w.night} />
        </Card>
      )}
      </Columns>
    </div>
  );
}

/** Cards dealt into n columns in reading order (1, 2, 3 on the first row…), each column as tall as its cards. */
function Columns({ n, children }: { n: number; children: ReactNode }) {
  const items = Children.toArray(children).filter(Boolean);
  const cols: ReactNode[][] = Array.from({ length: n }, () => []);
  items.forEach((it, i) => cols[i % n].push(it));
  return <div className="wx-cols" style={{ ['--n' as string]: n } as CSSProperties}>{cols.map((col, i) => <div key={i} className="wx-col">{col}</div>)}</div>;
}
