import { useEffect, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import type { Device, Media, Track } from '@control/model';
import { Card, Player, SearchField, Seg, Slider, Station, TrackRow } from '@control/ui';
import { api, HttpError } from '../lib/api';
import { useActions } from '../lib/actions';
import { deviceImage } from '../lib/derive';

type Act = ReturnType<typeof useActions>;

/**
 * A speaker: what plays now, then Radio (the stations of control.yaml) or
 * Musique (Spotify search; a touch on a result starts it on this speaker),
 * then the volume.
 */
export function MediaCard({ d, media, act, pending }: { d: Device; media?: Media; act: Act; pending: boolean }) {
  const radios = media?.radios ?? [];
  const spotify = !!media?.spotify;
  const [tab, setTab] = useState<'radio' | 'music'>(() => (spotify && (d.state.app === 'Spotify' || radios.length === 0) ? 'music' : 'radio'));
  const playing = d.state.playing === 'playing';
  const radio = radios.find((r) => r.id === d.state.radio);
  const toggle = () => act.device(d, { action: playing ? 'pause' : 'play' }, { quiet: true }).catch(() => {});
  return (
    <Card tight unavailable={!d.available} className="media-card">
      <div className="stack">
        <Player title={d.state.title || radio?.name || d.name} artist={[radio ? radio.genre : d.state.artist, d.state.app || d.name].filter(Boolean).join(' · ')}
          playing={playing} volume={d.state.volume} progress={playing && radio ? 'live' : undefined} cover={radio ? undefined : deviceImage(d)} pending={pending} onToggle={toggle} />
        {d.available && radios.length > 0 && spotify && (
          <Seg items={[{ id: 'radio', label: 'Radio' }, { id: 'music', label: 'Musique' }]} active={tab} onSelect={(id) => setTab(id as 'radio' | 'music')} />
        )}
        {d.available && tab === 'radio' && radios.length > 0 && (
          <div className="stations" role="list" aria-label="Stations de radio">
            {radios.map((r) => (
              <Station key={r.id} radio={r} playing={playing && d.state.radio === r.id} pending={pending}
                onPlay={() => act.device(d, { action: 'play_radio', params: { id: r.id } }, { quiet: true }).catch(() => {})} />
            ))}
          </div>
        )}
        {d.available && tab === 'music' && spotify && <MusicSearch d={d} act={act} pending={pending} playing={playing} />}
        {d.available && d.state.volume != null && (
          <Slider label="Volume" value={d.state.volume} cool ends={['Muet', 'Fort']} pending={pending}
            onCommit={(pct) => act.device(d, { action: 'set_volume', params: { pct } }, { quiet: true }).catch(() => {})} />
        )}
      </div>
    </Card>
  );
}

function MusicSearch({ d, act, pending, playing }: { d: Device; act: Act; pending: boolean; playing: boolean }) {
  const [q, setQ] = useState('');
  const [dq, setDq] = useState('');
  useEffect(() => { const t = setTimeout(() => setDq(q.trim()), 350); return () => clearTimeout(t); }, [q]);
  const res = useQuery<{ tracks: Track[] }, HttpError>({
    queryKey: ['spotify', dq],
    queryFn: () => api(`/api/media/search?q=${encodeURIComponent(dq)}`),
    enabled: dq.length >= 2, staleTime: 5 * 60_000, retry: 0,
  });
  const isCurrent = (t: Track) => d.state.app === 'Spotify' && d.state.title === t.title;
  return (
    <>
      <SearchField value={q} onChange={setQ} placeholder="Rechercher un titre, un artiste, un album" label="Rechercher dans Spotify" busy={res.isFetching} />
      {dq.length < 2 ? (
        <div className="media-empty">Un titre ou un artiste ; toucher un résultat le lance sur {d.name.toLowerCase()}.</div>
      ) : res.isError ? (
        <div className="media-empty" role="alert">{res.error.body.error}{res.error.body.detail ? ` : ${res.error.body.detail}` : ''}</div>
      ) : res.data && res.data.tracks.length === 0 ? (
        <div className="media-empty">Rien trouvé pour « {dq} ».</div>
      ) : res.data ? (
        <>
          <div className="media-sec">Résultats Spotify</div>
          <div className="tracks">
            {res.data.tracks.map((t) => (
              <TrackRow key={t.uri} track={t} playing={playing && isCurrent(t)} pending={pending}
                onPlay={() => {
                  if (isCurrent(t)) act.device(d, { action: playing ? 'pause' : 'play' }, { quiet: true }).catch(() => {});
                  else act.device(d, { action: 'play_spotify', params: { uri: t.uri, title: t.title, artists: t.artists } }, { quiet: true }).catch(() => {});
                }} />
            ))}
          </div>
        </>
      ) : null}
    </>
  );
}
