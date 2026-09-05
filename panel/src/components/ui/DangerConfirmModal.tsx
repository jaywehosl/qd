import { useEffect, useState } from 'react';
import { Button, Dialog } from '@/components/ds';
import { DownloadOutlined, WarningOutlined } from '@ant-design/icons';
interface DangerConfirmModalProps {
  open: boolean;
  /** Human-readable labels of the access-critical fields being changed. */
  fields: string[];
  /** Seconds the user must wait (read the warning) before confirm arms. */
  countdownSecs?: number;
  onConfirm: () => void;
  onCancel: () => void;
  /** Trigger the existing DB export (snapshot before risking lock-out). */
  onBackup: () => void;
}

const DEFAULT_COUNTDOWN = 15;

export default function DangerConfirmModal({
  open,
  fields,
  countdownSecs = DEFAULT_COUNTDOWN,
  onConfirm,
  onCancel,
  onBackup,
}: DangerConfirmModalProps) {
  const [remaining, setRemaining] = useState(countdownSecs);
  const [ack, setAck] = useState(false);

  useEffect(() => {
    if (!open) return;
    setRemaining(countdownSecs);
    setAck(false);
    const started = Date.now();
    const iv = window.setInterval(() => {
      const left = countdownSecs - Math.floor((Date.now() - started) / 1000);
      setRemaining(left > 0 ? left : 0);
      if (left <= 0) window.clearInterval(iv);
    }, 250);
    return () => window.clearInterval(iv);
  }, [open, countdownSecs]);

  const armed = remaining <= 0 && ack;

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => { if (!o) onCancel(); }}
      title={
        <span className="danger-modal__title">
          <WarningOutlined /> This change can lock you out
        </span>
      }
      footer={null}
    >
      <div className="danger-modal">
        <p className="danger-modal__lead">
          You are changing settings that control how the panel is reached. Behind a
          reverse proxy + cookie-gate, the wrong value here can <strong>permanently
          cut off access</strong> to the panel or subscription page — there is no
          undo from inside the panel.
        </p>

        <ul className="danger-modal__fields">
          {fields.map((f) => (
            <li key={f}>{f}</li>
          ))}
        </ul>

        <div className="danger-modal__backup">
          <span>Snapshot the database first, just in case:</span>
          <Button size="sm" icon={<DownloadOutlined />} onClick={onBackup}>
            Back up now
          </Button>
        </div>

        <label className="danger-modal__ack">
          <input type="checkbox" checked={ack} onChange={(e) => setAck(e.target.checked)} />
          <span>I have read the warning, understand the risk, and take responsibility for this change.</span>
        </label>

        <div className="danger-modal__actions">
          <Button onClick={onCancel}>Cancel</Button>
          <Button variant="primary" danger disabled={!armed} onClick={onConfirm}>
            {remaining > 0 ? `Yes — change it (${remaining}s)` : 'Yes — change it'}
          </Button>
        </div>
      </div>
    </Dialog>
  );
}
