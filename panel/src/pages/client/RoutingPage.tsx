import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { Navigate } from 'react-router-dom';

import { Spin } from '@/components/ui';
import { useClientState } from '@/hooks/useClientState';
import RoutingSection from './RoutingSection';

export default function RoutingPage() {
  const { t } = useTranslation();
  const { state, loading, connect, disconnect } = useClientState();

  const onReconnect = useCallback(async () => {
    await disconnect();
    await connect();
  }, [disconnect, connect]);

  if (loading) {
    return (
      <div className="client-boot">
        <Spin spinning size="large" description={t('loading')} />
      </div>
    );
  }

  if (!state?.imported) return <Navigate to="/client" replace />;

  return (
    <section className="feed-section">
      <div className="section-header">
        <h2>{t('client.menu.routing')}</h2>
      </div>
      <RoutingSection connected={state.connected} onReconnect={onReconnect} />
    </section>
  );
}
