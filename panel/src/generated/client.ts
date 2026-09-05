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
};

export const inboundsApi = {
  list<T = unknown>(params?: Record<string, unknown>, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.get<T>('/panel/api/inbounds/list', params, options);
  },

  options<T = unknown>(params?: Record<string, unknown>, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.get<T>('/panel/api/inbounds/options', params, options);
  },

  getById<T = unknown>(id: string | number, params?: Record<string, unknown>, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.get<T>(`/panel/api/inbounds/get/${enc(id)}`, params, options);
  },

  add<T = unknown>(body: { nodeId: number; port: number; remark: string; enable: boolean }, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/api/inbounds/add', body, { ...JSON_HEADERS, ...options });
  },

  updateById<T = unknown>(id: string | number, body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>(`/panel/api/inbounds/update/${enc(id)}`, body, { ...JSON_HEADERS, ...options });
  },

  setEnableById<T = unknown>(id: string | number, body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>(`/panel/api/inbounds/setEnable/${enc(id)}`, body, { ...JSON_HEADERS, ...options });
  },

  delById<T = unknown>(id: string | number, body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>(`/panel/api/inbounds/del/${enc(id)}`, body, options);
  },

  resetTrafficById<T = unknown>(id: string | number, body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>(`/panel/api/inbounds/${enc(id)}/resetTraffic`, body, options);
  },

  resetAllTraffics<T = unknown>(body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/api/inbounds/resetAllTraffics', body, options);
  },
};

export const serverApi = {
  status<T = unknown>(params?: Record<string, unknown>, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.get<T>('/panel/api/server/status', params, options);
  },
};

export const clientsApi = {
  list<T = unknown>(params?: Record<string, unknown>, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.get<T>('/panel/api/clients/list', params, options);
  },

  listPaged<T = unknown>(params: { page: number; pageSize: number; search: string; filter: string; sort: string; order: string }, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.get<T>('/panel/api/clients/list/paged', params, options);
  },

  getByEmail<T = unknown>(email: string | number, params?: Record<string, unknown>, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.get<T>(`/panel/api/clients/get/${enc(email)}`, params, options);
  },

  add<T = unknown>(body: { client: Record<string, unknown>; inboundIds: number[] }, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/api/clients/add', body, { ...JSON_HEADERS, ...options });
  },

  updateByEmail<T = unknown>(email: string | number, body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>(`/panel/api/clients/update/${enc(email)}`, body, { ...JSON_HEADERS, ...options });
  },

  delByEmail<T = unknown>(email: string | number, body: unknown, params: { keepTraffic: number }, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>(`/panel/api/clients/del/${enc(email)}`, body, { params: params, ...options });
  },

  attachByEmail<T = unknown>(email: string | number, body: { inboundIds: number[] }, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>(`/panel/api/clients/${enc(email)}/attach`, body, { ...JSON_HEADERS, ...options });
  },

  detachByEmail<T = unknown>(email: string | number, body: { inboundIds: number[] }, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>(`/panel/api/clients/${enc(email)}/detach`, body, { ...JSON_HEADERS, ...options });
  },

  resetAllTraffics<T = unknown>(body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/api/clients/resetAllTraffics', body, options);
  },

  bulkDel<T = unknown>(body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/api/clients/bulkDel', body, { ...JSON_HEADERS, ...options });
  },

  groupsBulkAdd<T = unknown>(body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/api/clients/groups/bulkAdd', body, { ...JSON_HEADERS, ...options });
  },

  groupsBulkRemove<T = unknown>(body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/api/clients/groups/bulkRemove', body, { ...JSON_HEADERS, ...options });
  },

  bulkAttach<T = unknown>(body: { emails: unknown[]; inboundIds: number[] }, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/api/clients/bulkAttach', body, { ...JSON_HEADERS, ...options });
  },

  bulkDetach<T = unknown>(body: { emails: unknown[]; inboundIds: number[] }, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/api/clients/bulkDetach', body, { ...JSON_HEADERS, ...options });
  },

  groups<T = unknown>(params?: Record<string, unknown>, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.get<T>('/panel/api/clients/groups', params, options);
  },

  groupsCreate<T = unknown>(body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/api/clients/groups/create', body, { ...JSON_HEADERS, ...options });
  },

  groupsRename<T = unknown>(body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/api/clients/groups/rename', body, { ...JSON_HEADERS, ...options });
  },

  groupsEntrypoints<T = unknown>(body: { name: string; entrypointIds: number[]; deviceLimit: number; allowExit: boolean }, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/api/clients/groups/entrypoints', body, { ...JSON_HEADERS, ...options });
  },

  groupsDelete<T = unknown>(body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/api/clients/groups/delete', body, { ...JSON_HEADERS, ...options });
  },

  resetTrafficByEmail<T = unknown>(email: string | number, body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>(`/panel/api/clients/resetTraffic/${enc(email)}`, body, options);
  },

  onlines<T = unknown>(body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/api/clients/onlines', body, options);
  },

  onlinesByNode<T = unknown>(body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/api/clients/onlinesByNode', body, options);
  },

  activeInbounds<T = unknown>(body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/api/clients/activeInbounds', body, options);
  },

  lastOnline<T = unknown>(body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/api/clients/lastOnline', body, options);
  },
};

export const nodesApi = {
  list<T = unknown>(params?: Record<string, unknown>, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.get<T>('/panel/api/nodes/list', params, options);
  },

  add<T = unknown>(body: { name: string; role: string; address: string; port: number; apiToken: string }, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/api/nodes/add', body, { ...JSON_HEADERS, ...options });
  },

  updateById<T = unknown>(id: string | number, body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>(`/panel/api/nodes/update/${enc(id)}`, body, { ...JSON_HEADERS, ...options });
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

  historyByIdByMetricByBucket<T = unknown>(id: string | number, metric: string | number, bucket: string | number, params?: Record<string, unknown>, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.get<T>(`/panel/api/nodes/${enc(id)}/history/${enc(metric)}/${enc(bucket)}`, params, options);
  },

  historyExportById<T = unknown>(id: string | number, params: { bucket: number }, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.get<T>(`/panel/api/nodes/${enc(id)}/history/export`, params, options);
  },

  logsByIdByCount<T = unknown>(id: string | number, count: string | number, body: { level: string; syslog: boolean }, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>(`/panel/api/nodes/${enc(id)}/logs/${enc(count)}`, body, { ...JSON_HEADERS, ...options });
  },

  logsDownloadById<T = unknown>(id: string | number, params?: Record<string, unknown>, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.get<T>(`/panel/api/nodes/${enc(id)}/logs/download`, params, options);
  },
};

export const publishApi = {
  draft<T = unknown>(params?: Record<string, unknown>, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.get<T>('/panel/api/publish/draft', params, options);
  },

  discard<T = unknown>(body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/api/publish/discard', body, options);
  },

  plan<T = unknown>(body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/api/publish/plan', body, options);
  },

  push<T = unknown>(body: { revision: number; nodeIds?: number[] }, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/api/publish/push', body, { ...JSON_HEADERS, ...options });
  },

  apply<T = unknown>(body: { revision: number }, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/api/publish/apply', body, { ...JSON_HEADERS, ...options });
  },

  status<T = unknown>(params?: Record<string, unknown>, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.get<T>('/panel/api/publish/status', params, options);
  },
};

export const backupApi = {
  pull<T = unknown>(body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/api/backup/pull', body, options);
  },

  restore<T = unknown>(form: FormData, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/api/backup/restore', form, options);
  },
};

export const settingsApi = {
  panelSettingAll<T = unknown>(body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/setting/all', body, options);
  },

  panelSettingDefaultSettings<T = unknown>(body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/setting/defaultSettings', body, options);
  },

  panelSettingUpdate<T = unknown>(body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/setting/update', body, { ...JSON_HEADERS, ...options });
  },

  panelSettingRestartPanel<T = unknown>(body?: unknown, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.post<T>('/panel/setting/restartPanel', body, options);
  },
};

export const websocketApi = {
  ws<T = unknown>(params?: Record<string, unknown>, options?: HttpOptions): Promise<Msg<T>> {
    return HttpUtil.get<T>('/ws', params, options);
  },
};
