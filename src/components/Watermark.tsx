import React, { useEffect, useRef, useCallback } from 'react';

interface WatermarkProps {
  text: string;
  subtext?: string;
}

const WATERMARK_STYLE: React.CSSProperties = {
  position: 'fixed',
  top: 0,
  left: 0,
  width: '100%',
  height: '100%',
  pointerEvents: 'none',
  zIndex: 9999,
  userSelect: 'none',
};

const Watermark: React.FC<WatermarkProps> = ({ text, subtext }) => {
  const containerRef = useRef<HTMLDivElement>(null);
  const observerRef = useRef<MutationObserver | null>(null);

  const renderWatermark = useCallback(() => {
    const container = containerRef.current;
    if (!container) return;

    const canvas = document.createElement('canvas');
    const dpr = window.devicePixelRatio || 1;
    const w = 280;
    const h = 160;
    canvas.width = w * dpr;
    canvas.height = h * dpr;

    const ctx = canvas.getContext('2d')!;
    ctx.scale(dpr, dpr);
    ctx.clearRect(0, 0, w, h);

    ctx.save();
    ctx.translate(w / 2, h / 2);
    ctx.rotate(-Math.PI / 6);

    ctx.globalAlpha = 0.10;
    ctx.fillStyle = '#1e293b';
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';

    ctx.font = '14px -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif';
    ctx.fillText(text, 0, -10);

    if (subtext) {
      ctx.font = '12px -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif';
      ctx.fillText(subtext, 0, 10);
    }
    ctx.restore();

    const base64 = canvas.toDataURL('image/png');
    container.style.backgroundImage = `url(${base64})`;
    container.style.backgroundRepeat = 'repeat';
    container.style.backgroundSize = `${w}px ${h}px`;
  }, [text, subtext]);

  useEffect(() => {
    renderWatermark();

    const container = containerRef.current;
    if (!container || !container.parentNode) return;

    observerRef.current = new MutationObserver((mutations) => {
      for (const m of mutations) {
        if (m.type === 'childList') {
          for (const node of Array.from(m.removedNodes)) {
            if (node === container) {
              container.parentNode?.appendChild(container);
              renderWatermark();
              return;
            }
          }
        }
        if (m.type === 'attributes' && m.target === container) {
          renderWatermark();
          Object.assign(container.style, WATERMARK_STYLE);
          return;
        }
      }
    });

    observerRef.current.observe(container.parentNode, { childList: true });
    observerRef.current.observe(container, { attributes: true, attributeFilter: ['style', 'class'] });

    return () => {
      observerRef.current?.disconnect();
    };
  }, [renderWatermark]);

  return <div ref={containerRef} style={WATERMARK_STYLE} />;
};

export default Watermark;
