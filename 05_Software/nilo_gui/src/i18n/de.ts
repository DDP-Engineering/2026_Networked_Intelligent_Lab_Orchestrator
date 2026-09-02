import type { Catalogue } from './en';

/**
 * The German catalogue.
 *
 * Typed as the complete key set, so a key added to English and forgotten here
 * fails the build rather than reaching a user as an English sentence in a German
 * interface. Product and protocol nouns — NILO, Bench Core, SIM, API, RX, TX —
 * are deliberately left as they are.
 */
export const de: Catalogue = {
  'app.name': 'NILO',
  'app.title': 'NILO Entwicklungsoberfläche',
  'nav.label': 'Hauptnavigation',
  'nav.section.working': 'Arbeitsbereich',
  'nav.section.instruments': 'Instrumente',
  'nav.section.core': 'System',
  'nav.bench': 'Bench',
  'nav.network': 'Netzwerk',
  'nav.plugins': 'Plugins',
  'nav.authentication': 'Authentifizierung',
  'nav.settings': 'Einstellungen',
  'nav.about': 'Über',
  'nav.collapse': 'Navigation einklappen',
  'nav.expand': 'Navigation ausklappen',

  'tabs.label': 'Geöffnete Seiten',
  'tabs.close': '{name} schließen',
  'tabs.none.title': 'Keine Seite geöffnet',
  'tabs.none.body': 'Wählen Sie links in der Navigation eine Seite aus.',

  'top.simulation': 'SIM',
  'top.simulation.label': 'Laufzeitprofil: Simulation',
  'top.network': 'Netzwerk',
  'top.appearance': 'Darstellung',
  'top.appearance.light': 'Hell',
  'top.appearance.dark': 'Dunkel',
  'top.appearance.system': 'System',
  'top.appearance.follows': 'Folgt Ihrem Gerät',
  'top.language': 'Sprache',
  'top.account': 'Konto',
  'top.signOut': 'Abmelden',
  'top.signedInAs': 'Angemeldet als {username}',

  'boundary.statement':
    'Nur Simulationsprofil — simulierte Ergebnisse sind niemals ein Nachweis von physischer Ausrüstung.',

  'signIn.title': 'Bei NILO anmelden',
  'signIn.eyebrow': 'Entwicklungsoberfläche',
  'signIn.intro':
    'Melden Sie sich mit Benutzernamen und Passwort einer Identität auf diesem Gerät an. Die erste Administratorin oder der erste Administrator wird einmalig an der Konsole angelegt; eine Selbstregistrierung gibt es nicht.',
  'signIn.username': 'Benutzername',
  'signIn.password': 'Passwort',
  'signIn.submit': 'Anmelden',
  'signIn.working': 'Anmeldung läuft…',
  'signIn.incomplete': 'Geben Sie Benutzernamen und Passwort ein.',
  'signIn.refused': 'Benutzername und Passwort wurden nicht akzeptiert.',
  'signIn.throttled':
    'Zu viele Fehlversuche für diesen Benutzernamen. Warten Sie das Sperrfenster ab und versuchen Sie es erneut.',
  'signIn.ended': 'Ihre Sitzung ist beendet. Melden Sie sich erneut an.',
  'signIn.signedOut': 'Sie sind abgemeldet.',
  'signIn.token.title': 'Oder mit einem Entwicklungs-Token verbinden',
  'signIn.token.intro':
    'Erzeugen Sie eines in WSL2 mit dem Entwicklungs-Token-Befehl. Es bleibt nur für diese Seite im Speicher und liest die Bench auf Viewer-Ebene; die Identitätsverwaltung erfordert eine angemeldete Administratorin oder einen angemeldeten Administrator.',
  'signIn.token.label': 'Entwicklungs-Bearer-Token',
  'signIn.token.submit': 'Verbinden',
  'signIn.token.incomplete':
    'Geben Sie das vom Entwicklungs-Backend erzeugte Token ein.',
  'signIn.endpoint': 'Endpunkt',

  'restoring.title': 'Sitzung wird wiederhergestellt…',
  'restoring.body':
    'Bench Core wird gefragt, ob dieser Browser bereits angemeldet ist.',

  'bench.title': 'Bench',
  'bench.connecting': 'Bench Core wird kontaktiert…',
  'bench.connecting.body':
    'Warten auf die authentifizierten Status-, Integrations- und Geräteantworten.',
  'bench.refresh': 'Aktualisieren',
  'bench.useDifferentToken': 'Anderes Token verwenden',
  'bench.signInAgain': 'Erneut anmelden',
  'bench.profile': 'Profil',
  'bench.status': 'Dienststatus',
  'bench.simulation': 'Simulation',
  'bench.buildVersion': 'Build-Version',
  'bench.apiVersion': 'API-Version',
  'bench.integrationProtocol': 'Integrationsprotokoll',
  'bench.yes': 'Ja',
  'bench.no': 'Nein',
  'bench.integrations': 'Integrationen',
  'bench.devices': 'Geräte',
  'bench.entries': '{count} Einträge',
  'bench.entry': '{count} Eintrag',
  'bench.scenario': 'Szenario: {name}',
  'bench.noIntegrations':
    'In diesem Simulationsszenario werden keine Integrationen angeboten.',
  'bench.noDevices': 'In diesem Simulationsszenario werden keine Geräte angeboten.',
  'bench.state': 'Zustand',
  'bench.operations': 'Operationen',
  'bench.availability': 'Verfügbarkeit',
  'bench.integration': 'Integration',
  'bench.kind': 'Art',
  'bench.capabilities': 'Fähigkeiten',

  'failure.unauthorized': 'Bench Core hat dieses Token abgelehnt.',
  'failure.unauthorized.advice':
    'Das Token wurde abgelehnt. Erzeugen Sie in WSL2 ein neues Entwicklungs-Token und geben Sie es erneut ein.',
  'failure.forbidden': 'Diese Identität darf die Bench nicht lesen.',
  'failure.forbidden.advice':
    'Die Anmeldeinformation ist gültig, aber die dahinterstehende Identität besitzt die für diese Ansicht nötige Berechtigung nicht. Eine erneute Anmeldung ändert das nicht; eine Administratorin oder ein Administrator muss die Rolle oder die Token-Berechtigungen erweitern.',
  'failure.rejected': 'Bench Core hat die Anfrage abgelehnt.',
  'failure.rejected.advice':
    'Bench Core hat die Anfrage verstanden und abgelehnt. Die angegebene Begründung steht unten.',
  'failure.unavailable': 'Bench Core ist nicht erreichbar.',
  'failure.unavailable.advice':
    'Auf dem konfigurierten Endpunkt hat nichts geantwortet. Prüfen Sie, ob der Simulationsserver in WSL2 läuft und auf diesem Loopback-Port lauscht.',
  'failure.malformed':
    'Bench Core hat mit einer nicht unterstützten Statusantwort geantwortet.',
  'failure.malformed.advice':
    'Der Endpunkt hat geantwortet, aber nicht mit einem Statusdokument, das diese Oberfläche versteht. Prüfen Sie, ob Backend und Oberfläche vom selben Stand sind.',
  'failure.tryAgain': 'Erneut versuchen',
  'failure.inventory.unauthorized':
    'Der Bereich „{resource}“ wurde für dieses Token abgelehnt.',
  'failure.inventory.forbidden':
    'Der Bereich „{resource}“ ist für diese Identität nicht erlaubt.',
  'failure.inventory.rejected': 'Die Anfrage für „{resource}“ wurde abgelehnt.',
  'failure.inventory.unavailable': 'Der Bereich „{resource}“ ist nicht erreichbar.',
  'failure.inventory.malformed':
    'Der Bereich „{resource}“ konnte nicht gelesen werden.',
  'failure.resource.integrations': 'Integrationsverzeichnis',
  'failure.resource.devices': 'Geräteverzeichnis',

  'network.title': 'Netzwerk',
  'network.subtitle': 'Gerätenetzwerk',
  'network.summary.open': 'Netzwerk öffnen',
  'network.unavailable.title':
    'Die Netzwerkauskunft des Geräts ist noch nicht verfügbar',
  'network.unavailable.body':
    'Bench Core stellt über seine versionierte API keine Schnittstellen-, Adress-, Verbindungsgeschwindigkeits- oder Verkehrsdaten bereit; diese Oberfläche hat also nichts Gemessenes zu zeigen. Nichts hier wird abgeleitet und kein Beispielwert wird angezeigt.',
  'network.reachability.title': 'Erreichbarkeit von Bench Core',
  'network.reachability.reachable':
    'Bench Core hat dieser Oberfläche über HTTP geantwortet. Das ist ausschließlich der Zustand dieser einen Verbindung — es ist nicht der LAN-, WLAN- oder VPN-Zustand des Geräts und sagt nichts darüber aus, ob ein entfernter Endpunkt erreichbar ist.',
  'network.reachability.unreachable':
    'Bench Core hat dieser Oberfläche nicht über HTTP geantwortet. Das ist ausschließlich der Zustand dieser einen Verbindung und sagt nichts über den LAN-, WLAN- oder VPN-Zustand des Geräts aus.',
  'network.table.interface': 'Schnittstelle',
  'network.table.state': 'Zustand',
  'network.table.address': 'IP-Adresse',
  'network.table.link': 'Verbindung',
  'network.table.rx': 'Aktuell RX',
  'network.table.tx': 'Aktuell TX',
  'network.interface.lan': 'LAN',
  'network.interface.wifi': 'WLAN',
  'network.interface.vpn': 'VPN',
  'network.notReported': 'Nicht gemeldet',
  'network.followUp':
    'Netzwerktelemetrie des Geräts, VPN-Bereitstellung und Netzwerkkonfiguration sind als spätere Arbeit vorgemerkt; nichts davon ist hier umgesetzt.',

  'page.notAvailable.title': '{name} ist noch nicht verfügbar',
  'page.plugins.body':
    'Plugin-Erkennung, -Installation, -Signierung, -Isolierung und Vertrauensrichtlinien sind nicht umgesetzt. Dieser Eintrag benennt, wo diese Verwaltung liegen wird; heute wird kein Fremdcode geladen.',
  'page.settings.body':
    'Anwendungs-, Geräte- und Benutzereinstellungen sind nicht umgesetzt. Darstellung und Sprache werden in der oberen Zeile gewählt und sind die einzigen Einstellungen, die diese Oberfläche speichert.',

  'about.title': 'Über',
  'about.build': 'Build',
  'about.component': 'Komponente',
  'about.value': 'Wert',
  'about.name': 'Name',
  'about.version': 'Version',
  'about.apiVersion': 'API-Version',
  'about.integrationProtocol': 'Integrationsprotokoll',
  'about.profile': 'Laufzeitprofil',
  'about.licence': 'Lizenz',
  'about.licence.body':
    'PolyForm Noncommercial License 1.0.0. Kommerzielle Nutzung erfordert eine gesonderte kommerzielle Lizenz.',
  'about.unavailable':
    'Build-Informationen sind nicht verfügbar, weil der Backend-Status nicht gelesen werden konnte.',

  'integration.identifier': 'Kennung',
  'integration.displayName': 'Anzeigename',
  'integration.state': 'Zustand',
  'integration.operations': 'Operationen',
  'integration.devices': 'Geräte',
  'integration.noDevices': 'Diese Integration meldet keine Geräte.',
  'integration.devicesUnavailable':
    'Das Geräteverzeichnis konnte nicht gelesen werden, daher ist unbekannt, welche Geräte zu dieser Integration gehören.',
  'integration.simulated': 'Simuliert',

  'auth.title': 'Authentifizierung',
  'auth.signedInAs': 'Angemeldet als {username} ({role}).',
  'auth.identitiesNote':
    'Identitäten werden deaktiviert statt gelöscht, damit Auditeinträge weiterhin ein reales Subjekt benennen.',
  'auth.csrfMissing':
    'Dieser Browser besitzt das mit der Sitzung ausgegebene CSRF-Token nicht mehr, daher lehnt Bench Core jede Änderung ab. Melden Sie sich ab und erneut an.',
  'auth.users': 'Benutzer',
  'auth.tokens': 'API-Token',
  'auth.newUser': 'Neuer Benutzer',
  'auth.newToken': 'Neues Token',
  'auth.reading': '{resource} wird gelesen…',
  'auth.readFailed': '{resource} konnte nicht gelesen werden.',
  'auth.resource.identities': 'Identitätsliste',
  'auth.resource.tokens': 'Token-Liste',
  'auth.noIdentities': 'Es existieren noch keine Identitäten.',
  'auth.noTokens': 'Es existieren noch keine API-Token.',
  'auth.column.username': 'Benutzername',
  'auth.column.role': 'Rolle',
  'auth.column.state': 'Zustand',
  'auth.column.actions': 'Aktionen',
  'auth.column.name': 'Name',
  'auth.column.owner': 'Eigentümer',
  'auth.column.scopes': 'Berechtigungen',
  'auth.column.expires': 'Läuft ab',
  'auth.enabled': 'aktiviert',
  'auth.disabled': 'deaktiviert',
  'auth.active': 'aktiv',
  'auth.revoked': 'widerrufen',
  'auth.never': 'nie',
  'auth.wholeRole': 'gesamte Rolle des Eigentümers',
  'auth.changeRole': 'Rolle ändern',
  'auth.disable': 'Deaktivieren',
  'auth.enable': 'Aktivieren',
  'auth.resetPassword': 'Passwort zurücksetzen',
  'auth.revoke': 'Widerrufen',
  'auth.revokeToken': 'Token widerrufen',
  'auth.createIdentity': 'Identität anlegen',
  'auth.createToken': 'Token anlegen',
  'auth.cancel': 'Abbrechen',
  'auth.working': 'Wird ausgeführt…',
  'auth.done': 'Fertig',
  'auth.copySecret': 'Geheimnis kopieren',

  'auth.dialog.newUser': 'Neuer Benutzer',
  'auth.dialog.changeRole': 'Rolle von {username} ändern',
  'auth.dialog.enableUser': '{username} aktivieren',
  'auth.dialog.disableUser': '{username} deaktivieren',
  'auth.dialog.resetPassword': 'Passwort von {username} zurücksetzen',
  'auth.dialog.newToken': 'Neues API-Token',
  'auth.dialog.revokeToken': '{name} widerrufen',
  'auth.dialog.secret': 'Das Geheimnis für {name}',
  'auth.field.username': 'Benutzername',
  'auth.field.role': 'Rolle',
  'auth.field.password': 'Passwort',
  'auth.field.confirmPassword': 'Passwort bestätigen',
  'auth.field.owner': 'Eigentümer',
  'auth.field.name': 'Name',
  'auth.field.scopes': 'Berechtigungen',
  'auth.field.expires': 'Ablauf (optional)',
  'auth.passwordMismatch': 'Die beiden Passwörter stimmen nicht überein.',
  'auth.expiryUnreadable':
    'Der Ablaufzeitpunkt ist kein Datum mit Uhrzeit, das diese Oberfläche lesen kann.',
  'auth.roleExplanation':
    '{username} ist derzeit {role}. Eine Rollenänderung gilt bereits für die nächste Anfrage dieser Identität.',
  'auth.scopeExplanation':
    'Die Berechtigungen eines Tokens sind seine Auswahl, eingeschränkt durch die aktuelle Rolle des Eigentümers; ein Token kann diese Rolle daher nie überschreiten. Wählen Sie keine Berechtigung aus, um dem Token die gesamte Rolle des Eigentümers zu geben.',
  'auth.resetExplanation':
    'Dies ersetzt das Passwort von {username} und beendet jede Browsersitzung dieser Identität. Das bisherige Passwort kann niemand zurücklesen.',
  'auth.disableExplanation':
    '{username} deaktivieren? Jede Browsersitzung dieser Identität endet sofort und ihre API-Token funktionieren nicht mehr.',
  'auth.enableExplanation':
    '{username} wieder aktivieren? Die Identität kann sich sofort wieder anmelden.',
  'auth.revokeExplanation':
    '{name} widerrufen, Eigentümer {owner}? Das Geheimnis funktioniert ab der nächsten Anfrage nicht mehr und lässt sich nicht wiederherstellen.',
  'auth.secretExplanation':
    'Dies ist das einzige Mal, dass das Geheimnis für {name} angezeigt wird. Bench Core speichert davon nur eine Einwegprüfsumme, es lässt sich später nicht mehr abrufen. Kopieren Sie es jetzt.',
  'auth.secretCopied': 'Das Token-Geheimnis liegt in der Zwischenablage.',
  'auth.secretCopyFailed':
    'Kopieren fehlgeschlagen. Markieren Sie den Wert oben und kopieren Sie ihn von Hand.',

  'auth.created': '{username} als {role} angelegt.',
  'auth.roleChanged': '{username} ist jetzt {role}.',
  'auth.stateChanged': '{username} ist jetzt {state}.',
  'auth.passwordReset': 'Das Passwort von {username} wurde ersetzt.',
  'auth.tokenCreated': 'Token {name} für {owner} angelegt.',
  'auth.tokenRevoked': 'Token {name} widerrufen.',
  'auth.sessionEnded': 'Ihre Sitzung endete, bevor dies angewendet werden konnte.',

  'mode.mock':
    'Die Antworten stammen aus dem eigenen Vertrags-Mock dieser Oberfläche; es wird kein Backend-Prozess kontaktiert.',
  'mode.live': 'Anfragen gehen an den konfigurierten Bench-Core-Entwicklungsendpunkt.',
};
