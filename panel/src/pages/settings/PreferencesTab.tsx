import { useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { Card, Input, Select } from '@/components/ds';
import { SettingListItem } from '@/components/ui';
import { HttpUtil } from '@/utils';
import type { AllSetting } from '@/models/setting';

interface PreferencesTabProps {
  allSetting: AllSetting;
  updateSetting: (patch: Partial<AllSetting>) => void;
}

interface Release {
  tag: string;
  signed: boolean;
}

const DATEPICKER_LIST: { name: string; value: 'gregorian' | 'jalalian' }[] = [
  { name: 'Gregorian (Standard)', value: 'gregorian' },
  { name: 'Jalalian (شمسی)', value: 'jalalian' },
];

export default function PreferencesTab({ allSetting, updateSetting }: PreferencesTabProps) {
  const { t } = useTranslation();

  const { data: releases = [], isFetching, isError } = useQuery<Release[]>({
    queryKey: ['releases'],
    queryFn: async () => {
      const msg = await HttpUtil.get<Release[]>('/panel/api/releases', undefined, { silent: true });
      if (!msg?.success) throw new Error(msg?.msg || 'releases');
      return msg.obj ?? [];
    },
    staleTime: 5 * 60 * 1000,
    retry: false,
  });

  const current = allSetting.clientVersion ?? '';
  const versionOptions = [
    { value: '', label: t('pages.settings.clientVersionOff') },
    ...releases.map((r) => ({
      value: r.tag,
      label: r.signed ? r.tag : `${r.tag} — ${t('pages.settings.clientVersionUnsigned')}`,
      disabled: !r.signed && r.tag !== current,
    })),
  ];
  if (current && !releases.some((r) => r.tag === current)) {
    versionOptions.push({ value: current, label: current, disabled: false });
  }

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
        title={t('pages.settings.clientVersion')}
        description={
          isError
            ? t('pages.settings.clientVersionNoList')
            : isFetching
              ? t('pages.settings.clientVersionLoading')
              : t('pages.settings.clientVersionDesc')
        }
      >
        <Select
          value={current}
          onChange={(v) => updateSetting({ clientVersion: v as string })}
          options={versionOptions}
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
