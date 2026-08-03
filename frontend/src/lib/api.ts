// Typed client for the panel REST API.

const TOKEN_KEY = 'amneziax.token'

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY)
}
export function setToken(token: string | null) {
  if (token) localStorage.setItem(TOKEN_KEY, token)
  else localStorage.removeItem(TOKEN_KEY)
}

export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
  ) {
    super(message)
  }
}

/** Fired when the server rejects our token so the shell can bounce to login. */
export const onUnauthorized = new Set<() => void>()

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = {}
  const token = getToken()
  if (token) headers.Authorization = `Bearer ${token}`
  if (body !== undefined) headers['Content-Type'] = 'application/json'

  const res = await fetch(path, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  })

  if (res.status === 401) {
    setToken(null)
    onUnauthorized.forEach((fn) => fn())
    throw new ApiError('unauthorized', 401)
  }

  const text = await res.text()
  const isJson = (res.headers.get('content-type') ?? '').includes('json')
  const parsed = text && isJson ? JSON.parse(text) : text

  if (!res.ok) {
    const message =
      parsed && typeof parsed === 'object' && 'error' in parsed
        ? String((parsed as { error: unknown }).error)
        : `HTTP ${res.status}`
    throw new ApiError(message, res.status)
  }
  return parsed as T
}

export const api = {
  get: <T,>(path: string) => request<T>('GET', path),
  post: <T,>(path: string, body?: unknown) => request<T>('POST', path, body ?? {}),
  put: <T,>(path: string, body?: unknown) => request<T>('PUT', path, body ?? {}),
  del: <T,>(path: string) => request<T>('DELETE', path),
}

// ------------------------------------------------------------------- types

export type AdminRole = 'OWNER' | 'ADMIN' | 'VIEWER'

export interface Admin {
  uuid: string
  username: string
  role: AdminRole
  isDisabled: boolean
  lastLoginAt: string | null
  createdAt: string
  updatedAt: string
}

export interface Inbound {
  uuid: string
  profileUuid: string
  profileName?: string
  tag: string
  type: string
  network: string
  security: string
  port: number
}

export interface ConfigProfile {
  uuid: string
  name: string
  config: unknown
  createdAt: string
  updatedAt: string
  inbounds: Inbound[] | null
  nodeUuids?: string[]
}

export type NodeHealth =
  | 'UNKNOWN'
  | 'CONNECTING'
  | 'ONLINE'
  | 'DEGRADED'
  | 'OFFLINE'
  | 'DISABLED'
  | 'TRAFFIC_LIMITED'

export type ResetStrategy = 'NO_RESET' | 'DAY' | 'WEEK' | 'MONTH'

export interface Node {
  uuid: string
  name: string
  address: string
  countryCode: string
  description: string
  tokenPreview: string
  isDisabled: boolean
  isConnected: boolean
  health: NodeHealth
  configProfileUuid: string | null
  configProfileName?: string
  activeInboundTags: string[]
  consumptionMultiplier: number
  trafficLimitBytes: number
  trafficUsedBytes: number
  trafficResetStrategy: ResetStrategy
  notifyPercent: number
  agentVersion: string
  xrayVersion: string
  xrayRunning: boolean
  xrayStartedAt: string | null
  configHash: string
  hostname: string
  os: string
  arch: string
  kernel: string
  cpuCount: number
  cpuModel: string
  cpuUsagePercent: number
  totalRamBytes: number
  usedRamBytes: number
  loadAvg1: number
  onlineUsers: number
  statusMessage: string
  lastStatusChangeAt: string | null
  lastConnectedAt: string | null
  viewPosition: number
  provider: string
  providerUrl: string
  costAmount: number
  costCurrency: string
  billingCycle: BillingCycle
  nextPaymentAt: string | null
  billingNotes: string
  tags: string[]
  createdAt: string
  updatedAt: string
}

export interface Host {
  uuid: string
  inboundUuid: string
  inboundTag?: string
  inboundType?: string
  configProfileUuid?: string
  configProfileName?: string
  remark: string
  address: string
  port: number
  path: string
  sni: string
  hostHeader: string
  alpn: string
  fingerprint: string
  publicKey: string
  shortId: string
  spiderX: string
  flow: string
  security: string
  allowInsecure: boolean
  tags: string[]
  isDisabled: boolean
  viewPosition: number
  createdAt: string
  updatedAt: string
}

export interface Squad {
  uuid: string
  name: string
  info: string
  inboundUuids: string[]
  inbounds?: Inbound[]
  membersCount: number
  createdAt: string
  updatedAt: string
}

export type UserStatus = 'ACTIVE' | 'DISABLED' | 'LIMITED' | 'EXPIRED'

export interface User {
  uuid: string
  shortUuid: string
  username: string
  subscriptionUuid: string
  subscriptionUrl: string
  vlessUuid: string
  status: UserStatus
  trafficLimitBytes: number
  usedTrafficBytes: number
  lifetimeUsedTrafficBytes: number
  trafficLimitStrategy: ResetStrategy
  expireAt: string | null
  onlineAt: string | null
  description: string
  tag: string
  email: string
  telegramId: number | null
  hwidDeviceLimit: number
  subLastUserAgent: string
  subLastOpenedAt: string | null
  squadUuids: string[]
  squads?: { uuid: string; name: string }[]
  createdAt: string
  updatedAt: string
}

export interface Settings {
  brandName: string
  brandTagline: string
  brandLogo: string
  brandAccent: string
  subscriptionTitle: string
  supportUrl: string
  currency: string
  subscriptionFormat: string
  updatedAt?: string
}

export type BillingCycle = 'NONE' | 'MONTHLY' | 'QUARTERLY' | 'YEARLY'

export interface SpendSummary {
  currency: string
  monthlyTotal: number
  yearlyTotal: number
  billedNodes: number
  costPerTb: number
  trafficThisMonthTb: number
  overdue: number
  byProvider: { provider: string; nodes: number; monthly: number }[]
  upcoming: {
    nodeUuid: string
    nodeName: string
    provider: string
    amount: number
    currency: string
    dueAt: string
    daysLeft: number
  }[]
}

export interface Device {
  userUuid: string
  hwid: string
  userAgent: string
  platform: string
  firstSeen: string
  lastSeen: string
}

export interface ApiToken {
  uuid: string
  name: string
  tokenPreview: string
  createdBy: string
  lastUsedAt: string | null
  expiresAt: string | null
  createdAt: string
}

export interface Overview {
  counters: {
    usersTotal: number
    usersActive: number
    usersDisabled: number
    usersLimited: number
    usersExpired: number
    usersOnline: number
    nodesTotal: number
    nodesOnline: number
    nodesDisabled: number
    hostsTotal: number
    squadsTotal: number
    profilesTotal: number
    trafficLast24hBytes: number
    trafficLast7dBytes: number
    trafficLast30dBytes: number
    trafficTotalBytes: number
  }
  panel: {
    version: string
    commit: string
    uptimeSeconds: number
    goVersion: string
    connectedNodes: number
  }
}

export interface TrafficPoint {
  at: string
  bytes: number
}

export interface TrafficStats {
  interval: string
  since: string
  total: TrafficPoint[]
  nodes: { nodeUuid: string; nodeName: string; points: TrafficPoint[]; totalBytes: number }[]
}

export interface PanelEvent {
  id: number
  kind: string
  actor: string
  subject: string
  message: string
  createdAt: string
}

export interface SubscriptionInfo {
  username: string
  status: UserStatus
  usedTrafficBytes: number
  trafficLimitBytes: number
  lifetimeUsedTrafficBytes: number
  expireAt: string | null
  subscriptionUrl: string
  links: string[]
  announcements?: Announcement[]
  title: string
  supportUrl?: string
  daysLeft?: number
}

export interface NotificationChannel {
  uuid: string
  name: string
  kind: 'WEBHOOK' | 'TELEGRAM'
  /** Secrets never come back from the API; only the non-secret keys appear. */
  config: { url?: string; chatId?: string }
  hasSecret: boolean
  events: string[]
  eventCount: number
  isEnabled: boolean
  lastOk: boolean | null
  lastDetail: string
  lastSentAt: string | null
  createdAt: string
  updatedAt: string
}

export interface NotificationDelivery {
  id: number
  channelUuid: string
  eventKind: string
  ok: boolean
  detail: string
  attempts: number
  durationMs: number
  createdAt: string
}

export interface Announcement {
  uuid: string
  title: string
  body: string
  level: 'INFO' | 'WARNING' | 'DANGER'
  isEnabled: boolean
  startsAt: string | null
  endsAt: string | null
  createdAt: string
  updatedAt: string
}
