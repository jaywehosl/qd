import { useEffect, useRef } from 'react';

export function useFormSeed(active: boolean, row: string | number, seed: () => void) {
  const latest = useRef(seed);
  latest.current = seed;

  useEffect(() => {
    if (!active) return;
    latest.current();
  }, [active, row]);
}
