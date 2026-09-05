import { lazy, Suspense, useEffect } from 'react';
import { createBrowserRouter, Navigate, useRouteError, type RouteObject } from 'react-router-dom';

import PanelLayout from '@/layouts/PanelLayout';
import ClientLayout from '@/layouts/ClientLayout';
import { looksLikeClientGone, showClientClosed } from '@/lib/client-closed';

const loadInbounds = () => import('@/pages/index/InboundsSection');
const loadClients = () => import('@/pages/index/ClientsSection');
const loadGroups = () => import('@/pages/index/GroupsSection');
const loadNodes = () => import('@/pages/index/NodesSection');
const loadSettings = () => import('@/pages/settings/SettingsPage');
const loadApiDocs = () => import('@/pages/api-docs/ApiDocsPage');
const loadClientPage = () => import('@/pages/client/ClientPage');
const loadClientRouting = () => import('@/pages/client/RoutingPage');
const loadClientSettings = () => import('@/pages/client/ClientSettingsPage');

const ROUTE_LOADERS: Record<string, () => Promise<unknown>> = {
  '/panel/inbounds': loadInbounds,
  '/panel/clients': loadClients,
  '/panel/groups': loadGroups,
  '/panel/nodes': loadNodes,
  '/panel/settings': loadSettings,
  '/panel/api-docs': loadApiDocs,
  '/client': loadClientPage,
  '/client/routing': loadClientRouting,
  '/client/settings': loadClientSettings,
};

export function prefetchRoute(key: string) {
  void ROUTE_LOADERS[key.split('#')[0]]?.();
}

export function prefetchRoutes() {
  for (const load of Object.values(ROUTE_LOADERS)) void load();
}

const InboundsSection = lazy(loadInbounds);
const ClientsSection = lazy(loadClients);
const GroupsSection = lazy(loadGroups);
const NodesSection = lazy(loadNodes);
const SettingsPage = lazy(loadSettings);
const ApiDocsPage = lazy(loadApiDocs);
const ClientPage = lazy(loadClientPage);
const ClientRoutingPage = lazy(loadClientRouting);
const ClientSettingsPage = lazy(loadClientSettings);

function withSuspense(node: React.ReactNode) {
  return <Suspense fallback={<div className="loading-spacer" />}>{node}</Suspense>;
}

function RouteError() {
  const error = useRouteError();
  const clientGone = looksLikeClientGone(error);

  useEffect(() => {
    if (clientGone) showClientClosed();
  }, [clientGone]);

  if (clientGone) return null;

  return (
    <div style={{ padding: 24, fontFamily: 'system-ui, sans-serif' }}>
      <h1 style={{ fontSize: 18, marginBottom: 8 }}>Something went wrong</h1>
      <pre style={{ whiteSpace: 'pre-wrap', fontSize: 13, opacity: 0.75 }}>
        {String((error as { message?: unknown })?.message ?? error)}
      </pre>
    </div>
  );
}

const routes: RouteObject[] = [
  {
    path: '/client',
    element: <ClientLayout />,
    errorElement: <RouteError />,
    children: [
      { index: true, element: withSuspense(<ClientPage />) },
      { path: 'routing', element: withSuspense(<ClientRoutingPage />) },
      { path: 'settings', element: withSuspense(<ClientSettingsPage />) },
    ],
  },
  {
    path: '/panel',
    element: <PanelLayout />,
    errorElement: <RouteError />,
    children: [
      { index: true, element: <Navigate to="/panel/inbounds" replace /> },
      { path: 'inbounds', element: withSuspense(<InboundsSection />) },
      { path: 'clients', element: withSuspense(<ClientsSection />) },
      { path: 'groups', element: withSuspense(<GroupsSection />) },
      { path: 'nodes', element: withSuspense(<NodesSection />) },
      { path: 'settings', element: withSuspense(<SettingsPage />) },
      { path: 'appearance', element: <Navigate to="/client/settings#preferences" replace /> },
      { path: 'api-docs', element: withSuspense(<ApiDocsPage />) },
    ],
  },
  { path: '/', element: <Navigate to="/client" replace /> },
];

function computeBasename() {
  const raw = (typeof window !== 'undefined' && window.X_UI_BASE_PATH) || '/';
  return raw.replace(/\/+$/, '');
}

export const router = createBrowserRouter(routes, {
  basename: computeBasename(),
});
