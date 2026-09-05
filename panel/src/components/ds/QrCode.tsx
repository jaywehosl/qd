import { QRCodeSVG } from 'qrcode.react';

export interface QrCodeProps {
  value: string;
  size?: number;
  fgColor?: string;
  bgColor?: string;
  level?: 'L' | 'M' | 'Q' | 'H';
  className?: string;
}

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
