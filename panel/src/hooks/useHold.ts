import { useRef, type MouseEvent } from 'react';

export function useHold(delay = 450) {
  const timer = useRef(0);
  const held = useRef(false);
  const stop = () => window.clearTimeout(timer.current);

  return (onTap: () => void, onHold: () => void) => ({
    onPointerDown: () => {
      held.current = false;
      stop();
      timer.current = window.setTimeout(() => {
        held.current = true;
        onHold();
      }, delay);
    },
    onPointerUp: stop,
    onPointerLeave: stop,
    onPointerCancel: stop,
    onContextMenu: (e: MouseEvent) => e.preventDefault(),
    onClick: () => {
      if (!held.current) onTap();
    },
  });
}
