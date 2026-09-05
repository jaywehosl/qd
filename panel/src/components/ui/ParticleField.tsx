import { useEffect, useRef, useState } from 'react';

/**
 * ParticleField — a GPU-rendered, CPU-simulated cloud of free-gliding pucks.
 *
 * The simulation runs in REAL pixels and every particle's collision radius is
 * tied to its rendered size (mass ∝ area), so the dots you see are the dots that
 * collide — a big puck shoves a small one, they bounce, momentum cascades. They
 * glide on near-zero friction (air hockey), bounce off the screen walls and each
 * other, and never spring "home". The pointer is a paddle that only acts while
 * MOVING (speed-gated): a still cursor does nothing. Renders nothing without
 * WebGL2 (the CSS aurora is the fallback).
 */
export interface ParticleFieldProps {
  className?: string;
  density?: number;
  palette?: [string, string, string];
  monochrome?: boolean;
  additive?: boolean;
  intensity?: number;
  interactive?: boolean;
  preset?: 'pucks' | 'neural' | 'nebula';
}

const DEFAULT_PALETTE: [string, string, string] = ['#3279F9', '#A855F7', '#14B8A6'];

function hexToRgb(hex: string): [number, number, number] {
  const h = hex.replace('#', '');
  const v = h.length === 3 ? h.split('').map((c) => c + c).join('') : h;
  const n = parseInt(v, 16);
  return [((n >> 16) & 255) / 255, ((n >> 8) & 255) / 255, (n & 255) / 255];
}

const VERT = `#version 300 es
precision highp float;
in vec2 a_pos;     // position in CSS pixels
in float a_size;   // diameter in CSS pixels
in float a_hue;    // 0..1 palette position
in float a_speed;  // px/frame
uniform vec2 u_res;
uniform float u_dpr;
out float v_glow;
out float v_hue;
void main() {
  v_hue = a_hue;
  v_glow = clamp(0.55 + a_size * 0.012 + a_speed * 0.05, 0.0, 1.8);
  vec2 clip = vec2(a_pos.x / u_res.x * 2.0 - 1.0, 1.0 - a_pos.y / u_res.y * 2.0);
  gl_Position = vec4(clip, 0.0, 1.0);
  gl_PointSize = a_size * u_dpr;
}`;

const FRAG = `#version 300 es
precision highp float;
in float v_glow;
in float v_hue;
uniform vec3 u_a;
uniform vec3 u_b;
uniform vec3 u_c;
uniform float u_mono;
uniform float u_intensity;
out vec4 frag;
vec3 pal(float t) {
  vec3 c = mix(u_a, u_b, smoothstep(0.0, 0.5, t));
  return mix(c, u_c, smoothstep(0.5, 1.0, t));
}
void main() {
  vec2 uv = gl_PointCoord * 2.0 - 1.0;
  float d2 = dot(uv, uv);
  if (d2 > 1.0) discard;
  float r = sqrt(d2);
  float a = smoothstep(0.96, 0.62, r) * 0.92 + exp(-d2 * 2.2) * 0.3;
  vec3 col = mix(pal(v_hue), u_a, u_mono);
  frag = vec4(col * v_glow, a * v_glow * u_intensity);
}`;

const LINE_VERT = `#version 300 es
precision highp float;
in vec3 a_line_data; // x, y, alpha
uniform vec2 u_res;
out float v_alpha;
void main() {
  v_alpha = a_line_data.z;
  vec2 clip = vec2(a_line_data.x / u_res.x * 2.0 - 1.0, 1.0 - a_line_data.y / u_res.y * 2.0);
  gl_Position = vec4(clip, 0.0, 1.0);
}`;

const LINE_FRAG = `#version 300 es
precision highp float;
in float v_alpha;
uniform vec3 u_color;
uniform float u_intensity;
out vec4 frag;
void main() {
  frag = vec4(u_color * v_alpha, v_alpha * 0.45 * u_intensity);
}`;

function compile(gl: WebGL2RenderingContext, type: number, src: string): WebGLShader | null {
  const sh = gl.createShader(type);
  if (!sh) return null;
  gl.shaderSource(sh, src);
  gl.compileShader(sh);
  if (!gl.getShaderParameter(sh, gl.COMPILE_STATUS)) {
    console.warn('[ParticleField] shader error:', gl.getShaderInfoLog(sh));
    gl.deleteShader(sh);
    return null;
  }
  return sh;
}

export default function ParticleField({
  className,
  density = 1,
  palette = DEFAULT_PALETTE,
  monochrome = false,
  additive = false,
  intensity = 0.95,
  interactive = true,
  preset = 'pucks',
}: ParticleFieldProps) {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const live = useRef({ palette, monochrome, additive, intensity, interactive, speed: 1.0, preset });
  const [themeTick, setThemeTick] = useState(0);
  const lastParticleSig = useRef('');

  useEffect(() => {
    if (typeof window === 'undefined') return;
    const handleThemeChange = () => {
      const rs = window.getComputedStyle(document.documentElement);
      const g = (k: string) => rs.getPropertyValue(k).trim();

      // STRUCTURAL params force a full particle rebuild (re-randomize positions
      // & buffers). Keep this list MINIMAL — only things that change the particle
      // SET, not their appearance. Colour/primary/speed are NOT here: they used
      // to be, so every Primary slider tick teleported every particle ("freak
      // out"). They're now applied live below without a rebuild.
      const isOff = g('--fx-particles') === 'off';
      const structSig = [
        isOff,
        g('--fx-particles-density') || String(density),
        g('--fx-particles-preset') || preset || 'pucks',
      ].join('|');

      // COSMETIC params are read every frame from live.current — update in place
      // so Primary recolours the cloud smoothly instead of resetting it. Default
      // colour mode is 'palette' so --color-primary drives particles even on the
      // empty/default theme (previously they used a hardcoded blue and ignored
      // Primary until the colour mode was toggled).
      const fxColor = g('--fx-particles-color');
      const fxSpeed = g('--fx-particles-speed');
      const fxInteractive = g('--fx-particles-interactive');
      const primaryHex = g('--color-primary') || '#3279F9';
      const effColor = fxColor || 'palette';
      let activePalette: [string, string, string] = palette;
      if (effColor === 'primary' || effColor === 'monochrome') {
        activePalette = [primaryHex, primaryHex, primaryHex];
      } else if (effColor === 'palette') {
        activePalette = [primaryHex, '#A855F7', '#14B8A6'];
      }
      live.current = {
        ...live.current,
        palette: activePalette,
        monochrome: fxColor === 'monochrome' ? true : monochrome,
        interactive: fxInteractive ? fxInteractive !== 'off' : interactive,
        speed: fxSpeed ? parseFloat(fxSpeed) : 1.0,
      };

      if (structSig !== lastParticleSig.current) {
        lastParticleSig.current = structSig;
        setThemeTick((t) => t + 1);
      }
    };
    window.addEventListener('uup-theme-changed', handleThemeChange);
    window.addEventListener('uup-theme-mode-changed', handleThemeChange);
    return () => {
      window.removeEventListener('uup-theme-changed', handleThemeChange);
      window.removeEventListener('uup-theme-mode-changed', handleThemeChange);
    };
  }, [density, interactive, monochrome, palette, preset]);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;

    const rootStyle = typeof window !== 'undefined' ? window.getComputedStyle(document.documentElement) : null;
    const fxParticles = rootStyle?.getPropertyValue('--fx-particles')?.trim();
    const isOff = fxParticles === 'off';
    if (isOff) {
      const gl = canvas.getContext('webgl2');
      if (gl) {
        gl.clearColor(0, 0, 0, 0);
        gl.clear(gl.COLOR_BUFFER_BIT);
      }
      return;
    }

    const fxDensity = rootStyle?.getPropertyValue('--fx-particles-density')?.trim();
    const fxSpeed = rootStyle?.getPropertyValue('--fx-particles-speed')?.trim();
    const fxInteractive = rootStyle?.getPropertyValue('--fx-particles-interactive')?.trim();
    const fxColor = rootStyle?.getPropertyValue('--fx-particles-color')?.trim();
    const fxPreset = rootStyle?.getPropertyValue('--fx-particles-preset')?.trim() as 'pucks' | 'neural' | 'nebula' | undefined;

    let activeDensity = fxDensity ? parseFloat(fxDensity) : density;
    let activeSpeed = fxSpeed ? parseFloat(fxSpeed) : 1.0;
    const activeInteractive = fxInteractive ? (fxInteractive !== 'off') : interactive;
    const activeMonochrome = fxColor === 'monochrome' ? true : monochrome;
    const activePreset = fxPreset || preset || 'pucks';

    // Default colour mode is 'palette' so --color-primary drives the particles
    // even on the empty/default theme (kept in sync with handleThemeChange).
    const primaryHex = rootStyle?.getPropertyValue('--color-primary')?.trim() || '#3279F9';
    const effColor = fxColor || 'palette';
    let activePalette = palette;
    if (effColor === 'primary' || effColor === 'monochrome') {
      activePalette = [primaryHex, primaryHex, primaryHex];
    } else if (effColor === 'palette') {
      activePalette = [primaryHex, '#A855F7', '#14B8A6'];
    }

    live.current = {
      palette: activePalette,
      monochrome: activeMonochrome,
      additive,
      intensity,
      interactive: activeInteractive,
      speed: activeSpeed,
      preset: activePreset,
    };

    const gl = canvas.getContext('webgl2', { antialias: true, alpha: true, premultipliedAlpha: false });
    if (!gl) return;

    const vs = compile(gl, gl.VERTEX_SHADER, VERT);
    const fs = compile(gl, gl.FRAGMENT_SHADER, FRAG);
    if (!vs || !fs) return;
    const prog = gl.createProgram()!;
    gl.attachShader(prog, vs);
    gl.attachShader(prog, fs);
    gl.linkProgram(prog);
    if (!gl.getProgramParameter(prog, gl.LINK_STATUS)) {
      console.warn('[ParticleField] link error:', gl.getProgramInfoLog(prog));
      return;
    }

    // Compile line shaders (for neural web)
    const lineVs = compile(gl, gl.VERTEX_SHADER, LINE_VERT);
    const lineFs = compile(gl, gl.FRAGMENT_SHADER, LINE_FRAG);
    let lineProg: WebGLProgram | null = null;
    let lineVao: WebGLVertexArrayObject | null = null;
    let lineBuf: WebGLBuffer | null = null;

    if (lineVs && lineFs) {
      lineProg = gl.createProgram()!;
      gl.attachShader(lineProg, lineVs);
      gl.attachShader(lineProg, lineFs);
      gl.linkProgram(lineProg);

      lineVao = gl.createVertexArray();
      gl.bindVertexArray(lineVao);

      lineBuf = gl.createBuffer();
      gl.bindBuffer(gl.ARRAY_BUFFER, lineBuf);
      const aLineData = gl.getAttribLocation(lineProg, 'a_line_data');
      gl.enableVertexAttribArray(aLineData);
      gl.vertexAttribPointer(aLineData, 3, gl.FLOAT, false, 0, 0);
    }

    const reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    const dpr = Math.min(window.devicePixelRatio || 1, 1.75);
    let W = 1;
    let H = 1;

    // ---- tunables (all in CSS pixels / px-per-frame) ----
    const SIZE_MIN = 4;     // smallest puck diameter (px)
    const SIZE_RANGE = 18;  // extra diameter for the biggest (rand² weighted)
    const COL_SCALE = 0.46; // collision radius as a fraction of visual diameter
    const FRICTION = 0.985; // glide friction
    const JITTER = 0.02;    // faint air (px/frame)
    const CUR_R = 120;      // paddle radius (px)
    const CUR_R2 = CUR_R * CUR_R;
    const PUSH = 6.0;       // paddle radial shove (px/frame at the tip)
    const DRAG = 0.55;      // fraction of cursor motion imparted (trail)
    const REST = 0.9;       // restitution between pucks
    const WALL = 0.86;      // wall restitution
    const MAXV = 11;        // speed cap (px/frame)

    const isNeural = activePreset === 'neural';
    const isNebula = activePreset === 'nebula';
    const maxInteractDist = isNeural ? 75 : (SIZE_MIN + SIZE_RANGE) * COL_SCALE * 2;
    const CELL = maxInteractDist;

    let count = 0;
    let px = new Float32Array(0);
    let py = new Float32Array(0);
    let vx = new Float32Array(0);
    let vy = new Float32Array(0);
    let rad = new Float32Array(0);   // collision radius (px)
    let invM = new Float32Array(0);  // 1 / mass (mass ∝ area)
    let resp = new Float32Array(0);  // cursor responsiveness (individuality)
    let size = new Float32Array(0);  // visual diameter (px)
    let hue = new Float32Array(0);
    let next = new Int32Array(0);
    let posArr = new Float32Array(0);
    let spdArr = new Float32Array(0);

    const lineDataArr = new Float32Array(24000 * 3);

    let gcols = 1;
    let grows = 1;
    let cellHead = new Int32Array(1);

    let posBuf: WebGLBuffer | null = null;
    let spdBuf: WebGLBuffer | null = null;
    let sizeBuf: WebGLBuffer | null = null;
    let hueBuf: WebGLBuffer | null = null;
    let vao: WebGLVertexArrayObject | null = null;

    function buildParticles() {
      W = canvas!.clientWidth || window.innerWidth;
      H = canvas!.clientHeight || window.innerHeight;
      const maxCount = isNebula ? 4500 : 3500;
      count = Math.max(1800, Math.min(maxCount, Math.floor(((W * H) / 360) * activeDensity)));
      px = new Float32Array(count);
      py = new Float32Array(count);
      vx = new Float32Array(count);
      vy = new Float32Array(count);
      rad = new Float32Array(count);
      invM = new Float32Array(count);
      resp = new Float32Array(count);
      size = new Float32Array(count);
      hue = new Float32Array(count);
      next = new Int32Array(count);
      posArr = new Float32Array(count * 2);
      spdArr = new Float32Array(count);
      for (let i = 0; i < count; i++) {
        const r = Math.random();
        const d = SIZE_MIN + SIZE_RANGE * r * r; // rand² -> many small, few large
        size[i] = d;
        rad[i] = d * COL_SCALE;
        invM[i] = 1 / (d * d); // mass ∝ area
        px[i] = Math.random() * W;
        py[i] = Math.random() * H;
        
        if (isNebula) {
          const dx = px[i] - W / 2;
          const dy = py[i] - H / 2;
          const dist = Math.sqrt(dx * dx + dy * dy) + 1;
          vx[i] = (-dy / dist) * 1.5 * activeSpeed;
          vy[i] = (dx / dist) * 1.5 * activeSpeed;
        } else {
          vx[i] = (Math.random() - 0.5) * 1.2 * activeSpeed;
          vy[i] = (Math.random() - 0.5) * 1.2 * activeSpeed;
        }
        resp[i] = 0.6 + Math.random() * 1.0;
        hue[i] = Math.random();
      }

      vao = gl!.createVertexArray();
      gl!.bindVertexArray(vao);

      posBuf = gl!.createBuffer();
      gl!.bindBuffer(gl!.ARRAY_BUFFER, posBuf);
      gl!.bufferData(gl!.ARRAY_BUFFER, posArr, gl!.DYNAMIC_DRAW);
      const aPos = gl!.getAttribLocation(prog, 'a_pos');
      gl!.enableVertexAttribArray(aPos);
      gl!.vertexAttribPointer(aPos, 2, gl!.FLOAT, false, 0, 0);

      spdBuf = gl!.createBuffer();
      gl!.bindBuffer(gl!.ARRAY_BUFFER, spdBuf);
      gl!.bufferData(gl!.ARRAY_BUFFER, spdArr, gl!.DYNAMIC_DRAW);
      const aSpd = gl!.getAttribLocation(prog, 'a_speed');
      gl!.enableVertexAttribArray(aSpd);
      gl!.vertexAttribPointer(aSpd, 1, gl!.FLOAT, false, 0, 0);

      sizeBuf = gl!.createBuffer();
      gl!.bindBuffer(gl!.ARRAY_BUFFER, sizeBuf);
      gl!.bufferData(gl!.ARRAY_BUFFER, size, gl!.STATIC_DRAW);
      const aSize = gl!.getAttribLocation(prog, 'a_size');
      gl!.enableVertexAttribArray(aSize);
      gl!.vertexAttribPointer(aSize, 1, gl!.FLOAT, false, 0, 0);

      hueBuf = gl!.createBuffer();
      gl!.bindBuffer(gl!.ARRAY_BUFFER, hueBuf);
      gl!.bufferData(gl!.ARRAY_BUFFER, hue, gl!.STATIC_DRAW);
      const aHue = gl!.getAttribLocation(prog, 'a_hue');
      gl!.enableVertexAttribArray(aHue);
      gl!.vertexAttribPointer(aHue, 1, gl!.FLOAT, false, 0, 0);

      gcols = Math.max(1, Math.ceil(W / CELL));
      grows = Math.max(1, Math.ceil(H / CELL));
      cellHead = new Int32Array(gcols * grows);
    }

    const U = {
      res: gl.getUniformLocation(prog, 'u_res'),
      dpr: gl.getUniformLocation(prog, 'u_dpr'),
      a: gl.getUniformLocation(prog, 'u_a'),
      b: gl.getUniformLocation(prog, 'u_b'),
      c: gl.getUniformLocation(prog, 'u_c'),
      mono: gl.getUniformLocation(prog, 'u_mono'),
      intensity: gl.getUniformLocation(prog, 'u_intensity'),
    };

    const lineU = lineProg ? {
      res: gl.getUniformLocation(lineProg, 'u_res'),
      color: gl.getUniformLocation(lineProg, 'u_color'),
      intensity: gl.getUniformLocation(lineProg, 'u_intensity'),
    } : null;

    function resize() {
      const w = canvas!.clientWidth || window.innerWidth;
      const h = canvas!.clientHeight || window.innerHeight;
      canvas!.width = Math.max(1, Math.floor(w * dpr));
      canvas!.height = Math.max(1, Math.floor(h * dpr));
      gl!.viewport(0, 0, canvas!.width, canvas!.height);
      W = w; H = h;
      
      gl!.useProgram(prog);
      gl!.uniform2f(U.res, W, H);
      gl!.uniform1f(U.dpr, dpr);

      if (lineProg && lineU) {
        gl!.useProgram(lineProg);
        gl!.uniform2f(lineU.res, W, H);
      }

      gcols = Math.max(1, Math.ceil(W / CELL));
      grows = Math.max(1, Math.ceil(H / CELL));
      cellHead = new Int32Array(gcols * grows);
    }

    buildParticles();
    resize();
    const ro = new ResizeObserver(resize);
    ro.observe(canvas);

    // ---- pointer (CSS px) ----
    const m = { x: -1e3, y: -1e3, lx: -1e3, ly: -1e3, inside: false };
    function onMove(e: PointerEvent) {
      const rect = canvas!.getBoundingClientRect();
      m.x = e.clientX - rect.left;
      m.y = e.clientY - rect.top;
      m.inside = true;
    }
    function onLeave() { m.inside = false; }
    window.addEventListener('pointermove', onMove, { passive: true });
    window.addEventListener('pointerleave', onLeave);

    gl.enable(gl.BLEND);

    let raf = 0;
    let last = performance.now();
    let firstMove = true;

    function frame(now: number) {
      const p = live.current;
      let dt = (now - last) / 16.6667;
      last = now;
      if (dt > 3) dt = 3;

      let mvx = 0;
      let mvy = 0;
      if (m.inside) {
        if (firstMove) { m.lx = m.x; m.ly = m.y; firstMove = false; }
        mvx = m.x - m.lx;
        mvy = m.y - m.ly;
        m.lx = m.x; m.ly = m.y;
      }
      const mspeed = Math.sqrt(mvx * mvx + mvy * mvy);
      const gate = p.interactive && m.inside ? Math.min(1, mspeed / 4) : 0;

      const targetX = m.inside ? m.x : W / 2;
      const targetY = m.inside ? m.y : H / 2;

      // 1) Air jitter + paddle OR Gravity Vortex Swarm forces
      for (let i = 0; i < count; i++) {
        if (isNebula) {
          const dx = targetX - px[i];
          const dy = targetY - py[i];
          const d2 = dx * dx + dy * dy;
          const d = Math.sqrt(d2);
          if (d > 5) {
            const inv = 1 / d;
            const pull = Math.min(0.22, 35 / (d + 20)) * p.speed * dt;
            vx[i] += dx * inv * pull;
            vy[i] += dy * inv * pull;
            
            const swirl = Math.min(0.32, 28 / (d + 10)) * p.speed * dt;
            vx[i] += -dy * inv * swirl;
            vy[i] += dx * inv * swirl;
          }
          vx[i] += (Math.random() - 0.5) * JITTER * p.speed * dt * 0.4;
          vy[i] += (Math.random() - 0.5) * JITTER * p.speed * dt * 0.4;
          vx[i] *= 0.985;
          vy[i] *= 0.985;
        } else {
          vx[i] += (Math.random() - 0.5) * JITTER * p.speed * dt;
          vy[i] += (Math.random() - 0.5) * JITTER * p.speed * dt;
          if (gate > 0.001) {
            const dx = px[i] - m.x;
            const dy = py[i] - m.y;
            const d2 = dx * dx + dy * dy;
            if (d2 < CUR_R2) {
              const infl = Math.exp(-d2 / (CUR_R2 * 0.33));
              if (infl > 0.003) {
                const inv = 1 / Math.sqrt(d2 + 1e-3);
                const r = resp[i] * infl * dt;
                vx[i] += (dx * inv * PUSH * gate + mvx * DRAG) * r;
                vy[i] += (dy * inv * PUSH * gate + mvy * DRAG) * r;
              }
            }
          }
        }
      }

      let lineNumPoints = 0;

      // 2) Elastic puck-vs-puck collisions via spatial grid (skipped in nebula mode)
      if (!isNebula) {
        cellHead.fill(-1);
        for (let i = 0; i < count; i++) {
          let cx = (px[i] / CELL) | 0;
          let cy = (py[i] / CELL) | 0;
          if (cx < 0) cx = 0; else if (cx >= gcols) cx = gcols - 1;
          if (cy < 0) cy = 0; else if (cy >= grows) cy = grows - 1;
          const ci = cy * gcols + cx;
          next[i] = cellHead[ci];
          cellHead[ci] = i;
        }
        for (let i = 0; i < count; i++) {
          const xi = px[i];
          const yi = py[i];
          const ri = rad[i];
          const imi = invM[i];
          let cx = (xi / CELL) | 0;
          let cy = (yi / CELL) | 0;
          if (cx < 0) cx = 0; else if (cx >= gcols) cx = gcols - 1;
          if (cy < 0) cy = 0; else if (cy >= grows) cy = grows - 1;
          for (let oy = -1; oy <= 1; oy++) {
            const yy = cy + oy;
            if (yy < 0 || yy >= grows) continue;
            for (let ox = -1; ox <= 1; ox++) {
              const xx = cx + ox;
              if (xx < 0 || xx >= gcols) continue;
              let j = cellHead[yy * gcols + xx];
              while (j !== -1) {
                if (j > i) {
                  const dx = xi - px[j];
                  const dy = yi - py[j];
                  const minD = ri + rad[j];
                  const d2 = dx * dx + dy * dy;
                  if (d2 < minD * minD && d2 > 1e-6) {
                    const d = Math.sqrt(d2);
                    const nx = dx / d;
                    const ny = dy / d;
                    const imj = invM[j];
                    const imSum = imi + imj;
                    const overlap = minD - d;
                    const sepI = overlap * (imi / imSum);
                    const sepJ = overlap * (imj / imSum);
                    px[i] += nx * sepI; py[i] += ny * sepI;
                    px[j] -= nx * sepJ; py[j] -= ny * sepJ;
                    const rel = (vx[i] - vx[j]) * nx + (vy[i] - vy[j]) * ny;
                    if (rel < 0) {
                      const jimp = (-(1 + REST) * rel) / imSum;
                      vx[i] += jimp * imi * nx; vy[i] += jimp * imi * ny;
                      vx[j] -= jimp * imj * nx; vy[j] -= jimp * imj * ny;
                    }
                  }

                  // Gather neural constellation lines (closer than 75px)
                  if (isNeural && d2 < 75 * 75 && lineNumPoints < 24000 - 6) {
                    const d = Math.sqrt(d2);
                    const alpha = 1.0 - d / 75.0;
                    lineDataArr[lineNumPoints * 3] = xi;
                    lineDataArr[lineNumPoints * 3 + 1] = yi;
                    lineDataArr[lineNumPoints * 3 + 2] = alpha;
                    lineNumPoints++;

                    lineDataArr[lineNumPoints * 3] = px[j];
                    lineDataArr[lineNumPoints * 3 + 1] = py[j];
                    lineDataArr[lineNumPoints * 3 + 2] = alpha;
                    lineNumPoints++;
                  }
                }
                j = next[j];
              }
            }
          }
        }
      }

      // 3) Integrate positions, friction, bounds check
      for (let i = 0; i < count; i++) {
        if (!isNebula) {
          vx[i] *= FRICTION;
          vy[i] *= FRICTION;
          let s = Math.sqrt(vx[i] * vx[i] + vy[i] * vy[i]);
          if (s > MAXV * p.speed) { const k = (MAXV * p.speed) / s; vx[i] *= k; vy[i] *= k; s = MAXV * p.speed; }
        }
        let s = Math.sqrt(vx[i] * vx[i] + vy[i] * vy[i]);
        let nxp = px[i] + vx[i] * p.speed * dt;
        let nyp = py[i] + vy[i] * p.speed * dt;
        const ri = rad[i];

        if (nxp < ri) { nxp = ri; vx[i] = -vx[i] * (isNebula ? 0.3 : WALL); }
        else if (nxp > W - ri) { nxp = W - ri; vx[i] = -vx[i] * (isNebula ? 0.3 : WALL); }
        if (nyp < ri) { nyp = ri; vy[i] = -vy[i] * (isNebula ? 0.3 : WALL); }
        else if (nyp > H - ri) { nyp = H - ri; vy[i] = -vy[i] * (isNebula ? 0.3 : WALL); }

        px[i] = nxp; py[i] = nyp;
        posArr[i * 2] = nxp;
        posArr[i * 2 + 1] = nyp;
        spdArr[i] = s;
      }

      gl!.bindBuffer(gl!.ARRAY_BUFFER, posBuf);
      gl!.bufferSubData(gl!.ARRAY_BUFFER, 0, posArr);
      gl!.bindBuffer(gl!.ARRAY_BUFFER, spdBuf);
      gl!.bufferSubData(gl!.ARRAY_BUFFER, 0, spdArr);

      gl!.blendFunc(gl!.SRC_ALPHA, p.additive ? gl!.ONE : gl!.ONE_MINUS_SRC_ALPHA);
      gl!.clearColor(0, 0, 0, 0);
      gl!.clear(gl!.COLOR_BUFFER_BIT);

      const a = hexToRgb(p.palette[0]);
      const b = hexToRgb(p.palette[1]);
      const c = hexToRgb(p.palette[2]);

      // Draw constellation web lines
      if (isNeural && lineProg && lineU && lineNumPoints > 0) {
        gl!.useProgram(lineProg);
        gl!.bindVertexArray(lineVao);
        gl!.bindBuffer(gl!.ARRAY_BUFFER, lineBuf);
        gl!.bufferData(gl!.ARRAY_BUFFER, lineDataArr.subarray(0, lineNumPoints * 3), gl!.DYNAMIC_DRAW);

        gl!.blendFunc(gl!.SRC_ALPHA, gl!.ONE); // Additive blending for glow threads
        gl!.uniform3f(lineU.color, a[0], a[1], a[2]);
        gl!.uniform1f(lineU.intensity, p.intensity);
        gl!.drawArrays(gl!.LINES, 0, lineNumPoints);
      }

      // Draw standard particles
      gl!.useProgram(prog);
      gl!.bindVertexArray(vao);
      gl!.blendFunc(gl!.SRC_ALPHA, p.additive ? gl!.ONE : gl!.ONE_MINUS_SRC_ALPHA);
      gl!.uniform3f(U.a, a[0], a[1], a[2]);
      gl!.uniform3f(U.b, b[0], b[1], b[2]);
      gl!.uniform3f(U.c, c[0], c[1], c[2]);
      gl!.uniform1f(U.mono, p.monochrome ? 1 : 0);
      gl!.uniform1f(U.intensity, p.intensity);
      gl!.drawArrays(gl!.POINTS, 0, count);

      if (!reduceMotion) raf = requestAnimationFrame(frame);
    }
    raf = requestAnimationFrame(frame);

    function onVisibility() {
      if (document.hidden) {
        cancelAnimationFrame(raf);
      } else if (!reduceMotion) {
        last = performance.now();
        raf = requestAnimationFrame(frame);
      }
    }
    document.addEventListener('visibilitychange', onVisibility);

    return () => {
      cancelAnimationFrame(raf);
      ro.disconnect();
      window.removeEventListener('pointermove', onMove);
      window.removeEventListener('pointerleave', onLeave);
      document.removeEventListener('visibilitychange', onVisibility);
      if (gl) {
        if (prog) gl.deleteProgram(prog);
        if (vs) gl.deleteShader(vs);
        if (fs) gl.deleteShader(fs);
        if (posBuf) gl.deleteBuffer(posBuf);
        if (spdBuf) gl.deleteBuffer(spdBuf);
        if (sizeBuf) gl.deleteBuffer(sizeBuf);
        if (hueBuf) gl.deleteBuffer(hueBuf);
        if (vao) gl.deleteVertexArray(vao);

        if (lineProg) gl.deleteProgram(lineProg);
        if (lineVs) gl.deleteShader(lineVs);
        if (lineFs) gl.deleteShader(lineFs);
        if (lineBuf) gl.deleteBuffer(lineBuf);
        if (lineVao) gl.deleteVertexArray(lineVao);
      }
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [density, themeTick, monochrome, interactive, palette]);

  // additive / intensity are per-frame uniforms read from live.current — update
  // them in place instead of re-running the heavy effect. The login screen flips
  // these on every theme switch (additive={isDark}, intensity={isDark?…}); if
  // they were effect deps the WebGL context tore down + rebuilt mid-switch,
  // blanking the full-screen canvas for a frame = the login "white flash".
  useEffect(() => {
    live.current = { ...live.current, additive, intensity };
  }, [additive, intensity]);

  // Lose WebGL context only when the canvas element actually unmounts
  useEffect(() => {
    return () => {
      const canvas = canvasRef.current;
      if (canvas) {
        const glCtx = canvas.getContext('webgl2');
        if (glCtx) {
          const ext = glCtx.getExtension('WEBGL_lose_context');
          if (ext) ext.loseContext();
        }
      }
    };
  }, []);

  return <canvas ref={canvasRef} className={className} aria-hidden="true" />;
}
