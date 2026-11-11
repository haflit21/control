import { useEffect, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import type { Album, Device, Media, MusicResults, Track } from '@control/model';
import { AlbumTile, Card, MediaPlayer, SearchField, Seg, Station, TrackRow } from '@control/ui';
import { api, HttpError } from '../lib/api';
import { useActions } from '../lib/actions';

type Act = ReturnType<typeof useActions>;
type Tab = 'radio' | 'music' | 'favs';

const FAVS_KEY = ['music-favorites'];

/**
 * A speaker: the player (artwork, progress, previous / play / next, volume),
 * then Radio (the stations of control.yaml), Musique (search through Music
 * Assistant: titles and albums) or Favoris (the household's starred titles
 * and albums). A touch on a result plays it on this speaker.
 */
export function MediaCard({ d, media, act, pending }: { d: Device; media?: Media; act: Act; pending: boolean }) {
  const radios = media?.radios ?? [];
  const music = !!media?.music;
  const [tab, setTab] = useState<Tab>(() => (music && ((!d.state.radio && d.state.playing === 'playing' && !!d.state.title) || radios.length === 0) ? 'music' : 'radio'));
  const playing = d.state.playing === 'playing';
  const radio = radios.find((r) => r.id === d.state.radio);
  const caps = new Set(d.capabilities ?? []);
  const send = (a: Parameters<Act['device']>[1]) => act.device(d, a, { quiet: true }).catch(() => {});
  const toggle = () => send({ action: playing ? 'pause' : 'play' });
  const tabs = [
    ...(radios.length > 0 ? [{ id: 'radio', label: 'Radio' }] : []),
    ...(music ? [{ id: 'music', label: 'Musique' }, { id: 'favs', label: 'Favoris' }] : []),
  ];
  const title = radio ? radio.name : d.state.title || d.name;
  const sub = radio ? [radio.genre, d.state.title && d.state.title !== radio.name ? d.state.title : ''].filter(Boolean).join(' · ')
    : [d.state.artist, d.state.album].filter(Boolean).join(' · ');
  return (
    <Card tight unavailable={!d.available} className="media-card">
      <div className="stack">
        <MediaPlayer title={title} sub={sub || undefined} app={radio ? 'Radio' : appLabel(d.state.app)} cover={radio ? undefined : d.state.cover} radio={radio}
          playing={playing} pending={pending} elapsed={d.state.elapsed} elapsedAt={d.state.elapsedAt} duration={d.state.duration}
          canPrev={caps.has('previous')} canNext={caps.has('next')} canSeek={caps.has('seek')} volume={d.available ? d.state.volume : undefined}
          onToggle={toggle} onPrev={() => send({ action: 'previous' })} onNext={() => send({ action: 'next' })}
          onSeek={(position) => send({ action: 'seek', params: { position } })}
          onVolume={(pct) => send({ action: 'set_volume', params: { pct } })} />
        {d.available && tabs.length > 1 && <Seg items={tabs} active={tab} onSelect={(id) => setTab(id as Tab)} />}
        {d.available && tab === 'radio' && radios.length > 0 && (
          <div className="stations" role="list" aria-label="Stations de radio">
            {radios.map((r) => (
              <Station key={r.id} radio={r} playing={playing && d.state.radio === r.id} pending={pending}
                onPlay={() => send({ action: 'play_radio', params: { id: r.id } })} />
            ))}
          </div>
        )}
        {d.available && music && tab === 'music' && <MusicSearch d={d} source={media?.musicSource} send={send} pending={pending} playing={playing} />}
        {d.available && music && tab === 'favs' && <Favorites d={d} send={send} pending={pending} playing={playing} />}
      </div>
    </Card>
  );
}

/** The Cast app name, except the generic receiver that plays our streams. */
function appLabel(app?: string): string | undefined {
  return app && !/default media receiver/i.test(app) ? app : undefined;
}

/** The provider-independent part of a URI: spotify--x://track/1 and spotify://track/1 are the same title. */
function uriKey(uri?: string): string {
  if (!uri) return '';
  const i = uri.indexOf('://');
  if (i >= 0) return uri.slice(i + 3);
  const p = uri.split(':');
  return p.length >= 3 ? `${p[p.length - 2]}/${p[p.length - 1]}` : uri;
}

type Send = (a: Parameters<Act['device']>[1]) => Promise<unknown>;

function useFavorites() {
  const qc = useQueryClient();
  const q = useQuery<MusicResults, HttpError>({ queryKey: FAVS_KEY, queryFn: () => api('/api/media/favorites'), staleTime: 60_000 });
  const keys = new Set([...(q.data?.tracks ?? []).map((t) => t.uri), ...(q.data?.albums ?? []).map((a) => a.uri)]);
  const set = (kind: 'track' | 'album', item: Track | Album, on: boolean) => {
    // optimistic: the star changes at once; the server's list replaces it
    qc.setQueryData<MusicResults>(FAVS_KEY, (old) => {
      const cur = old ?? { tracks: [], albums: [] };
      if (kind === 'track') return { ...cur, tracks: on ? [item as Track, ...cur.tracks.filter((t) => t.uri !== item.uri)] : cur.tracks.filter((t) => t.uri !== item.uri) };
      return { ...cur, albums: on ? [item as Album, ...cur.albums.filter((a) => a.uri !== item.uri)] : cur.albums.filter((a) => a.uri !== item.uri) };
    });
    api<MusicResults>('/api/media/favorites', { method: 'POST', body: JSON.stringify({ kind, item, on }) })
      .then((r) => qc.setQueryData(FAVS_KEY, r))
      .catch(() => qc.invalidateQueries({ queryKey: FAVS_KEY }));
  };
  return { q, has: (uri: string) => keys.has(uri), set };
}

function Results({ d, res, send, pending, playing, favs }: { d: Device; res: MusicResults; send: Send; pending: boolean; playing: boolean; favs: ReturnType<typeof useFavorites> }) {
  const cur = d.state.radio ? '' : uriKey(d.state.uri);
  const isCurrent = (t: Track) => !d.state.radio && (cur ? uriKey(t.uri) === cur : !!d.state.title && d.state.title === t.title && (d.state.album ?? '') === (t.album ?? ''));
  const albumCurrent = (a: Album) => !d.state.radio && !!d.state.album && d.state.album === a.title;
  return (
    <>
      {res.albums.length > 0 && (
        <>
          <div className="media-sec">Albums</div>
          <div className="albums">
            {res.albums.map((a) => (
              <AlbumTile key={a.uri} album={a} playing={playing && albumCurrent(a)} pending={pending}
                fav={favs.has(a.uri)} onFav={() => favs.set('album', a, !favs.has(a.uri))}
                onPlay={() => send({ action: 'play_music', params: { uri: a.uri, kind: 'album', title: a.title, artists: a.artists } })} />
            ))}
          </div>
        </>
      )}
      {res.tracks.length > 0 && (
        <>
          <div className="media-sec">Titres</div>
          <div className="tracks">
            {res.tracks.map((t) => (
              <TrackRow key={t.uri} track={t} playing={playing && isCurrent(t)} pending={pending}
                fav={favs.has(t.uri)} onFav={() => favs.set('track', t, !favs.has(t.uri))}
                onPlay={() => {
                  if (isCurrent(t)) send({ action: playing ? 'pause' : 'play' });
                  else send({ action: 'play_music', params: { uri: t.uri, kind: 'track', title: t.title, artists: t.artists } });
                }} />
            ))}
          </div>
        </>
      )}
    </>
  );
}

function MusicSearch({ d, source, send, pending, playing }: { d: Device; source?: string; send: Send; pending: boolean; playing: boolean }) {
  const [q, setQ] = useState('');
  const [dq, setDq] = useState('');
  useEffect(() => { const t = setTimeout(() => setDq(q.trim()), 350); return () => clearTimeout(t); }, [q]);
  const favs = useFavorites();
  const res = useQuery<MusicResults, HttpError>({
    queryKey: ['music', dq],
    queryFn: () => api(`/api/media/search?q=${encodeURIComponent(dq)}`),
    enabled: dq.length >= 2, staleTime: 5 * 60_000, retry: 0,
  });
  const empty = res.data && res.data.tracks.length === 0 && res.data.albums.length === 0;
  return (
    <>
      <SearchField value={q} onChange={setQ} placeholder="Rechercher un titre, un artiste, un album" label="Rechercher de la musique" busy={res.isFetching} />
      {dq.length < 2 ? (
        <div className="media-empty">Un titre, un artiste ou un album ; toucher un résultat le lance sur {d.name.toLowerCase()}, l’étoile le garde dans Favoris.</div>
      ) : res.isError ? (
        <div className="media-empty" role="alert">{res.error.body.error}{res.error.body.detail ? ` : ${res.error.body.detail}` : ''}</div>
      ) : empty ? (
        <div className="media-empty">Rien trouvé pour « {dq} ».</div>
      ) : res.data ? (
        <>
          {source && <div className="media-src">Résultats · {source}</div>}
          <Results d={d} res={res.data} send={send} pending={pending} playing={playing} favs={favs} />
        </>
      ) : null}
    </>
  );
}

function Favorites({ d, send, pending, playing }: { d: Device; send: Send; pending: boolean; playing: boolean }) {
  const favs = useFavorites();
  if (favs.q.isError) return <div className="media-empty" role="alert">{favs.q.error.body.error}</div>;
  if (!favs.q.data) return null;
  if (favs.q.data.tracks.length === 0 && favs.q.data.albums.length === 0) {
    return <div className="media-empty">Aucun favori. Dans Musique, l’étoile d’un titre ou d’un album le garde ici.</div>;
  }
  return <Results d={d} res={favs.q.data} send={send} pending={pending} playing={playing} favs={favs} />;
}
