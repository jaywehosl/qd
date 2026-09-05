import { useMemo } from 'react';
import SwaggerUI from 'swagger-ui-react';
import { useTheme } from '@/hooks/useTheme';
const basePath = window.X_UI_BASE_PATH || '';
const openApiUrl = `${basePath}panel/api/openapi.json`;

export default function ApiDocsPage() {
  const { isDark, isUltra } = useTheme();

  const pageClass = useMemo(() => {
    const classes = ['api-docs-page'];
    if (isDark) classes.push('is-dark');
    if (isUltra) classes.push('is-ultra');
    return classes.join(' ');
  }, [isDark, isUltra]);

  return (
    <div className={pageClass}>
      <div className="content-shell">
        <div className="content-area">
          <div className="docs-wrapper">
            <SwaggerUI
              url={openApiUrl}
              docExpansion="list"
              deepLinking={false}
              tryItOutEnabled
            />
          </div>
        </div>
      </div>
    </div>
  );
}
