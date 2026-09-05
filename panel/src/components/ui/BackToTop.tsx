import { useEffect, useState } from 'react';
import { ArrowUpOutlined } from '@ant-design/icons';
interface BackToTopProps {
  /** Returns the scroll container; defaults to the window. */
  target?: () => HTMLElement | Window | null;
  visibilityHeight?: number;
}

/** Floating "scroll to top" pill (DS-styled). Replaces the CustomUI FloatButton. */
export default function BackToTop({ target, visibilityHeight = 200 }: BackToTopProps) {
  const [visible, setVisible] = useState(false);

  useEffect(() => {
    const el = target ? target() : window;
    if (!el) return;
    const onScroll = () => {
      const top = el === window ? window.scrollY : (el as HTMLElement).scrollTop;
      setVisible(top > visibilityHeight);
    };
    onScroll();
    el.addEventListener('scroll', onScroll);
    return () => el.removeEventListener('scroll', onScroll);
  }, [target, visibilityHeight]);

  if (!visible) return null;

  const scrollToTop = () => {
    const el = target ? target() : window;
    el?.scrollTo({ top: 0, behavior: 'smooth' });
  };

  return (
    <button type="button" className="ds-backtop" aria-label="Back to top" onClick={scrollToTop}>
      <ArrowUpOutlined />
    </button>
  );
}
