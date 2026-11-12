import { useState } from 'react';
import type { Home } from '@control/model';
import { Card, CardHead, Pict, Seg } from '@control/ui';
import { useActions, usePending } from '../lib/actions';
import { Favorites, MusicSearch, SpeakerPlayer, Stations, WithAlbum, useSend } from './MediaCard';

/**
 * /medias — a speaker's player, then side by side, without tabs: the radios,
 * the music search (Music Assistant, Spotify inside) and the favourites. An
 * album opens in its column.
 */
export function MediaPage({ home }: { home: Home }) {
  const speakers = home.rooms.flatMap((room) => room.devices.filter((d) => d.kind === 'media').map((d) => ({ room, d })));
  const [id, setId] = useState<string | undefined>(undefined);
  const cur = speakers.find((s) => s.d.id === id) ?? speakers[0];
  const act = useActions();
  const pending = usePending();
  if (!cur) {
    return <div className="page"><header className="dash-top hrow"><div className="greet"><h1>Médias</h1><p>Aucune enceinte configurée</p></div></header></div>;
  }
  return <Speaker key={cur.d.id} home={home} cur={cur} speakers={speakers} onPick={setId} act={act} pending={!!pending[cur.d.id]} />;
}

type Pick = { room: Home['rooms'][number]; d: Home['rooms'][number]['devices'][number] };

function Speaker({ home, cur, speakers, onPick, act, pending }: { home: Home; cur: Pick; speakers: Pick[]; onPick: (id: string) => void; act: ReturnType<typeof useActions>; pending: boolean }) {
  const { room, d } = cur;
  const media = home.media;
  const send = useSend(d, act);
  const playing = d.state.playing === 'playing';
  const music = !!media?.music;
  return (
    <div className="page">
      <header className="dash-top hrow">
        <div className="greet"><h1>Médias</h1><p>{d.name} · {room.name}</p></div>
        {speakers.length > 1 && <Seg items={speakers.map((s) => ({ id: s.d.id, label: s.room.name }))} active={d.id} onSelect={onPick} />}
      </header>
      <div className={d.available ? 'media-page' : 'media-page is-unavailable'}>
        <SpeakerPlayer d={d} media={media} send={send} pending={pending} />
        {d.available && (
          <div className="media-cols">
            {(media?.radios.length ?? 0) > 0 && (
              <Card tight className="media-col">
                <CardHead small pict={<Pict icon="radio" />} title="Radio" sub={`${media!.radios.length} stations`} />
                <Stations d={d} media={media} send={send} pending={pending} one />
              </Card>
            )}
            {music && (
              <Card tight className="media-col">
                <CardHead small pict={<Pict icon="search" />} title="Recherche" sub={media?.musicSource ?? 'Titres et albums'} />
                <WithAlbum d={d} send={send} pending={pending}>{(onOpen) => <MusicSearch d={d} send={send} pending={pending} playing={playing} onOpen={onOpen} />}</WithAlbum>
              </Card>
            )}
            {music && (
              <Card tight className="media-col">
                <CardHead small pict={<Pict icon="star" />} title="Favoris" sub="Titres et albums de la maison" />
                <WithAlbum d={d} send={send} pending={pending}>{(onOpen) => <Favorites d={d} send={send} pending={pending} playing={playing} onOpen={onOpen} />}</WithAlbum>
              </Card>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
