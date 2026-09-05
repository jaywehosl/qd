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
        response: '{\n  "success": true,\n  "obj": {\n    "changed": true,\n    "nodes": 3,\n    "reconnected": false\n  }\n}',
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
    ],
  },

  {
    id: 'inbounds',
    title: 'Entrypoints',
    description:
      'An entrypoint is a UDP port on one node that clients dial. It carries no protocol configuration — the datapath is the same everywhere — only where it listens, whether it is open, and how fast it is allowed to run. Which clients may use it is decided elsewhere: put the entrypoint in a group, give the group to a client. Every write here edits the draft; nothing reaches a node until the draft is published.',
    endpoints: [
      {
        method: 'GET',
        path: '/panel/api/inbounds/list',
        summary: 'Every entrypoint with its node, port and traffic counters. The counters are telemetry collected from the nodes, not something the panel maintains.',
        response:
          '{\n  "success": true,\n  "obj": [\n    {\n      "id": 1,\n      "nodeId": 1,\n      "tag": "node-1-443",\n      "remark": "main 443",\n      "port": 443,\n      "listen": "0.0.0.0",\n      "protocol": "qd",\n      "enable": true,\n      "up": 41884672000,\n      "down": 318774112000,\n      "clientCount": 3\n    }\n  ]\n}',
      },
      {
        method: 'GET',
        path: '/panel/api/inbounds/options',
        summary: 'Picker projection: id, node, tag, port and the enable flag, without counters or certificate state. Feeds the entrypoint chooser in the group editor.',
        response:
          '{\n  "success": true,\n  "obj": [\n    {\n      "id": 1,\n      "nodeId": 1,\n      "tag": "node-1-443",\n      "remark": "main 443",\n      "port": 443,\n      "protocol": "qd",\n      "enable": true\n    }\n  ]\n}',
      },
      {
        method: 'GET',
        path: '/panel/api/inbounds/get/:id',
        summary: 'One entrypoint by id.',
        params: [
          { name: 'id', in: 'path', type: 'number', desc: 'Entrypoint ID.' },
        ],
      },
      {
        method: 'POST',
        path: '/panel/api/inbounds/add',
        summary: 'Open a new entrypoint on a node. The port must be free on that node — the check is per node, not global, since two nodes may both listen on 443. Draft edit.',
        params: [
          { name: 'nodeId', in: 'body (json)', type: 'number', desc: 'Node the port is opened on.' },
          { name: 'port', in: 'body (json)', type: 'number', desc: 'UDP port clients dial.' },
          { name: 'remark', in: 'body (json)', type: 'string', desc: 'Human-readable name.' },
          { name: 'enable', in: 'body (json)', type: 'boolean', desc: 'Whether the port accepts connections.' },
        ],
        body: '{\n  "nodeId": 1,\n  "port": 8443,\n  "remark": "spare 8443",\n  "enable": true\n}',
        errorResponse:
          '{\n  "success": false,\n  "msg": "Port 8443 is already used on this node"\n}',
      },
      {
        method: 'POST',
        path: '/panel/api/inbounds/update/:id',
        summary: 'Change an entrypoint: port, name and enable flag. Moving the port rewrites the connection URI of every client whose group holds this entrypoint — they need the new link once the draft is published.',
        params: [
          { name: 'id', in: 'path', type: 'number', desc: 'Entrypoint ID.' },
        ],
        body: '{\n  "port": 8443,\n  "remark": "spare 8443",\n  "enable": true\n}',
      },
      {
        method: 'POST',
        path: '/panel/api/inbounds/setEnable/:id',
        summary: 'Flip only the enable flag. A disabled entrypoint stops accepting connections but stays in its groups, so re-enabling it does not require touching any client.',
        params: [
          { name: 'id', in: 'path', type: 'number', desc: 'Entrypoint ID.' },
        ],
        body: '{\n  "enable": false\n}',
      },
      {
        method: 'POST',
        path: '/panel/api/inbounds/del/:id',
        summary: 'Close an entrypoint and drop it from every group holding it. Clients whose group is left with no entrypoints keep their records but lose their route — check the group before deleting. Draft edit.',
        params: [
          { name: 'id', in: 'path', type: 'number', desc: 'Entrypoint ID.' },
        ],
      },
      {
        method: 'POST',
        path: '/panel/api/inbounds/:id/resetTraffic',
        summary: 'Zero the accumulated traffic of one entrypoint. Panel-side baseline shift, like the client counterpart — the node keeps counting and is not told.',
        params: [
          { name: 'id', in: 'path', type: 'number', desc: 'Entrypoint ID.' },
        ],
      },
      {
        method: 'POST',
        path: '/panel/api/inbounds/resetAllTraffics',
        summary: 'Zero the accumulated traffic of every entrypoint. Collected history is discarded and cannot be recovered from the nodes — their own counters restarted at their last datapath start, not at yours.',
      },
    ],
  },

  {
    id: 'server',
    title: 'Server',
    description:
      'State of the machine the panel itself runs on. Node health lives under /panel/api/nodes.',
    endpoints: [
      {
        method: 'GET',
        path: '/panel/api/server/status',
        summary: 'Snapshot of the machine the panel runs on — CPU, memory, swap, disk, network IO, load, open sockets. This is the operator\'s own workstation, not a node: node health comes from /nodes/list and /nodes/{id}/history. Cached briefly and refreshed on a short interval.',
        response: '{\n  "success": true,\n  "obj": {\n    "cpu": 12.5,\n    "mem": { "current": 2147483648, "total": 8589934592 },\n    "swap": { "current": 0, "total": 4294967296 },\n    "disk": { "current": 53687091200, "total": 268435456000 },\n    "netIO": { "up": 1073741824, "down": 2147483648 },\n    "tcpCount": 42,\n    "udpCount": 8,\n    "uptime": 486000,\n    "load": { "load1": 0.5, "load5": 0.3, "load15": 0.2 }\n  }\n}',
      },
    ],
  },

  {
    id: 'clients',
    title: 'Clients',
    description:
      'A client is an identity — a tag people know it by and a key the datapath checks. Which entrypoints it can reach comes from the group it carries, not from the client record. Everything under /panel/api/clients that writes, edits the draft; the nodes keep serving the previous revision until the draft is published. Traffic, devices and IP log on a client are collected from the nodes and flow the other way — they are never published back.',
    endpoints: [
      {
        method: 'GET',
        path: '/panel/api/clients/list',
        summary: 'Every client in the draft, unpaginated. Used where the whole set has to be shown at once — picking group members, for instance. The clients page uses /list/paged instead.',
        response:
          '{\n  "success": true,\n  "obj": [\n    {\n      "id": 1,\n      "email": "alice@example.com",\n      "subId": "abcd1234",\n      "uuid": "...",\n      "totalGB": 53687091200,\n      "expiryTime": 1735689600000,\n      "enable": true,\n      "reverse": null,\n      "inboundIds": [3, 5],\n      "traffic": { "up": 1024, "down": 4096, "enable": true }\n    }\n  ]\n}',
      },
      {
        method: 'GET',
        path: '/panel/api/clients/list/paged',
        summary: 'Filter, sort and paginate clients server-side. Search matches the tag and the comment only. Sortable columns: createdAt, updatedAt, lastOnline, email (tag), enable, group, traffic, expiryTime. Rows are slim — fetch /get/:email for the full record. The summary counters are computed over the whole table, not the page, so they hold still while paging.',
        params: [
          { name: 'page', in: 'query', type: 'number', desc: '1-indexed page number. Defaults to 1.' },
          { name: 'pageSize', in: 'query', type: 'number', desc: 'Rows per page. Defaults to 25, capped at 200.' },
          { name: 'search', in: 'query', type: 'string', desc: 'Case-insensitive substring match on the tag and the comment. Keys are never searched.' },
          { name: 'filter', in: 'query', type: 'string', desc: 'Status bucket: online | active | deactive | expiring.' },
          { name: 'sort', in: 'query', type: 'string', desc: 'createdAt | updatedAt | lastOnline | email | enable | group | traffic | expiryTime.' },
          { name: 'order', in: 'query', type: 'string', desc: 'ascend or descend.' },
        ],
        response:
          '{\n  "success": true,\n  "obj": {\n    "items": [\n      {\n        "email": "alice@example.com",\n        "subId": "abcd1234",\n        "enable": true,\n        "totalGB": 53687091200,\n        "expiryTime": 1735689600000,\n        "limitIp": 0,\n        "reset": 0,\n        "inboundIds": [3, 5],\n        "traffic": { "up": 1024, "down": 4096, "enable": true },\n        "createdAt": 1735000000000,\n        "updatedAt": 1735100000000\n      }\n    ],\n    "total": 2000,\n    "filtered": 47,\n    "page": 1,\n    "pageSize": 25,\n    "summary": {\n      "total": 2000,\n      "active": 1850,\n      "online": ["alice@example.com"],\n      "depleted": [],\n      "expiring": [],\n      "deactive": []\n    }\n  }\n}',
      },
      {
        method: 'GET',
        path: '/panel/api/clients/get/:email',
        summary: 'One client with everything the edit and info dialogs need: key, group, expiry, comment, plus the devices and IP log collected from the nodes.',
        params: [
          { name: 'email', in: 'path', type: 'string', desc: 'Client email (unique identifier).' },
        ],
        response:
          '{\n  "success": true,\n  "obj": {\n    "client": { "id": 1, "email": "alice@example.com", ... },\n    "inboundIds": [3, 5]\n  }\n}',
      },
      {
        method: 'POST',
        path: '/panel/api/clients/add',
        summary: 'Create a client. The key (uuid) is generated when omitted. This writes to the draft — the client cannot connect anywhere until the draft is published to the nodes.',
        params: [
          { name: 'client', in: 'body (json)', type: 'object', desc: 'Client fields: email, subId, id (uuid), password, auth, flow, totalGB, expiryTime, limitIp, tgId (numeric Telegram user ID, 0 = none), comment, enable.' },
          { name: 'inboundIds', in: 'body (json)', type: 'integer[]', desc: 'Inbound IDs to attach the client to. At least one required.' },
        ],
        body: '{\n  "client": {\n    "email": "alice@example.com",\n    "totalGB": 53687091200,\n    "expiryTime": 1735689600000,\n    "tgId": 0,\n    "limitIp": 0,\n    "enable": true\n  },\n  "inboundIds": [3, 5]\n}',
        response: '{\n  "success": true,\n  "msg": "Client added"\n}',
      },
      {
        method: 'POST',
        path: '/panel/api/clients/update/:email',
        summary: 'Replace a client record: tag, key, expiry, comment, group. The server replaces the row rather than patching it, so send the full set you want to keep. A draft edit — nodes keep serving the old record until the next publish. Changing the key revokes the old one at that moment, not before.',
        params: [
          { name: 'email', in: 'path', type: 'string', desc: 'Current client email (unique identifier).' },
        ],
        body: '{\n  "email": "alice@example.com",\n  "totalGB": 107374182400,\n  "expiryTime": 1767225600000,\n  "tgId": 123456789,\n  "enable": true\n}',
        response: '{\n  "success": true,\n  "msg": "Client updated"\n}',
      },
      {
        method: 'POST',
        path: '/panel/api/clients/del/:email',
        summary: 'Remove a client from the draft. Its traffic history, devices and IP log are dropped with it unless keepTraffic=1 — those are collected records, not configuration, and outlive the client only if asked. The client keeps working on the nodes until the draft is published.',
        params: [
          { name: 'email', in: 'path', type: 'string', desc: 'Client email (unique identifier).' },
          { name: 'keepTraffic', in: 'query', type: 'integer', desc: 'Pass 1 to keep the collected traffic, devices and IP log after the client is gone.' },
        ],
        response: '{\n  "success": true,\n  "msg": "Client deleted"\n}',
      },
      {
        method: 'POST',
        path: '/panel/api/clients/:email/attach',
        summary: 'Attach a client to entrypoints directly, bypassing its group. The normal path is to put the entrypoint in a group and give the client that group; this exists for the one-off case. Draft edit.',
        params: [
          { name: 'email', in: 'path', type: 'string', desc: 'Client email (unique identifier).' },
          { name: 'inboundIds', in: 'body (json)', type: 'integer[]', desc: 'Inbound IDs to attach.' },
        ],
        body: '{\n  "inboundIds": [7, 9]\n}',
        response: '{\n  "success": true\n}',
      },
      {
        method: 'POST',
        path: '/panel/api/clients/:email/detach',
        summary: 'Remove entrypoints attached directly to this client. Entrypoints the client reaches through its group are unaffected — drop the group instead. Draft edit.',
        params: [
          { name: 'email', in: 'path', type: 'string', desc: 'Client email (unique identifier).' },
          { name: 'inboundIds', in: 'body (json)', type: 'integer[]', desc: 'Inbound IDs to detach.' },
        ],
        body: '{\n  "inboundIds": [5]\n}',
        response: '{\n  "success": true\n}',
      },
      {
        method: 'POST',
        path: '/panel/api/clients/resetAllTraffics',
        summary: 'Zero the accumulated traffic of every client. Panel-side bookkeeping only: the nodes keep counting and are told nothing, the panel simply moves its baseline. Nothing is published and no client is interrupted.',
        response: '{\n  "success": true\n}',
      },
      {
        method: 'POST',
        path: '/panel/api/clients/bulkDel',
        summary: 'Delete many clients in one draft edit, processed in order so each delete sees the previous one committed. keepTraffic=true retains their collected traffic, devices and IP log.',
        body: '{\n  "emails": ["alice", "bob"],\n  "keepTraffic": false\n}',
        response: '{\n  "success": true,\n  "obj": {\n    "deleted": 2,\n    "skipped": [\n      { "email": "carol", "reason": "client not found" }\n    ]\n  }\n}',
      },
      {
        method: 'POST',
        path: '/panel/api/clients/groups/bulkAdd',
        summary: 'Put many clients into a group at once, replacing whatever group they carried. A group name that does not exist yet is created. To take clients out of a group use /groups/bulkRemove. Draft edit.',
        body: '{\n  "emails": ["alice", "bob"],\n  "group": "customer-a"\n}',
        response: '{\n  "success": true,\n  "obj": {\n    "affected": 2\n  }\n}',
      },
      {
        method: 'POST',
        path: '/panel/api/clients/groups/bulkRemove',
        summary: 'Take many clients out of their group. The clients stay; only the group is cleared, which leaves them with whatever entrypoints are attached to them directly — possibly none. Draft edit.',
        body: '{\n  "emails": ["alice", "bob"]\n}',
        response: '{\n  "success": true,\n  "obj": {\n    "affected": 2\n  }\n}',
      },
      {
        method: 'POST',
        path: '/panel/api/clients/bulkAttach',
        summary: 'Attach many clients to many entrypoints in one call. Clients already attached to a target are reported as skipped rather than failing the batch. Draft edit.',
        params: [
          { name: 'emails', in: 'body (json)', type: 'array', desc: 'Emails of existing clients to attach.' },
          { name: 'inboundIds', in: 'body (json)', type: 'integer[]', desc: 'Target inbound IDs to attach every client to.' },
        ],
        body: '{\n  "emails": ["alice", "bob"],\n  "inboundIds": [7, 9]\n}',
        response: '{\n  "success": true,\n  "obj": {\n    "attached": ["alice", "bob"],\n    "skipped": ["bob"],\n    "errors": []\n  }\n}',
      },
      {
        method: 'POST',
        path: '/panel/api/clients/bulkDetach',
        summary: 'Detach many clients from many entrypoints in one call. Only direct attachments are touched; pairs that were never attached are silently skipped. Reaching an entrypoint through a group is unaffected. Draft edit.',
        params: [
          { name: 'emails', in: 'body (json)', type: 'array', desc: 'Emails of existing clients to detach.' },
          { name: 'inboundIds', in: 'body (json)', type: 'integer[]', desc: 'Inbound IDs to detach the clients from.' },
        ],
        body: '{\n  "emails": ["alice", "bob"],\n  "inboundIds": [7, 9]\n}',
        response: '{\n  "success": true,\n  "obj": {\n    "detached": ["alice", "bob"],\n    "skipped": [],\n    "errors": []\n  }\n}',
      },
      {
        method: 'GET',
        path: '/panel/api/clients/groups',
        summary: 'Every group with its entrypoint set and member count. A group holds entrypoints, not clients: assigning it to a client is what grants that client those entrypoints and what fills its connection URI.',
        response: '{\n  "success": true,\n  "obj": [\n    { "name": "customer-a", "clientCount": 5 },\n    { "name": "internal", "clientCount": 0 }\n  ]\n}',
      },
      {
        method: 'POST',
        path: '/panel/api/clients/groups/create',
        summary: 'Create an empty group. It becomes selectable immediately, but a group without entrypoints grants nothing — add entrypoints with /groups/entrypoints.',
        body: '{\n  "name": "customer-a"\n}',
        response: '{\n  "success": true,\n  "obj": {\n    "name": "customer-a"\n  }\n}',
      },
      {
        method: 'POST',
        path: '/panel/api/clients/groups/rename',
        summary: 'Rename a group and carry the new name to every client holding it, in one transaction. Returns how many clients were relabelled. Draft edit — the connection URIs handed out afterwards carry the new name.',
        body: '{\n  "oldName": "customer-a",\n  "newName": "tier-1"\n}',
        response: '{\n  "success": true,\n  "obj": {\n    "affected": 5\n  }\n}',
      },
      {
        method: 'POST',
        path: '/panel/api/clients/groups/entrypoints',
        summary: 'Replace the entrypoint set of a group. These become the only entrypoints its clients can reach and the only ones encoded into their connection URI. An empty set leaves the group granting nothing. Draft edit.',
        params: [
          { name: 'name', in: 'body (json)', type: 'string', desc: 'Group name.' },
          { name: 'entrypointIds', in: 'body (json)', type: 'integer[]', desc: 'Entrypoint IDs that make up the group.' },
          { name: 'deviceLimit', in: 'body (json)', type: 'integer', desc: 'How many devices each client in this group may use. 0 leaves them unlimited; a limit set on the client itself overrides this one.' },
          { name: 'allowExit', in: 'body (json)', type: 'boolean', desc: 'Whether clients in this group may route through an exit node. Off by default; a client may override it either way.' },
        ],
        body: '{\n  "name": "russia-in",\n  "entrypointIds": [1, 2]\n}',
        response: '{\n  "success": true,\n  "obj": {\n    "affected": 2\n  }\n}',
      },
      {
        method: 'POST',
        path: '/panel/api/clients/groups/delete',
        summary: 'Remove a group and clear it from every client that carried it. The clients themselves are kept — filter by group and use /bulkDel to remove them. Returns how many were cleared.',
        body: '{\n  "name": "customer-a"\n}',
        response: '{\n  "success": true,\n  "obj": {\n    "affected": 5\n  }\n}',
      },
      {
        method: 'POST',
        path: '/panel/api/clients/resetTraffic/:email',
        summary: 'Zero one client\'s accumulated traffic. Same panel-side baseline shift as resetAllTraffics — the node\'s own counters are untouched.',
        params: [
          { name: 'email', in: 'path', type: 'string', desc: 'Client email.' },
        ],
      },
      {
        method: 'POST',
        path: '/panel/api/clients/onlines',
        summary: 'Tags of the clients currently connected, deduplicated across every node the panel can reach. Live state read from the nodes, not draft configuration.',
        response: '{\n  "success": true,\n  "obj": ["user1", "user2"]\n}',
      },
      {
        method: 'POST',
        path: '/panel/api/clients/onlinesByNode',
        summary: 'The same online tags, grouped by the node that reported them, so a client connected through two nodes is visible on both. Nodes the panel has no channel to are absent rather than empty.',
        response: '{\n  "success": true,\n  "obj": {\n    "0": ["user1"],\n    "3": ["user1", "user2"]\n  }\n}',
      },
      {
        method: 'POST',
        path: '/panel/api/clients/activeInbounds',
        summary: 'Entrypoint tags that carried traffic within the heartbeat window, grouped by node. Pairs with onlinesByNode so a client holding several entrypoints is only marked active on the ones it actually used.',
        response: '{\n  "success": true,\n  "obj": {\n    "0": ["inbound-443", "inbound-8443"]\n  }\n}',
      },
      {
        method: 'POST',
        path: '/panel/api/clients/lastOnline',
        summary: 'Client tag to the unix timestamp of its last connection. This is the last time the client established a tunnel, not the last time it fetched anything from the panel.',
        response: '{\n  "success": true,\n  "obj": {\n    "user1": 1700000000,\n    "user2": 1699999000\n  }\n}',
      },
    ],
  },

  {
    id: 'nodes',
    title: 'Nodes',
    description:
      'A node is a machine running the XDP datapath. The panel reaches every node over the same UDP transport the tunnel uses, on the same port, in frames the node only accepts from a key it holds — anything else is dropped without a reply. Nodes never talk to each other for configuration; an ingress forwarding to an egress is traffic, not config, and the peer list for that comes from the panel. Adding a node here only records it: the machine itself is provisioned by the deploy script the Add dialog generates.',
    endpoints: [
      {
        method: 'GET',
        path: '/panel/api/nodes/list',
        summary: 'Every node with its role, health and revision. appliedRevision is what the node is actually running — compare it against the last published revision to see which nodes still owe an apply.',
        response:
          '{\n  "success": true,\n  "obj": [\n    {\n      "id": 1,\n      "name": "node-1",\n      "address": "node-1.example.com",\n      "port": 443,\n      "role": "ingress",\n      "enable": true,\n      "status": "online",\n      "latencyMs": 8,\n      "cpuPct": 0.9,\n      "memPct": 11.4,\n      "uptimeSecs": 486000,\n      "onlineCount": 2,\n      "revision": 43,\n      "appliedRevision": 43,\n      "lastHeartbeat": 1735689600,\n      "lastError": ""\n    }\n  ]\n}',
      },
      {
        method: 'POST',
        path: '/panel/api/nodes/add',
        summary: 'Record a new node and mint the admin key only this node will accept. Nothing is contacted — the machine does not exist yet. The Add dialog turns the same values into a deploy script; running it on the new machine is what brings the node up, after which it reports in and leaves the "waiting" state.',
        params: [
          { name: 'name', in: 'body (json)', type: 'string', desc: 'Node tag.' },
          { name: 'role', in: 'body (json)', type: 'string', desc: 'ingress (authorises clients, exits or forwards) or egress (authorises ingress peers only).' },
          { name: 'address', in: 'body (json)', type: 'string', desc: 'The node address.' },
          { name: 'port', in: 'body (json)', type: 'number', desc: 'Port shared by the tunnel (UDP) and the control channel (TCP).' },
          
          { name: 'apiToken', in: 'body (json)', type: 'string', desc: 'The one key the whole network shares. Born with the first node and handed to every other one by its deploy script; it seals the transport and is what an admin proves itself with.' },
        ],
        body: '{\n  "name": "node-9",\n  "role": "egress",\n  "address": "node-9.example.com",\n  "port": 443,\n  "apiToken": "7f3a…",\n  "enable": true\n}',
      },
      {
        method: 'POST',
        path: '/panel/api/nodes/update/:id',
        summary: 'Change what the panel may change on a live node: tag and role. Address, port and admin key are baked in by the deploy script and are not editable here — changing them means redeploying. Switching the role changes which projection the node receives, so it takes effect on the next publish.',
        params: [
          { name: 'id', in: 'path', type: 'number', desc: 'Node ID.' },
        ],
        body: '{\n  "name": "node-9",\n  "role": "ingress"\n}',
      },
      {
        method: 'POST',
        path: '/panel/api/nodes/setEnable/:id',
        summary: 'Take a node in or out of the network. A disabled node is skipped by publishes and stops being counted as a route for the groups holding its entrypoints; the machine keeps running until it is given a configuration that says otherwise.',
        params: [
          { name: 'id', in: 'path', type: 'number', desc: 'Node ID.' },
        ],
        body: '{\n  "enable": false\n}',
      },
      {
        method: 'POST',
        path: '/panel/api/nodes/del/:id',
        summary: 'Forget a node. Its entrypoints go with it and drop out of every group that held them — clients whose group is left empty lose their route, so check the groups first. The machine is not touched and keeps running the last configuration it received until it is torn down by hand.',
        params: [
          { name: 'id', in: 'path', type: 'number', desc: 'Node ID.' },
        ],
      },
      {
        method: 'POST',
        path: '/panel/api/nodes/probe/:id',
        summary: 'Open the control channel now and refresh the node\'s cached health — latency, load, uptime, applied revision. Drives both the manual probe and the Reconnect button on an offline node. Latency is measured panel to node and describes the operator\'s path, not what any client experiences.',
        params: [
          { name: 'id', in: 'path', type: 'number', desc: 'Node ID.' },
        ],
        response: '{\n  "success": true,\n  "obj": {\n    "status": "online",\n    "latencyMs": 8\n  }\n}',
      },
      {
        method: 'POST',
        path: '/panel/api/nodes/restartAll',
        summary: 'Signal every reachable node to restart its datapath. Offline nodes are reported as skipped rather than queued. The node must flush its accumulated counters to disk before restarting — XDP maps reset to zero, and a restart that skips the flush silently loses the period.',
        response: '{\n  "success": true,\n  "obj": [\n    { "id": 1, "name": "node-1", "ok": true },\n    { "id": 3, "name": "node-7", "ok": false, "error": "node is offline" }\n  ]\n}',
      },
      {
        method: 'GET',
        path: '/panel/api/nodes/:id/history/:metric/:bucket',
        summary: 'Aggregated metric history recorded by the node\'s own runtime worker. The worker samples continuously and persists locally, so a window the panel was closed for is still returned once it reconnects.',
        params: [
          { name: 'id', in: 'path', type: 'number', desc: 'Node ID.' },
          { name: 'metric', in: 'path', type: 'string', desc: 'cpu | mem | swap | netUp | netDown | pktUp | pktDown | tcpCount | udpCount | diskRead | diskWrite | diskUsage | online | load1 | load5 | load15.' },
          { name: 'bucket', in: 'path', type: 'number', desc: 'Window in minutes. 1 … 43200 (30 days).' },
        ],
        response: '{\n  "success": true,\n  "obj": [\n    { "t": 1735689600, "v": 12.4 },\n    { "t": 1735689630, "v": 13.1 }\n  ]\n}',
      },
      {
        method: 'GET',
        path: '/panel/api/nodes/:id/history/export',
        summary: 'Raw, unaggregated samples for the requested window, as stored by the node worker. Intended for saving a local copy, not for charting.',
        params: [
          { name: 'id', in: 'path', type: 'number', desc: 'Node ID.' },
          { name: 'bucket', in: 'query', type: 'number', desc: 'Window in minutes. 1 … 43200 (30 days).' },
        ],
      },
      {
        method: 'POST',
        path: '/panel/api/nodes/:id/logs/:count',
        summary: 'Tail the node\'s log over the control channel, so the node has to be reachable — an offline node returns nothing rather than a cached copy.',
        params: [
          { name: 'id', in: 'path', type: 'number', desc: 'Node ID.' },
          { name: 'count', in: 'path', type: 'number', desc: 'How many lines to return.' },
          { name: 'level', in: 'body (json)', type: 'string', desc: 'debug | info | notice | warning | err.' },
          { name: 'syslog', in: 'body (json)', type: 'boolean', desc: 'Read the system journal instead of the node\'s own log. A node without systemd should say so rather than return an empty list.' },
        ],
        body: '{\n  "level": "info",\n  "syslog": false\n}',
        response: '{\n  "success": true,\n  "obj": ["2026/08/19 15:24:03 INFO - QD: xdp attached in native mode"]\n}',
      },
      {
        method: 'GET',
        path: '/panel/api/nodes/:id/logs/download',
        summary: 'Whole log bundle from the node as one blob — every level, not the filtered view. For saving a local copy.',
        params: [
          { name: 'id', in: 'path', type: 'number', desc: 'Node ID.' },
        ],
      },
    ],
  },

  {
    id: 'publish',
    title: 'Publish',
    description:
      'Network state — clients, groups, entrypoints, node roles — is edited as a local draft and only reaches the nodes when the operator publishes it. Publishing runs in three phases (plan, push, apply) so a half-delivered rollout never becomes a half-applied one. Each node stores the revision it is running; the panel compares that against the current one to show drift.',
    endpoints: [
      {
        method: 'GET',
        path: '/panel/api/publish/draft',
        summary: 'Summary of what the local draft changes against the last published revision. Drives the Publish/Discard pair in the header: absent or empty means the draft matches what the nodes run, and neither button is shown.',
        response: '{\n  "success": true,\n  "obj": {\n    "revision": 43,\n    "publishedRevision": 42,\n    "changes": [\n      { "kind": "client", "name": "vasya", "action": "updated" },\n      { "kind": "group", "name": "russia-in", "action": "entrypoints" }\n    ]\n  }\n}',
      },
      {
        method: 'POST',
        path: '/panel/api/publish/discard',
        summary: 'Throw the draft away and return to the last published revision. Local only — nothing is sent to any node. Without this a draft the operator decided against would still be waiting at the next panel start.',
        response: '{\n  "success": true,\n  "obj": { "revision": 42 }\n}',
      },
      {
        method: 'POST',
        path: '/panel/api/publish/plan',
        summary: 'Phase 1. Freeze the draft into a new revision and build each node\'s configuration from it. Returns the participating nodes — the ones the panel currently holds a control channel to. Nodes already known to be unreachable are listed as skipped and do not hold the rollout up; they pick the revision up when they reconnect.',
        response: '{\n  "success": true,\n  "obj": {\n    "revision": 43,\n    "targets": [\n      { "id": 1, "name": "node-1", "role": "ingress", "bytes": 20480 },\n      { "id": 2, "name": "node-4", "role": "egress", "bytes": 4096 }\n    ],\n    "skipped": [\n      { "id": 3, "name": "node-7", "reason": "no control channel" }\n    ]\n  }\n}',
      },
      {
        method: 'POST',
        path: '/panel/api/publish/push',
        summary: 'Phase 2. Ship the planned configuration to every target. A node that fails is retried three times, three seconds apart, before it is reported as failed; the operator can then retry it by hand. Delivered configuration is staged on the node and is NOT live until /apply.',
        params: [
          { name: 'revision', in: 'body (json)', type: 'number', desc: 'Revision returned by /plan.' },
          { name: 'nodeIds', in: 'body (json)', type: 'integer[]', desc: 'Restrict the push to these nodes. Omit to push to every target of the plan — used when retrying just the failures.', optional: true },
        ],
        body: '{\n  "revision": 43\n}',
        response: '{\n  "success": true,\n  "obj": [\n    { "id": 1, "name": "node-1", "state": "staged" },\n    { "id": 2, "name": "node-4", "state": "failed", "attempts": 3, "error": "control channel reset" }\n  ]\n}',
      },
      {
        method: 'POST',
        path: '/panel/api/publish/apply',
        summary: 'Phase 3. Tell every node holding the staged revision to switch to it. The switch is seamless — established sessions are not dropped. A node applies only the revision it actually staged, so an apply can never activate a partial push.',
        params: [
          { name: 'revision', in: 'body (json)', type: 'number', desc: 'Revision to activate.' },
        ],
        body: '{\n  "revision": 43\n}',
        response: '{\n  "success": true,\n  "obj": [\n    { "id": 1, "name": "node-1", "state": "applied", "appliedRevision": 43 }\n  ]\n}',
      },
      {
        method: 'GET',
        path: '/panel/api/publish/status',
        summary: 'Current state of the rollout in flight, per node. Lets the publish dialog be closed and reopened without losing the run — reopening resumes at the phase the rollout is actually in rather than starting over.',
        response: '{\n  "success": true,\n  "obj": {\n    "revision": 43,\n    "phase": "push",\n    "nodes": [\n      { "id": 1, "name": "node-1", "state": "staged" },\n      { "id": 2, "name": "node-4", "state": "pushing", "attempts": 1 }\n    ]\n  }\n}',
      },
    ],
  },

  {
    id: 'backup',
    title: 'Backup',
    description:
      'Database snapshots taken from the nodes themselves, not from the panel. The panel holds the draft; the nodes hold what is actually running, which is what a backup has to capture.',
    endpoints: [
      {
        method: 'POST',
        path: '/panel/api/backup/pull',
        summary: 'Collect the live database from every reachable node, compare the copies by checksum, and return a single snapshot. Nodes that disagree are reported so a node that silently drifted is visible instead of being averaged away.',
        response: '{\n  "success": true,\n  "obj": {\n    "revision": 42,\n    "sha256": "9f2b…",\n    "agreed": [1, 2],\n    "diverged": [\n      { "id": 3, "name": "node-7", "sha256": "41ac…", "revision": 31 }\n    ]\n  }\n}',
      },
      {
        method: 'POST',
        path: '/panel/api/backup/restore',
        summary: 'Push a snapshot taken earlier to every reachable node and activate it. Runs through the same stage-then-apply path as a publish, so a restore cannot leave half the network on the old database and half on the new one.',
        params: [
          { name: 'file', in: 'body (multipart)', type: 'file', desc: 'Snapshot produced by /backup/pull.' },
        ],
        response: '{\n  "success": true,\n  "obj": [\n    { "id": 1, "name": "node-1", "state": "applied" }\n  ]\n}',
      },
    ],
  },

  {
    id: 'settings',
    title: 'Settings',
    description:
      'Preferences of the panel itself — how it renders and when it warns. Nothing here is network state: none of it is published, none of it reaches a node.',
    endpoints: [
      {
        method: 'POST',
        path: '/panel/setting/all',
        summary: 'Every panel preference. Six fields — page size, expiry and traffic warning thresholds, remark model, datepicker, timezone. These change how the panel renders and when it warns; none of them reach a node, and none of them appear in any published revision.',
        response: '{\n  "success": true,\n  "obj": {\n    "webPort": 2053,\n    "webCertFile": "",\n    "webKeyFile": "",\n    "webBasePath": "/",\n    "subPort": 10882,\n    "subPath": "/sub/",\n    "tgBotEnable": false,\n    "tgBotToken": "",\n    ...\n  }\n}',
      },
      {
        method: 'POST',
        path: '/panel/setting/defaultSettings',
        summary: 'The values a fresh install starts from. Same shape as /all.',
      },
      {
        method: 'POST',
        path: '/panel/setting/update',
        summary: 'Persist the panel preferences. The panel saves these on its own shortly after they settle rather than behind a button — the header is reserved for the network draft, which is the thing that has to be published deliberately.',
        body: '{\n  "webPort": 2053,\n  "webBasePath": "/",\n  "subPort": 10882,\n  "subPath": "/sub/",\n  "tgBotEnable": false,\n  ...\n}',
      },
      {
        method: 'POST',
        path: '/panel/setting/restartPanel',
        summary: 'Restart the panel process. Rarely needed: preferences take effect immediately and nothing here is bound to a listening port. The connection drops and the panel comes back on the same URL.',
      },
    ],
  },

  {
    id: 'websocket',
    title: 'WebSocket',
    description:
      'Live pushes instead of polling. One connection carries machine status, notifications raised by the panel’s own long-running work, and cache invalidations. Each message has a <code>type</code> field that identifies the payload shape.',
    endpoints: [
      {
        method: 'GET',
        path: '/ws',
        summary: 'Upgrade to a WebSocket for live pushes. Carries what would otherwise be polled — machine status, notifications, and cache invalidations raised by the panel\'s own work.',
      },
      {
        method: 'WS',
        path: '→ type: status',
        summary: 'Machine snapshot pushed on a short interval — the same shape as GET /panel/api/server/status.',
        response: '{\n  "type": "status",\n  "data": { "cpu": 12.5, "mem": { "current": 2147483648, "total": 8589934592 }, "uptime": 486000 }\n}',
      },
      {
        method: 'WS',
        path: '→ type: notification',
        summary: 'In-panel notification. Raised by the panel\'s own long-running work: a publish finishing, a restore completing, a node dropping off the control channel.',
        response: '{\n  "type": "notification",\n  "title": "Publish applied",\n  "body": "Revision 43 is live on 2 of 3 nodes",\n  "severity": "success"\n}',
      },
      {
        method: 'WS',
        path: '→ type: invalidate',
        summary: 'Tells the UI to re-fetch a resource whose cached copy is now stale — after a publish applies, or after a node reports a revision the panel did not expect.',
        response: '{\n  "type": "invalidate",\n  "resource": "inbounds"\n}',
      },
    ],
  },
];
