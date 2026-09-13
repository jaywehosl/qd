import type { NodeRecord } from '@/api/queries/useNodesQuery';

export interface DBInboundRecord {
  id: number;
  enable: boolean;
  remark: string;
  port: number;
  protocol: string;
  up: number;
  down: number;
  total: number;
  expiryTime: number;
  _expiryTime: { valueOf(): number } | null;
  nodeId?: number | null;
  settings: unknown;
}

export interface ClientCountEntry {
  clients: number;
  active: string[];
  deactive: string[];
  depleted: string[];
  expiring: string[];
  online: string[];
}

export type RowAction =
  | 'edit'
  | 'showInfo'
  | 'export'
  | 'subs'
  | 'clipboard'
  | 'delete'
  | 'resetTraffic'
  | 'clone'
  | 'attachExisting'
  | 'attachClients'
  | 'detachClients'
  | 'addToGroup';

export type GeneralAction = 'import' | 'export' | 'subs' | 'resetInbounds';

export interface InboundListProps {
  dbInbounds: DBInboundRecord[];
  clientCount: Record<number, ClientCountEntry>;
  onlineClients: string[];
  lastOnlineMap: Record<string, number>;
  expireDiff: number;
  trafficDiff: number;
  pageSize: number;
  isMobile: boolean;
  subEnable: boolean;
  nodesById: Map<number, NodeRecord>;
  hasActiveNode: boolean;
  onAddInbound?: () => void;
  onGeneralAction?: (key: GeneralAction) => void;
  roleLabel?: string;
  onRowAction: (action: { key: RowAction; dbInbound: DBInboundRecord }) => void;
  onBulkDelete: (ids: number[]) => Promise<boolean>;
  onToggleGuide?: () => void;
}
