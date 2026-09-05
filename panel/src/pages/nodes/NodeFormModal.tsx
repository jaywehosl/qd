import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { CopyOutlined } from '@ant-design/icons';

import { Button, Dialog, Field, Input, Select, Tag, Tooltip, TooltipProvider } from '@/components/ds';
import { getMessage } from '@/utils/messageBus';
import { ClipboardManager, HttpUtil } from '@/utils';
import { useFormSeed } from '@/hooks/useFormSeed';
import type { NodeRecord } from '@/api/queries/useNodesQuery';
import type { Msg } from '@/utils';
import { NodeEditSchema, NodeFormSchema, type NodeFormValues } from '@/schemas/node';
const DEPLOY_SCRIPT_URL = 'https://raw.githubusercontent.com/jaywehosl/qd/main/install-node.sh';

type Mode = 'add' | 'edit';

interface NodeFormModalProps {
  open: boolean;
  mode: Mode;
  node: NodeRecord | null;
  taken?: string[];
  usedIds?: number[];
  save: (payload: Partial<NodeRecord>) => Promise<Msg<unknown>>;
  onOpenChange: (open: boolean) => void;
}

type NamePools = { ingress: string[]; egress: string[] };

interface NetworkIdentity {
  key?: string;
  adminUuid?: string;
  adminTag?: string;
}

function newUuid(): string {
  return crypto.randomUUID();
}

function defaultValues(): NodeFormValues {
  return {
    id: 0,
    uuid: newUuid(),
    name: '',
    role: 'ingress',
    dnsPrimary: '',
    dnsSecondary: '',
    authority: '',
    certPath: '',
    keyPath: '',
    address: '',
    port: 443,
    apiToken: '',
    enable: true,
  };
}

export default function NodeFormModal({
  open,
  mode,
  node,
  taken = [],
  usedIds = [],
  save,
  onOpenChange,
}: NodeFormModalProps) {
  const { t } = useTranslation();
  const message = getMessage();
  const isEdit = mode === 'edit';

  const [values, setValues] = useState<NodeFormValues>(defaultValues);
  const [submitting, setSubmitting] = useState(false);
  const [script, setScript] = useState('');
  const [pools, setPools] = useState<NamePools>({ ingress: [], egress: [] });
  const [identity, setIdentity] = useState({ adminUuid: '', adminTag: '' });

  function set<K extends keyof NodeFormValues>(key: K, value: NodeFormValues[K]) {
    setValues((prev) => ({ ...prev, [key]: value }));
    setScript('');
  }

  useFormSeed(open, `${mode}:${node?.id ?? 0}`, () => {
    const base = defaultValues();
    setValues(isEdit && node
      ? { ...base, ...(node as unknown as Partial<NodeFormValues>), id: node.id }
      : base);
    setScript('');
  });

  useEffect(() => {
    if (!open || isEdit) return;
    const next = usedIds.reduce((top, id) => (id > top ? id : top), 0) + 1;
    setValues((prev) => (prev.id === next ? prev : { ...prev, id: next }));
  }, [open, isEdit, usedIds]);

  useEffect(() => {
    if (!open || isEdit) return;
    let alive = true;
    void (async () => {
      const msg = await HttpUtil.get<NetworkIdentity>('/panel/api/network/key', undefined, { silent: true });
      const seen = msg?.success ? msg.obj : undefined;
      if (!alive || !seen) return;
      if (seen.key) setValues((prev) => ({ ...prev, apiToken: seen.key as string }));
      setIdentity({ adminUuid: seen.adminUuid ?? '', adminTag: seen.adminTag ?? '' });
    })();
    return () => { alive = false; };
  }, [open, isEdit]);

  useEffect(() => {
    if (!open) return;
    let alive = true;
    void (async () => {
      const msg = await HttpUtil.get<NamePools>('/panel/api/nodes/names', undefined, { silent: true });
      if (alive && msg?.success && msg.obj) setPools(msg.obj);
    })();
    return () => { alive = false; };
  }, [open]);

  const nameOptions = useMemo(() => {
    const pool = values.role === 'egress' ? pools.egress : pools.ingress;
    const held = new Set(taken.filter((name) => name !== node?.name));
    const free = pool.filter((name) => !held.has(name)).sort((a, b) => a.localeCompare(b));
    const current = values.name.trim();
    if (current && !free.includes(current)) free.unshift(current);
    return free.map((name) => ({ value: name, label: name }));
  }, [pools, values.role, values.name, taken, node?.name]);

  useEffect(() => {
    if (!open) return;
    const pool = values.role === 'egress' ? pools.egress : pools.ingress;
    if (pool.length === 0) return;
    if (values.name && pool.includes(values.name)) return;

    const held = new Set(taken.filter((name) => name !== node?.name));
    const free = pool.filter((name) => !held.has(name));
    if (free.length > 0) set('name', free[Math.floor(Math.random() * free.length)]);
  }, [open, values.role, pools]);

  const title = useMemo(
    () => (isEdit ? (
      <span className="ef-title">
        {t('edit', { defaultValue: 'Edit' })}
        <Tag tone="success" className="ef-title__node">{node?.name || '—'}</Tag>
        {t('pages.nodes.nodeWord', { defaultValue: 'node' })}
      </span>
    ) : t('pages.nodes.addNode')),
    [isEdit, node?.name, t],
  );

  const addressLabel = t('pages.nodes.ipAddress');
  const addressPlaceholder = t('pages.nodes.ipPlaceholder');

  function buildPayload(v: NodeFormValues): Partial<NodeRecord> {
    const common = {
      id: v.id || 0,
      name: v.name.trim(),
      role: v.role,
      port: v.port,
      dnsPrimary: v.dnsPrimary.trim(),
      dnsSecondary: v.dnsSecondary.trim(),
      authority: v.authority.trim(),
      certPath: v.certPath.trim(),
      keyPath: v.keyPath.trim(),
    };
    if (isEdit) return common;
    return {
      ...common,
      uuid: v.uuid,
      address: v.address.trim(),
      apiToken: v.apiToken.trim(),
      enable: v.enable,
      status: 'waiting',
    };
  }

  function validate(): NodeFormValues | null {
    const result = isEdit
      ? NodeEditSchema.safeParse(values)
      : NodeFormSchema.safeParse(values);
    if (!result.success) {
      message.error(t(result.error.issues[0]?.message ?? 'pages.nodes.toasts.fillRequired'));
      return null;
    }
    return { ...values, ...result.data };
  }

  function onGenerate() {
    const v = validate();
    if (!v) return;
    if (!identity.adminUuid) {
      message.error(t('pages.nodes.toasts.noAdmin', {
        defaultValue: 'The panel does not know which administrator it is running as yet',
      }));
      return;
    }

    const domain = (v.authority.trim() || v.address.trim()).replace(/:\d+$/, '');
    const args = [
      `--key ${v.apiToken}`,
      `--tag ${v.name.trim()}`,
      `--role ${v.role}`,
      `--domain ${domain}`,
      `--port ${v.port}`,
      `--node-id ${v.id}`,
      `--node-uuid ${v.uuid}`,
      `--admin-uuid ${identity.adminUuid}`,
      `--admin ${identity.adminTag}`,
    ].join(' ');
    setScript(`bash <(curl -Ls ${DEPLOY_SCRIPT_URL}) ${args}`);
  }

  async function copyScript() {
    if (!script) return;
    const ok = await ClipboardManager.copyText(script);
    if (ok) message.success(t('copied'));
  }

  async function submit() {
    const v = validate();
    if (!v) return;
    setSubmitting(true);
    try {
      const msg = await save(buildPayload(v));
      if (msg?.success) onOpenChange(false);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => { if (!o && !submitting) onOpenChange(false); }}
      title={title}
      width={720}
      autoHeight
      okText={t('save')}
      confirmLoading={submitting}
      onOk={submit}
    >
      <div className="node-form">
        <div className="node-form-grid cols-2">
          <Field label={t('pages.nodes.tag')}>
            <Select
              value={values.name}
              onChange={(v) => set('name', String(v))}
              options={nameOptions}
            />
          </Field>
          <Field label={t('pages.nodes.role')}>
            <Select
              value={values.role}
              onChange={(v) => set('role', v as NodeFormValues['role'])}
              options={[
                { value: 'ingress', label: t('pages.nodes.roleIngress') },
                { value: 'egress', label: t('pages.nodes.roleEgress') },
              ]}
            />
          </Field>
        </div>

        <div className="node-form-grid cols-2">
          <Field label={t('pages.nodes.dnsPrimary', { defaultValue: 'DNS upstream, first' })}>
            <Input
              value={values.dnsPrimary}
              placeholder={t('pages.nodes.dnsInherit', { defaultValue: 'empty — use the network setting' })}
              onChange={(e) => set('dnsPrimary', e.target.value)}
            />
          </Field>
          <Field label={t('pages.nodes.dnsSecondary', { defaultValue: 'DNS upstream, second' })}>
            <Input
              value={values.dnsSecondary}
              placeholder={t('pages.nodes.dnsInherit', { defaultValue: 'empty — use the network setting' })}
              onChange={(e) => set('dnsSecondary', e.target.value)}
            />
          </Field>
        </div>
        <div className="node-form-grid">
          <Field label={t('pages.nodes.authority', { defaultValue: 'Domain clients dial' })}>
            <Input
              value={values.authority}
              placeholder={t('pages.nodes.authorityHint', { defaultValue: 'empty — the address above' })}
              onChange={(e) => set('authority', e.target.value)}
            />
          </Field>
        </div>

        <div className={isEdit ? 'node-form-grid' : 'node-form-grid cols-address'}>
          {!isEdit && (
            <Field label={addressLabel}>
              <Input
                value={values.address}
                placeholder={addressPlaceholder}
                onChange={(e) => set('address', e.target.value)}
              />
            </Field>
          )}
          <Field label={t('pages.nodes.port')}>
            <Input
              type="number"
              min={1}
              max={65535}
              value={values.port}
              onChange={(e) => set('port', Number(e.target.value) || 0)}
            />
          </Field>
        </div>

        {!isEdit && (
          <>
            <div className="deploy-row">
              <Button variant="primary" onClick={onGenerate}>{t('pages.nodes.generateDeploy')}</Button>
              {script && (
                <>
                  <div className="deploy-script">
                    <code className="deploy-script__text">{script}</code>
                    <TooltipProvider>
                      <Tooltip title={t('copy')}>
                        <Button size="sm" variant="text" icon={<CopyOutlined />} aria-label={t('copy')} onClick={copyScript} />
                      </Tooltip>
                    </TooltipProvider>
                  </div>
                </>
              )}
            </div>
          </>
        )}
      </div>
    </Dialog>
  );
}
