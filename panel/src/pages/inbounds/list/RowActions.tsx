import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, DropdownMenu, type MenuEntry } from '@/components/ds';
import {
  MoreOutlined,
  CopyOutlined,
  ExportOutlined,
  RetweetOutlined,
  BlockOutlined,
  DeleteOutlined,
  InfoCircleOutlined,
  TagsOutlined,
  UsergroupAddOutlined,
  UsergroupDeleteOutlined,
} from '@ant-design/icons';

import { isInboundMultiUser } from './helpers';
import type { DBInboundRecord, RowAction } from './types';

interface RowActionsMenuProps {
  record: DBInboundRecord;
  subEnable: boolean;
  hasClients: boolean;
  onClick: (key: RowAction) => void;
  isMobile?: boolean;
}

export function buildRowActionsMenu({
  record,
  subEnable,
  t,
  hasClients,
  onClick,
}: {
  record: DBInboundRecord;
  subEnable: boolean;
  t: (k: string) => string;
  isMobile?: boolean;
  hasClients?: boolean;
  onClick: (key: RowAction) => void;
}): MenuEntry[] {
  const items: MenuEntry[] = [];
  const add = (key: RowAction, icon: ReactNode, label: string, danger?: boolean) =>
    items.push({ key, icon, label, danger, onSelect: () => onClick(key) });

  if (isInboundMultiUser(record)) {
    add('export', <ExportOutlined />, t('pages.inbounds.export'));
    if (subEnable) add('subs', <ExportOutlined />, `${t('pages.inbounds.export')} — ${t('pages.settings.subSettings')}`);
  } else {
    add('showInfo', <InfoCircleOutlined />, t('pages.inbounds.inboundInfo'));
  }
  add('clipboard', <CopyOutlined />, t('pages.inbounds.exportInbound'));
  add('resetTraffic', <RetweetOutlined />, t('pages.inbounds.resetTraffic'));
  add('clone', <BlockOutlined />, t('pages.inbounds.clone'));
  if (isInboundMultiUser(record)) {
    add('attachExisting', <UsergroupAddOutlined />, t('pages.inbounds.attachExistingClients'));
  }
  if (isInboundMultiUser(record) && hasClients) {
    add('attachClients', <UsergroupAddOutlined />, t('pages.inbounds.attachClients'));
    add('detachClients', <UsergroupDeleteOutlined />, t('pages.inbounds.detachClients'));
    add('addToGroup', <TagsOutlined />, t('pages.inbounds.addClientsToGroup'));
    items.push({ type: 'divider' });

  } else {
    items.push({ type: 'divider' });
  }
  add('delete', <DeleteOutlined />, t('delete'), true);
  return items;
}

export function RowActionsCell({ record, subEnable, hasClients, onClick }: RowActionsMenuProps) {
  const { t } = useTranslation();
  return (
    <div className="action-buttons">
      <DropdownMenu
        items={buildRowActionsMenu({ record, subEnable, t, hasClients, onClick })}
        trigger={<Button variant="text" size="sm" icon={<MoreOutlined />} />}
      />
    </div>
  );
}
