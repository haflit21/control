// Web push on this device: permission, subscription with the server's VAPID key,
// and the server-side registration. Everything stays per device (the browser owns
// the subscription; the server only keeps the endpoint + keys to encrypt to).
import { useCallback, useEffect, useState } from 'react';
import { api } from './api';

export type PushState = 'unsupported' | 'denied' | 'off' | 'on' | 'busy';

export function pushSupported(): boolean {
  return typeof window !== 'undefined' && 'serviceWorker' in navigator && 'PushManager' in window && 'Notification' in window && window.isSecureContext;
}

async function registration(): Promise<ServiceWorkerRegistration | undefined> {
  if (!('serviceWorker' in navigator)) return undefined;
  const reg = (await navigator.serviceWorker.getRegistration()) ?? (await navigator.serviceWorker.register('/sw.js').catch(() => undefined));
  if (!reg) return undefined;
  return navigator.serviceWorker.ready;
}

function toKey(b64: string): Uint8Array {
  const pad = '='.repeat((4 - (b64.length % 4)) % 4);
  const raw = atob((b64 + pad).replace(/-/g, '+').replace(/_/g, '/'));
  return Uint8Array.from(raw, (c) => c.charCodeAt(0));
}

export async function currentSubscription(): Promise<PushSubscription | null> {
  const reg = await registration();
  return reg ? reg.pushManager.getSubscription() : null;
}

export async function subscribe(): Promise<void> {
  const perm = await Notification.requestPermission();
  if (perm !== 'granted') throw new Error(perm === 'denied' ? 'Notifications refusées dans le navigateur' : 'Permission non accordée');
  const reg = await registration();
  if (!reg) throw new Error('Service worker indisponible');
  const { key } = await api<{ key: string }>('/api/push/key');
  const sub = (await reg.pushManager.getSubscription()) ?? (await reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: toKey(key) as BufferSource }));
  await api('/api/push/subscribe', { method: 'POST', body: JSON.stringify(sub.toJSON()) });
}

export async function unsubscribe(): Promise<void> {
  const sub = await currentSubscription();
  if (!sub) return;
  await api('/api/push/unsubscribe', { method: 'POST', body: JSON.stringify({ endpoint: sub.endpoint }) }).catch(() => {});
  await sub.unsubscribe();
}

export async function sendTest(): Promise<number> {
  const r = await api<{ sent: number }>('/api/push/test', { method: 'POST', body: '{}' });
  return r.sent;
}

/** State of push on this device + toggle. */
export function usePush(): [PushState, (on: boolean) => Promise<void>, string | undefined] {
  const [state, setState] = useState<PushState>(pushSupported() ? 'busy' : 'unsupported');
  const [error, setError] = useState<string>();
  useEffect(() => {
    if (!pushSupported()) return;
    let live = true;
    (async () => {
      if (Notification.permission === 'denied') { if (live) setState('denied'); return; }
      const sub = await currentSubscription().catch(() => null);
      if (live) setState(sub ? 'on' : 'off');
    })();
    return () => { live = false; };
  }, []);
  const set = useCallback(async (on: boolean) => {
    setError(undefined);
    setState('busy');
    try {
      if (on) await subscribe(); else await unsubscribe();
      setState(on ? 'on' : 'off');
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Échec');
      setState(Notification.permission === 'denied' ? 'denied' : on ? 'off' : 'on');
    }
  }, []);
  return [state, set, error];
}
