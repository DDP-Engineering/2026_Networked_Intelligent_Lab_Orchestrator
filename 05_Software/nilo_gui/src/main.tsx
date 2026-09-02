import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';

import App from './App';
import {
  createConfiguredClient,
  readGuiEnvironment,
  type EnvironmentRecord,
} from './config/environment';
import { LanguageProvider } from './i18n/LanguageProvider';
import { ThemeProvider } from './theme/ThemeProvider';
import './index.css';

const root = document.getElementById('root');

if (root === null) {
  throw new Error('NILO GUI root element is missing');
}

// A configuration error is raised here, before anything renders, so a wrong
// VITE_NILO_* value fails loudly instead of silently falling back.
const environment = import.meta.env as unknown as EnvironmentRecord;
const resolved = readGuiEnvironment(environment);
const client = createConfiguredClient(environment);

createRoot(root).render(
  <StrictMode>
    <ThemeProvider>
      <LanguageProvider>
        <App
          client={client}
          apiMode={resolved.apiMode}
          endpoint={resolved.apiBaseUrl}
        />
      </LanguageProvider>
    </ThemeProvider>
  </StrictMode>,
);
