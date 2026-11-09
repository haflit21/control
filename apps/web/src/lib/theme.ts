// Appearance per device: auto (follows the system), dark (the reference look) or light.
// Stored in localStorage (a per-device convenience; falls back to auto when storage is unavailable)
// and applied as <html data-theme>, which tokens.css already understands.
import { useSyncExternalStore } from 'react';

export type ThemeChoice = 'auto' | 'dark' | 'light';
const KEY = 'maison.theme';
const BG: Record<'dark' | 'light', string> = { dark: '#0d1017', light: '#f1eee8' };

function read(): ThemeChoice {
  try {
    const v = localStorage.getItem(KEY);
    return v === 'dark' || v === 'light' ? v : 'auto';
  } catch {
    return 'auto';
  }
}

let current: ThemeChoice = read();
const subs = new Set<() => void>();
const mq = typeof window !== 'undefined' ? window.matchMedia('(prefers-color-scheme: dark)') : undefined;

/** The theme actually shown (auto resolved against the system). */
export function resolvedTheme(c: ThemeChoice = current): 'dark' | 'light' {
  if (c !== 'auto') return c;
  return mq?.matches ? 'dark' : 'light';
}

export function applyTheme(c: ThemeChoice = current) {
  const root = document.documentElement;
  if (c === 'auto') delete root.dataset.theme; else root.dataset.theme = c;
  // one theme-color meta that follows the choice (status bar of the installed app)
  const metas = document.querySelectorAll<HTMLMetaElement>('meta[name="theme-color"]');
  metas.forEach((m, i) => { if (i > 0) m.remove(); });
  const meta = metas[0] ?? Object.assign(document.createElement('meta'), { name: 'theme-color' });
  meta.removeAttribute('media');
  meta.content = BG[resolvedTheme(c)];
  if (!meta.parentNode) document.head.appendChild(meta);
}

export function setTheme(c: ThemeChoice) {
  current = c;
  try { if (c === 'auto') localStorage.removeItem(KEY); else localStorage.setItem(KEY, c); } catch { /* private mode */ }
  applyTheme(c);
  subs.forEach((f) => f());
}

mq?.addEventListener('change', () => { if (current === 'auto') { applyTheme('auto'); subs.forEach((f) => f()); } });

export function useTheme(): ThemeChoice {
  return useSyncExternalStore((f) => { subs.add(f); return () => { subs.delete(f); }; }, () => current);
}
