import { QRCodeSVG } from 'qrcode.react';

export interface QrCodeProps {
  value: string;
  size?: number;
  /** Foreground (modules) colour. */
  fgColor?: string;
  /** Background colour. */
  bgColor?: string;
  /** Error-correction level. */
  level?: 'L' | 'M' | 'Q' | 'H';
  className?: string;
}

/** SVG QR renderer (replaces antd QRCode). Always renders pure black-on-white
 *  by default so the code scans regardless of theme. */
export function QrCode({
  value,
  size = 240,
  fgColor = '#000000',
  bgColor = '#ffffff',
  level = 'M',
  className,
}: QrCodeProps) {
  return (
    <QRCodeSVG
      value={value}
      size={size}
      fgColor={fgColor}
      bgColor={bgColor}
      level={level}
      className={className}
    />
  );
}
