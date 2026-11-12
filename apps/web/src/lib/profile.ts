// Who is in front of the screen. A device that opted in (« Reconnaître au réveil »,
// the wall tablet) takes one photo when the sleep screen is touched and asks the
// server who it is; the home page then greets the person and shows the shared
// calendars plus theirs. The photo only lives in memory for the request.
// Recognition personalises the screen, it authorises nothing.
import { useSyncExternalStore } from 'react';
import type { Agenda, Member } from '@control/model';
import { api, HttpError } from './api';
import { pushToast } from './actions';

export type Profile =
  | { state: 'none' }
  | { state: 'looking' }
  | { state: 'known'; person: Member; score: number }
  | { state: 'unknown'; faces: number; score: number }
  | { state: 'error'; message: string };

export type Match = { person: Member | null; score: number; faces: number; threshold: number };

const KEY = 'control.faceOnWake'; // per device: only the devices that opted in open their camera
let current: Profile = { state: 'none' };
let onWake = readSetting();
const subs = new Set<() => void>();
const notify = () => subs.forEach((f) => f());
const subscribe = (f: () => void) => { subs.add(f); return () => { subs.delete(f); }; };

export function useProfile(): Profile {
  return useSyncExternalStore(subscribe, () => current);
}
export function setProfile(p: Profile) { current = p; notify(); }
/** Back to the neutral home (sleep, « Pas toi ? »). */
export function clearProfile() { if (current.state !== 'none') setProfile({ state: 'none' }); }

/** The person recognised, if any. */
export function profilePerson(p: Profile): Member | undefined {
  return p.state === 'known' ? p.person : undefined;
}

// ---- per-device setting ----

function readSetting(): boolean {
  try { return localStorage.getItem(KEY) === '1'; } catch { return false; }
}
export function setFaceOnWake(on: boolean) {
  try { if (on) localStorage.setItem(KEY, '1'); else localStorage.removeItem(KEY); } catch { /* private mode: kept for this visit */ }
  onWake = on;
  notify();
}
export function useFaceOnWake(): boolean {
  return useSyncExternalStore(subscribe, () => onWake);
}
export function faceOnWake(): boolean { return onWake; }

// ---- camera ----

export function cameraSupported(): boolean {
  return typeof navigator !== 'undefined' && !!navigator.mediaDevices?.getUserMedia;
}

export type Camera = { video: HTMLVideoElement; stop: () => void };

/** Opens the front camera into `video` (a hidden one when absent: Safari only renders a video that is in the page). */
export async function openCamera(video?: HTMLVideoElement): Promise<Camera> {
  const own = !video;
  const v = video ?? Object.assign(document.createElement('video'), { muted: true, playsInline: true });
  if (own) {
    v.setAttribute('playsinline', '');
    v.style.cssText = 'position:fixed;left:0;bottom:0;width:2px;height:2px;opacity:0;pointer-events:none';
    document.body.appendChild(v);
  }
  let stream: MediaStream | undefined;
  const stop = () => {
    stream?.getTracks().forEach((t) => t.stop());
    v.srcObject = null;
    if (own) v.remove();
  };
  try {
    stream = await navigator.mediaDevices.getUserMedia({ video: { facingMode: 'user', width: { ideal: 640 }, height: { ideal: 480 } }, audio: false });
    v.srcObject = stream;
    await v.play();
    if (!v.videoWidth) await new Promise((r) => v.addEventListener('loadeddata', r, { once: true }));
    return { video: v, stop };
  } catch (e) {
    stop();
    throw e;
  }
}

/** One JPEG of what the camera sees now. */
export function snap(video: HTMLVideoElement): Promise<Blob> {
  if (!video.videoWidth) return Promise.reject(new Error('la caméra ne donne pas encore d’image'));
  const c = document.createElement('canvas');
  c.width = video.videoWidth;
  c.height = video.videoHeight;
  c.getContext('2d')!.drawImage(video, 0, 0);
  return new Promise((ok, ko) => c.toBlob((b) => (b ? ok(b) : ko(new Error('photo impossible'))), 'image/jpeg', 0.85));
}

export function identify(photo: Blob): Promise<Match> {
  return api<Match>('/api/face/identify', { method: 'POST', body: photo, headers: { 'Content-Type': 'image/jpeg' } });
}

export function cameraError(e: unknown): string {
  if (e instanceof HttpError) return [e.body.error, e.body.detail].filter(Boolean).join(' · ');
  if (e instanceof DOMException && (e.name === 'NotAllowedError' || e.name === 'SecurityError')) return 'Caméra refusée : autorise-la pour cette adresse dans les réglages du navigateur.';
  if (e instanceof DOMException && e.name === 'NotFoundError') return 'Aucune caméra sur cet appareil.';
  if (!cameraSupported()) return 'Caméra indisponible : l’app doit être ouverte en HTTPS.';
  return e instanceof Error ? e.message : 'Erreur';
}

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));
let waking = 0;

/**
 * The sleep screen was touched: open the camera (inside the tap), let it settle
 * its exposure, take a photo (up to 3 when no face is seen), close the camera,
 * ask the server. The home page shows the neutral greeting meanwhile.
 */
export async function recognizeOnWake(): Promise<void> {
  const run = ++waking;
  setProfile({ state: 'looking' });
  let cam: Camera | undefined;
  try {
    cam = await openCamera();
    await sleep(400);
    let m: Match | undefined;
    for (let i = 0; i < 3 && run === waking; i++) {
      if (i) await sleep(250);
      m = await identify(await snap(cam.video));
      if (m.faces > 0) break;
    }
    cam.stop();
    cam = undefined;
    if (run !== waking || !m) return; // a newer wake or a sleep took over
    setProfile(m.person ? { state: 'known', person: m.person, score: m.score } : { state: 'unknown', faces: m.faces, score: m.score });
  } catch (e) {
    cam?.stop();
    if (run !== waking) return;
    const message = cameraError(e);
    setProfile({ state: 'error', message });
    pushToast({ title: 'Reconnaissance impossible', detail: message, tone: 'error' });
  }
}

/** Going to sleep: forget who was there (and drop a recognition still running). */
export function sleepProfile() {
  waking++;
  clearProfile();
}

/** What a person sees of the agenda: the shared calendars and their own. */
export function agendaFor(agenda: Agenda, personId: string): Agenda {
  const shown = new Set(agenda.calendars.filter((c) => !c.owner || c.owner === personId).map((c) => c.id));
  return { ...agenda, calendars: agenda.calendars.filter((c) => shown.has(c.id)), events: agenda.events.filter((e) => shown.has(e.calendar)) };
}
