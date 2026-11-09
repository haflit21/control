// Maison service worker: caches the app shell (HTML + hashed assets) so the
// PWA opens instantly; never caches /api (no house data stored offline).
const SHELL = 'maison-shell-v2';
self.addEventListener('install', (e) => { self.skipWaiting(); });
self.addEventListener('activate', (e) => { e.waitUntil(caches.keys().then((ks) => Promise.all(ks.filter((k) => k !== SHELL).map((k) => caches.delete(k)))).then(() => self.clients.claim())); });
self.addEventListener('fetch', (e) => {
  const url = new URL(e.request.url);
  if (e.request.method !== 'GET' || url.origin !== location.origin || url.pathname.startsWith('/api/') || url.pathname.startsWith('/login')) return;
  // photos: network first (so a replaced file shows up), cached copy when offline
  if (url.pathname.startsWith('/media/')) {
    e.respondWith(caches.open(SHELL).then((c) => fetch(e.request).then((r) => { if (r.ok) c.put(e.request, r.clone()); return r; }).catch(async () => (await c.match(e.request)) || Response.error())));
    return;
  }
  if (url.pathname.startsWith('/assets/')) {
    e.respondWith(caches.open(SHELL).then(async (c) => (await c.match(e.request)) || fetch(e.request).then((r) => { if (r.ok) c.put(e.request, r.clone()); return r; })));
    return;
  }
  // navigation and the rest: network first, cached shell as a fallback
  e.respondWith(fetch(e.request).then((r) => { if (r.ok && e.request.mode === 'navigate') caches.open(SHELL).then((c) => c.put('/', r.clone())); return r; }).catch(() => caches.match('/')));
});

// Notifications push (le serveur chiffre un JSON {title, body, tag, url, level}).
self.addEventListener('push', (e) => {
  let m = { title: 'Maison', body: '' };
  try { m = { ...m, ...e.data.json() }; } catch { m.body = e.data ? e.data.text() : ''; }
  e.waitUntil(self.registration.showNotification(m.title, {
    body: m.body, tag: m.tag || undefined, renotify: !!m.tag, icon: '/icons/icon-192.png', badge: '/icons/badge.png',
    data: { url: m.url || '/' }, requireInteraction: m.level === 'crit', vibrate: m.level === 'crit' ? [200, 100, 200, 100, 200] : [100],
  }));
});
self.addEventListener('notificationclick', (e) => {
  e.notification.close();
  const url = new URL((e.notification.data && e.notification.data.url) || '/', self.location.origin).href;
  e.waitUntil(self.clients.matchAll({ type: 'window', includeUncontrolled: true }).then((list) => {
    const win = list.find((c) => 'focus' in c);
    if (win) return win.navigate ? win.navigate(url).then((c) => c && c.focus()) : win.focus();
    return self.clients.openWindow(url);
  }));
});
