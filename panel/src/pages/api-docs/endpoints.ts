export type HttpMethod = 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE' | 'WS';
export type ParamLocation =
  | 'path'
  | 'query'
  | 'header'
  | 'body'
  | 'body (form)'
  | 'body (json)'
  | 'body (multipart)';
export type ParamType =
  | 'string'
  | 'integer'
  | 'integer[]'
  | 'number'
  | 'boolean'
  | 'object'
  | 'object[]'
  | 'array'
  | 'file';

export interface EndpointParam {
  name: string;
  in: ParamLocation;
  type: ParamType;
  desc?: string;
  optional?: boolean;
  defaultValue?: string | number | boolean;
}

export interface Endpoint {
  method: HttpMethod;
  path: string;
  summary: string;
  description?: string;
  deprecated?: boolean;
  params?: EndpointParam[];
  body?: string;
  response?: string;
  errorResponse?: string;
  errorStatus?: number;
}

export interface SubscriptionHeader {
  name: string;
  desc: string;
}

export interface Section {
  id: string;
  title: string;
  description?: string;
  subHeader?: SubscriptionHeader[];
  endpoints: Endpoint[];
}

export const sections: readonly Section[] = [
  {
    id: 'client',
    title: 'Client',
    description:
      'The local API of the client itself, served on 127.0.0.1 by the same process that runs the tunnel. It has nothing to do with the node API above — this is the page in the browser talking to the daemon in the tray. Requests carry the session token the tray handed over and are checked against Origin; a page that cannot present one gets nothing. The admin panel is served by this same process and appears only once a node has confirmed the imported key.',
    endpoints: [
      {
        method: 'GET',
        path: '/client/api/state',
        summary: 'Everything the shell needs in one call: whether a subscription has been imported at all, whether the tunnel is up, whether the imported key is an admin one, and the current toggle positions. Drives the choice between the import screen and the client proper.',
        response: '{\n  "success": true,\n  "obj": {\n    "imported": true,\n    "admin": true,\n    "connected": true,\n    "node": { "id": 1, "name": "node-1", "latencyMs": 8 },\n    "nodes": { "total": 3, "reachable": 2 },\n    "egress": false,\n    "adblock": true,\n    "subscription": {\n      "lastRefresh": 1735689600000,\n      "intervalMinutes": 60,\n      "expiresAt": 0\n    }\n  }\n}',
      },
      {
        method: 'POST',
        path: '/client/api/import',
        summary: 'Import a connection URI. The client stores it, contacts a node to find out what the key grants, and returns the resulting state — including whether this key makes the holder an admin. That answer comes from the node, never from parsing the link.',
        params: [
          { name: 'uri', in: 'body (json)', type: 'string', desc: 'The qd:// link handed out by the panel.' },
        ],
        body: '{\n  "uri": "qd://3048e6a9…@node-1.example.com:443?alt=node-1.example.com%3A8443#russia-in"\n}',
        errorResponse: '{\n  "success": false,\n  "msg": "No node accepted this key"\n}',
      },
      {
        method: 'POST',
        path: '/client/api/connect',
        summary: 'Bring the tunnel up. The client races every entrypoint it knows and keeps whichever answers first; the losers are told to stop expecting it. Returns once a winner is chosen, not once the first packet flows.',
      },
      {
        method: 'POST',
        path: '/client/api/disconnect',
        summary: 'Take the tunnel down and release the adapter. Routing rules and toggles are remembered.',
      },
      {
        method: 'POST',
        path: '/client/api/toggle',
        summary: 'Flip the exit and adblock switches. Adblock takes effect immediately — it is only a list in the local resolver. Exit applies to NEW flows: an established connection cannot be moved to a different exit without the far end seeing its peer change address, so live ones finish where they started. If the group does not permit an exit, the request is refused and the reason surfaces in notifications rather than being swallowed.',
        params: [
          { name: 'egress', in: 'body (json)', type: 'boolean', desc: 'Route through an exit node.', optional: true },
          { name: 'adblock', in: 'body (json)', type: 'boolean', desc: 'Filter the advert list in the local resolver.', optional: true },
        ],
        body: '{\n  "egress": true\n}',
        errorResponse: '{\n  "success": false,\n  "msg": "This subscription does not allow exit nodes"\n}',
      },
      {
        method: 'POST',
        path: '/client/api/subscription/refresh',
        summary: 'Fetch the subscription now instead of waiting for the interval. Works with or without the tunnel up — it rides the same transport but does not need it. If the node list changed but the node currently carrying the tunnel is still in it, the tunnel is left alone.',
        response: '{\n  "success": true,\n  "obj": {\n    "nodes": 3\n  }\n}',
      },
      {
        method: 'GET',
        path: '/client/api/nodes',
        summary: 'The entrypoints this subscription grants, with the measured latency of the last race and which one currently carries the tunnel. An admin sees every node in the network rather than the group\'s.',
        response: '{\n  "success": true,\n  "obj": [\n    { "id": 1, "name": "node-1", "role": "ingress", "latencyMs": 8, "selected": true, "reachable": true },\n    { "id": 2, "name": "node-4", "role": "egress", "latencyMs": 31, "selected": false, "reachable": true }\n  ]\n}',
      },
      {
        method: 'GET',
        path: '/client/api/notifications',
        summary: 'What the daemon has to say, newest first. The log lives in the daemon rather than in the page because the tunnel keeps running with no browser open: an automatic subscription refresh, a re-race after the network changed, a refused exit — all happen while nobody is looking, and still have to be there afterwards. This is the same list the tray balloon and the Android notification draw from, so a message never exists in only one of the three.',
        response: '{\n  "success": true,\n  "obj": {\n    "unread": 2,\n    "items": [\n      {\n        "id": 12,\n        "severity": "warning",\n        "text": "Exit nodes were refused: this subscription\'s group does not allow them.",\n        "ts": 1735689600000,\n        "read": false\n      },\n      {\n        "id": 11,\n        "severity": "info",\n        "text": "Subscription checked: 2 entrypoints reachable, nothing changed.",\n        "ts": 1735689000000,\n        "read": true\n      }\n    ]\n  }\n}',
      },
      {
        method: 'POST',
        path: '/client/api/notifications/read',
        summary: 'Mark one notification read, or all of them with id 0. Closing the bell is what sends this, not opening it: acknowledging on open would wipe the "new" highlight before it has been read, which is the one thing the highlight exists for. Read entries stay in the list; only the badge count changes.',
        params: [
          { name: 'id', in: 'body (json)', type: 'number', desc: 'The notification to mark, or 0 for everything currently listed.' },
        ],
        body: '{\n  "id": 0\n}',
      },
      {
        method: 'POST',
        path: '/client/api/notifications/dismiss',
        summary: 'Drop one notification for good. This is what the cross on a row sends: a row the reader has dealt with should leave the strip rather than sit there marked read, which is the difference between an acknowledgement and a filed message. Unlike marking read, it cannot be undone.',
        params: [
          { name: 'id', in: 'body (json)', type: 'number', desc: 'The notification to remove.' },
        ],
        body: '{\n  "id": 12\n}',
      },
      {
        method: 'POST',
        path: '/client/api/notifications/clear',
        summary: 'Throw the log away. Deliberate and unrecoverable — nothing here is stored anywhere else.',
      },
      {
        method: 'GET',
        path: '/client/api/history/:window',
        summary: 'The counters the tunnel already writes to its console, kept in a ring on this machine and never sent anywhere. Every series comes back in one call — the whole window is a few hundred numbers, and separate requests per metric would only risk the series drifting out of alignment with each other. The ring is capped, so a window longer than it holds returns what there is. Points are averaged down to at most 180, never sampled: a one-second loss spike inside a twenty-second bucket has to survive into the picture, and picking one sample per bucket would drop it most of the time. Everything is per second. While the tunnel is down the samples are zeros rather than absent, so a gap in service reads as a flat line instead of a hole.',
        params: [
          { name: 'window', in: 'path', type: 'number', desc: 'Minutes of history: 1, 5, 15 or 60.' },
        ],
        response: '{\n  "success": true,\n  "obj": {\n    "window": 5,\n    "points": [\n      {\n        "t": 1735689600,\n        "up": 3145728,\n        "down": 24117248,\n        "pktOut": 4494,\n        "pktIn": 18552,\n        "lost": 0,\n        "drops": 0,\n        "reorder": 1,\n        "retries": 0,\n        "sendDrop": 0,\n        "sendErr": 0,\n        "dnsQueries": 7,\n        "dnsCached": 5,\n        "dnsUpstream": 2,\n        "adblock": 1\n      }\n    ]\n  }\n}',
        errorResponse: '{\n  "success": false,\n  "msg": "Unknown window 240"\n}',
      },
      {
        method: 'GET',
        path: '/client/api/routing',
        summary: 'The per-process routing rules and the default everything without a rule falls back to. Processes only — no domains, no regex, no geoip: attribution comes from the connection table, which knows a local port and a PID and nothing else. `applyMode` says whether a change can land on a live tunnel: `live` on Windows, `restart` on Android, where VpnService fixes its per-app rules when the tunnel is built. When the mode is `restart` and rules have moved since, `pendingRestart` is set and the page offers a reconnect — it never does one on its own, because an unasked-for reconnect reads as a dropped tunnel.',
        response: '{\n  "success": true,\n  "obj": {\n    "defaultRole": "tunnel",\n    "applyMode": "live",\n    "pendingRestart": false,\n    "rules": [\n      {\n        "id": 1,\n        "process": "steam.exe",\n        "path": "C:\\\\Program Files (x86)\\\\Steam\\\\steam.exe",\n        "role": "direct",\n        "running": true,\n        "matched": 3\n      }\n    ]\n  }\n}',
      },
      {
        method: 'POST',
        path: '/client/api/routing',
        summary: 'Replace the whole rule set and the default in one write. A full replace rather than per-rule edits: the set is a handful of rows, and replacing it cannot half-apply or leave two rules fighting over one process. Ids are assigned here, not sent; a rule that survives the write keeps its id and its match counter, so changing a role does not reset what it has claimed. Duplicates by full path — or by name when there is no path — collapse to one. Rules take effect on NEW flows: an established connection finishes on the route it started on, because moving it would change the address its peer sees.',
        params: [
          { name: 'defaultRole', in: 'body (json)', type: 'string', desc: 'direct | tunnel | egress | noEgress — what a process with no rule does, and what carries traffic the system sends on its own behalf, which has no PID to attribute.' },
          { name: 'rules', in: 'body (json)', type: 'object[]', desc: 'The complete set. Each entry: process (file name), path (optional, the exact binary), role.' },
        ],
        body: '{\n  "defaultRole": "tunnel",\n  "rules": [\n    {\n      "process": "steam.exe",\n      "path": "C:\\\\Program Files (x86)\\\\Steam\\\\steam.exe",\n      "role": "direct"\n    },\n    { "process": "bank-client.exe", "role": "direct" }\n  ]\n}',
        errorResponse: '{\n  "success": false,\n  "msg": "Unknown role bypass for steam.exe"\n}',
      },
      {
        method: 'GET',
        path: '/client/api/routing/processes',
        summary: 'What is running with an open connection right now, for the rule picker. Read out of the same connection table the datapath uses for attribution, so it costs nothing extra. `connections` is how many flows the process holds — the page sorts by it, because someone opening this list came to route something that is moving traffic, not to read an alphabet. A process that is not running can still be given a rule by typing its name; that rule matches by name alone, wherever the binary was started from.',
        response: '{\n  "success": true,\n  "obj": [\n    {\n      "name": "chrome.exe",\n      "path": "C:\\\\Program Files\\\\Google\\\\Chrome\\\\Application\\\\chrome.exe",\n      "pid": 4120,\n      "connections": 41\n    }\n  ]\n}',
      },
      {
        method: 'GET',
        path: '/client/api/settings',
        summary: 'Everything the client remembers about how it should behave: how often to refresh the subscription, and what to do at startup. Appearance is stored here too — the page is served by this process, so the look belongs to the client and not to the panel a plain user never opens.',
        response: '{\n  "success": true,\n  "obj": {\n    "refreshMinutes": 60,\n    "autostart": true,\n    "autostartBehaviour": "connect",\n    "manualBehaviour": "open"\n  }\n}',
      },
      {
        method: 'POST',
        path: '/client/api/settings',
        summary: 'Persist the client settings. The page edits a draft and sends the whole set on Save, so a half-typed number never reaches the daemon; every field is optional and an absent one is left as it was. None of this needs the tunnel to come down. The stored values are echoed back — a clamped one comes back clamped, and the form redraws from the answer rather than from what it sent.',
        params: [
          { name: 'refreshMinutes', in: 'body (json)', type: 'number', desc: 'Subscription refresh interval, 1 … 1440.' },
          { name: 'autostart', in: 'body (json)', type: 'boolean', desc: 'Register the client to start with the system.' },
          { name: 'autostartBehaviour', in: 'body (json)', type: 'string', desc: 'tray | open | connect | openConnect — what an automatic start does.' },
          { name: 'manualBehaviour', in: 'body (json)', type: 'string', desc: 'tray | open | openConnect — what launching it by hand does.' },
        ],
        body: '{\n  "refreshMinutes": 60,\n  "autostart": true,\n  "autostartBehaviour": "connect",\n  "manualBehaviour": "open"\n}',
      },
      {
        method: 'GET',
        path: '/client/api/about',
        summary: 'What this client is, as the network sees it: when the record was created, how much it has moved, when the subscription runs out and where the traffic went. The site list is read out of the local resolver and never leaves the machine — it is here so the person can see it, and clearing it is one of the reset options.',
        response: '{\n  "success": true,\n  "obj": {\n    "tag": "vasya",\n    "createdAt": 1735689600000,\n    "up": 2147483648,\n    "down": 31138512896,\n    "expiresAt": 0,\n    "topSites": [\n      { "host": "youtube.com", "hits": 4210 },\n      { "host": "github.com", "hits": 890 }\n    ]\n  }\n}',
      },
      {
        method: 'POST',
        path: '/client/api/reset',
        summary: 'Forget local state — preferences, routing rules and the site list the local resolver keeps. With subscription=true the imported link goes too and the next start opens the import screen again. The tunnel is not torn down by the first form; the second has nothing left to dial with.',
        params: [
          { name: 'subscription', in: 'body (json)', type: 'boolean', desc: 'Also drop the imported URI.', optional: true },
        ],
        body: '{\n  "subscription": false\n}',
      },
      {
        method: 'GET',
        path: '/client/api/setup',
        summary: 'Windows only. Whether this copy of the client runs from Program Files. A copy that runs from anywhere else shows the install screen instead of the client.',
        response: '{\n  "success": true,\n  "obj": {\n    "installed": false,\n    "offered": true,\n    "path": "C:\\\\Program Files\\\\qd",\n    "autostart": true\n  }\n}',
      },
      {
        method: 'POST',
        path: '/client/api/setup/install',
        summary: 'Windows only. Copy the running client into Program Files, add it to the Start menu and to the list of installed apps, then restart from the installed copy; a tunnel that was up comes back. Removal is done from Windows settings and takes the subscription and settings with it.',
        params: [
          { name: 'autostart', in: 'body (json)', type: 'boolean', desc: 'Start with Windows.' },
          { name: 'desktop', in: 'body (json)', type: 'boolean', desc: 'Put a shortcut on the desktop.' },
        ],
        body: '{\n  "autostart": true,\n  "desktop": true\n}',
      },
    ],
  },

  {
    id: 'entrypoints',
    title: 'Entrypoints',
    description:
      'An entrypoint is the port of an ingress node that clients dial. Groups hold entrypoints; a client reaches the entrypoints of its group.',
    endpoints: [
      {
        method: 'GET',
        path: '/panel/api/entrypoints',
        summary: 'Entrypoints of ingress nodes, labelled with the node tag.',
        response: '{\n  "success": true,\n  "obj": [\n    { "id": 1, "nodeId": 1, "tag": "Loki", "port": 443, "enable": true }\n  ]\n}',
      },
    ],
  },

  {
    id: 'server',
    title: 'Server',
    endpoints: [
      {
        method: 'GET',
        path: '/panel/api/server/status',
        summary: 'Machine status of the first reachable node, with the count of nodes and of nodes online.',
      },
      {
        method: 'GET',
        path: '/panel/api/server/getDb',
        summary: 'Download the network database from a reachable node.',
      },
      {
        method: 'POST',
        path: '/panel/api/server/importDB',
        summary: 'Replace the network database on every node with an uploaded one.',
        params: [
          { name: 'db', in: 'body (multipart)', type: 'file', desc: 'A database saved by /server/getDb.' },
        ],
      },
      {
        method: 'POST',
        path: '/panel/api/server/sync',
        summary: 'Copy the network database onto one node.',
        params: [
          { name: 'nodeId', in: 'body (json)', type: 'integer', desc: 'Node ID.' },
        ],
      },
      {
        method: 'GET',
        path: '/panel/api/network/key',
        summary: 'The network key and the administrator this panel runs as. Feeds the deploy command of a new node.',
        response: '{\n  "success": true,\n  "obj": { "key": "7f3a…", "adminUuid": "…", "adminTag": "admin" }\n}',
      },
      {
        method: 'GET',
        path: '/panel/api/releases',
        summary: 'Client releases that can be offered to clients.',
      },
    ],
  },

  {
    id: 'clients',
    title: 'Clients',
    description:
      'A client is a tag and a key. The entrypoints it reaches come from its group. Writes go to every node at once; traffic, devices and addresses are gathered from the nodes.',
    endpoints: [
      {
        method: 'GET',
        path: '/panel/api/clients/list',
        summary: 'Every client, unpaginated.',
        response:
          '{\n  "success": true,\n  "obj": [\n    {\n      "id": 1,\n      "tag": "alice",\n      "uuid": "…",\n      "group": "family",\n      "enable": true,\n      "expiryAt": 1735689600000,\n      "deviceLimit": 0,\n      "allowExit": 0,\n      "admin": false,\n      "comment": "",\n      "entrypointIds": [3, 5],\n      "uri": "qd://…",\n      "traffic": { "up": 1024, "down": 4096, "lastOnline": 1735600000000 },\n      "devices": [],\n      "addresses": [],\n      "exits": []\n    }\n  ]\n}',
      },
      {
        method: 'GET',
        path: '/panel/api/clients/list/paged',
        summary: 'Search, filter, sort and paginate clients. The summary is counted over all clients, not the page.',
        params: [
          { name: 'page', in: 'query', type: 'integer', desc: '1-indexed page number.' },
          { name: 'pageSize', in: 'query', type: 'integer', desc: 'Rows per page, 50 by default.' },
          { name: 'search', in: 'query', type: 'string', desc: 'Substring of the tag or the comment.' },
          { name: 'group', in: 'query', type: 'string', desc: 'Group tag.' },
          { name: 'status', in: 'query', type: 'string', desc: 'online | active | expiring | expired | disabled.' },
          { name: 'sort', in: 'query', type: 'string', desc: 'createdAt | updatedAt | lastOnline | tag | traffic | expiryAt.' },
          { name: 'order', in: 'query', type: 'string', desc: 'ascend | descend.' },
        ],
        response:
          '{\n  "success": true,\n  "obj": {\n    "items": [],\n    "total": 120,\n    "filtered": 7,\n    "page": 1,\n    "pageSize": 50,\n    "groups": ["family"],\n    "summary": {\n      "total": 120,\n      "active": 110,\n      "online": ["alice"],\n      "expired": [],\n      "expiring": [],\n      "disabled": []\n    }\n  }\n}',
      },
      {
        method: 'POST',
        path: '/panel/api/clients/add',
        summary: 'Create a client. A key is generated when uuid is empty.',
        body: '{\n  "tag": "alice",\n  "uuid": "…",\n  "group": "family",\n  "enable": true,\n  "expiryAt": 0,\n  "deviceLimit": 0,\n  "allowExit": 0,\n  "admin": false,\n  "comment": ""\n}',
      },
      {
        method: 'POST',
        path: '/panel/api/clients/update/:id',
        summary: 'Change a client. Only the fields sent are changed.',
        params: [
          { name: 'id', in: 'path', type: 'integer', desc: 'Client ID.' },
        ],
        body: '{\n  "enable": false\n}',
      },
      {
        method: 'POST',
        path: '/panel/api/clients/del/:id',
        summary: 'Delete a client.',
        params: [
          { name: 'id', in: 'path', type: 'integer', desc: 'Client ID.' },
        ],
      },
      {
        method: 'POST',
        path: '/panel/api/clients/resetTraffic/:id',
        summary: 'Zero the traffic counters of a client on every node.',
        params: [
          { name: 'id', in: 'path', type: 'integer', desc: 'Client ID.' },
        ],
      },
      {
        method: 'GET',
        path: '/panel/api/clients/onlines',
        summary: 'Tags of the clients connected now.',
      },
      {
        method: 'GET',
        path: '/panel/api/clients/lastOnline',
        summary: 'When each client was last seen, by tag.',
      },
      {
        method: 'POST',
        path: '/panel/api/clients/devices/block',
        summary: 'Block or unblock a device of a client.',
        body: '{\n  "clientId": 1,\n  "fingerprint": "…",\n  "blocked": true\n}',
      },
      {
        method: 'POST',
        path: '/panel/api/clients/devices/forget',
        summary: 'Forget a device of a client.',
        body: '{\n  "clientId": 1,\n  "fingerprint": "…"\n}',
      },
      {
        method: 'POST',
        path: '/panel/api/clients/addresses/forget',
        summary: 'Forget one address of a client, or all of them when ip is empty.',
        body: '{\n  "clientId": 1,\n  "ip": ""\n}',
      },
      {
        method: 'POST',
        path: '/panel/api/clients/exits/forget',
        summary: 'Forget an exit node a client has used.',
        body: '{\n  "clientId": 1,\n  "nodeId": 4\n}',
      },
      {
        method: 'GET',
        path: '/panel/api/clients/groups',
        summary: 'Every group with its entrypoints, limits and client count.',
        response: '{\n  "success": true,\n  "obj": [\n    { "id": 1, "tag": "family", "clientCount": 4, "entrypointIds": [1], "deviceLimit": 0, "allowExit": true, "allowDev": false, "allowCore": false }\n  ]\n}',
      },
      {
        method: 'POST',
        path: '/panel/api/clients/groups/save',
        summary: 'Create a group, or change one when id is given.',
        body: '{\n  "id": 1,\n  "tag": "family",\n  "entrypointIds": [1],\n  "deviceLimit": 0,\n  "allowExit": true,\n  "allowDev": false,\n  "allowCore": false\n}',
      },
      {
        method: 'POST',
        path: '/panel/api/clients/groups/members',
        summary: 'Move clients into a group, or out of every group with group 0.',
        body: '{\n  "group": 1,\n  "clients": [3, 4]\n}',
      },
      {
        method: 'POST',
        path: '/panel/api/clients/groups/delete',
        summary: 'Delete a group. Its clients are left without a group.',
        body: '{\n  "id": 1\n}',
      },
    ],
  },

  {
    id: 'nodes',
    title: 'Nodes',
    endpoints: [
      {
        method: 'GET',
        path: '/panel/api/nodes/list',
        summary: 'Every node with its health, revision and client counts.',
        response: '{\n  "success": true,\n  "obj": [\n    { "id": 1, "tag": "Loki", "address": "node-1.example.com", "port": 443, "role": "ingress", "enable": true, "status": "online", "latencyMs": 8, "version": "0.3.7-alpha", "revision": 42, "appliedRevision": 42, "clientCount": 12, "onlineCount": 3 }\n  ]\n}',
      },
      {
        method: 'GET',
        path: '/panel/api/nodes/names',
        summary: 'Tags a new node may take, by role.',
      },
      {
        method: 'POST',
        path: '/panel/api/nodes/add',
        summary: 'Record a node. The deploy command from the Add dialog brings the machine up.',
        body: '{\n  "id": 2,\n  "tag": "Zeus",\n  "role": "egress",\n  "address": "node-2.example.com",\n  "port": 443,\n  "uuid": "…",\n  "natSlot": 1,\n  "enable": true\n}',
      },
      {
        method: 'POST',
        path: '/panel/api/nodes/update/:id',
        summary: 'Change a node.',
        params: [
          { name: 'id', in: 'path', type: 'integer', desc: 'Node ID.' },
        ],
      },
      {
        method: 'POST',
        path: '/panel/api/nodes/setEnable/:id',
        summary: 'Take a node in or out of the network.',
        params: [
          { name: 'id', in: 'path', type: 'integer', desc: 'Node ID.' },
        ],
        body: '{\n  "enable": false\n}',
      },
      {
        method: 'POST',
        path: '/panel/api/nodes/del/:id',
        summary: 'Switch a node off and forget it.',
        params: [
          { name: 'id', in: 'path', type: 'integer', desc: 'Node ID.' },
        ],
      },
      {
        method: 'POST',
        path: '/panel/api/nodes/probe/:id',
        summary: 'Ask a node for its version and measure the round trip.',
        params: [
          { name: 'id', in: 'path', type: 'integer', desc: 'Node ID.' },
        ],
        response: '{\n  "success": true,\n  "obj": { "status": "online", "latencyMs": 8, "version": "0.3.7-alpha" }\n}',
      },
      {
        method: 'POST',
        path: '/panel/api/nodes/restartAll',
        summary: 'Restart every reachable node.',
      },
      {
        method: 'GET',
        path: '/panel/api/nodes/:id/history/:metric/:bucket',
        summary: 'Metric history recorded by the node.',
        params: [
          { name: 'id', in: 'path', type: 'integer', desc: 'Node ID.' },
          { name: 'metric', in: 'path', type: 'string', desc: 'Metric key.' },
          { name: 'bucket', in: 'path', type: 'integer', desc: 'Bucket size in seconds.' },
          { name: 'window', in: 'query', type: 'integer', desc: 'Window in seconds.' },
        ],
      },
      {
        method: 'GET',
        path: '/panel/api/nodes/:id/history/export',
        summary: 'Raw history samples of the node.',
        params: [
          { name: 'id', in: 'path', type: 'integer', desc: 'Node ID.' },
        ],
      },
      {
        method: 'POST',
        path: '/panel/api/nodes/:id/logs/:count',
        summary: 'The last lines of the node log.',
        params: [
          { name: 'id', in: 'path', type: 'integer', desc: 'Node ID.' },
          { name: 'count', in: 'path', type: 'integer', desc: 'How many lines.' },
          { name: 'level', in: 'body (json)', type: 'string', desc: 'Lowest level to return.' },
        ],
      },
      {
        method: 'POST',
        path: '/panel/api/nodes/:id/logs/clear',
        summary: 'Clear the node log.',
        params: [
          { name: 'id', in: 'path', type: 'integer', desc: 'Node ID.' },
        ],
      },
      {
        method: 'GET',
        path: '/panel/api/nodes/:id/logs/download',
        summary: 'The whole node log as one text.',
        params: [
          { name: 'id', in: 'path', type: 'integer', desc: 'Node ID.' },
        ],
      },
    ],
  },

  {
    id: 'dns',
    title: 'DNS',
    endpoints: [
      { method: 'GET', path: '/panel/api/dns/records', summary: 'Local DNS records of the network.' },
      { method: 'POST', path: '/panel/api/dns/records/save', summary: 'Create or change a local DNS record.' },
      { method: 'POST', path: '/panel/api/dns/records/del', summary: 'Delete a local DNS record.', body: '{\n  "id": 1\n}' },
      { method: 'GET', path: '/panel/api/dns/stats', summary: 'Resolver counters of every node.' },
      { method: 'POST', path: '/panel/api/dns/flush', summary: 'Drop the resolver cache on every node.' },
      { method: 'GET', path: '/panel/api/dns/lists', summary: 'Blocklists and when each node last fetched them.' },
      { method: 'POST', path: '/panel/api/dns/lists/refresh', summary: 'Fetch the blocklists again on every node.' },
    ],
  },

  {
    id: 'settings',
    title: 'Settings',
    description:
      'Page size and the expiry warning stay with this panel; every other setting is network state and is written to the nodes.',
    endpoints: [
      {
        method: 'POST',
        path: '/panel/setting/all',
        summary: 'Every setting.',
      },
      {
        method: 'POST',
        path: '/panel/setting/update',
        summary: 'Change settings. Only the fields sent are changed.',
        body: '{\n  "expireDiff": 3,\n  "mtu": 1500\n}',
      },
      {
        method: 'POST',
        path: '/panel/setting/theme',
        summary: 'Save the panel theme.',
      },
    ],
  },

  {
    id: 'websocket',
    title: 'WebSocket',
    endpoints: [
      {
        method: 'GET',
        path: '/ws',
        summary: 'Live pushes: nodes, traffic (onlineClients) and client_stats.',
      },
    ],
  },
];
