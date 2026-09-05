import { Suspense, useEffect, useState, type ReactNode } from 'react';

interface LazyMountProps {
  when: boolean;
  fallback?: ReactNode;
  children: ReactNode;
}

export default function LazyMount({ when, fallback = null, children }: LazyMountProps) {
  const [mounted, setMounted] = useState(when);
  useEffect(() => {
    if (when && !mounted) setMounted(true);
  }, [when, mounted]);
  if (!mounted) return null;
  return <Suspense fallback={fallback}>{children}</Suspense>;
}
