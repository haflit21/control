// « Personnes et visages » : the household (control.yaml people:) and the photos
// the face service keeps for each. Photos are taken here, by the device that
// will recognise them (the wall tablet, at its place): 3 to 5 each, facing the
// camera then slightly turned. Only face prints are kept, never the photos.
import { useCallback, useEffect, useRef, useState } from 'react';
import type { Member } from '@control/model';
import { Icon, Pict, Popover, Toggle, cx } from '@control/ui';
import { api } from '../lib/api';
import { pushToast } from '../lib/actions';
import { cameraError, cameraSupported, identify, openCamera, setFaceOnWake, snap, useFaceOnWake, type Camera } from '../lib/profile';

type PersonFaces = Member & { samples: number | null };
type People = { people: PersonFaces[]; face: boolean; error?: string };
type Mode = { kind: 'list' } | { kind: 'enrol'; person: PersonFaces } | { kind: 'test' };
type Note = { text: string; tone?: 'ok' | 'warn' | 'err' };

function photos(n: number | null): string {
  if (n === null) return 'Photos : inconnu';
  if (n === 0) return 'Aucune photo';
  return n === 1 ? '1 photo · en ajouter 2 ou 3' : `${n} photos`;
}

/** « de Marion », « d’Idris » */
function de(name: string): string {
  return /^[aeiouyhàâéèêëîïôûü]/i.test(name) ? `d’${name}` : `de ${name}`;
}

export function Avatar({ m, size }: { m: Member; size?: 's' }) {
  return <span className={cx('avatar', size === 's' && '-s')} style={m.color ? { background: m.color, borderColor: m.color, color: '#15171c' } : undefined} aria-hidden="true">{m.name.charAt(0)}</span>;
}

export function PeopleSheet({ onClose }: { onClose: () => void }) {
  const [data, setData] = useState<People | null>(null);
  const [mode, setMode] = useState<Mode>({ kind: 'list' });
  const [forget, setForget] = useState<string | null>(null);
  const onWake = useFaceOnWake();

  const load = useCallback(() => {
    api<People>('/api/people').then(setData).catch((e) => setData({ people: [], face: false, error: cameraError(e) }));
  }, []);
  useEffect(load, [load]);

  const doForget = async (p: PersonFaces) => {
    try {
      const r = await api<{ message: string }>(`/api/people/${encodeURIComponent(p.id)}/faces`, { method: 'DELETE' });
      pushToast({ title: r.message, tone: 'ok' });
    } catch (e) {
      pushToast({ title: 'Oubli impossible', detail: cameraError(e), tone: 'error' });
    }
    setForget(null);
    load();
  };

  return (
    <Popover onClose={onClose} label="Personnes et visages">
      <div className="sheet-head">
        <Pict icon="person" size="l" state="on" />
        <div><div className="sheet-t">Personnes et visages</div><div className="sheet-d">Reconnaître qui réveille l’écran : son nom et son agenda à l’accueil.</div></div>
      </div>

      {!data && <div className="people-note">Chargement…</div>}
      {data && !data.face && <div className="people-note -warn">Reconnaissance non configurée : <code>face.url</code> dans control.yaml et <code>FACE_API_KEY</code> dans les secrets.</div>}
      {data?.error && <div className="people-note -warn">{data.error}</div>}

      {data && mode.kind === 'list' && (
        <>
          <div className="list people">
            {data.people.map((p) => (
              <div key={p.id} className="row people-row">
                <Avatar m={p} />
                <div className="row-main"><div className="row-name">{p.name}</div><div className="row-sub">{photos(p.samples)}</div></div>
                {data.face && (forget === p.id ? (
                  <span className="people-act">
                    <button type="button" className="btn -ghost" onClick={() => setForget(null)}>Annuler</button>
                    <button type="button" className="btn -danger" onClick={() => void doForget(p)}>Oublier</button>
                  </span>
                ) : (
                  <span className="people-act">
                    {!!p.samples && <button type="button" className="btn -ghost" onClick={() => setForget(p.id)} aria-label={`Oublier le visage ${de(p.name)}`}><Icon name="trash" /></button>}
                    <button type="button" className="btn" onClick={() => setMode({ kind: 'enrol', person: p })} aria-label={`Ajouter des photos ${de(p.name)}`}><Icon name="camera" />Photos</button>
                  </span>
                ))}
              </div>
            ))}
            {data.people.length === 0 && <div className="people-note">Aucune personne : ajoute-les dans <code>people:</code> de control.yaml.</div>}
          </div>
          {data.face && (
            <>
              <div className="sheet-sec">Cet appareil</div>
              <div className="people-wake">
                <Pict icon="camera" size="s" state={onWake ? 'on' : 'off'} />
                <div className="people-wake-t">Reconnaître au réveil<small>{cameraSupported() ? 'Une photo quand on touche l’écran de veille' : 'Caméra indisponible (HTTPS requis)'}</small></div>
                <Toggle small on={onWake} disabled={!cameraSupported()} label={onWake ? 'Ne plus reconnaître au réveil sur cet appareil' : 'Reconnaître au réveil sur cet appareil'} onChange={setFaceOnWake} />
              </div>
              <div className="people-foot">
                <button type="button" className="btn" onClick={() => setMode({ kind: 'test' })} disabled={!cameraSupported()}><Icon name="eye" />Qui suis-je ?</button>
              </div>
            </>
          )}
        </>
      )}

      {data && mode.kind !== 'list' && <CameraPane mode={mode} onDone={() => { setMode({ kind: 'list' }); load(); }} />}
    </Popover>
  );
}

/** Live preview + shutter: adds a photo to a person, or says who is in front. */
function CameraPane({ mode, onDone }: { mode: Exclude<Mode, { kind: 'list' }>; onDone: () => void }) {
  const video = useRef<HTMLVideoElement>(null);
  const cam = useRef<Camera | null>(null);
  const [ready, setReady] = useState(false);
  const [busy, setBusy] = useState(false);
  const [note, setNote] = useState<Note | null>(null);
  const [count, setCount] = useState(mode.kind === 'enrol' ? mode.person.samples ?? 0 : 0);

  useEffect(() => {
    let gone = false;
    openCamera(video.current!).then((c) => { if (gone) c.stop(); else { cam.current = c; setReady(true); } })
      .catch((e) => { if (!gone) setNote({ text: cameraError(e), tone: 'err' }); });
    return () => { gone = true; cam.current?.stop(); cam.current = null; };
  }, []);

  const shoot = async () => {
    if (!cam.current || busy) return;
    setBusy(true);
    try {
      const photo = await snap(cam.current.video);
      if (mode.kind === 'enrol') {
        const r = await api<{ samples: number; own: number | null }>(`/api/people/${encodeURIComponent(mode.person.id)}/faces`, { method: 'POST', body: photo, headers: { 'Content-Type': 'image/jpeg' } });
        setCount(r.samples);
        const close = r.own == null ? '' : r.own >= 0.5 ? ' · ressemble bien aux précédentes' : ' · différente des précédentes : vérifie la lumière';
        setNote({ text: `Photo ${r.samples} enregistrée${close}`, tone: r.own != null && r.own < 0.5 ? 'warn' : 'ok' });
      } else {
        const m = await identify(photo);
        setNote(m.person ? { text: `C’est ${m.person.name} (ressemblance ${m.score.toFixed(2)}, seuil ${m.threshold.toFixed(2)})`, tone: 'ok' }
          : m.faces === 0 ? { text: 'Aucun visage : mets-toi face à la caméra.', tone: 'warn' }
          : { text: `Visage inconnu (ressemblance la plus proche ${m.score.toFixed(2)}, seuil ${m.threshold.toFixed(2)})`, tone: 'warn' });
      }
    } catch (e) {
      setNote({ text: cameraError(e), tone: 'err' });
    }
    setBusy(false);
  };

  const title = mode.kind === 'enrol' ? `Photos ${de(mode.person.name)}` : 'Qui suis-je ?';
  const tip = mode.kind === 'enrol'
    ? 'Une personne à la fois, à la place habituelle : de face, puis tourné un peu à gauche, à droite, avec et sans lunettes. 3 à 5 photos.'
    : 'Mets-toi face à la caméra, comme au réveil de l’écran.';
  return (
    <div className="cam-pane">
      <div className="cam-title">{title}{mode.kind === 'enrol' && <span className="muted"> · {count} photo{count > 1 ? 's' : ''}</span>}</div>
      <div className="cam-view"><video ref={video} playsInline muted /></div>
      <p className="people-note">{tip}</p>
      {note && <p className={cx('people-note', note.tone && `-${note.tone}`)} role="status">{note.text}</p>}
      <div className="people-foot">
        <button type="button" className="btn -ghost" onClick={onDone}>{mode.kind === 'enrol' ? 'Terminé' : 'Retour'}</button>
        <button type="button" className="btn -primary" disabled={!ready || busy} onClick={() => void shoot()}><Icon name="camera" />{mode.kind === 'enrol' ? 'Prendre une photo' : 'Qui est-ce ?'}</button>
      </div>
    </div>
  );
}
