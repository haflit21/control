import { useState } from 'react';
import type { CSSProperties } from 'react';
import type { Device, HS } from '@control/model';
import { Pwr, Slider, cx } from './primitives';

/* ---------- Light panel: brightness, white temperature, hue / saturation, presets ---------- */

export const LIGHT_PRESETS: Array<{ id: string; label: string; css: string; kelvin?: number; hs?: HS }> = [
  { id: 'warm', label: 'Blanc chaud', css: '#ffb46b', kelvin: 2700 },
  { id: 'neutral', label: 'Blanc neutre', css: '#fff0dc', kelvin: 4000 },
  { id: 'day', label: 'Lumière du jour', css: '#dfe9ff', kelvin: 6000 },
  { id: 'amber', label: 'Ambre', css: 'hsl(30 85% 60%)', hs: { h: 30, s: 85 } },
  { id: 'red', label: 'Rouge', css: 'hsl(0 80% 58%)', hs: { h: 0, s: 85 } },
  { id: 'violet', label: 'Violet', css: 'hsl(290 60% 60%)', hs: { h: 290, s: 65 } },
  { id: 'blue', label: 'Bleu', css: 'hsl(205 85% 60%)', hs: { h: 205, s: 85 } },
  { id: 'green', label: 'Vert', css: 'hsl(140 60% 50%)', hs: { h: 140, s: 70 } },
];

/** CSS colour of a light from its state (hue/sat or white temperature). */
export function lightCSS(color: HS | undefined, kelvin: number | undefined): string {
  if (color && color.s >= 10) return `hsl(${Math.round(color.h)} ${Math.round(color.s)}% 60%)`;
  if (kelvin != null) {
    const t = Math.min(1, Math.max(0, (kelvin - 2000) / 4500));
    // warm amber → neutral → cool bluish white
    const h = t < 0.5 ? 32 : 215;
    const sat = t < 0.5 ? Math.round(85 - t * 2 * 70) : Math.round((t - 0.5) * 2 * 40);
    const l = 60 + Math.round(t * 28);
    return `hsl(${h} ${sat}% ${l}%)`;
  }
  return '#ffd9b0';
}

export function kelvinLabel(k: number): string {
  return `${k.toLocaleString('fr-FR')} K`;
}

export function LightPanel({ d, pending, onPower, onBrightness, onKelvin, onColor }: {
  d: Device; pending?: boolean;
  onPower: (on: boolean) => void; onBrightness: (pct: number) => void; onKelvin: (k: number) => void; onColor: (hs: HS) => void;
}) {
  const s = d.state;
  const canTemp = d.capabilities.includes('set_color_temp');
  const canColor = d.capabilities.includes('set_color');
  const [hueDraft, setHueDraft] = useState<number | undefined>(undefined);
  const [satDraft, setSatDraft] = useState<number | undefined>(undefined);
  const hue = hueDraft ?? s.color?.h ?? 30;
  const sat = satDraft ?? s.color?.s ?? 85;
  const inColor = !!s.color && s.color.s >= 10;
  const tint = lightCSS(inColor ? { h: hue, s: sat } : undefined, s.colorTemp ?? 2700);
  const activePreset = LIGHT_PRESETS.find((p) => (p.hs && inColor && Math.abs(p.hs.h - (s.color?.h ?? -999)) < 8) || (p.kelvin && !inColor && s.colorTemp != null && Math.abs(p.kelvin - s.colorTemp) < 300))?.id;
  const sub = [s.on ? 'Allumée' : 'Éteinte', s.on && s.brightness != null ? `${Math.round(s.brightness)} %` : '', s.on && inColor ? 'couleur' : s.on && s.colorTemp ? kelvinLabel(s.colorTemp) : ''].filter(Boolean).join(' · ');
  return (
    <div className={cx('light-panel', pending && 'is-pending')} style={{ ['--tint' as string]: tint, ['--hue' as string]: `hsl(${Math.round(hue)} 85% 55%)` } as CSSProperties}>
      <div className="hrow">
        <div className="hrow" style={{ gap: 12 }}><span className="light-sample" aria-hidden="true" style={{ opacity: s.on ? 1 : 0.25 }} /><div><div className="card-title -s">{d.name}</div><div className="card-sub">{sub}</div></div></div>
        <Pwr on={!!s.on} label={s.on ? 'Éteindre' : 'Allumer'} onClick={() => onPower(!s.on)} pending={pending} />
      </div>
      <Slider className="-dim" label="Intensité" value={s.on ? s.brightness ?? 100 : 0} min={1} max={100} ends={['Doux', 'Plein']} onCommit={onBrightness} disabled={!d.available} />
      {canTemp && <Slider className="-temp" label="Blanc" value={s.colorTemp ?? 2700} min={2000} max={6500} step={100} display={kelvinLabel} ends={['Chaud', 'Froid']} onCommit={onKelvin} disabled={!d.available} />}
      {canColor && (
        <>
          <Slider className="-hue" label="Teinte" value={hue} min={0} max={360} display={(v) => `${Math.round(v)}°`} onDraft={setHueDraft} onCommit={(h) => { setHueDraft(undefined); onColor({ h, s: sat }); }} disabled={!d.available} />
          <Slider className="-sat" label="Saturation" value={sat} min={0} max={100} onDraft={setSatDraft} onCommit={(v) => { setSatDraft(undefined); onColor({ h: hue, s: v }); }} disabled={!d.available} />
        </>
      )}
      <div className="swatches" role="group" aria-label="Préréglages">
        {LIGHT_PRESETS.filter((p) => (p.kelvin ? canTemp : canColor)).map((p) => (
          <button key={p.id} type="button" className={cx('swatch', activePreset === p.id && 'is-on')} style={{ ['--c' as string]: p.css } as CSSProperties} aria-label={p.label} title={p.label} onClick={() => (p.kelvin ? onKelvin(p.kelvin) : onColor(p.hs!))} />
        ))}
      </div>
    </div>
  );
}
