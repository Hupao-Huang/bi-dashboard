import React, { useEffect, useRef } from 'react';

interface WatermarkProps {
  text: string;   // 第一行：用户名
  subtext?: string; // 第二行：时间或角色
}

const Watermark: React.FC<WatermarkProps> = ({ text, subtext }) => {
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const canvas = document.createElement('canvas');
    const dpr = window.devicePixelRatio || 1;
    const w = 280;
    const h = 160;
    canvas.width = w * dpr;
    canvas.height = h * dpr;
    canvas.style.width = `${w}px`;
    canvas.style.height = `${h}px`;

    const ctx = canvas.getContext('2d')!;
    ctx.scale(dpr, dpr);
    ctx.clearRect(0, 0, w, h);

    ctx.save();
    ctx.translate(w / 2, h / 2);
    ctx.rotate(-Math.PI / 6); // -30度倾斜

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

  return (
    <div
      ref={containerRef}
      style={{
        position: 'fixed',
        top: 0,
        left: 0,
        width: '100%',
        height: '100%',
        pointerEvents: 'none',
        zIndex: 9999,
        userSelect: 'none',
      }}
    />
  );
};

export default Watermark;
