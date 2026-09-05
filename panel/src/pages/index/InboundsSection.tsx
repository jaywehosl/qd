import { useTranslation } from 'react-i18next';

import InboundsPage from '@/pages/inbounds/InboundsPage';
import SectionBoundary from '@/components/utility/SectionBoundary';

export default function InboundsSection() {
  const { t } = useTranslation();

  return (
    <div className="panel-page">
      <div className="section-header">
        <h2>{t('menu.inbounds')}</h2>
      </div>
      <SectionBoundary name="Inbounds">
        <InboundsPage />
      </SectionBoundary>
    </div>
  );
}
