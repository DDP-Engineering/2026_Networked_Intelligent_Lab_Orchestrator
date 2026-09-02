import type { Catalogue } from './en';

/**
 * The Romanian catalogue.
 *
 * Typed as the complete key set, so a key added to English and forgotten here
 * fails the build rather than reaching a user as an English sentence in a
 * Romanian interface. Product and protocol nouns — NILO, Bench Core, SIM, API,
 * RX, TX — are deliberately left as they are.
 */
export const ro: Catalogue = {
  'app.name': 'NILO',
  'app.title': 'Interfața de dezvoltare NILO',
  'nav.label': 'Navigare principală',
  'nav.section.working': 'Spațiu de lucru',
  'nav.section.instruments': 'Instrumente',
  'nav.section.core': 'Sistem',
  'nav.bench': 'Bench',
  'nav.network': 'Rețea',
  'nav.plugins': 'Plugin-uri',
  'nav.authentication': 'Autentificare',
  'nav.settings': 'Setări',
  'nav.about': 'Despre',
  'nav.collapse': 'Restrânge navigarea',
  'nav.expand': 'Extinde navigarea',

  'tabs.label': 'Pagini deschise',
  'tabs.close': 'Închide {name}',
  'tabs.none.title': 'Nicio pagină deschisă',
  'tabs.none.body': 'Alegeți o pagină din navigarea din stânga.',

  'top.simulation': 'SIM',
  'top.simulation.label': 'Profil de execuție: simulare',
  'top.network': 'Rețea',
  'top.appearance': 'Aspect',
  'top.appearance.light': 'Luminos',
  'top.appearance.dark': 'Întunecat',
  'top.appearance.system': 'Sistem',
  'top.appearance.follows': 'Urmează dispozitivul dumneavoastră',
  'top.language': 'Limbă',
  'top.account': 'Cont',
  'top.signOut': 'Deconectare',
  'top.signedInAs': 'Autentificat ca {username}',

  'boundary.statement':
    'Doar profil de simulare — rezultatele simulate nu reprezintă niciodată dovezi de la echipamente fizice.',

  'signIn.title': 'Autentificare în NILO',
  'signIn.eyebrow': 'Interfață de dezvoltare',
  'signIn.intro':
    'Autentificați-vă cu numele de utilizator și parola unei identități de pe acest aparat. Primul administrator este creat o singură dată la consolă; nu există înregistrare proprie.',
  'signIn.username': 'Nume utilizator',
  'signIn.password': 'Parolă',
  'signIn.submit': 'Autentificare',
  'signIn.working': 'Se autentifică…',
  'signIn.incomplete': 'Introduceți atât numele de utilizator, cât și parola.',
  'signIn.refused': 'Numele de utilizator și parola nu au fost acceptate.',
  'signIn.throttled':
    'Prea multe încercări eșuate pentru acest nume de utilizator. Așteptați expirarea intervalului de blocare și încercați din nou.',
  'signIn.ended': 'Sesiunea dumneavoastră s-a încheiat. Autentificați-vă din nou.',
  'signIn.signedOut': 'Sunteți deconectat.',
  'signIn.token.title': 'Sau conectați-vă cu un token de dezvoltare',
  'signIn.token.intro':
    'Generați unul în WSL2 cu comanda pentru token de dezvoltare. Este păstrat în memorie doar pentru această pagină și citește bench-ul la nivel de Viewer; administrarea identităților necesită un administrator autentificat.',
  'signIn.token.label': 'Token bearer de dezvoltare',
  'signIn.token.submit': 'Conectare',
  'signIn.token.incomplete': 'Introduceți tokenul emis de backendul de dezvoltare.',
  'signIn.endpoint': 'Punct final',

  'restoring.title': 'Se restaurează sesiunea…',
  'restoring.body': 'Se întreabă Bench Core dacă acest browser este deja autentificat.',

  'bench.title': 'Bench',
  'bench.connecting': 'Se contactează Bench Core…',
  'bench.connecting.body':
    'Se așteaptă răspunsurile autentificate pentru stare, integrări și dispozitive.',
  'bench.refresh': 'Reîmprospătare',
  'bench.useDifferentToken': 'Folosiți alt token',
  'bench.signInAgain': 'Autentificare din nou',
  'bench.profile': 'Profil',
  'bench.status': 'Starea serviciului',
  'bench.simulation': 'Simulare',
  'bench.buildVersion': 'Versiune build',
  'bench.apiVersion': 'Versiune API',
  'bench.integrationProtocol': 'Protocol de integrare',
  'bench.yes': 'Da',
  'bench.no': 'Nu',
  'bench.integrations': 'Integrări',
  'bench.devices': 'Dispozitive',
  'bench.entries': '{count} intrări',
  'bench.entry': '{count} intrare',
  'bench.scenario': 'scenariu: {name}',
  'bench.noIntegrations': 'În acest scenariu de simulare nu sunt oferite integrări.',
  'bench.noDevices': 'În acest scenariu de simulare nu sunt oferite dispozitive.',
  'bench.state': 'Stare',
  'bench.operations': 'Operațiuni',
  'bench.availability': 'Disponibilitate',
  'bench.integration': 'Integrare',
  'bench.kind': 'Tip',
  'bench.capabilities': 'Capabilități',

  'failure.unauthorized': 'Bench Core a respins acest token.',
  'failure.unauthorized.advice':
    'Tokenul a fost respins. Generați un nou token de dezvoltare în WSL2 și introduceți-l din nou.',
  'failure.forbidden': 'Această identitate nu poate citi bench-ul.',
  'failure.forbidden.advice':
    'Credențialul este valid, dar identitatea din spatele lui nu deține permisiunea necesară acestui ecran. O nouă autentificare nu schimbă acest lucru; un administrator trebuie să extindă rolul sau permisiunile tokenului.',
  'failure.rejected': 'Bench Core a refuzat cererea.',
  'failure.rejected.advice':
    'Bench Core a înțeles cererea și a refuzat-o. Motivul indicat este mai jos.',
  'failure.unavailable': 'Bench Core nu este accesibil.',
  'failure.unavailable.advice':
    'Nimic nu a răspuns la punctul final configurat. Verificați dacă serverul de simulare rulează în WSL2 și ascultă pe acel port loopback.',
  'failure.malformed': 'Bench Core a răspuns cu o stare neacceptată.',
  'failure.malformed.advice':
    'Punctul final a răspuns, dar nu cu un document de stare pe care această interfață îl înțelege. Verificați dacă backendul și interfața provin din aceeași revizie.',
  'failure.tryAgain': 'Încercați din nou',
  'failure.inventory.unauthorized':
    'Secțiunea „{resource}” a fost respinsă pentru acest token.',
  'failure.inventory.forbidden':
    'Secțiunea „{resource}” nu este permisă pentru această identitate.',
  'failure.inventory.rejected': 'Cererea pentru „{resource}” a fost refuzată.',
  'failure.inventory.unavailable': 'Secțiunea „{resource}” nu este accesibilă.',
  'failure.inventory.malformed': 'Secțiunea „{resource}” nu a putut fi citită.',
  'failure.resource.integrations': 'inventarul de integrări',
  'failure.resource.devices': 'inventarul de dispozitive',

  'network.title': 'Rețea',
  'network.subtitle': 'Rețeaua aparatului',
  'network.summary.open': 'Deschide Rețea',
  'network.unavailable.title': 'Raportarea rețelei aparatului nu este încă disponibilă',
  'network.unavailable.body':
    'Bench Core nu expune prin API-ul său versionat date despre interfețe, adrese, viteza legăturii sau trafic, deci această interfață nu are nimic măsurat de afișat. Nimic de aici nu este dedus și nicio valoare ilustrativă nu este afișată.',
  'network.reachability.title': 'Accesibilitatea Bench Core',
  'network.reachability.reachable':
    'Bench Core a răspuns acestei interfețe prin HTTP. Aceasta este starea exclusiv a acestei conexiuni — nu este starea LAN, Wi-Fi sau VPN a aparatului și nu spune nimic despre accesibilitatea vreunui punct final la distanță.',
  'network.reachability.unreachable':
    'Bench Core nu a răspuns acestei interfețe prin HTTP. Aceasta este starea exclusiv a acestei conexiuni și nu spune nimic despre starea LAN, Wi-Fi sau VPN a aparatului.',
  'network.table.interface': 'Interfață',
  'network.table.state': 'Stare',
  'network.table.address': 'Adresă IP',
  'network.table.link': 'Legătură',
  'network.table.rx': 'RX curent',
  'network.table.tx': 'TX curent',
  'network.interface.lan': 'LAN',
  'network.interface.wifi': 'Wi-Fi',
  'network.interface.vpn': 'VPN',
  'network.notReported': 'Neraportat',
  'network.followUp':
    'Telemetria de rețea a aparatului, furnizarea VPN și configurarea rețelei sunt urmărite ca activități ulterioare; niciuna nu este implementată aici.',

  'page.notAvailable.title': '{name} nu este încă disponibil',
  'page.plugins.body':
    'Descoperirea, instalarea, semnarea, izolarea și politica de încredere pentru plugin-uri nu sunt implementate. Această intrare indică unde va exista acea administrare; astăzi nu se încarcă niciun cod terț.',
  'page.settings.body':
    'Preferințele de aplicație, de aparat și de utilizator nu sunt implementate. Aspectul și limba se aleg din rândul de sus și sunt singurele preferințe pe care această interfață le păstrează.',

  'about.title': 'Despre',
  'about.build': 'Build',
  'about.component': 'Componentă',
  'about.value': 'Valoare',
  'about.name': 'Nume',
  'about.version': 'Versiune',
  'about.apiVersion': 'Versiune API',
  'about.integrationProtocol': 'Protocol de integrare',
  'about.profile': 'Profil de execuție',
  'about.licence': 'Licență',
  'about.licence.body':
    'PolyForm Noncommercial License 1.0.0. Utilizarea comercială necesită o licență comercială separată.',
  'about.unavailable':
    'Informațiile despre build nu sunt disponibile deoarece starea backendului nu a putut fi citită.',

  'integration.identifier': 'Identificator',
  'integration.displayName': 'Nume afișat',
  'integration.state': 'Stare',
  'integration.operations': 'Operațiuni',
  'integration.devices': 'Dispozitive',
  'integration.noDevices': 'Această integrare nu raportează niciun dispozitiv.',
  'integration.devicesUnavailable':
    'Inventarul de dispozitive nu a putut fi citit, deci nu se știe ce dispozitive aparțin acestei integrări.',
  'integration.simulated': 'Simulat',

  'auth.title': 'Autentificare',
  'auth.signedInAs': 'Autentificat ca {username} ({role}).',
  'auth.identitiesNote':
    'Identitățile sunt dezactivate, nu șterse, astfel încât înregistrările de audit să indice în continuare un subiect real.',
  'auth.csrfMissing':
    'Acest browser nu mai deține tokenul anti-falsificare emis odată cu sesiunea, deci Bench Core va refuza orice modificare. Deconectați-vă și autentificați-vă din nou.',
  'auth.users': 'Utilizatori',
  'auth.tokens': 'Tokenuri API',
  'auth.newUser': 'Utilizator nou',
  'auth.newToken': 'Token nou',
  'auth.reading': 'Se citește {resource}…',
  'auth.readFailed': '{resource} nu a putut fi citit.',
  'auth.resource.identities': 'lista de identități',
  'auth.resource.tokens': 'lista de tokenuri',
  'auth.noIdentities': 'Încă nu există identități.',
  'auth.noTokens': 'Încă nu există tokenuri API.',
  'auth.column.username': 'Nume utilizator',
  'auth.column.role': 'Rol',
  'auth.column.state': 'Stare',
  'auth.column.actions': 'Acțiuni',
  'auth.column.name': 'Nume',
  'auth.column.owner': 'Proprietar',
  'auth.column.scopes': 'Permisiuni',
  'auth.column.expires': 'Expiră',
  'auth.enabled': 'activat',
  'auth.disabled': 'dezactivat',
  'auth.active': 'activ',
  'auth.revoked': 'revocat',
  'auth.never': 'niciodată',
  'auth.wholeRole': 'întregul rol al proprietarului',
  'auth.changeRole': 'Schimbă rolul',
  'auth.disable': 'Dezactivează',
  'auth.enable': 'Activează',
  'auth.resetPassword': 'Resetează parola',
  'auth.revoke': 'Revocă',
  'auth.revokeToken': 'Revocă tokenul',
  'auth.createIdentity': 'Creează identitatea',
  'auth.createToken': 'Creează tokenul',
  'auth.cancel': 'Anulează',
  'auth.working': 'Se execută…',
  'auth.done': 'Gata',
  'auth.copySecret': 'Copiază secretul',

  'auth.dialog.newUser': 'Utilizator nou',
  'auth.dialog.changeRole': 'Schimbă rolul lui {username}',
  'auth.dialog.enableUser': 'Activează {username}',
  'auth.dialog.disableUser': 'Dezactivează {username}',
  'auth.dialog.resetPassword': 'Resetează parola lui {username}',
  'auth.dialog.newToken': 'Token API nou',
  'auth.dialog.revokeToken': 'Revocă {name}',
  'auth.dialog.secret': 'Secretul pentru {name}',
  'auth.field.username': 'Nume utilizator',
  'auth.field.role': 'Rol',
  'auth.field.password': 'Parolă',
  'auth.field.confirmPassword': 'Confirmați parola',
  'auth.field.owner': 'Proprietar',
  'auth.field.name': 'Nume',
  'auth.field.scopes': 'Permisiuni',
  'auth.field.expires': 'Expirare (opțional)',
  'auth.passwordMismatch': 'Cele două parole nu coincid.',
  'auth.expiryUnreadable':
    'Expirarea nu este o dată și oră pe care această interfață o poate citi.',
  'auth.roleExplanation':
    '{username} este în prezent {role}. O schimbare de rol se aplică chiar de la următoarea cerere a acelei identități.',
  'auth.scopeExplanation':
    'Permisiunile unui token sunt selecția sa restrânsă de rolul curent al proprietarului, deci un token nu poate depăși niciodată acel rol. Nu selectați nicio permisiune pentru a da tokenului întregul rol al proprietarului.',
  'auth.resetExplanation':
    'Aceasta înlocuiește parola lui {username} și încheie toate sesiunile de browser pe care le deține. Nimeni nu poate citi parola anterioară.',
  'auth.disableExplanation':
    'Dezactivați {username}? Fiecare sesiune de browser a identității se încheie imediat, iar tokenurile sale API nu mai funcționează.',
  'auth.enableExplanation':
    'Reactivați {username}? Identitatea se va putea autentifica imediat.',
  'auth.revokeExplanation':
    'Revocați {name}, deținut de {owner}? Secretul său nu mai funcționează de la următoarea cerere și nu poate fi restaurat.',
  'auth.secretExplanation':
    'Aceasta este singura dată când secretul pentru {name} este afișat. Bench Core păstrează doar o amprentă unidirecțională a lui, deci nu mai poate fi recuperat. Copiați-l acum.',
  'auth.secretCopied': 'Secretul tokenului este în clipboard.',
  'auth.secretCopyFailed':
    'Copierea a eșuat. Selectați valoarea de mai sus și copiați-o manual.',

  'auth.created': '{username} a fost creat ca {role}.',
  'auth.roleChanged': '{username} este acum {role}.',
  'auth.stateChanged': '{username} este acum {state}.',
  'auth.passwordReset': 'Parola lui {username} a fost înlocuită.',
  'auth.tokenCreated': 'A fost creat tokenul {name} pentru {owner}.',
  'auth.tokenRevoked': 'Tokenul {name} a fost revocat.',
  'auth.sessionEnded':
    'Sesiunea dumneavoastră s-a încheiat înainte ca aceasta să fie aplicată.',

  'mode.mock':
    'Răspunsurile provin din mock-ul de contract al acestei interfețe; niciun proces backend nu este contactat.',
  'mode.live': 'Cererile merg către punctul final de dezvoltare Bench Core configurat.',
};
