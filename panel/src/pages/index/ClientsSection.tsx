import { useTranslation } from 'react-i18next';

import ClientsPage from '@/pages/clients/ClientsPage';
import SectionBoundary from '@/components/utility/SectionBoundary';

export default function ClientsSection() {
  const { t } = useTranslation();

  return (
    <div className="panel-page">
      <div className="section-header">
        <h2>{t('menu.clients')}</h2>
      </div>
      <SectionBoundary name="Clients">
        <ClientsPage />
      </SectionBoundary>
    </div>
  );
}
