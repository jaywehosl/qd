import type { SubSettings } from '../useInbounds';

export interface ClientStats {
  email: string;
  up: number;
  down: number;
  total: number;
  expiryTime: number;
  enable?: boolean;
}

export interface ClientSetting {
  email?: string;
  id?: string;
  password?: string;
  subId?: string;
  totalGB?: number;
  expiryTime?: number;
  comment?: string;
  tgId?: string;
  enable?: boolean;
  limitIp?: number;
  created_at?: number;
  updated_at?: number;
}

export interface InboundInfo {
  protocol: string;
  clients: ClientSetting[];
  settings: Record<string, unknown>;
}

export interface DBInboundLike {
  id: number;
  address: string;
  port: number;
  listen: string;
  protocol: string;
  remark: string;
  enable?: boolean;
  settings: unknown;
  clientStats?: ClientStats[];
}

export interface InboundInfoModalProps {
  open: boolean;
  onClose: () => void;
  dbInbound: DBInboundLike | null;
  clientIndex?: number;
  expireDiff?: number;
  trafficDiff?: number;
  ipLimitEnable?: boolean;
  tgBotEnable?: boolean;
  subSettings?: SubSettings;
  lastOnlineMap?: Record<string, number>;
}
