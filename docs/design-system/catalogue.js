// Maison — catalogue behaviour: theme toggle, phone toggle, swatches + contrast audit, icon grid, markup details, toc spy.
(() => {
  const THEME_KEY = 'maison.ds.theme';
  const root = document.documentElement;
  const themeBtn = document.getElementById('theme'), themeL = document.getElementById('theme-l');
  const $ = id => document.getElementById(id);
  const label = { system: 'Système', dark: 'Sombre', light: 'Clair' };
  function applyTheme(v) { if (v === 'light' || v === 'dark') root.dataset.theme = v; else delete root.dataset.theme; if (themeL) themeL.textContent = label[v || 'system']; if (themeBtn) themeBtn.title = 'Thème : ' + label[v || 'system']; }
  let saved = null; try { saved = localStorage.getItem(THEME_KEY); } catch {}
  applyTheme(saved);
  themeBtn && themeBtn.addEventListener('click', () => {
    const cur = root.dataset.theme || 'system'; const next = cur === 'system' ? 'dark' : cur === 'dark' ? 'light' : 'system';
    root.classList.add('no-transitions'); applyTheme(next);
    requestAnimationFrame(() => requestAnimationFrame(() => root.classList.remove('no-transitions')));
    try { next === 'system' ? localStorage.removeItem(THEME_KEY) : localStorage.setItem(THEME_KEY, next); } catch {}
  });
  const phoneBtn = document.getElementById('phone'), main = document.getElementById('main');
  const url = new URL(location.href);
  if (main && phoneBtn && url.searchParams.get('phone') === '1') { main.classList.add('phone'); phoneBtn.setAttribute('aria-pressed', 'true'); }
  if (url.searchParams.get('theme')) applyTheme(url.searchParams.get('theme'));
  phoneBtn && main && phoneBtn.addEventListener('click', () => { const on = main.classList.toggle('phone'); phoneBtn.setAttribute('aria-pressed', String(on)); });

  if (!document.getElementById('icons')) return; // mobile / dashboard pages: nothing more to build

  // icons
  const grid = document.getElementById('icons');
  document.querySelectorAll('symbol[id^="i-"]').forEach(s => {
    const c = document.createElement('div'); c.className = 'icon-cell';
    c.innerHTML = `<svg class="ico -l"><use href="#${s.id}"/></svg><span></span>`; c.lastElementChild.textContent = s.id.slice(2); grid.appendChild(c);
  });

  // swatches + contrast (glass is composited over --bg before measuring)
  const tokNeutral = ['--bg', '--surface-solid', '--surface', '--surface-2', '--surface-3', '--fg', '--fg-muted', '--fg-faint', '--border', '--border-strong'];
  const tokSem = ['accent', 'night', 'heat', 'cool', 'ok', 'warn', 'crit', 'neutral'];
  const all = [...tokNeutral, '--accent', '--accent-fg', '--accent-subtle', '--night', '--night-subtle'];
  tokSem.forEach(k => { if (k === 'accent' || k === 'night') return; all.push(`--${k}-fg`, `--${k}-bg`, `--${k}-bd`); });
  function readTokens(theme) {
    const prev = root.dataset.theme; root.classList.add('no-transitions');
    if (theme) root.dataset.theme = theme; else delete root.dataset.theme;
    const cs = getComputedStyle(root); const out = {};
    all.forEach(k => out[k] = cs.getPropertyValue(k).trim());
    if (prev) root.dataset.theme = prev; else delete root.dataset.theme;
    requestAnimationFrame(() => root.classList.remove('no-transitions'));
    return out;
  }
  const D = readTokens('dark'), L = readTokens('light');
  applyTheme(root.dataset.theme || null);
  const parse = c => { c = c.trim(); let m;
    if ((m = c.match(/^#([0-9a-f]{6})$/i))) return [parseInt(m[1].slice(0, 2), 16), parseInt(m[1].slice(2, 4), 16), parseInt(m[1].slice(4, 6), 16), 1];
    if ((m = c.match(/^rgba?\(([^)]+)\)$/))) { const p = m[1].split(/[,\s/]+/).filter(Boolean).map(parseFloat); return [p[0], p[1], p[2], p[3] ?? 1]; }
    return [0, 0, 0, 1]; };
  const over = (fgc, bgc) => { const a = fgc[3]; return [0, 1, 2].map(i => Math.round(fgc[i] * a + bgc[i] * (1 - a))); };
  const lum = rgb => { const c = rgb.map(v => v / 255).map(v => v <= .03928 ? v / 12.92 : ((v + .055) / 1.055) ** 2.4); return .2126 * c[0] + .7152 * c[1] + .0722 * c[2]; };
  const cr = (fg, bg) => { const x = lum(fg), y = lum(bg); return (Math.max(x, y) + .05) / (Math.min(x, y) + .05); };
  // resolve a token to an opaque color in a theme: composite over the solid surface (itself over bg)
  const solid = (T, k) => { const bg = parse(T['--bg']); const c = parse(T[k]); return c[3] < 1 ? over(c, bg) : c.slice(0, 3); };
  const glassSurface = T => over(parse(T['--surface']), parse(T['--bg']));
  const swN = document.getElementById('sw-neutral');
  tokNeutral.forEach(k => {
    const d = document.createElement('div'); d.className = 'sw';
    const chip = document.createElement('div'); chip.className = 'sw-chip'; chip.style.background = `var(${k})`;
    chip.style.color = k.startsWith('--fg') ? 'var(--bg)' : 'var(--fg-muted)'; chip.textContent = k.startsWith('--fg') ? 'Aa' : '';
    const n = document.createElement('div'); n.className = 'sw-name'; n.textContent = k;
    const v = document.createElement('div'); v.className = 'sw-val'; v.textContent = `${D[k]} · ${L[k]}`;
    d.append(chip, n, v); swN.appendChild(d);
  });
  const swS = document.getElementById('sw-sem');
  tokSem.forEach(k => {
    const d = document.createElement('div'); d.className = 'sw';
    const chip = document.createElement('div'); chip.className = 'sw-chip';
    if (k === 'accent') { chip.style.background = 'var(--accent)'; chip.style.color = 'var(--accent-fg)'; chip.style.boxShadow = 'var(--accent-glow)'; chip.textContent = 'Accent'; }
    else if (k === 'night') { chip.style.background = 'var(--night-subtle)'; chip.style.color = 'var(--night)'; chip.textContent = 'Nuit'; }
    else { chip.style.background = `var(--${k}-bg)`; chip.style.color = `var(--${k}-fg)`; chip.style.borderColor = `var(--${k}-bd)`; chip.textContent = { heat: 'Chauffe', cool: 'Froid', ok: 'OK', warn: 'Attention', crit: 'Alerte', neutral: 'Neutre' }[k]; }
    const n = document.createElement('div'); n.className = 'sw-name'; n.textContent = k === 'accent' ? '--accent' : k === 'night' ? '--night' : `--${k}-fg / -bg / -bd`;
    const key = k === 'accent' ? '--accent' : k === 'night' ? '--night' : `--${k}-fg`;
    const v = document.createElement('div'); v.className = 'sw-val'; v.textContent = `${D[key]} · ${L[key]}`;
    d.append(chip, n, v); swS.appendChild(d);
  });
  const pairs = [
    ['--fg sur verre', '--fg', 'glass', 4.5], ['--fg sur --bg', '--fg', '--bg', 4.5],
    ['--fg-muted sur verre', '--fg-muted', 'glass', 4.5], ['--fg-muted sur --surface-solid', '--fg-muted', '--surface-solid', 4.5],
    ['--fg-faint sur verre (méta)', '--fg-faint', 'glass', 3], ['--border-strong sur --bg', '--border-strong', '--bg', 3],
    ['--accent-fg sur --accent', '--accent-fg', '--accent', 4.5], ['--accent sur --bg', '--accent', '--bg', 3], ['--night sur verre', '--night', 'glass', 4.5],
  ];
  tokSem.filter(k => k !== 'accent' && k !== 'night').forEach(k => { pairs.push([`--${k}-fg sur --${k}-bg (verre)`, `--${k}-fg`, `--${k}-bg`, 4.5], [`--${k}-fg sur verre`, `--${k}-fg`, 'glass', 4.5]); });
  const resolve = (T, k) => k === 'glass' ? glassSurface(T) : k.endsWith('-bg') && k !== '--bg' ? over(parse(T[k]), glassSurface(T)) : solid(T, k);
  const tb = document.querySelector('#contrast tbody');
  pairs.forEach(([name, a, b, req]) => {
    const tr = document.createElement('tr');
    const cd = cr(resolve(D, a), resolve(D, b)), cl = cr(resolve(L, a), resolve(L, b));
    const cell = v => { const td = document.createElement('td'); td.className = 'num cr ' + (v < req ? 'is-fail' : 'is-aa'); td.textContent = v.toFixed(2).replace('.', ',') + ' : 1'; return td; };
    const n = document.createElement('td'); n.textContent = name; const r = document.createElement('td'); r.textContent = `≥ ${req} : 1`;
    tr.append(n, cell(cd), cell(cl), r); tb.appendChild(tr);
  });

  // auto "Balisage" under each widget example
  document.querySelectorAll('section.ds:not(#apercu):not(#couleurs):not(#typo):not(#espace):not(#icones):not(#inventaire) .ex').forEach(ex => {
    const det = document.createElement('details'); const sum = document.createElement('summary'); sum.textContent = 'Balisage'; const pre = document.createElement('pre');
    const clone = ex.cloneNode(true); clone.querySelectorAll('.ex-l, details, .arc svg, .dots i').forEach(n => n.remove());
    pre.textContent = clone.innerHTML.replace(/^\s*\n/gm, '').replace(/^ {4}/gm, '').trim();
    det.append(sum, pre); ex.appendChild(det);
  });

  // toc follows scroll
  const links = [...document.querySelectorAll('.toc a')]; const secs = links.map(a => document.querySelector(a.getAttribute('href')));
  const io = new IntersectionObserver(es => { es.forEach(e => { if (e.isIntersecting) { links.forEach(l => l.classList.toggle('is-active', l.getAttribute('href') === '#' + e.target.id)); } }); }, { rootMargin: '-20% 0px -70% 0px' });
  secs.forEach(s => s && io.observe(s));
})();
