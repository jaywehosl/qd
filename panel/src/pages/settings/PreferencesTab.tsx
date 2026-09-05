import { useTranslation } from 'react-i18next';
import { Card, Input, Select } from '@/components/ds';
import { SettingListItem } from '@/components/ui';
import type { AllSetting } from '@/models/setting';

interface PreferencesTabProps {
  allSetting: AllSetting;
  updateSetting: (patch: Partial<AllSetting>) => void;
}

const DATEPICKER_LIST: { name: string; value: 'gregorian' | 'jalalian' }[] = [
  { name: 'Gregorian (Standard)', value: 'gregorian' },
  { name: 'Jalalian (شمسی)', value: 'jalalian' },
];

export default function PreferencesTab({ allSetting, updateSetting }: PreferencesTabProps) {
  const { t } = useTranslation();

  return (
    <Card title={t('pages.settings.preferences')}>
      <SettingListItem
        paddings="small"
        title={t('pages.settings.pageSize')}
        description={t('pages.settings.pageSizeDesc')}
      >
        <Input
          type="number"
          min={0}
          max={1000}
          step={5}
          value={allSetting.pageSize}
          onChange={(e) => updateSetting({ pageSize: Number(e.target.value) || 0 })}
        />
      </SettingListItem>

      <SettingListItem
        paddings="small"
        title={t('pages.settings.refreshMinutes', { defaultValue: 'Subscription refresh (minutes)' })}
        description={t('pages.settings.refreshMinutesDesc', {
          defaultValue: 'How often every client re-reads its subscription. A client that sets its own interval keeps it.',
        })}
      >
        <Input
          type="number"
          min={1}
          max={1440}
          value={allSetting.refreshMinutes ?? 60}
          onChange={(e) => updateSetting({ refreshMinutes: Math.max(1, Number(e.target.value) || 1) })}
        />
      </SettingListItem>

      <SettingListItem
        paddings="small"
        title={t('pages.settings.timeZone')}
        description={t('pages.settings.timeZoneDesc')}
      >
        <Input
          value={allSetting.timeLocation}
          onChange={(e) => updateSetting({ timeLocation: e.target.value })}
        />
      </SettingListItem>

      <SettingListItem
        paddings="small"
        title={t('pages.settings.datepicker')}
        description={t('pages.settings.datepickerDescription')}
      >
        <Select
          value={allSetting.datepicker || 'gregorian'}
          onChange={(v) => updateSetting({ datepicker: v as 'gregorian' | 'jalalian' })}
          options={DATEPICKER_LIST.map((d) => ({ value: d.value, label: d.name }))}
        />
      </SettingListItem>

      <SettingListItem
        paddings="small"
        title={t('pages.settings.expireTimeDiff')}
        description={t('pages.settings.expireTimeDiffDesc')}
      >
        <Input
          type="number"
          min={0}
          value={allSetting.expireDiff}
          onChange={(e) => updateSetting({ expireDiff: Number(e.target.value) || 0 })}
        />
      </SettingListItem>

      <SettingListItem
        paddings="small"
        title={t('pages.settings.trafficDiff')}
        description={t('pages.settings.trafficDiffDesc')}
      >
        <Input
          type="number"
          min={0}
          max={100}
          value={allSetting.trafficDiff}
          onChange={(e) => updateSetting({ trafficDiff: Number(e.target.value) || 0 })}
        />
      </SettingListItem>
    </Card>
  );
}
