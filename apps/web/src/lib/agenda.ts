// Agenda: API calls and the voice recorder.
// The server owns the events; every screen gets the new agenda through SSE
// after a write, so nothing here patches the cache by hand.
import type { AgendaEvent, EventDraft } from '@maison/model';
import { api } from './api';

export type Saved = { ok: boolean; message?: string; event?: AgendaEvent };

export const getEvent = (id: string) => api<AgendaEvent>(`/api/agenda/events/${encodeURIComponent(id)}`);
export const createEvent = (e: AgendaEvent) => api<Saved>('/api/agenda/events', { method: 'POST', body: JSON.stringify(e) });
export const updateEvent = (id: string, e: AgendaEvent) => api<Saved>(`/api/agenda/events/${encodeURIComponent(id)}`, { method: 'PUT', body: JSON.stringify(e) });
export const deleteEvent = (id: string) => api<Saved>(`/api/agenda/events/${encodeURIComponent(id)}`, { method: 'DELETE' });

export type Understood = { draft: EventDraft; warning?: string };
export const parseText = (text: string) => api<Understood>('/api/agenda/parse', { method: 'POST', body: JSON.stringify({ text }) });
export const sendVoice = (audio: Blob) => api<Understood>('/api/agenda/voice', { method: 'POST', body: audio, headers: { 'Content-Type': audio.type || 'audio/webm' } });

/** The mic needs a secure context (HTTPS, or localhost) and MediaRecorder. */
export function micSupported(): boolean {
  return typeof window !== 'undefined' && window.isSecureContext && !!navigator.mediaDevices?.getUserMedia && typeof MediaRecorder !== 'undefined';
}

function pickMime(): string | undefined {
  for (const t of ['audio/webm;codecs=opus', 'audio/webm', 'audio/mp4', 'audio/ogg;codecs=opus']) {
    if (MediaRecorder.isTypeSupported?.(t)) return t;
  }
  return undefined; // let the browser choose (Safari: audio/mp4)
}

export type Recording = {
  /** stop now and deliver what was said */
  stop: () => void;
  /** stop and throw it away */
  cancel: () => void;
  /** the recording, once stopped (rejects if cancelled or empty) */
  done: Promise<Blob>;
  /** (re)attach the level and timer callbacks (when the UI that shows them mounts later) */
  watch: (onLevel?: (level: number) => void, onTick?: (ms: number) => void) => void;
};

export type RecOptions = {
  /** 0–1, about 30 times a second */
  onLevel?: (level: number) => void;
  /** milliseconds since the start, every 250 ms */
  onTick?: (ms: number) => void;
  /** stop by itself after this much silence once speech has started (default 1.6 s) */
  silenceMs?: number;
  /** hard limit (default 20 s) */
  maxMs?: number;
};

/**
 * Starts recording from the mic. Stops by itself after a pause following speech,
 * or at maxMs. The stream is released as soon as it stops (the iPhone's orange dot goes away).
 */
export async function record(opts: RecOptions = {}): Promise<Recording> {
  const silenceMs = opts.silenceMs ?? 1600;
  const maxMs = opts.maxMs ?? 20000;
  // the AudioContext is created before the permission prompt: Safari only lets it run
  // when it is made inside the tap that started the recording
  const AC: typeof AudioContext | undefined = window.AudioContext ?? (window as unknown as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext;
  const ac = AC ? new AC() : undefined;
  let stream: MediaStream;
  try {
    stream = await navigator.mediaDevices.getUserMedia({ audio: { echoCancellation: true, noiseSuppression: true, channelCount: 1 } });
  } catch (e) {
    void ac?.close().catch(() => {});
    throw e;
  }
  void ac?.resume().catch(() => {});
  const mime = pickMime();
  const rec = new MediaRecorder(stream, mime ? { mimeType: mime } : undefined);
  const chunks: Blob[] = [];
  let cancelled = false;
  rec.ondataavailable = (e) => { if (e.data.size > 0) chunks.push(e.data); };

  // level meter + end-of-speech detection
  const analyser = ac?.createAnalyser();
  if (ac && analyser) {
    analyser.fftSize = 1024;
    ac.createMediaStreamSource(stream).connect(analyser);
  }
  const buf = new Float32Array(1024);
  const t0 = performance.now();
  let spoke = false;
  let lastVoice = t0;
  let raf = 0;
  let onLevel = opts.onLevel;
  let onTick = opts.onTick;
  const tick = window.setInterval(() => onTick?.(performance.now() - t0), 250);

  const release = () => {
    cancelAnimationFrame(raf);
    clearInterval(tick);
    stream.getTracks().forEach((t) => t.stop());
    void ac?.close().catch(() => {});
  };

  const done = new Promise<Blob>((resolve, reject) => {
    rec.onstop = () => {
      release();
      const blob = new Blob(chunks, { type: (rec.mimeType || mime || 'audio/webm').split(';')[0] });
      if (cancelled) reject(new Error('annulé'));
      else if (blob.size < 200) reject(new Error('Rien enregistré'));
      else resolve(blob);
    };
    rec.onerror = () => { release(); reject(new Error('Le micro s’est arrêté')); };
  });

  const stop = () => { if (rec.state !== 'inactive') rec.stop(); };
  const loop = () => {
    const now = performance.now();
    if (analyser) {
      analyser.getFloatTimeDomainData(buf);
      let sum = 0;
      for (let i = 0; i < buf.length; i++) sum += buf[i] * buf[i];
      const rms = Math.sqrt(sum / buf.length);
      onLevel?.(Math.min(1, rms * 8));
      if (rms > 0.02) { spoke = true; lastVoice = now; }
      if (spoke && now - lastVoice > silenceMs) { stop(); return; }
    }
    if (now - t0 > maxMs) { stop(); return; }
    raf = requestAnimationFrame(loop);
  };
  rec.start(250);
  raf = requestAnimationFrame(loop);
  return { stop, cancel: () => { cancelled = true; stop(); }, done, watch: (l, t) => { onLevel = l; onTick = t; } };
}

/** "2026-11-12" for a Date, in local time. */
export function isoDay(d: Date): string {
  const p = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}
