import { useTranslation } from 'react-i18next';

import GroupsPage from '@/pages/groups/GroupsPage';
import SectionBoundary from '@/components/utility/SectionBoundary';

export default function GroupsSection() {
  const { t } = useTranslation();

  return (
    <div className="panel-page">
      <div className="section-header">
        <h2>{t('menu.groups')}</h2>
      </div>
      <SectionBoundary name="Groups">
        <GroupsPage />
      </SectionBoundary>
    </div>
  );
}
