export type LinkState = 'on' | 'off' | 'error';

export default function BrandMark({
  state = 'off',
  exit = false,
}: {
  state?: LinkState;
  exit?: boolean;
}) {
  return (
    <span className={`brand-mark-wrap is-${state}`}>
      <svg className={`brand-mark${exit ? ' is-out' : ''}`} viewBox="0 0 60 60" aria-hidden="true">
        <g className="brand-mark__face brand-mark__face--off">
          <path d="M11.9 17.9 L16 15.9 L20 18.2 L24 16.1 L28 17.6" />
          <path d="M32 39.9 L36 39.9 M39 39.9 L43 39.9" className="brand-mark__dash" />
          <path d="M30.7 11.8 L30.7 50.7" className="brand-mark__rail" />
        </g>
        <g className="brand-mark__face brand-mark__face--on">
          <path d="M12 17.7 L15.3 15.6 L18.6 17.4 L21.4 16.9" />
          <path d="M21.4 33.3 L26 31.5 L30.6 33.6 L35.2 31.2 L39.7 33" />
          <path d="M41 38.7 L44 38.7 M46.5 38.7 L48.1 38.7" className="brand-mark__dash" />
          <path d="M21.4 11.8 L21.4 48.9" className="brand-mark__rail" />
          <path d="M39.7 11.8 L39.7 48.9" className="brand-mark__rail" />
        </g>
      </svg>

      <svg className="brand-badge" viewBox="0 0 16 16" aria-hidden="true">
        <circle className="brand-badge__disc" cx="8" cy="8" r="8" />
        {state === 'on' && <path className="brand-badge__sign" d="M4.6 8.2 L7 10.5 L11.4 5.8" />}
        {state === 'off' && <path className="brand-badge__sign" d="M4.8 8 L11.2 8" />}
        {state === 'error' && <path className="brand-badge__sign" d="M5.5 5.5 L10.5 10.5 M10.5 5.5 L5.5 10.5" />}
      </svg>
    </span>
  );
}
