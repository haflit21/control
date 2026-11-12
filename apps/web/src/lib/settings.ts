// The settings popover (this device, the household) is owned by App; the home header (phone, where the rail is hidden) opens it from here.
import { useSyncExternalStore } from 'react';

let open = false;
const subs = new Set<() => void>();
export function setSettings(v: boolean) { open = v; subs.forEach((f) => f()); }
export function useSettings(): boolean {
  return useSyncExternalStore((f) => { subs.add(f); return () => { subs.delete(f); }; }, () => open);
}
