import { HttpUtil, type Msg, type HttpOptions } from '@/utils';

const JSON_HEADERS: HttpOptions = { headers: { 'Content-Type': 'application/json' } };

function enc(v: string | number): string {
  return encodeURIComponent(String(v));
}

export const clientApi = {
  clientApiState<T = unknown>(params?: Record<string, unknown>, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.get<T>('/client/api/state', params, options);
  },

  clientApiImport<T = unknown>(body: { uri: string }, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/client/api/import', body, { ...JSON_HEADERS, ...options });
  },

  clientApiConnect<T = unknown>(body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/client/api/connect', body, options);
  },

  clientApiDisconnect<T = unknown>(body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/client/api/disconnect', body, options);
  },

  clientApiToggle<T = unknown>(body: { egress?: boolean; adblock?: boolean }, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/client/api/toggle', body, { ...JSON_HEADERS, ...options });
  },

  clientApiSubscriptionRefresh<T = unknown>(body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/client/api/subscription/refresh', body, options);
  },

  clientApiNodes<T = unknown>(params?: Record<string, unknown>, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.get<T>('/client/api/nodes', params, options);
  },

  clientApiNotifications<T = unknown>(params?: Record<string, unknown>, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.get<T>('/client/api/notifications', params, options);
  },

  clientApiNotificationsRead<T = unknown>(body: { id: number }, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/client/api/notifications/read', body, { ...JSON_HEADERS, ...options });
  },

  clientApiNotificationsDismiss<T = unknown>(body: { id: number }, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/client/api/notifications/dismiss', body, { ...JSON_HEADERS, ...options });
  },

  clientApiNotificationsClear<T = unknown>(body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/client/api/notifications/clear', body, options);
  },

  clientApiHistoryByWindow<T = unknown>(window: string | number, params?: Record<string, unknown>, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.get<T>(`/client/api/history/${enc(window)}`, params, options);
  },

  clientApiRouting<T = unknown>(params?: Record<string, unknown>, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.get<T>('/client/api/routing', params, options);
  },

  postClientApiRouting<T = unknown>(body: { defaultRole: string; rules: unknown[] }, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/client/api/routing', body, { ...JSON_HEADERS, ...options });
  },

  clientApiRoutingProcesses<T = unknown>(params?: Record<string, unknown>, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.get<T>('/client/api/routing/processes', params, options);
  },

  clientApiSettings<T = unknown>(params?: Record<string, unknown>, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.get<T>('/client/api/settings', params, options);
  },

  postClientApiSettings<T = unknown>(body: { refreshMinutes: number; autostart: boolean; autostartBehaviour: string; manualBehaviour: string }, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/client/api/settings', body, { ...JSON_HEADERS, ...options });
  },

  clientApiAbout<T = unknown>(params?: Record<string, unknown>, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.get<T>('/client/api/about', params, options);
  },

  clientApiReset<T = unknown>(body: { subscription?: boolean }, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/client/api/reset', body, { ...JSON_HEADERS, ...options });
  },

  clientApiSetup<T = unknown>(params?: Record<string, unknown>, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.get<T>('/client/api/setup', params, options);
  },

  clientApiSetupInstall<T = unknown>(body: { autostart: boolean; desktop: boolean }, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/client/api/setup/install', body, { ...JSON_HEADERS, ...options });
  },
};

export const entrypointsApi = {
  entrypoints<T = unknown>(params?: Record<string, unknown>, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.get<T>('/panel/api/entrypoints', params, options);
  },
};

export const serverApi = {
  status<T = unknown>(params?: Record<string, unknown>, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.get<T>('/panel/api/server/status', params, options);
  },

  getDb<T = unknown>(params?: Record<string, unknown>, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.get<T>('/panel/api/server/getDb', params, options);
  },

  importDB<T = unknown>(form: FormData, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/api/server/importDB', form, options);
  },

  sync<T = unknown>(body: { nodeId: number }, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/api/server/sync', body, { ...JSON_HEADERS, ...options });
  },

  key<T = unknown>(params?: Record<string, unknown>, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.get<T>('/panel/api/network/key', params, options);
  },

  releases<T = unknown>(params?: Record<string, unknown>, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.get<T>('/panel/api/releases', params, options);
  },
};

export const clientsApi = {
  list<T = unknown>(params?: Record<string, unknown>, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.get<T>('/panel/api/clients/list', params, options);
  },

  listPaged<T = unknown>(params: { page: number; pageSize: number; search: string; group: string; status: string; sort: string; order: string }, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.get<T>('/panel/api/clients/list/paged', params, options);
  },

  add<T = unknown>(body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/api/clients/add', body, { ...JSON_HEADERS, ...options });
  },

  updateById<T = unknown>(id: string | number, body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>(`/panel/api/clients/update/${enc(id)}`, body, { ...JSON_HEADERS, ...options });
  },

  delById<T = unknown>(id: string | number, body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>(`/panel/api/clients/del/${enc(id)}`, body, options);
  },

  resetTrafficById<T = unknown>(id: string | number, body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>(`/panel/api/clients/resetTraffic/${enc(id)}`, body, options);
  },

  onlines<T = unknown>(params?: Record<string, unknown>, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.get<T>('/panel/api/clients/onlines', params, options);
  },

  lastOnline<T = unknown>(params?: Record<string, unknown>, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.get<T>('/panel/api/clients/lastOnline', params, options);
  },

  devicesBlock<T = unknown>(body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/api/clients/devices/block', body, { ...JSON_HEADERS, ...options });
  },

  devicesForget<T = unknown>(body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/api/clients/devices/forget', body, { ...JSON_HEADERS, ...options });
  },

  addressesForget<T = unknown>(body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/api/clients/addresses/forget', body, { ...JSON_HEADERS, ...options });
  },

  exitsForget<T = unknown>(body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/api/clients/exits/forget', body, { ...JSON_HEADERS, ...options });
  },

  groups<T = unknown>(params?: Record<string, unknown>, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.get<T>('/panel/api/clients/groups', params, options);
  },

  groupsSave<T = unknown>(body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/api/clients/groups/save', body, { ...JSON_HEADERS, ...options });
  },

  groupsMembers<T = unknown>(body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/api/clients/groups/members', body, { ...JSON_HEADERS, ...options });
  },

  groupsDelete<T = unknown>(body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/api/clients/groups/delete', body, { ...JSON_HEADERS, ...options });
  },
};

export const nodesApi = {
  list<T = unknown>(params?: Record<string, unknown>, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.get<T>('/panel/api/nodes/list', params, options);
  },

  names<T = unknown>(params?: Record<string, unknown>, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.get<T>('/panel/api/nodes/names', params, options);
  },

  add<T = unknown>(body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/api/nodes/add', body, { ...JSON_HEADERS, ...options });
  },

  updateById<T = unknown>(id: string | number, body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>(`/panel/api/nodes/update/${enc(id)}`, body, options);
  },

  setEnableById<T = unknown>(id: string | number, body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>(`/panel/api/nodes/setEnable/${enc(id)}`, body, { ...JSON_HEADERS, ...options });
  },

  delById<T = unknown>(id: string | number, body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>(`/panel/api/nodes/del/${enc(id)}`, body, options);
  },

  probeById<T = unknown>(id: string | number, body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>(`/panel/api/nodes/probe/${enc(id)}`, body, options);
  },

  restartAll<T = unknown>(body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/api/nodes/restartAll', body, options);
  },

  historyByIdByMetricByBucket<T = unknown>(id: string | number, metric: string | number, bucket: string | number, params: { window: number }, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.get<T>(`/panel/api/nodes/${enc(id)}/history/${enc(metric)}/${enc(bucket)}`, params, options);
  },

  historyExportById<T = unknown>(id: string | number, params?: Record<string, unknown>, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.get<T>(`/panel/api/nodes/${enc(id)}/history/export`, params, options);
  },

  logsByIdByCount<T = unknown>(id: string | number, count: string | number, body: { level: string }, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>(`/panel/api/nodes/${enc(id)}/logs/${enc(count)}`, body, { ...JSON_HEADERS, ...options });
  },

  logsClearById<T = unknown>(id: string | number, body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>(`/panel/api/nodes/${enc(id)}/logs/clear`, body, options);
  },

  logsDownloadById<T = unknown>(id: string | number, params?: Record<string, unknown>, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.get<T>(`/panel/api/nodes/${enc(id)}/logs/download`, params, options);
  },
};

export const dnsApi = {
  records<T = unknown>(params?: Record<string, unknown>, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.get<T>('/panel/api/dns/records', params, options);
  },

  recordsSave<T = unknown>(body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/api/dns/records/save', body, options);
  },

  recordsDel<T = unknown>(body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/api/dns/records/del', body, { ...JSON_HEADERS, ...options });
  },

  stats<T = unknown>(params?: Record<string, unknown>, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.get<T>('/panel/api/dns/stats', params, options);
  },

  flush<T = unknown>(body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/api/dns/flush', body, options);
  },

  lists<T = unknown>(params?: Record<string, unknown>, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.get<T>('/panel/api/dns/lists', params, options);
  },

  listsRefresh<T = unknown>(body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/api/dns/lists/refresh', body, options);
  },
};

export const settingsApi = {
  panelSettingAll<T = unknown>(body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/setting/all', body, options);
  },

  panelSettingUpdate<T = unknown>(body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/setting/update', body, { ...JSON_HEADERS, ...options });
  },

  panelSettingTheme<T = unknown>(body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/setting/theme', body, options);
  },
};

export const websocketApi = {
  ws<T = unknown>(params?: Record<string, unknown>, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.get<T>('/ws', params, options);
  },
};
