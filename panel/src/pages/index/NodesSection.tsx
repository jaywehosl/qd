import { useTranslation } from 'react-i18next';

import NodesPage from '@/pages/nodes/NodesPage';
import SectionBoundary from '@/components/utility/SectionBoundary';

export default function NodesSection() {
  const { t } = useTranslation();

  return (
    <div className="panel-page">
      <div className="section-header">
        <h2>{t('menu.nodes')}</h2>
      </div>
      <SectionBoundary name="Nodes">
        <NodesPage />
      </SectionBoundary>
    </div>
  );
}
