import { useEffect, useState, useRef } from 'react';
interface TerminalBootLoaderProps {
  onComplete: () => void;
}

export default function TerminalBootLoader({ onComplete }: TerminalBootLoaderProps) {
  const [fadeClass, setFadeClass] = useState('');
  const canvasRef = useRef<HTMLCanvasElement>(null);

  const onCompleteRef = useRef(onComplete);
  useEffect(() => {
    onCompleteRef.current = onComplete;
  }, [onComplete]);

  useEffect(() => {
    // Session check to skip loader if already shown in this session
    const isLoaded = sessionStorage.getItem('antigravity_boot_loaded');
    if (isLoaded === 'true') {
      onCompleteRef.current();
      return;
    }

    // Canvas particle system (Google Antigravity theme)
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    let animationId: number;
    let width = (canvas.width = window.innerWidth);
    let height = (canvas.height = window.innerHeight);

    const handleResize = () => {
      width = canvas.width = window.innerWidth;
      height = canvas.height = window.innerHeight;
    };
    window.addEventListener('resize', handleResize);

    // Particles config
    const particlesCount = Math.min(60, Math.floor((width * height) / 20000));
    const particles: Array<{
      x: number;
      y: number;
      vx: number;
      vy: number;
      radius: number;
    }> = [];

    for (let i = 0; i < particlesCount; i++) {
      particles.push({
        x: Math.random() * width,
        y: Math.random() * height,
        vx: (Math.random() - 0.5) * 0.8,
        vy: (Math.random() - 0.5) * 0.8,
        radius: Math.random() * 2 + 1,
      });
    }

    // Draw frame loop
    const animate = () => {
      ctx.clearRect(0, 0, width, height);
      ctx.fillStyle = '#f8f9fc';
      ctx.fillRect(0, 0, width, height);

      // Draw grid points (Google Antigravity Dot Grid)
      ctx.fillStyle = 'rgba(50, 121, 249, 0.05)';
      const gridSize = 40;
      for (let x = 0; x < width; x += gridSize) {
        for (let y = 0; y < height; y += gridSize) {
          ctx.beginPath();
          ctx.arc(x, y, 1, 0, Math.PI * 2);
          ctx.fill();
        }
      }

      // Draw particles & links
      for (let i = 0; i < particlesCount; i++) {
        const p1 = particles[i];
        p1.x += p1.vx;
        p1.y += p1.vy;

        // Boundaries check
        if (p1.x < 0 || p1.x > width) p1.vx *= -1;
        if (p1.y < 0 || p1.y > height) p1.vy *= -1;

        ctx.beginPath();
        ctx.arc(p1.x, p1.y, p1.radius, 0, Math.PI * 2);
        ctx.fillStyle = 'rgba(50, 121, 249, 0.4)';
        ctx.fill();

        // Connect particles if close
        for (let j = i + 1; j < particlesCount; j++) {
          const p2 = particles[j];
          const dist = Math.hypot(p1.x - p2.x, p1.y - p2.y);
          if (dist < 150) {
            ctx.beginPath();
            ctx.moveTo(p1.x, p1.y);
            ctx.lineTo(p2.x, p2.y);
            ctx.strokeStyle = `rgba(50, 121, 249, ${0.15 * (1 - dist / 150)})`;
            ctx.lineWidth = 0.8;
            ctx.stroke();
          }
        }
      }

      animationId = requestAnimationFrame(animate);
    };

    animate();

    // Fade and complete timeouts
    const fadeTimeout = setTimeout(() => {
      setFadeClass('fade-out');
    }, 2200);

    const completeTimeout = setTimeout(() => {
      sessionStorage.setItem('antigravity_boot_loaded', 'true');
      onCompleteRef.current();
    }, 2600);

    return () => {
      window.removeEventListener('resize', handleResize);
      cancelAnimationFrame(animationId);
      clearTimeout(fadeTimeout);
      clearTimeout(completeTimeout);
    };
  }, []);

  // If already shown in session, render nothing
  const isLoaded = sessionStorage.getItem('antigravity_boot_loaded');
  if (isLoaded === 'true') return null;

  return (
    <div className={`boot-loader-root ${fadeClass}`}>
      <canvas ref={canvasRef} className="boot-loader-canvas" />
      <div className="boot-loader-hud">
        <div className="boot-loader-logo">
          <div className="logo-spinner" />
          <span className="logo-text">ANTIGRAVITY</span>
        </div>
        <div className="boot-loader-sub">SYSTEM LOADING // MISSION CONTROL</div>
        <div className="boot-progress-bar">
          <div className="boot-progress-fill" />
        </div>
      </div>
    </div>
  );
}
