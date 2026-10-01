import { useRef } from 'react';
import { useTranslation } from 'react-i18next';

import { Card, Input, Select } from '@/components/ds';
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

  const brutal = (allSetting.brutalMbit ?? 0) > 0;
  const kept = useRef(brutal ? allSetting.brutalMbit : 100);

  return (
    <div className="settings-tab">
      <Card title={t('pages.settings.listener')}>
        <SettingListItem
          paddings="small"
          title={t('pages.settings.pool')}
          description={t('pages.settings.poolDesc')}
        >
          <Input {...text('pool', '10.7.0.0/16')} placeholder="10.7.0.0/16" />
        </SettingListItem>

        <SettingListItem
          paddings="small"
          title={t('pages.settings.ech')}
          description={t('pages.settings.echDesc')}
        >
          <Input {...text('echName', '')} placeholder={t('pages.settings.echOff')} />
        </SettingListItem>
      </Card>

      <Card title={t('pages.settings.carriage')}>
        <SettingListItem
          paddings="small"
          title={t('pages.settings.congestion')}
          description={t('pages.settings.congestionDesc')}
        >
          <Select
            value={brutal ? 'brutal' : 'bbr'}
            onChange={(mode) => updateSetting({ brutalMbit: mode === 'brutal' ? kept.current : 0 })}
            options={[
              { value: 'bbr', label: t('pages.settings.congestionBbr') },
              { value: 'brutal', label: t('pages.settings.congestionBrutal') },
            ]}
          />
        </SettingListItem>

        {brutal ? (
          <SettingListItem
            paddings="small"
            title={t('pages.settings.brutal')}
            description={t('pages.settings.brutalDesc')}
          >
            <Input
              type="number"
              min={1}
              max={10000}
              value={allSetting.brutalMbit}
              onChange={(e) => {
                kept.current = Math.max(1, Number(e.target.value) || 1);
                updateSetting({ brutalMbit: kept.current });
              }}
            />
          </SettingListItem>
        ) : (
          <SettingListItem
            paddings="small"
            title={t('pages.settings.bbrProfile')}
            description={t('pages.settings.bbrProfileDesc')}
          >
            <Select
              value={allSetting.bbrProfile || 'standard'}
              onChange={(bbrProfile) => updateSetting({ bbrProfile })}
              options={[
                { value: 'conservative', label: t('pages.settings.bbrConservative') },
                { value: 'standard', label: t('pages.settings.bbrStandard') },
                { value: 'aggressive', label: t('pages.settings.bbrAggressive') },
              ]}
            />
          </SettingListItem>
        )}

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
    </div>
  );
}
