import { lazy, Suspense, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { Navigate, useLocation } from 'react-router-dom';

import { Spin } from '@/components/ui';
import { useClientState } from '@/hooks/useClientState';
import ConnectScreen from './ConnectScreen';
const ImportPage = lazy(() => import('./ImportPage'));

export default function ClientPage() {
  const { t } = useTranslation();
  const { hash } = useLocation();
  const {
    state, loading,
    importUri, connect, disconnect, setEgress, setAdblock,
    refreshSubscription, refreshing,
  } = useClientState();

  const onImport = useCallback(async (uri: string) => {
    const next = await importUri(uri);
    return next ? { ok: true } : { ok: false, error: t('client.import.failed') };
  }, [importUri, t]);

  if (hash === '#routing') return <Navigate to="/client/routing" replace />;

  if (loading) {
    return (
      <div className="client-boot">
        <Spin spinning size="large" description={t('loading')} />
      </div>
    );
  }

  if (!state?.imported) {
    return (
      <Suspense fallback={<div className="client-boot"><Spin spinning size="large" /></div>}>
        <ImportPage onImport={onImport} />
      </Suspense>
    );
  }

  return (
    <section className="feed-section">
      <div className="section-header">
        <h2>{t('client.menu.connect')}</h2>
      </div>
      <ConnectScreen
        state={state}
        onConnect={connect}
        onDisconnect={disconnect}
        onEgress={setEgress}
        onAdblock={setAdblock}
        onRefresh={refreshSubscription}
        refreshing={refreshing}
      />
    </section>
  );
}
