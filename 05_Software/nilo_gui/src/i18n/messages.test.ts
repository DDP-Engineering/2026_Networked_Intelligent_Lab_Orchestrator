import { describe, expect, it } from 'vitest';

import { de } from './de';
import { en } from './en';
import { LANGUAGES } from './languages';
import { ro } from './ro';

/**
 * One catalogue per language, with the same keys in each.
 *
 * The build already enforces this — `de` and `ro` are declared as the complete
 * key set, so a key added to English and forgotten elsewhere is a type error.
 * These assertions make the same rule visible in a test run, and catch the
 * opposite mistake the type cannot: a key that exists everywhere but was left
 * untranslated.
 */

const CATALOGUES = { de, ro } as const;

/**
 * Messages a translation may legitimately share with English.
 *
 * Either the word is the same in that language, or it is a product or protocol
 * noun this interface deliberately does not translate. Anything not listed here
 * and identical to English is an untranslated string, which the type system
 * cannot see because the key is present.
 */
const SHARED = new Set([
  'app.name',
  'nav.bench',
  'nav.plugins',
  'nav.section.core',
  'top.simulation',
  // Protocol terms and the standard live-runtime label are shared.
  'top.appliance',
  'network.table.gateway',
  // Details is also the German noun.
  'network.details',
  'top.appearance.system',
  'network.interface.lan',
  'network.interface.vpn',
  'network.interface.wifi',
  // "Tunnel" is the German word for it as well.
  'network.fact.tunnel',
  'about.build',
  'about.name',
  'about.version',
  'auth.field.name',
  'auth.column.name',
  'bench.title',
  'bench.simulation',
  'bench.integration',
  // Product nouns, protocol terms and words the languages share.
  'activity.actor.system',
  'activity.category.audit',
  'activity.category.debug',
  'activity.col.actor',
  'activity.details',
  'activity.filter.actor',
  'network.dn.access.local',
  'network.dn.access.wireguard',
  'network.dn.col.status',
  'network.dn.field.name',
  'network.dn.state.conflict',
  'network.settings.gateway',
  'network.wifi.strength',
  'network.wifi.title',
  // Navigation is the German word as well.
  'nav.drawer',
  // Shared product nouns, abbreviations and words the languages share.
  'network.role.uplink',
  'network.fact.control',
  'network.settings.group.dns',
]);

describe('the message catalogues', () => {
  it('offers only languages that have a catalogue', () => {
    expect(LANGUAGES.map((entry) => entry.code)).toEqual(['en', 'de', 'ro']);
    for (const entry of LANGUAGES) {
      expect(Object.keys(entry.catalogue).length).toBe(Object.keys(en).length);
    }
  });

  it.each(Object.keys(CATALOGUES))('defines every English key in %s', (code) => {
    const catalogue = CATALOGUES[code as keyof typeof CATALOGUES];

    expect(Object.keys(catalogue).sort()).toEqual(Object.keys(en).sort());
  });

  it.each(Object.keys(CATALOGUES))('leaves no message in %s empty', (code) => {
    const catalogue = CATALOGUES[code as keyof typeof CATALOGUES];

    for (const [key, value] of Object.entries(catalogue)) {
      expect(value.trim(), `${code} ${key}`).not.toBe('');
    }
  });

  it.each(Object.keys(CATALOGUES))(
    'actually translates %s rather than copying English',
    (code) => {
      const catalogue = CATALOGUES[code as keyof typeof CATALOGUES];
      const copied = Object.entries(catalogue).filter(
        ([key, value]) =>
          !SHARED.has(key) && value === (en as Record<string, string>)[key],
      );

      expect(copied.map(([key]) => key)).toEqual([]);
    },
  );

  it.each(Object.keys(CATALOGUES))('keeps every placeholder %s needs', (code) => {
    const catalogue = CATALOGUES[code as keyof typeof CATALOGUES];
    const placeholders = (text: string) => (text.match(/\{(\w+)\}/g) ?? []).sort();

    for (const [key, value] of Object.entries(catalogue)) {
      // A translation that drops a placeholder renders a sentence with a hole
      // in it, which the type system cannot see.
      expect(placeholders(value), `${code} ${key}`).toEqual(
        placeholders((en as Record<string, string>)[key]),
      );
    }
  });
});
