import { NotificationProvider, MessageBanner } from '../components/MessageBanner';
import { createRoot } from 'react-dom/client';
import { createBenchCoreClient, sessionCredential } from '../api/client';
import { LanguageProvider } from '../i18n/LanguageProvider';
import { ThemeProvider } from '../theme/ThemeProvider';
import { SerialWorkspacePage } from '../components/pages/SerialWorkspacePage';
import '../index.css';
const view =
  new URLSearchParams(location.search).get('view') === 'sessions' ? 'sessions' : 'live';
createRoot(document.getElementById('root')!).render(
  <ThemeProvider initial="light">
    <LanguageProvider initial="en">
      <main style={{ height: '100dvh', minWidth: 0, position: 'relative' }}>
        <NotificationProvider>
          {new URLSearchParams(location.search).has('notice') && (
            <MessageBanner tone="warning">Test notification</MessageBanner>
          )}
          <SerialWorkspacePage
            view={view}
            client={createBenchCoreClient({ baseUrl: '' })}
            credential={sessionCredential('test-csrf')}
            session={{
              apiVersion: 'nilo.http/1',
              username: 'admin',
              role: 'administrator',
              permissions: ['bench.read', 'bench.execute', 'plugins.manage'],
              credential: 'session',
              csrfToken: 'test-csrf',
            }}
            device={{
              id: 'serial-1',
              display_name: 'USB console',
              revision: 1,
              enabled: true,
              configured: true,
              presence: 'present',
              readiness: 'ready',
            }}
            onSessionEnded={() => {}}
          />
        </NotificationProvider>
      </main>
    </LanguageProvider>
  </ThemeProvider>,
);
