import { useEffect, useState } from 'react';

export default function FpsMeter() {
  const [fps, setFps] = useState(0);
  const [worst, setWorst] = useState(0);

  useEffect(() => {
    let frame = 0;
    let count = 0;
    let longest = 0;
    let since = performance.now();
    let last = since;
    const tick = (now: number) => {
      frame = requestAnimationFrame(tick);
      longest = Math.max(longest, now - last);
      last = now;
      count++;
      if (now - since >= 1000) {
        setFps(Math.round((count * 1000) / (now - since)));
        setWorst(Math.round(longest));
        count = 0;
        longest = 0;
        since = now;
      }
    };
    frame = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(frame);
  }, []);

  return (
    <div
      style={{
        position: 'fixed', left: 8, bottom: 8, zIndex: 9999, pointerEvents: 'none',
        padding: '4px 8px', borderRadius: 8, background: 'rgba(0,0,0,0.7)', color: '#fff',
        font: '12px/1.2 ui-monospace, monospace', fontVariantNumeric: 'tabular-nums',
      }}
    >
      {fps} fps · worst {worst} ms
    </div>
  );
}
