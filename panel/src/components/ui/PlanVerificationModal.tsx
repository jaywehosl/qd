import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Dialog } from '@/components/ds';
import { SafetyCertificateOutlined, CloseCircleOutlined } from '@ant-design/icons';
interface PlanVerificationModalProps {
  open: boolean;
  title: string;
  original: any;
  modified: any;
  tasks?: string[];
  onConfirm: () => void;
  onCancel: () => void;
  confirmLoading?: boolean;
}

export function generateJsonDiff(oldObj: any, newObj: any) {
  // Deep clone and clean up internal UI state fields like keys or timestamps to avoid noise in diff
  const cleanObject = (obj: any) => {
    if (!obj) return null;
    const cloned = JSON.parse(JSON.stringify(obj));
    const removeUIFields = (item: any) => {
      if (item && typeof item === 'object') {
        delete item.id;
        delete item.key;
        delete item._key;
        delete item.up;
        delete item.down;
        Object.keys(item).forEach(k => removeUIFields(item[k]));
      }
    };
    removeUIFields(cloned);
    return cloned;
  };

  const oldClean = cleanObject(oldObj);
  const newClean = cleanObject(newObj);

  const oldStr = oldClean ? JSON.stringify(oldClean, null, 2) : '';
  const newStr = newClean ? JSON.stringify(newClean, null, 2) : '';

  if (!oldStr) {
    return newStr.split('\n').map(line => ({ type: 'add' as const, text: line }));
  }
  if (!newStr) {
    return oldStr.split('\n').map(line => ({ type: 'remove' as const, text: line }));
  }

  const oldLines = oldStr.split('\n');
  const newLines = newStr.split('\n');
  const n = oldLines.length;
  const m = newLines.length;

  const dp: number[][] = Array(n + 1).fill(0).map(() => Array(m + 1).fill(0));

  for (let r = 1; r <= n; r++) {
    for (let c = 1; c <= m; c++) {
      if (oldLines[r - 1] === newLines[c - 1]) {
        dp[r][c] = dp[r - 1][c - 1] + 1;
      } else {
        dp[r][c] = Math.max(dp[r - 1][c], dp[r][c - 1]);
      }
    }
  }

  const diff: { type: 'add' | 'remove' | 'normal'; text: string }[] = [];
  let r = n, c = m;
  while (r > 0 || c > 0) {
    if (r > 0 && c > 0 && oldLines[r - 1] === newLines[c - 1]) {
      diff.unshift({ type: 'normal', text: oldLines[r - 1] });
      r--;
      c--;
    } else if (c > 0 && (r === 0 || dp[r][c - 1] >= dp[r - 1][c])) {
      diff.unshift({ type: 'add', text: newLines[c - 1] });
      c--;
    } else {
      diff.unshift({ type: 'remove', text: oldLines[r - 1] });
      r--;
    }
  }

  return diff;
}

export default function PlanVerificationModal({
  open,
  title,
  original,
  modified,
  tasks,
  onConfirm,
  onCancel,
  confirmLoading,
}: PlanVerificationModalProps) {
  const { t } = useTranslation();

  const diff = useMemo(() => generateJsonDiff(original, modified), [original, modified]);

  const defaultTasks = useMemo(() => {
    if (tasks && tasks.length > 0) return tasks;
    const list = [];
    if (!original) {
      list.push(t('pages.inbounds.addInbound'));
    } else if (!modified) {
      list.push(t('delete') || 'Delete resource');
    } else {
      list.push(t('update') || 'Update configuration');
    }
    list.push('Compile and validate configuration schema');
    list.push('Inject parameters into database transaction');
    list.push('Safely push updates to active xray core instance');
    return list;
  }, [original, modified, tasks, t]);

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => { if (!o) onCancel(); }}
      title={<span className="plan-title">{title}</span>}
      width={720}
      footer={(
        <>
          <Button onClick={onCancel} icon={<CloseCircleOutlined />}>
            {t('cancel') || 'Cancel'}
          </Button>
          <Button
            variant="primary"
            onClick={onConfirm}
            loading={confirmLoading}
            icon={<SafetyCertificateOutlined />}
            className="btn-execute-plan"
          >
            Verify & Execute Change
          </Button>
        </>
      )}
    >
      <Alert
        tone="info"
        title="Trust Verification Required"
        description="Verify proposed configuration changes below before deployment. Review the auto-generated implementation plan and code diff."
        className="plan-alert"
      />

      <div className="plan-section-title">Implementation Steps</div>
      <ul className="plan-task-list">
        {defaultTasks.map((task, idx) => (
          <li key={idx} className="plan-task-item">
            <span className="task-checkbox">[x]</span> {task}
          </li>
        ))}
      </ul>

      <div className="plan-section-title">Configuration Diff (JSON)</div>
      <div className="plan-diff-viewport">
        {diff.map((line, idx) => (
          <div key={idx} className={`diff-line diff-line-${line.type}`}>
            <span className="diff-marker">
              {line.type === 'add' ? '+' : line.type === 'remove' ? '-' : ' '}
            </span>
            <span className="diff-text">{line.text}</span>
          </div>
        ))}
      </div>
    </Dialog>
  );
}
