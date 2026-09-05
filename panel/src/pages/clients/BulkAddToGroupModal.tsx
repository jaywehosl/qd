import { useEffect, useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Dialog, Field, Input } from '@/components/ds';
import { getMessage } from '@/utils/messageBus';

interface BulkAddToGroupModalProps {
  open: boolean;
  count: number;
  groups: string[];
  onOpenChange: (open: boolean) => void;
  onSubmit: (group: string) => Promise<{ affected?: number } | null>;
}

export default function BulkAddToGroupModal({
  open,
  count,
  groups,
  onOpenChange,
  onSubmit,
}: BulkAddToGroupModalProps) {
  const { t } = useTranslation();
  const message = getMessage();
  const [value, setValue] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const listId = useId();

  useEffect(() => {
    if (open) setValue('');
  }, [open]);

  async function submit() {
    const next = value.trim();
    if (!next) return;
    setSubmitting(true);
    try {
      const result = await onSubmit(next);
      if (result) {
        const affected = result.affected ?? 0;
        message.success(t('pages.clients.addToGroupSuccessToast', { count: affected, group: next }));
        onOpenChange(false);
      }
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => !o && onOpenChange(false)}
      title={t('pages.clients.addToGroupTitle', { count })}
      okText={t('add')}
      okDisabled={!value.trim()}
      confirmLoading={submitting}
      onOk={submit}
    >
      <Field label={t('pages.clients.group')}>
        <Input
          list={listId}
          value={value}
          placeholder={t('pages.clients.groupName')}
          onChange={(e) => setValue(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && submit()}
          autoFocus
        />
        <datalist id={listId}>
          {groups.map((g) => <option key={g} value={g} />)}
        </datalist>
      </Field>
    </Dialog>
  );
}
