export const keys = {
  server: {
    status: () => ['server', 'status'] as const,
  },
  nodes: {
    root: () => ['nodes'] as const,
    list: () => ['nodes', 'list'] as const,
  },
  settings: {
    root: () => ['settings'] as const,
    all: () => ['settings', 'all'] as const,
  },
  entrypoints: () => ['entrypoints'] as const,
  clients: {
    root: () => ['clients'] as const,
    list: (params: unknown) => ['clients', 'list', params] as const,
    all: () => ['clients', 'all'] as const,
    onlines: () => ['clients', 'onlines'] as const,
    groups: () => ['clients', 'groups'] as const,
  },
} as const;
