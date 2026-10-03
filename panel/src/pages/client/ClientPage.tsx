import { useTranslation } from 'react-i18next';
import { Navigate, useLocation, useOutletContext } from 'react-router-dom';

import { Spin } from '@/components/ui';
import { useClientState } from '@/hooks/useClientState';
import ConnectScreen from './ConnectScreen';

export default function ClientPage() {
  const { t } = useTranslation();
  const { hash } = useLocation();
  const { ready, setStaging } = useOutletContext<{ ready: boolean; setStaging: (busy: boolean) => void }>();
  const {
    state, loading, refetch,
    importUri, connect, disconnect, setEgress, setAdblock,
    refreshSubscription, refreshing,
  } = useClientState();

  if (hash === '#routing') return <Navigate to="/client/routing" replace />;

  if (loading || !state) {
    return (
      <div className="client-boot">
        <Spin spinning size="large" description={t('loading')} />
      </div>
    );
  }

  return (
    <section className="feed-section">
      <div className={`section-header${ready ? '' : ' is-veiled'}`}>
        <h2>{t('client.menu.connect')}</h2>
      </div>
      <ConnectScreen
        state={state}
        onConnect={connect}
        onDisconnect={disconnect}
        onEgress={setEgress}
        onAdblock={setAdblock}
        onRefresh={refreshSubscription}
        onImport={importUri}
        onSettle={refetch}
        onStage={setStaging}
        refreshing={refreshing}
      />
    </section>
  );
}
