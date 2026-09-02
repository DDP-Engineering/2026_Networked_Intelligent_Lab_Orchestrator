/**
 * The English catalogue, and the key set every other language must satisfy.
 *
 * Nothing technical is in here. Permission names, role identifiers, protocol
 * names, API versions, interface names and everything the backend reports are
 * rendered from data, because translating a value the server matches on turns a
 * language choice into a failure nobody associates with the language menu.
 */
export const en = {
  // Shell and navigation
  'app.name': 'NILO',
  'app.title': 'NILO Development Interface',
  'nav.label': 'Main',
  'nav.section.working': 'Workspace',
  'nav.section.instruments': 'Instruments',
  'nav.section.core': 'System',
  'nav.bench': 'Bench',
  'nav.network': 'Network',
  'nav.plugins': 'Plugins',
  'nav.authentication': 'Authentication',
  'nav.settings': 'Settings',
  'nav.about': 'About',
  'nav.collapse': 'Collapse navigation',
  'nav.expand': 'Expand navigation',

  // Tabs
  'tabs.label': 'Open pages',
  'tabs.close': 'Close {name}',
  'tabs.none.title': 'No page is open',
  'tabs.none.body': 'Choose a page from the navigation on the left.',

  // Top-row controls
  'top.simulation': 'SIM',
  'top.simulation.label': 'Runtime profile: simulation',
  'top.network': 'Network',
  'top.appearance': 'Appearance',
  'top.appearance.light': 'Light',
  'top.appearance.dark': 'Dark',
  'top.appearance.system': 'System',
  'top.appearance.follows': 'Follows your device',
  'top.language': 'Language',
  'top.account': 'Account',
  'top.signOut': 'Sign out',
  'top.signedInAs': 'Signed in as {username}',

  // The boundary statement, which every screen carries
  'boundary.statement':
    'Simulation profile only — simulated results never represent evidence from physical equipment.',

  // Sign-in
  'signIn.title': 'Sign in to NILO',
  'signIn.eyebrow': 'Development interface',
  'signIn.intro':
    'Sign in with the username and password of an identity on this appliance. The first administrator is created once at the console; there is no self-registration.',
  'signIn.username': 'Username',
  'signIn.password': 'Password',
  'signIn.submit': 'Sign in',
  'signIn.working': 'Signing in…',
  'signIn.incomplete': 'Enter both a username and a password.',
  'signIn.refused': 'That username and password were not accepted.',
  'signIn.throttled':
    'Too many failed attempts for that username. Wait for the lock-out window to pass and try again.',
  'signIn.ended': 'Your session ended. Sign in again to continue.',
  'signIn.signedOut': 'You are signed out.',
  'signIn.token.title': 'Or connect with a development bearer token',
  'signIn.token.intro':
    'Issue one in WSL2 with the development token command. It is held in memory for this page only and reads the bench at the Viewer level; identity administration needs a signed-in administrator.',
  'signIn.token.label': 'Development bearer token',
  'signIn.token.submit': 'Connect',
  'signIn.token.incomplete':
    'Enter the bearer token issued by the development backend.',
  'signIn.endpoint': 'Endpoint',

  // Session restoration
  'restoring.title': 'Restoring your session…',
  'restoring.body': 'Asking Bench Core whether this browser is already signed in.',

  // Bench page
  'bench.title': 'Bench',
  'bench.connecting': 'Contacting Bench Core…',
  'bench.connecting.body':
    'Waiting for the authenticated status, integration, and device responses.',
  'bench.refresh': 'Refresh',
  'bench.useDifferentToken': 'Use a different token',
  'bench.signInAgain': 'Sign in again',
  'bench.profile': 'Profile',
  'bench.status': 'Service status',
  'bench.simulation': 'Simulation',
  'bench.buildVersion': 'Build version',
  'bench.apiVersion': 'API version',
  'bench.integrationProtocol': 'Integration protocol',
  'bench.yes': 'Yes',
  'bench.no': 'No',
  'bench.integrations': 'Integrations',
  'bench.devices': 'Devices',
  'bench.entries': '{count} entries',
  'bench.entry': '{count} entry',
  'bench.scenario': 'scenario: {name}',
  'bench.noIntegrations': 'No integrations are offered in this simulation scenario.',
  'bench.noDevices': 'No devices are offered in this simulation scenario.',
  'bench.state': 'State',
  'bench.operations': 'Operations',
  'bench.availability': 'Availability',
  'bench.integration': 'Integration',
  'bench.kind': 'Kind',
  'bench.capabilities': 'Capabilities',

  // Bench failures
  'failure.unauthorized': 'Bench Core rejected this token.',
  'failure.unauthorized.advice':
    'The token was refused. Issue a new development token in WSL2 and enter it again.',
  'failure.forbidden': 'This identity may not read the bench.',
  'failure.forbidden.advice':
    'The credential is valid, but the identity behind it does not hold the permission this screen needs. Signing in again will not change that; an administrator has to widen the role or the token scopes.',
  'failure.rejected': 'Bench Core refused the request.',
  'failure.rejected.advice':
    'Bench Core understood the request and declined it. The reason it gave is below.',
  'failure.unavailable': 'Bench Core is not reachable.',
  'failure.unavailable.advice':
    'Nothing answered on the configured endpoint. Confirm the simulation server is running in WSL2 and listening on that loopback port.',
  'failure.malformed': 'Bench Core replied with an unsupported status response.',
  'failure.malformed.advice':
    'The endpoint answered, but not with a status document this interface understands. Check that the backend and this interface are from the same revision.',
  'failure.tryAgain': 'Try again',
  'failure.inventory.unauthorized': 'The {resource} was refused for this token.',
  'failure.inventory.forbidden': 'The {resource} is not permitted for this identity.',
  'failure.inventory.rejected': 'The {resource} request was refused.',
  'failure.inventory.unavailable': 'The {resource} is not reachable.',
  'failure.inventory.malformed': 'The {resource} could not be read.',
  'failure.resource.integrations': 'integration inventory',
  'failure.resource.devices': 'device inventory',

  // Network
  'network.title': 'Network',
  'network.subtitle': 'Appliance network',
  'network.summary.open': 'Open Network',
  'network.unavailable.title': 'Appliance network reporting is not yet available',
  'network.unavailable.body':
    'Bench Core exposes no interface, address, link-speed or traffic data on its versioned API, so this interface has nothing measured to show. Nothing here is inferred and no illustrative value is displayed.',
  'network.reachability.title': 'Bench Core reachability',
  'network.reachability.reachable':
    'Bench Core answered this interface over HTTP. That is the state of this one connection only — it is not the appliance LAN, Wi-Fi or VPN state, and it says nothing about whether any remote endpoint is reachable.',
  'network.reachability.unreachable':
    'Bench Core did not answer this interface over HTTP. That is the state of this one connection only, and it says nothing about the appliance LAN, Wi-Fi or VPN state.',
  'network.table.interface': 'Interface',
  'network.table.state': 'State',
  'network.table.address': 'IP address',
  'network.table.link': 'Link',
  'network.table.rx': 'Current RX',
  'network.table.tx': 'Current TX',
  'network.interface.lan': 'LAN',
  'network.interface.wifi': 'Wi-Fi',
  'network.interface.vpn': 'VPN',
  'network.notReported': 'Not reported',
  'network.followUp':
    'Appliance network telemetry, VPN provisioning and network configuration are tracked as later work; none of it is implemented here.',

  // Pages that are not built yet
  'page.notAvailable.title': '{name} is not available yet',
  'page.plugins.body':
    'Plugin discovery, installation, signing, isolation and trust policy are not implemented. This entry names where that management will live; nothing loads third-party code today.',
  'page.settings.body':
    'Application, appliance and user preferences are not implemented. Theme and language are chosen from the top row and are the only preferences this interface keeps.',

  // About
  'about.title': 'About',
  'about.build': 'Build',
  'about.component': 'Component',
  'about.value': 'Value',
  'about.name': 'Name',
  'about.version': 'Version',
  'about.apiVersion': 'API version',
  'about.integrationProtocol': 'Integration protocol',
  'about.profile': 'Runtime profile',
  'about.licence': 'Licence',
  'about.licence.body':
    'PolyForm Noncommercial License 1.0.0. Commercial use requires a separate commercial licence.',
  'about.unavailable':
    'Build information is unavailable because the backend status could not be read.',

  // Integration page
  'integration.identifier': 'Identifier',
  'integration.displayName': 'Display name',
  'integration.state': 'State',
  'integration.operations': 'Operations',
  'integration.devices': 'Devices',
  'integration.noDevices': 'This integration reports no devices.',
  'integration.devicesUnavailable':
    'The device inventory could not be read, so it is not known which devices this integration owns.',
  'integration.simulated': 'Simulated',

  // Authentication page
  'auth.title': 'Authentication',
  'auth.signedInAs': 'Signed in as {username} ({role}).',
  'auth.identitiesNote':
    'Identities are disabled rather than deleted, so audit records keep naming a real subject.',
  'auth.csrfMissing':
    'This browser no longer holds the request-forgery token issued with the session, so Bench Core will refuse any change. Sign out and sign in again.',
  'auth.users': 'Users',
  'auth.tokens': 'API Tokens',
  'auth.newUser': 'New user',
  'auth.newToken': 'New token',
  'auth.reading': 'Reading the {resource}…',
  'auth.readFailed': 'The {resource} could not be read.',
  'auth.resource.identities': 'identity listing',
  'auth.resource.tokens': 'token listing',
  'auth.noIdentities': 'No identities exist yet.',
  'auth.noTokens': 'No API tokens exist yet.',
  'auth.column.username': 'Username',
  'auth.column.role': 'Role',
  'auth.column.state': 'State',
  'auth.column.actions': 'Actions',
  'auth.column.name': 'Name',
  'auth.column.owner': 'Owner',
  'auth.column.scopes': 'Scopes',
  'auth.column.expires': 'Expires',
  'auth.enabled': 'enabled',
  'auth.disabled': 'disabled',
  'auth.active': 'active',
  'auth.revoked': 'revoked',
  'auth.never': 'never',
  'auth.wholeRole': "owner's whole role",
  'auth.changeRole': 'Change role',
  'auth.disable': 'Disable',
  'auth.enable': 'Enable',
  'auth.resetPassword': 'Reset password',
  'auth.revoke': 'Revoke',
  'auth.revokeToken': 'Revoke token',
  'auth.createIdentity': 'Create identity',
  'auth.createToken': 'Create token',
  'auth.cancel': 'Cancel',
  'auth.working': 'Working…',
  'auth.done': 'Done',
  'auth.copySecret': 'Copy secret',

  // Authentication dialogs
  'auth.dialog.newUser': 'New user',
  'auth.dialog.changeRole': 'Change the role of {username}',
  'auth.dialog.enableUser': 'Enable {username}',
  'auth.dialog.disableUser': 'Disable {username}',
  'auth.dialog.resetPassword': 'Reset the password of {username}',
  'auth.dialog.newToken': 'New API token',
  'auth.dialog.revokeToken': 'Revoke {name}',
  'auth.dialog.secret': 'The secret for {name}',
  'auth.field.username': 'Username',
  'auth.field.role': 'Role',
  'auth.field.password': 'Password',
  'auth.field.confirmPassword': 'Confirm password',
  'auth.field.owner': 'Owner',
  'auth.field.name': 'Name',
  'auth.field.scopes': 'Scopes',
  'auth.field.expires': 'Expires (optional)',
  'auth.passwordMismatch': 'The two passwords do not match.',
  'auth.expiryUnreadable': 'The expiry is not a date and time this interface can read.',
  'auth.roleExplanation':
    '{username} is currently a {role}. A role change applies to that identity’s very next request.',
  'auth.scopeExplanation':
    'A token’s permissions are its scopes narrowed by its owner’s current role, so a token can never exceed that role. Leave every scope unselected to give the token the owner’s whole role.',
  'auth.resetExplanation':
    'This replaces the password of {username} and ends every browser session it holds. Nobody can read the previous password back.',
  'auth.disableExplanation':
    'Disable {username}? Every browser session it holds ends at once, and its API tokens stop working.',
  'auth.enableExplanation':
    'Re-enable {username}? It will be able to sign in again immediately.',
  'auth.revokeExplanation':
    'Revoke {name}, owned by {owner}? Its secret stops working on the next request and cannot be restored.',
  'auth.secretExplanation':
    'This is the only time the secret for {name} is shown. Bench Core stores only a one-way verifier of it, so it cannot be retrieved later. Copy it now.',
  'auth.secretCopied': 'The token secret is on the clipboard.',
  'auth.secretCopyFailed':
    'Copying failed. Select the value above and copy it by hand.',

  // Authentication outcomes
  'auth.created': 'Created {username} as {role}.',
  'auth.roleChanged': '{username} is now a {role}.',
  'auth.stateChanged': '{username} is now {state}.',
  'auth.passwordReset': 'The password of {username} was replaced.',
  'auth.tokenCreated': 'Created the token {name} for {owner}.',
  'auth.tokenRevoked': 'Revoked the token {name}.',
  'auth.sessionEnded': 'Your session ended before this could be applied.',

  // Mode note
  'mode.mock':
    'Answers come from this interface’s own contract mock; no backend process is contacted.',
  'mode.live': 'Requests go to the configured Bench Core development endpoint.',
} as const;

/** Every key the interface may ask for. */
export type MessageKey = keyof typeof en;

/** The shape each translation has to fill completely. */
export type Catalogue = Record<MessageKey, string>;
