// Maison — widget behaviour that plain CSS cannot do: dots meters, arc gauges, weather glyphs, sparklines,
// all drawn from data attributes. Call MaisonWidgets.enhance(root) after inserting markup; runs once on load.
window.MaisonWidgets = (() => {
  const NS = 'http://www.w3.org/2000/svg';
  const svgEl = (tag, attrs = {}) => { const e = document.createElementNS(NS, tag); for (const k in attrs) e.setAttribute(k, attrs[k]); return e; };

  // ---- weather glyphs: the 15 Home Assistant conditions, day / night ----
  // Markup: <div class="wx-glyph" data-cond="rainy" [data-night]></div>; sizes: -xs 32, -s 56, (none) 96, -xl 140.
  const CLOUD = 'M28 80a17 17 0 0 1-3-33.7A24 24 0 0 1 70 50a15 15 0 0 1-1 30z';
  const CLOUD_BACK = 'M44 58a13 13 0 0 1-1.6-25.9A18 18 0 0 1 78 36a11 11 0 0 1 1 22z';
  const FX = { // drawn in a 60 × 32 box under the cloud
    rain:    [['path', { d: 'M13 4l-4 13M30 4l-4 13M47 4l-4 13', class: 'g-rain' }]],
    pouring: [['path', { d: 'M8 2l-5 18M20 2l-5 18M32 2l-5 18M44 2l-5 18M56 2l-5 18', class: 'g-rain -heavy' }]],
    snow:    [['path', { d: 'M12 3v14M6 6.5l12 7M18 6.5l-12 7M30 3v14M24 6.5l12 7M36 6.5l-12 7M48 3v14M42 6.5l12 7M54 6.5l-12 7', class: 'g-snow' }]],
    sleet:   [['path', { d: 'M12 4l-4 13M48 4l-4 13', class: 'g-rain' }], ['path', { d: 'M30 3v14M24 6.5l12 7M36 6.5l-12 7', class: 'g-snow' }]],
    hail:    [['circle', { cx: 12, cy: 10, r: 4, class: 'g-hail' }], ['circle', { cx: 30, cy: 14, r: 4, class: 'g-hail' }], ['circle', { cx: 48, cy: 8, r: 4, class: 'g-hail' }]],
    bolt:    [['path', { d: 'M34 0l-12 16h9l-7 16 16-20h-9l7-12z', class: 'g-bolt' }]],
    boltrain:[['path', { d: 'M12 4l-4 13M52 4l-4 13', class: 'g-rain' }], ['path', { d: 'M34 0l-12 16h9l-7 16 16-20h-9l7-12z', class: 'g-bolt' }]],
    fog:     [['path', { d: 'M4 6h52M10 16h40M18 26h26', class: 'g-fog' }]],
    wind:    [['path', { d: 'M4 8h36a7 7 0 1 0-7-7M4 20h50a7 7 0 1 1-7 7M4 30h24', class: 'g-wind' }]],
    bang:    [['path', { d: 'M30 2L14 30h32z', class: 'g-bang' }], ['path', { d: 'M30 12v9M30 25v.5', class: 'g-bang-i' }]],
  };
  const COND = {
    'sunny':           { disc: 'sun' },
    'clear-night':     { disc: 'moon' },
    'partlycloudy':    { disc: 'auto', small: true, cloud: true },
    'cloudy':          { cloud: true, back: true },
    'fog':             { cloud: true, fx: 'fog', low: true },
    'rainy':           { cloud: true, fx: 'rain' },
    'pouring':         { cloud: true, fx: 'pouring', dark: true },
    'lightning':       { cloud: true, fx: 'bolt', dark: true },
    'lightning-rainy': { cloud: true, fx: 'boltrain', dark: true },
    'snowy':           { cloud: true, fx: 'snow' },
    'snowy-rainy':     { cloud: true, fx: 'sleet' },
    'hail':            { cloud: true, fx: 'hail', dark: true },
    'windy':           { wind: true },
    'windy-variant':   { cloud: true, fx: 'wind' },
    'exceptional':     { cloud: true, fx: 'bang', dark: true },
  };
  const LABEL = {
    'sunny': 'Ensoleillé', 'clear-night': 'Nuit claire', 'partlycloudy': 'Éclaircies', 'cloudy': 'Couvert', 'fog': 'Brouillard',
    'rainy': 'Pluie', 'pouring': 'Fortes pluies', 'lightning': 'Orages', 'lightning-rainy': 'Orages et pluie', 'snowy': 'Neige',
    'snowy-rainy': 'Neige et pluie', 'hail': 'Grêle', 'windy': 'Venteux', 'windy-variant': 'Vent et nuages', 'exceptional': 'Phénomène exceptionnel',
  };
  const LABEL_NIGHT = { 'partlycloudy': 'Nuit nuageuse', 'sunny': 'Nuit claire' };
  function weatherGlyph(el) {
    const cond = el.dataset.cond, night = el.hasAttribute('data-night'); const spec = COND[cond] || COND['cloudy'];
    el.innerHTML = ''; el.classList.toggle('-night', night); el.classList.toggle('-dark', !!spec.dark);
    if (spec.disc) {
      const d = document.createElement('i'); d.className = 'g-disc';
      if (spec.disc === 'moon' || night) d.classList.add('-moon');
      if (spec.small) d.classList.add('-small'); el.appendChild(d);
    }
    if (spec.cloud) {
      const s = svgEl('svg', { viewBox: '0 0 100 100', class: 'g-cloud' + (spec.small ? ' -with-disc' : '') + (spec.low ? ' -low' : '') });
      if (spec.back) s.appendChild(svgEl('path', { d: CLOUD_BACK, class: 'g-cloud-back' }));
      s.appendChild(svgEl('path', { d: CLOUD })); el.appendChild(s);
    }
    if (spec.wind) { const s = svgEl('svg', { viewBox: '0 0 60 32', class: 'g-fx -solo' }); FX.wind.forEach(([t, a]) => s.appendChild(svgEl(t, a))); el.appendChild(s); }
    if (spec.fx) { const s = svgEl('svg', { viewBox: '0 0 60 32', class: 'g-fx' }); FX[spec.fx].forEach(([t, a]) => s.appendChild(svgEl(t, a))); el.appendChild(s); }
    if (!el.getAttribute('aria-label') && !el.hasAttribute('aria-hidden')) { el.setAttribute('role', 'img'); el.setAttribute('aria-label', (night && LABEL_NIGHT[cond]) || LABEL[cond] || cond); }
  }

  // ---- sparkline: <svg class="spark" data-v="9,8,8,7,7,6" [data-min data-max]></svg> ----
  function spark(el) {
    const v = el.dataset.v.split(',').map(Number); if (v.length < 2) return;
    const min = el.dataset.min !== undefined ? +el.dataset.min : Math.min(...v), max = el.dataset.max !== undefined ? +el.dataset.max : Math.max(...v);
    const W = 100, H = 30, pad = 4; const x = i => (i / (v.length - 1)) * W, y = n => H - pad - ((n - min) / ((max - min) || 1)) * (H - 2 * pad);
    el.setAttribute('viewBox', `0 0 ${W} ${H}`); el.setAttribute('preserveAspectRatio', 'none'); el.innerHTML = '';
    // smooth path (Catmull-Rom → bezier)
    let d = `M${x(0)} ${y(v[0])}`;
    for (let i = 0; i < v.length - 1; i++) {
      const p0 = v[i - 1] ?? v[i], p1 = v[i], p2 = v[i + 1], p3 = v[i + 2] ?? p2;
      const c1x = x(i) + (x(i + 1) - x(i)) / 3, c1y = y(p1) + (y(p2) - y(p0)) / 6, c2x = x(i + 1) - (x(i + 1) - x(i)) / 3, c2y = y(p2) - (y(p3) - y(p1)) / 6;
      d += ` C${c1x} ${c1y} ${c2x} ${c2y} ${x(i + 1)} ${y(p2)}`;
    }
    el.appendChild(svgEl('path', { d: d + ` L${W} ${H} L0 ${H} Z`, class: 'spark-fill' }));
    el.appendChild(svgEl('path', { d, class: 'spark-line' }));
    el.setAttribute('aria-hidden', 'true');
  }

  function enhance(root = document) {
    // dots meters
    root.querySelectorAll('.dots[data-n]').forEach(d => { const n = +d.dataset.n, on = +d.dataset.on; d.setAttribute('role', 'meter'); d.setAttribute('aria-valuenow', on); d.setAttribute('aria-valuemax', n); d.innerHTML = ''; for (let i = 0; i < n; i++) { const s = document.createElement('i'); if (i < on) s.className = 'is-on'; d.appendChild(s); } });
    // arc gauges: 33 ticks over 240°, lit up to data-v %
    root.querySelectorAll('.arc[data-v]').forEach(a => {
      const v = Math.max(0, Math.min(100, +a.dataset.v)); const N = 33, R = 44, cx = 50, cy = 52;
      const svg = svgEl('svg', { viewBox: '0 0 100 76', 'aria-hidden': 'true' });
      for (let i = 0; i < N; i++) {
        const t = i / (N - 1), ang = (210 - t * 240) * Math.PI / 180; const len = i % 4 === 0 ? 9 : 6;
        svg.appendChild(svgEl('line', { x1: cx + Math.cos(ang) * R, y1: cy - Math.sin(ang) * R, x2: cx + Math.cos(ang) * (R - len), y2: cy - Math.sin(ang) * (R - len), class: 'tick' + (v > 0 && t <= v / 100 ? ' is-on' : '') }));
      }
      a.querySelector(':scope > svg')?.remove(); a.prepend(svg); a.setAttribute('role', 'meter'); a.setAttribute('aria-valuenow', v); a.setAttribute('aria-valuemin', 0); a.setAttribute('aria-valuemax', 100);
    });
    root.querySelectorAll('.wx-glyph[data-cond]').forEach(weatherGlyph);
    root.querySelectorAll('svg.spark[data-v]').forEach(spark);
  }
  document.addEventListener('DOMContentLoaded', () => enhance(document));
  return { enhance, weatherGlyph, spark, conditions: Object.keys(COND), labels: LABEL, labelsNight: LABEL_NIGHT };
})();
