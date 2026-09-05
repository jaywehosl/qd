import { useTranslation } from 'react-i18next';

import { Card, Input } from '@/components/ds';
import { SettingListItem } from '@/components/ui';
import type { AllSetting } from '@/models/setting';

interface DatapathTabProps {
  allSetting: AllSetting;
  updateSetting: (patch: Partial<AllSetting>) => void;
}

export default function DatapathTab({ allSetting, updateSetting }: DatapathTabProps) {
  const { t } = useTranslation();

  const num = (key: keyof AllSetting, fallback: number) => ({
    value: (allSetting[key] as number) ?? fallback,
    onChange: (e: React.ChangeEvent<HTMLInputElement>) =>
      updateSetting({ [key]: Number(e.target.value) || 0 } as Partial<AllSetting>),
  });

  const text = (key: keyof AllSetting, fallback: string) => ({
    value: (allSetting[key] as string) ?? fallback,
    onChange: (e: React.ChangeEvent<HTMLInputElement>) =>
      updateSetting({ [key]: e.target.value } as Partial<AllSetting>),
  });

  return (
    <>
      <Card title={t('pages.settings.listener')}>
        <SettingListItem
          paddings="small"
          title={t('pages.settings.pool')}
          description={t('pages.settings.poolDesc')}
        >
          <Input {...text('pool', '10.7.0.0/16')} placeholder="10.7.0.0/16" />
        </SettingListItem>
      </Card>

      <Card title={t('pages.settings.carriage')}>
        <SettingListItem
          paddings="small"
          title={t('pages.settings.brutal')}
          description={t('pages.settings.brutalDesc')}
        >
          <Input type="number" min={0} max={10000} {...num('brutalMbit', 0)} />
        </SettingListItem>

        <SettingListItem
          paddings="small"
          title={t('pages.settings.mtu')}
          description={t('pages.settings.mtuDesc')}
        >
          <Input type="number" min={576} max={9000} {...num('mtu', 1400)} />
        </SettingListItem>

        <SettingListItem
          paddings="small"
          title={t('pages.settings.maxStreams')}
          description={t('pages.settings.maxStreamsDesc')}
        >
          <Input type="number" min={16} max={1048576} {...num('maxStreams', 65536)} />
        </SettingListItem>

        <SettingListItem
          paddings="small"
          title={t('pages.settings.streamWindow')}
          description={t('pages.settings.streamWindowDesc')}
        >
          <div className="setting-pair">
            <Input type="number" min={64} {...num('streamWindowKb', 2048)} />
            <Input type="number" min={64} {...num('maxStreamWindowKb', 6144)} />
          </div>
        </SettingListItem>

        <SettingListItem
          paddings="small"
          title={t('pages.settings.connWindow')}
          description={t('pages.settings.connWindowDesc')}
        >
          <div className="setting-pair">
            <Input type="number" min={64} {...num('connWindowKb', 3072)} />
            <Input type="number" min={64} {...num('maxConnWindowKb', 15360)} />
          </div>
        </SettingListItem>

        <SettingListItem
          paddings="small"
          title={t('pages.settings.socketBuffer')}
          description={t('pages.settings.socketBufferDesc')}
        >
          <Input type="number" min={256} {...num('socketBufferKb', 2048)} />
        </SettingListItem>

        <SettingListItem
          paddings="small"
          title={t('pages.settings.idle')}
          description={t('pages.settings.idleDesc')}
        >
          <div className="setting-pair">
            <Input type="number" min={5} {...num('idleSeconds', 30)} />
            <Input type="number" min={1} {...num('keepAliveSeconds', 15)} />
          </div>
        </SettingListItem>

        <SettingListItem
          paddings="small"
          title={t('pages.settings.statsSeconds')}
          description={t('pages.settings.statsSecondsDesc')}
        >
          <Input type="number" min={0} max={3600} {...num('statsSeconds', 5)} />
        </SettingListItem>
      </Card>
    </>
  );
}
