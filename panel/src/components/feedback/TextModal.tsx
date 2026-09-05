import { Button, Dialog, Textarea } from '@/components/ds';
import { CopyOutlined, DownloadOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';

import { ClipboardManager, FileManager } from '@/utils';
import { getMessage } from '@/utils/messageBus';

interface TextModalProps {
  open: boolean;
  onClose: () => void;
  title: string;
  content: string;
  fileName?: string;
}

export default function TextModal({ open, onClose, title, content, fileName = '' }: TextModalProps) {
  const { t } = useTranslation();

  async function copy() {
    const ok = await ClipboardManager.copyText(content || '');
    if (ok) {
      getMessage().success(t('copied'));
      onClose();
    }
  }

  function download() {
    if (!fileName) return;
    FileManager.downloadTextFile(content, fileName);
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => { if (!o) onClose(); }}
      title={title}
      width={640}
      footer={(
        <>
          {fileName && (
            <Button icon={<DownloadOutlined />} onClick={download}>{fileName}</Button>
          )}
          <Button variant="primary" icon={<CopyOutlined />} onClick={copy}>{t('copy')}</Button>
        </>
      )}
    >
      <Textarea
        value={content}
        readOnly
        rows={14}
        style={{
          fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
          fontSize: 12,
          overflowY: 'auto',
        }}
      />
    </Dialog>
  );
}
