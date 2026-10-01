export const platformEn = {
  'platform.title': 'Nilo Settings',
  'platform.cameraTitle': 'Camera settings',
  'platform.serialTitle': 'Serial settings',
  'platform.column.setting': 'Setting',
  'platform.column.range': 'Allowed range',
  'platform.column.value': 'Value',
  'platform.column.usage': 'Usage',
  'platform.help.approvalPolicy':
    'Ask requires approval before each operation. Auto allows validated operations without another prompt.',
  'platform.help.updatePlugin':
    'Install the newer verified plugin package advertised by this appliance.',
  'platform.help.discoverPlugin':
    'Scan connected hardware for devices managed by this plugin.',
  'platform.reading': 'Reading settings…',
  'platform.readFailed': 'Settings could not be read.',
  'platform.invalid':
    'Enter whole numbers within the shown ranges. The default job timeout cannot exceed the maximum, and serial idle time cannot exceed its lifetime.',
  'platform.conflict':
    'Settings changed in another session. Reload this page before saving.',
  'platform.saveFailed': 'Settings could not be saved.',
  'platform.saved': 'Settings saved.',
  'platform.save': 'Save settings',
  'platform.usage': '{used} of {limit} in use',
  'platform.unit.jobs': 'jobs',
  'platform.unit.connections': 'connections',
  'platform.unit.streams': 'streams',
  'platform.unit.seconds': 'seconds',
  'platform.unit.minutes': 'minutes',
  'platform.executingJobs': 'Executing finite jobs',
  'platform.outstandingJobs': 'Outstanding jobs',
  'platform.serialConnections': 'Active serial connections',
  'platform.cameraStreams': 'Active camera streams',
  'platform.defaultJobTimeout': 'Default job timeout',
  'platform.maxJobTimeout': 'Maximum job timeout',
  'platform.serialLifetime': 'Serial connection lifetime',
  'platform.serialIdle': 'Serial idle disconnect',
  'platform.cameraLifetime': 'Camera session lifetime',
  'platform.help.executingJobs': 'Finite jobs executing across all plugins and users.',
  'platform.help.outstandingJobs':
    'Jobs pending approval, queued or running across the appliance.',
  'platform.help.serialConnections': 'Connected serial ports across users and devices.',
  'platform.help.cameraStreams': 'Camera streams across users and cameras.',
  'platform.help.defaultJobTimeout':
    'Deadline for a job when its client omits a timeout; includes approval and queue time.',
  'platform.help.maxJobTimeout':
    'Largest timeout a client may request for one finite job.',
  'platform.help.serialLifetime': 'Maximum age of a new serial connection.',
  'platform.help.serialIdle':
    'Disconnect an unrecorded serial connection after this long without access.',
  'platform.help.cameraLifetime':
    'Maximum age of a new camera session; its runner receives one extra minute for cleanup.',
} as const;

export const platformDe: Record<keyof typeof platformEn, string> = {
  'platform.title': 'Nilo-Einstellungen',
  'platform.cameraTitle': 'Kameraeinstellungen',
  'platform.serialTitle': 'Serielle Einstellungen',
  'platform.column.setting': 'Einstellung',
  'platform.column.range': 'Erlaubter Bereich',
  'platform.column.value': 'Wert',
  'platform.column.usage': 'Nutzung',
  'platform.help.approvalPolicy':
    'Fragen erfordert vor jedem Vorgang eine Genehmigung. Automatisch erlaubt geprüfte Vorgänge ohne weitere Nachfrage.',
  'platform.help.updatePlugin':
    'Das neuere, von diesem Gerät geprüfte Plugin-Paket installieren.',
  'platform.help.discoverPlugin':
    'Angeschlossene Hardware nach Geräten dieses Plugins durchsuchen.',
  'platform.reading': 'Einstellungen werden gelesen…',
  'platform.readFailed': 'Einstellungen konnten nicht gelesen werden.',
  'platform.invalid':
    'Ganze Zahlen innerhalb der angezeigten Bereiche eingeben. Der Standard-Timeout darf den Höchstwert nicht überschreiten; die serielle Leerlaufzeit darf die Laufzeit nicht überschreiten.',
  'platform.conflict':
    'Die Einstellungen wurden in einer anderen Sitzung geändert. Laden Sie die Seite vor dem Speichern neu.',
  'platform.saveFailed': 'Einstellungen konnten nicht gespeichert werden.',
  'platform.saved': 'Einstellungen gespeichert.',
  'platform.save': 'Einstellungen speichern',
  'platform.usage': '{used} von {limit} belegt',
  'platform.unit.jobs': 'Jobs',
  'platform.unit.connections': 'Verbindungen',
  'platform.unit.streams': 'Streams',
  'platform.unit.seconds': 'Sekunden',
  'platform.unit.minutes': 'Minuten',
  'platform.executingJobs': 'Ausgeführte endliche Jobs',
  'platform.outstandingJobs': 'Offene Jobs',
  'platform.serialConnections': 'Aktive serielle Verbindungen',
  'platform.cameraStreams': 'Aktive Kamerastreams',
  'platform.defaultJobTimeout': 'Standard-Job-Timeout',
  'platform.maxJobTimeout': 'Maximaler Job-Timeout',
  'platform.serialLifetime': 'Laufzeit serieller Verbindungen',
  'platform.serialIdle': 'Serieller Leerlaufabbruch',
  'platform.cameraLifetime': 'Laufzeit der Kamerasitzung',
  'platform.help.executingJobs':
    'Gleichzeitig ausgeführte endliche Jobs über alle Plugins und Benutzer.',
  'platform.help.outstandingJobs':
    'Jobs mit ausstehender Genehmigung, in Warteschlange oder in Ausführung.',
  'platform.help.serialConnections':
    'Verbundene serielle Ports über Benutzer und Geräte hinweg.',
  'platform.help.cameraStreams': 'Kamerastreams über Benutzer und Kameras hinweg.',
  'platform.help.defaultJobTimeout':
    'Frist, wenn der Client keinen Timeout angibt; enthält Genehmigungs- und Wartezeit.',
  'platform.help.maxJobTimeout':
    'Größter Timeout, den ein Client für einen endlichen Job anfordern darf.',
  'platform.help.serialLifetime': 'Höchstalter einer neuen seriellen Verbindung.',
  'platform.help.serialIdle':
    'Nicht aufgezeichnete Verbindung nach dieser Zeit ohne Zugriff trennen.',
  'platform.help.cameraLifetime':
    'Höchstalter einer neuen Kamerasitzung; der Runner erhält eine weitere Minute zum Aufräumen.',
};

export const platformRo: Record<keyof typeof platformEn, string> = {
  'platform.title': 'Setări Nilo',
  'platform.cameraTitle': 'Setări cameră',
  'platform.serialTitle': 'Setări seriale',
  'platform.column.setting': 'Setare',
  'platform.column.range': 'Interval permis',
  'platform.column.value': 'Valoare',
  'platform.column.usage': 'Utilizare',
  'platform.help.approvalPolicy':
    'Întreabă cere aprobare înaintea fiecărei operații. Automat permite operații validate fără o nouă confirmare.',
  'platform.help.updatePlugin':
    'Instalează pachetul plugin mai nou, verificat de acest aparat.',
  'platform.help.discoverPlugin':
    'Caută în hardware-ul conectat dispozitive administrate de acest plugin.',
  'platform.reading': 'Se citesc setările…',
  'platform.readFailed': 'Setările nu au putut fi citite.',
  'platform.invalid':
    'Introduceți numere întregi în intervalele afișate. Timpul implicit nu poate depăși maximul, iar inactivitatea serială nu poate depăși durata conexiunii.',
  'platform.conflict':
    'Setările au fost modificate în altă sesiune. Reîncărcați pagina înainte de salvare.',
  'platform.saveFailed': 'Setările nu au putut fi salvate.',
  'platform.saved': 'Setările au fost salvate.',
  'platform.save': 'Salvează setările',
  'platform.usage': '{used} din {limit} în uz',
  'platform.unit.jobs': 'joburi',
  'platform.unit.connections': 'conexiuni',
  'platform.unit.streams': 'fluxuri',
  'platform.unit.seconds': 'secunde',
  'platform.unit.minutes': 'minute',
  'platform.executingJobs': 'Joburi finite în execuție',
  'platform.outstandingJobs': 'Joburi active sau în așteptare',
  'platform.serialConnections': 'Conexiuni seriale active',
  'platform.cameraStreams': 'Fluxuri de cameră active',
  'platform.defaultJobTimeout': 'Timp implicit pentru job',
  'platform.maxJobTimeout': 'Timp maxim pentru job',
  'platform.serialLifetime': 'Durata conexiunii seriale',
  'platform.serialIdle': 'Deconectare serială la inactivitate',
  'platform.cameraLifetime': 'Durata sesiunii camerei',
  'platform.help.executingJobs':
    'Joburi finite executate simultan pentru toți utilizatorii și toate pluginurile.',
  'platform.help.outstandingJobs':
    'Joburi în aprobare, în coadă sau în execuție pe aparat.',
  'platform.help.serialConnections':
    'Porturi seriale conectate pentru toți utilizatorii și dispozitivele.',
  'platform.help.cameraStreams':
    'Fluxuri active pentru toate camerele și utilizatorii.',
  'platform.help.defaultJobTimeout':
    'Termen când clientul nu indică un timp; include aprobarea și așteptarea în coadă.',
  'platform.help.maxJobTimeout':
    'Cel mai mare timp pe care îl poate cere un client pentru un job finit.',
  'platform.help.serialLifetime': 'Vârsta maximă a unei conexiuni seriale noi.',
  'platform.help.serialIdle':
    'Deconectează o conexiune neînregistrată după această perioadă fără acces.',
  'platform.help.cameraLifetime':
    'Vârsta maximă a unei sesiuni noi; runnerul primește încă un minut pentru închidere.',
};
