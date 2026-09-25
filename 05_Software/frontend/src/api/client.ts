import {
  parsePlugins,
  parsePlugin,
  parseInstrumentDevices,
  parseInstrumentJobs,
  parseInstrumentJob,
  type InstrumentPlugin,
  type InstrumentDevice,
  type InstrumentJob,
  type InstrumentSubmission,
  type PluginAction,
} from './bench';
import {
  WIREGUARD_PATH,
  parseWireGuardStatus,
  type WireGuardStatus,
  type WireGuardChange,
} from './wireguard';
import {
  ACTIVITY_EXPORT_PATH,
  ACTIVITY_PATH,
  ACTIVITY_RETENTION_PATH,
  activityQuery,
  parseActivityPageBody,
  parseActivityRetentionSettingsBody,
  type ActivityFilter,
  type ActivityPage,
  type ActivityRetentionSettings,
} from './activity';
import {
  DEVICE_NETWORKS_PATH,
  deviceNetworkPath,
  parseAccessInstructionsBody,
  parseDeviceNetworkCheckBody,
  parseDeviceNetworksBody,
  type AccessInstructions,
  type DeviceNetwork,
  type DeviceNetworkChange,
  type DeviceNetworkCheck,
} from './devicenetworks';
import {
  interfaceSettingsPreviewPath,
  parseWiFiScanBody,
  wifiJoinPreviewPath,
  wifiScanPath,
  type InterfaceSettingsDraft,
  type WiFiJoinDraft,
  type WiFiScan,
} from './settings';
import {
  COMMISSIONING_PATH,
  MANAGEMENT_ACCESS_PATH,
  parseAdapterRoleConflict,
  parseManagementAccess,
  type AdapterRoleConflict,
  type ManagementAccessStatus,
} from './management-access';
import {
  CSRF_HEADER_NAME,
  DEVICES_PATH,
  IDENTITIES_PATH,
  PASSWORD_POLICY_PATH,
  INTEGRATIONS_PATH,
  NETWORK_OPERATIONS_PATH,
  NETWORK_STATUS_PATH,
  POWER_PATH,
  SESSION_PATH,
  SIGN_IN_PATH,
  SIGN_OUT_PATH,
  STATUS_PATH,
  TOKENS_PATH,
  parsePasswordPolicyBody,
  type PasswordPolicy,
  identityPath,
  identityResourcePath,
  interfacePreviewPath,
  networkOperationPath,
  networkStatisticsPath,
  normalizeBaseUrl,
  parseDevicesBody,
  parseIdentitiesBody,
  parseIdentityBody,
  parseIntegrationsBody,
  parseIssuedTokenBody,
  parseNetworkOperationBody,
  parseNetworkOperationsBody,
  parseNetworkStatisticsBody,
  parseNetworkStatusBody,
  parsePowerResultBody,
  parseSessionBody,
  parseStatusBody,
  parseTokensBody,
  resourceUrl,
  tokenResourcePath,
  tokenRevocationPath,
  type BenchCoreIdentity,
  type BenchCoreRole,
  type BenchCoreSession,
  type BenchCoreStatus,
  type DeviceInventory,
  type IdentityInventory,
  type IntegrationInventory,
  type IssuedToken,
  type NetworkOperation,
  type NetworkStatistics,
  type NetworkStatus,
  type ParseResult,
  type PowerAction,
  type PowerResult,
  type StatisticsPeriod,
  type TokenInventory,
} from './contract';
import {
  APPLIANCE_PATH,
  parseApplianceMetadata,
  parseIdentifyResult,
  type ApplianceMetadata,
  type IdentifyResult,
} from './appliance';

/** The subset of a fetch response this client depends on. */
export interface TransportResponse {
  readonly status: number;
  readonly ok: boolean;
  text(): Promise<string>;
  blob?(): Promise<Blob>;
}

/** The subset of a fetch request this client sends. */
export interface TransportRequest {
  method: string;
  headers: Record<string, string>;
  body?: string;
  /**
   * Always `same-origin`. Every path is origin-relative, and the browser
   * session lives in a cookie the page cannot read, so the cookie has to be
   * attached by the browser or the request is anonymous.
   */
  credentials: 'same-origin';
}

/**
 * A fetch-shaped function. The global `fetch` satisfies it, and so does the
 * development mock, which is what lets the GUI run with no backend.
 */
export type BenchCoreTransport = (
  url: string,
  request: TransportRequest,
) => Promise<TransportResponse>;

/**
 * How one request proves who is asking.
 *
 * A browser session sends no `Authorization` header at all: its secret is in an
 * `HttpOnly` cookie the browser attaches and the page never sees. What the page
 * does hold is the cross-site request forgery token, which it repeats on every
 * state-changing request. A bearer credential is the reverse — a header, and no
 * cookie for a cross-site request to abuse.
 */
export type Credential =
  | { readonly kind: 'session'; readonly csrfToken: string }
  | { readonly kind: 'bearer'; readonly token: string };

/** A credential that authenticates with a pasted development bearer token. */
export function bearerCredential(token: string): Credential {
  return { kind: 'bearer', token };
}

/** A credential that authenticates with the server-managed session cookie. */
export function sessionCredential(csrfToken: string): Credential {
  return { kind: 'session', csrfToken };
}

/**
 * Why a Bench Core request could not be completed.
 *
 * `unauthorized` and `forbidden` are deliberately separate: signing in again
 * fixes the first and achieves nothing for the second. `rejected` is a request
 * Bench Core understood and refused with a reason the caller can act on.
 */
export type BenchCoreFailure =
  'unauthorized' | 'forbidden' | 'rejected' | 'unavailable' | 'malformed';

/** A Bench Core request failure with a machine-readable cause. */
export class BenchCoreError extends Error {
  readonly failure: BenchCoreFailure;
  readonly detail: string;
  /** The stable error code Bench Core sent, or the empty string. */
  readonly code: string;
  /** Validated safe metadata for an adapter-role collision, when supplied. */
  readonly adapterRoleConflict: AdapterRoleConflict | null;

  constructor(
    failure: BenchCoreFailure,
    detail: string,
    code = '',
    adapterRoleConflict: AdapterRoleConflict | null = null,
  ) {
    super(`bench core request failed: ${failure}: ${detail}`);
    this.name = 'BenchCoreError';
    this.failure = failure;
    this.detail = detail;
    this.code = code;
    this.adapterRoleConflict = adapterRoleConflict;
  }
}

/** What an administrator submits to create an identity. */
export interface NewIdentity {
  username: string;
  role: BenchCoreRole;
  password: string;
}

/** What an administrator submits to create an API token. */
export interface NewToken {
  owner: string;
  name: string;
  scopes: string[];
  /** An RFC 3339 instant, or null for a token that does not expire. */
  expiresAt: string | null;
}

/** The authenticated Bench Core operations the GUI can perform. */
export interface BenchCoreClient {
  readonly baseUrl: string;
  listPlugins(credential: Credential): Promise<InstrumentPlugin[]>;
  pluginAction(
    credential: Credential,
    id: string,
    action: PluginAction,
    mode?: string,
  ): Promise<InstrumentPlugin>;
  discoverInstruments(credential: Credential, id: string): Promise<InstrumentDevice[]>;
  listInstrumentJobs(credential: Credential): Promise<InstrumentJob[]>;
  getInstrumentJob(credential: Credential, id: string): Promise<InstrumentJob>;
  submitInstrumentJob(
    credential: Credential,
    input: InstrumentSubmission,
  ): Promise<InstrumentJob>;
  instrumentJobAction(
    credential: Credential,
    id: string,
    action: 'approve' | 'cancel',
  ): Promise<InstrumentJob>;
  downloadInstrumentArtifact(credential: Credential, id: string): Promise<Blob>;

  getStatus(credential: Credential): Promise<BenchCoreStatus>;
  getIntegrations(credential: Credential): Promise<IntegrationInventory>;
  getDevices(credential: Credential): Promise<DeviceInventory>;

  getAppliance?(credential: Credential): Promise<ApplianceMetadata>;
  setApplianceFriendlyName?(
    credential: Credential,
    friendlyName: string,
  ): Promise<ApplianceMetadata>;
  identifyAppliance?(credential: Credential): Promise<IdentifyResult>;

  getNetworkStatus(credential: Credential): Promise<NetworkStatus>;
  getManagementAccess(credential: Credential): Promise<ManagementAccessStatus>;
  setManagementAccess(
    credential: Credential,
    interfaces: string[],
    confirmCurrentInterface: boolean,
  ): Promise<ManagementAccessStatus>;
  confirmManagementAccess(credential: Credential): Promise<ManagementAccessStatus>;
  recoverManagementAccess(credential: Credential): Promise<ManagementAccessStatus>;
  /**
   * Ends first boot: fixes the allowlist to the interfaces chosen and records
   * the appliance commissioned. The appliance refuses a selection it could not
   * be reached through.
   */
  completeCommissioning(
    credential: Credential,
    interfaces: string[],
    confirmCurrentInterface: boolean,
  ): Promise<ManagementAccessStatus>;
  getWireGuardStatus(credential: Credential): Promise<WireGuardStatus>;
  previewWireGuardChange(
    credential: Credential,
    change: WireGuardChange,
  ): Promise<NetworkOperation>;
  getNetworkOperation(credential: Credential, id: string): Promise<NetworkOperation>;
  getNetworkStatistics(
    credential: Credential,
    interfaceId: string,
    period: StatisticsPeriod,
  ): Promise<NetworkStatistics>;
  previewInterfaceChange(
    credential: Credential,
    interfaceId: string,
    action: 'enable' | 'disable',
  ): Promise<NetworkOperation>;
  acknowledgeNetworkOperation(
    credential: Credential,
    id: string,
    revision: string,
  ): Promise<NetworkOperation>;
  confirmNetworkOperation(
    credential: Credential,
    id: string,
  ): Promise<NetworkOperation>;
  rollbackNetworkOperation(
    credential: Credential,
    id: string,
  ): Promise<NetworkOperation>;
  /** Recent managed operations, newest first, for reattaching after a reload. */
  listNetworkOperations(credential: Credential): Promise<NetworkOperation[]>;

  previewInterfaceSettings(
    credential: Credential,
    interfaceId: string,
    settings: InterfaceSettingsDraft,
  ): Promise<NetworkOperation>;
  scanWiFi(credential: Credential, interfaceId: string): Promise<WiFiScan>;
  previewWiFiJoin(
    credential: Credential,
    interfaceId: string,
    join: WiFiJoinDraft,
  ): Promise<NetworkOperation>;

  listDeviceNetworks(credential: Credential): Promise<DeviceNetwork[]>;
  previewDeviceNetworkChange(
    credential: Credential,
    change: DeviceNetworkChange,
  ): Promise<NetworkOperation>;
  checkDeviceNetwork(
    credential: Credential,
    id: string,
    tcpPort: number,
  ): Promise<DeviceNetworkCheck>;
  getAccessInstructions(
    credential: Credential,
    id: string,
  ): Promise<AccessInstructions>;

  listActivity(credential: Credential, filter: ActivityFilter): Promise<ActivityPage>;
  /** The export as text, exactly as the appliance wrote it. */
  exportActivity(
    credential: Credential,
    filter: ActivityFilter,
    format: 'jsonl' | 'csv',
  ): Promise<string>;
  getActivityRetention(credential: Credential): Promise<ActivityRetentionSettings>;
  setActivityRetention(
    credential: Credential,
    settings: ActivityRetentionSettings,
  ): Promise<ActivityRetentionSettings>;

  /** The appliance's current password policy. */
  getPasswordPolicy(credential: Credential): Promise<PasswordPolicy>;
  /** Replace the whole policy; the appliance refuses a partial one. */
  setPasswordPolicy(
    credential: Credential,
    policy: PasswordPolicy,
  ): Promise<PasswordPolicy>;

  signIn(username: string, password: string): Promise<BenchCoreSession>;
  currentSession(credential: Credential): Promise<BenchCoreSession>;
  signOut(credential: Credential): Promise<void>;

  requestPower(credential: Credential, action: PowerAction): Promise<PowerResult>;

  listIdentities(credential: Credential): Promise<IdentityInventory>;
  createIdentity(
    credential: Credential,
    identity: NewIdentity,
  ): Promise<BenchCoreIdentity>;
  setIdentityRole(
    credential: Credential,
    username: string,
    role: BenchCoreRole,
  ): Promise<void>;
  setIdentityEnabled(
    credential: Credential,
    username: string,
    enabled: boolean,
  ): Promise<void>;
  resetIdentityPassword(
    credential: Credential,
    username: string,
    password: string,
  ): Promise<void>;
  /**
   * Remove one identity permanently.
   *
   * This is not the disable above with a stronger word: nothing it removes can
   * be restored, and Bench Core refuses it while the identity is the last
   * enabled administrator or still owns any API token.
   */
  deleteIdentity(credential: Credential, username: string): Promise<void>;

  listTokens(credential: Credential): Promise<TokenInventory>;
  createToken(credential: Credential, token: NewToken): Promise<IssuedToken>;
  revokeToken(credential: Credential, id: string): Promise<void>;
  /**
   * Remove one API token record permanently.
   *
   * Revoking stops the secret and keeps the record; this removes the record,
   * whatever state it was in. The two are separate operations because they
   * answer different questions.
   */
  deleteToken(credential: Credential, id: string): Promise<void>;
}

export interface BenchCoreClientOptions {
  baseUrl: string;
  transport?: BenchCoreTransport;
}

interface Exchange<T> {
  path: string;
  credential: Credential | null;
  method?: string;
  body?: unknown;
  parse?: (body: unknown) => ParseResult<T>;
}

/** Build a client for one Bench Core base URL. */
export function createBenchCoreClient(
  options: BenchCoreClientOptions,
): BenchCoreClient {
  const baseUrl = normalizeBaseUrl(options.baseUrl);
  const transport = options.transport ?? defaultTransport;

  /**
   * One request path for every resource, so a rejected credential, an
   * unreachable backend, a refused change and a body outside the contract are
   * classified identically whichever route was asked for. A bearer token is
   * only ever an Authorization header value; it never reaches the URL, and
   * neither does a password, which travels in the body of one POST.
   */
  /**
   * One GET whose body is handed back as the text the appliance wrote, for an
   * export that is meant to be kept rather than decoded.
   */
  async function exchangeText(path: string, credential: Credential): Promise<string> {
    const headers: Record<string, string> = {};
    if (credential.kind === 'bearer') {
      headers.Authorization = `Bearer ${credential.token}`;
    }
    let response: TransportResponse;
    try {
      response = await transport(resourceUrl(baseUrl, path), {
        method: 'GET',
        headers,
        credentials: 'same-origin',
      });
    } catch (error) {
      throw new BenchCoreError('unavailable', describe(error));
    }
    if (!response.ok) {
      throw await refusalOf(response);
    }
    return response.text();
  }

  async function exchange<T>(request: Exchange<T>): Promise<T> {
    const url = resourceUrl(baseUrl, request.path);
    const method = request.method ?? 'GET';
    const headers: Record<string, string> = { Accept: 'application/json' };
    let body: string | undefined;

    if (request.credential?.kind === 'bearer') {
      headers.Authorization = `Bearer ${request.credential.token}`;
    }
    if (request.credential?.kind === 'session' && changesState(method)) {
      headers[CSRF_HEADER_NAME] = request.credential.csrfToken;
    }
    if (request.body !== undefined) {
      headers['Content-Type'] = 'application/json';
      body = JSON.stringify(request.body);
    }

    let response: TransportResponse;
    try {
      response = await transport(url, {
        method,
        headers,
        body,
        credentials: 'same-origin',
      });
    } catch (error) {
      throw new BenchCoreError('unavailable', describe(error));
    }

    if (!response.ok) {
      throw await refusalOf(response);
    }

    if (request.parse === undefined) {
      return undefined as T;
    }
    let decoded: unknown;
    try {
      decoded = JSON.parse(await response.text());
    } catch (error) {
      throw new BenchCoreError(
        'malformed',
        `the response to ${request.path} is not valid JSON: ${describe(error)}`,
      );
    }
    const parsed = request.parse(decoded);
    if (!parsed.ok) {
      throw new BenchCoreError('malformed', parsed.violation);
    }
    return parsed.value;
  }

  return {
    baseUrl,
    listPlugins: (credential) =>
      exchange({ path: '/api/v1/plugins', credential, parse: parsePlugins }),
    pluginAction: (credential, id, action, mode) =>
      exchange({
        path: `/api/v1/plugins/${encodeURIComponent(id)}/${action}`,
        credential,
        method: 'POST',
        body: mode ? { mode } : {},
        parse: parsePlugin,
      }),
    discoverInstruments: (credential, id) =>
      exchange({
        path: `/api/v1/plugins/${encodeURIComponent(id)}/discover`,
        credential,
        method: 'POST',
        body: {},
        parse: parseInstrumentDevices,
      }),
    getInstrumentJob: (credential, id) =>
      exchange({
        path: `/api/v1/bench/jobs/${encodeURIComponent(id)}`,
        credential,
        parse: parseInstrumentJob,
      }),
    listInstrumentJobs: (credential) =>
      exchange({ path: '/api/v1/bench/jobs', credential, parse: parseInstrumentJobs }),
    submitInstrumentJob: (credential, input) =>
      exchange({
        path: '/api/v1/bench/jobs',
        credential,
        method: 'POST',
        body: input,
        parse: parseInstrumentJob,
      }),
    instrumentJobAction: (credential, id, action) =>
      exchange({
        path: `/api/v1/bench/jobs/${encodeURIComponent(id)}/${action}`,
        credential,
        method: 'POST',
        body: {},
        parse: parseInstrumentJob,
      }),
    downloadInstrumentArtifact: async (credential, id) => {
      const headers: Record<string, string> = {};
      if (credential.kind === 'bearer')
        headers.Authorization = `Bearer ${credential.token}`;
      const response = await transport(
        resourceUrl(baseUrl, `/api/v1/bench/jobs/${encodeURIComponent(id)}/artifact`),
        { method: 'GET', headers, credentials: 'same-origin' },
      );
      if (!response.ok) throw await refusalOf(response);
      if (!response.blob)
        throw new BenchCoreError('unavailable', 'Binary download is unavailable');
      return response.blob();
    },

    getStatus: (credential) =>
      exchange({ path: STATUS_PATH, credential, parse: parseStatusBody }),
    getIntegrations: (credential) =>
      exchange({ path: INTEGRATIONS_PATH, credential, parse: parseIntegrationsBody }),
    getDevices: (credential) =>
      exchange({ path: DEVICES_PATH, credential, parse: parseDevicesBody }),

    getAppliance: (credential) =>
      exchange({ path: APPLIANCE_PATH, credential, parse: parseApplianceMetadata }),
    setApplianceFriendlyName: (credential, friendlyName) =>
      exchange({
        path: `${APPLIANCE_PATH}/friendly-name`,
        credential,
        method: 'PUT',
        body: { friendly_name: friendlyName },
        parse: parseApplianceMetadata,
      }),
    identifyAppliance: (credential) =>
      exchange({
        path: `${APPLIANCE_PATH}/identify`,
        credential,
        method: 'POST',
        body: {},
        parse: parseIdentifyResult,
      }),

    getNetworkStatus: (credential) =>
      exchange({
        path: NETWORK_STATUS_PATH,
        credential,
        parse: parseNetworkStatusBody,
      }),
    getManagementAccess: (credential) =>
      exchange({
        path: MANAGEMENT_ACCESS_PATH,
        credential,
        parse: parseManagementAccess,
      }),
    setManagementAccess: (credential, interfaces, confirmCurrentInterface) =>
      exchange({
        path: MANAGEMENT_ACCESS_PATH,
        credential,
        method: 'PUT',
        body: { interfaces, confirm_current_interface: confirmCurrentInterface },
        parse: parseManagementAccess,
      }),
    confirmManagementAccess: (credential) =>
      exchange({
        path: `${MANAGEMENT_ACCESS_PATH}/confirm`,
        credential,
        method: 'POST',
        body: {},
        parse: parseManagementAccess,
      }),
    completeCommissioning: (credential, interfaces, confirmCurrentInterface) =>
      exchange({
        path: COMMISSIONING_PATH,
        credential,
        method: 'POST',
        body: { interfaces, confirm_current_interface: confirmCurrentInterface },
        parse: parseManagementAccess,
      }),
    recoverManagementAccess: (credential) =>
      exchange({
        path: `${MANAGEMENT_ACCESS_PATH}/recover`,
        credential,
        method: 'POST',
        body: {},
        parse: parseManagementAccess,
      }),
    getWireGuardStatus: (credential) =>
      exchange({ path: WIREGUARD_PATH, credential, parse: parseWireGuardStatus }),
    previewWireGuardChange: (credential, change) =>
      exchange({
        path:
          WIREGUARD_PATH +
          (change.action === 'install' ? '/install/preview' : '/preview'),
        credential,
        method: 'POST',
        body: change.action === 'install' ? {} : change,
        parse: parseNetworkOperationBody,
      }),
    getNetworkOperation: (credential, id) =>
      exchange({
        path: networkOperationPath(id),
        credential,
        parse: parseNetworkOperationBody,
      }),
    getNetworkStatistics: (credential, interfaceId, period) =>
      exchange({
        path: networkStatisticsPath(interfaceId, period),
        credential,
        parse: parseNetworkStatisticsBody,
      }),
    previewInterfaceChange: (credential, interfaceId, action) =>
      exchange({
        path: interfacePreviewPath(interfaceId),
        credential,
        method: 'POST',
        body: { action },
        parse: parseNetworkOperationBody,
      }),
    acknowledgeNetworkOperation: (credential, id, revision) =>
      exchange({
        path: networkOperationPath(id, 'acknowledge'),
        credential,
        method: 'POST',
        // The revision goes back exactly as it was shown, which is what binds
        // this agreement to that preview and nothing else.
        body: { revision },
        parse: parseNetworkOperationBody,
      }),
    confirmNetworkOperation: (credential, id) =>
      exchange({
        path: networkOperationPath(id, 'confirm'),
        credential,
        method: 'POST',
        body: {},
        parse: parseNetworkOperationBody,
      }),
    rollbackNetworkOperation: (credential, id) =>
      exchange({
        path: networkOperationPath(id, 'rollback'),
        credential,
        method: 'POST',
        body: {},
        parse: parseNetworkOperationBody,
      }),
    listNetworkOperations: (credential) =>
      exchange({
        path: NETWORK_OPERATIONS_PATH,
        credential,
        parse: parseNetworkOperationsBody,
      }),

    previewInterfaceSettings: (credential, interfaceId, settings) =>
      exchange({
        path: interfaceSettingsPreviewPath(interfaceId),
        credential,
        method: 'POST',
        body: settings,
        parse: parseNetworkOperationBody,
      }),
    scanWiFi: (credential, interfaceId) =>
      exchange({
        path: wifiScanPath(interfaceId),
        credential,
        parse: parseWiFiScanBody,
      }),
    previewWiFiJoin: (credential, interfaceId, join) =>
      exchange({
        path: wifiJoinPreviewPath(interfaceId),
        credential,
        method: 'POST',
        // The password travels in this one body and nowhere else.
        body: join,
        parse: parseNetworkOperationBody,
      }),

    listDeviceNetworks: (credential) =>
      exchange({
        path: DEVICE_NETWORKS_PATH,
        credential,
        parse: parseDeviceNetworksBody,
      }),
    previewDeviceNetworkChange: (credential, change) =>
      exchange({
        path: `${DEVICE_NETWORKS_PATH}/preview`,
        credential,
        method: 'POST',
        body: change,
        parse: parseNetworkOperationBody,
      }),
    checkDeviceNetwork: (credential, id, tcpPort) =>
      exchange({
        path: deviceNetworkPath(id, 'checks'),
        credential,
        method: 'POST',
        body: { tcp_port: tcpPort },
        parse: parseDeviceNetworkCheckBody,
      }),
    getAccessInstructions: (credential, id) =>
      exchange({
        path: deviceNetworkPath(id, 'access-instructions'),
        credential,
        parse: parseAccessInstructionsBody,
      }),

    listActivity: (credential, filter) =>
      exchange({
        path: ACTIVITY_PATH + activityQuery(filter),
        credential,
        parse: parseActivityPageBody,
      }),
    exportActivity: (credential, filter, format) =>
      exchangeText(ACTIVITY_EXPORT_PATH + activityQuery(filter, format), credential),
    getActivityRetention: (credential) =>
      exchange({
        path: ACTIVITY_RETENTION_PATH,
        credential,
        parse: parseActivityRetentionSettingsBody,
      }),
    setActivityRetention: (credential, settings) =>
      exchange({
        path: ACTIVITY_RETENTION_PATH,
        credential,
        method: 'PUT',
        body: {
          retention_days: settings.retentionDays,
          quota_bytes: settings.quotaBytes,
        },
        parse: parseActivityRetentionSettingsBody,
      }),

    getPasswordPolicy: (credential) =>
      exchange({
        path: PASSWORD_POLICY_PATH,
        credential,
        parse: parsePasswordPolicyBody,
      }),
    setPasswordPolicy: (credential, policy) =>
      exchange({
        path: PASSWORD_POLICY_PATH,
        credential,
        method: 'PUT',
        body: {
          minimum_length: policy.minimumLength,
          require_lowercase: policy.requireLowercase,
          require_uppercase: policy.requireUppercase,
          require_digit: policy.requireDigit,
          require_special: policy.requireSpecial,
        },
        parse: parsePasswordPolicyBody,
      }),

    signIn: (username, password) =>
      exchange({
        path: SIGN_IN_PATH,
        credential: null,
        method: 'POST',
        body: { username, password },
        parse: parseSessionBody,
      }),
    currentSession: (credential) =>
      exchange({ path: SESSION_PATH, credential, parse: parseSessionBody }),
    signOut: (credential) =>
      exchange<void>({ path: SIGN_OUT_PATH, credential, method: 'POST' }),
    requestPower: (credential, action) =>
      exchange({
        path: POWER_PATH,
        credential,
        method: 'POST',
        body: { action },
        parse: parsePowerResultBody,
      }),

    listIdentities: (credential) =>
      exchange({ path: IDENTITIES_PATH, credential, parse: parseIdentitiesBody }),
    createIdentity: (credential, identity) =>
      exchange({
        path: IDENTITIES_PATH,
        credential,
        method: 'POST',
        body: identity,
        parse: parseIdentityBody,
      }),
    setIdentityRole: (credential, username, role) =>
      exchange<void>({
        path: identityPath(username, 'role'),
        credential,
        method: 'PUT',
        body: { role },
      }),
    setIdentityEnabled: (credential, username, enabled) =>
      exchange<void>({
        path: identityPath(username, 'enabled'),
        credential,
        method: 'PUT',
        body: { enabled },
      }),
    resetIdentityPassword: (credential, username, password) =>
      exchange<void>({
        path: identityPath(username, 'password'),
        credential,
        method: 'PUT',
        body: { password },
      }),
    deleteIdentity: (credential, username) =>
      exchange<void>({
        path: identityResourcePath(username),
        credential,
        method: 'DELETE',
      }),

    listTokens: (credential) =>
      exchange({ path: TOKENS_PATH, credential, parse: parseTokensBody }),
    createToken: (credential, token) =>
      exchange({
        path: TOKENS_PATH,
        credential,
        method: 'POST',
        body: {
          owner: token.owner,
          name: token.name,
          scopes: token.scopes,
          expires_at: token.expiresAt,
        },
        parse: parseIssuedTokenBody,
      }),
    revokeToken: (credential, id) =>
      exchange<void>({ path: tokenRevocationPath(id), credential, method: 'POST' }),
    deleteToken: (credential, id) =>
      exchange<void>({ path: tokenResourcePath(id), credential, method: 'DELETE' }),
  };
}

/**
 * Classify an unsuccessful answer.
 *
 * The three statuses Bench Core uses deliberately mean different things to a
 * human: `401` says the credential is gone, `403` says this identity may not do
 * this, and a `400`, `404`, `409` or `429` says the request itself has to
 * change. Everything else is the backend failing, which is not the caller's
 * fault and not something a different form entry would fix.
 */
async function refusalOf(response: TransportResponse): Promise<BenchCoreError> {
  if (response.status === 401) {
    return new BenchCoreError(
      'unauthorized',
      'Bench Core did not accept the credential (HTTP 401)',
      'unauthorized',
    );
  }
  const { code, message, adapterRoleConflict } = await readRefusal(response);
  if (response.status === 403) {
    return new BenchCoreError(
      'forbidden',
      message || 'This identity is not permitted to do that (HTTP 403).',
      code,
      adapterRoleConflict,
    );
  }
  if ([400, 404, 409, 429].includes(response.status)) {
    return new BenchCoreError(
      'rejected',
      message || `Bench Core refused the request (HTTP ${response.status}).`,
      code,
      adapterRoleConflict,
    );
  }
  return new BenchCoreError(
    'unavailable',
    `Bench Core answered with HTTP ${response.status}`,
    code,
  );
}

/** Read the stable code and the fixed message from a refusal body, if any. */
async function readRefusal(response: TransportResponse): Promise<{
  code: string;
  message: string;
  adapterRoleConflict: AdapterRoleConflict | null;
}> {
  try {
    const decoded: unknown = JSON.parse(await response.text());
    if (typeof decoded === 'object' && decoded !== null) {
      const body = decoded as {
        error?: unknown;
        message?: unknown;
        details?: unknown;
      };
      const code = typeof body.error === 'string' ? body.error : '';
      return {
        code,
        // Role-conflict prose is never a browser contract. Consumers render
        // validated details through their locale and use a fixed localized
        // fallback when those details are malformed.
        message:
          code === 'network_interface_role_conflict'
            ? ''
            : typeof body.message === 'string'
              ? body.message
              : '',
        adapterRoleConflict:
          code === 'network_interface_role_conflict'
            ? parseAdapterRoleConflict(body.details)
            : null,
      };
    }
  } catch {
    // A refusal without a readable body is still a refusal; the status alone
    // carries the meaning, and inventing a reason here would be a guess.
  }
  return { code: '', message: '', adapterRoleConflict: null };
}

function changesState(method: string): boolean {
  return method !== 'GET' && method !== 'HEAD' && method !== 'OPTIONS';
}

function defaultTransport(
  url: string,
  request: TransportRequest,
): Promise<TransportResponse> {
  return fetch(url, request);
}

function describe(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
