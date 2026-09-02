import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

/**
 * The shell's layout and its theming contract, asserted by reading the shipped
 * stylesheets. jsdom applies no CSS and Vitest stubs CSS imports out, so the
 * files on disk are the only place these rules can be checked.
 */

const tokensCss = stylesheet('./styles/tokens.css');
const shellCss = stylesheet('./styles/shell.css');
const indexCss = stylesheet('./index.css');

function stylesheet(name: string): string {
  // Comments and @import lines are stripped so a documented rule is still found
  // by its selector; this crude reader splits on braces and would otherwise
  // read the preamble as part of the first rule's head.
  return readFileSync(new URL(name, import.meta.url), 'utf8')
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/^\s*@import[^;]*;\s*$/gm, '');
}

/**
 * Every top-level declaration for one selector, ignoring anything inside a
 * media query. A selector can appear in more than one rule — `body` is bounded
 * in one and painted in another — so all of them are returned together.
 */
function declarationsOf(css: string, selector: string): string {
  const matches: string[] = [];
  for (const block of css.split('@media')[0].split('}')) {
    const [head, body] = block.split('{');
    if (head === undefined || body === undefined) {
      continue;
    }
    if (
      head
        .split(',')
        .map((entry) => entry.trim())
        .includes(selector)
    ) {
      matches.push(body);
    }
  }
  if (matches.length === 0) {
    throw new Error(`no top-level rule for ${selector}`);
  }
  return matches.join('\n');
}

/** Every custom property a block declares. */
function tokensIn(block: string): string[] {
  return [...block.matchAll(/(--[\w-]+)\s*:/g)].map((match) => match[1]).sort();
}

/** The body of one selector's rule anywhere in the file, @media included. */
function ruleAnywhere(css: string, selector: string): string {
  const index = css.indexOf(selector);
  if (index === -1) {
    throw new Error(`no rule for ${selector}`);
  }
  const open = css.indexOf('{', index);
  return css.slice(open + 1, css.indexOf('}', open));
}

describe('single-viewport layout', () => {
  it('bounds the document to the viewport and refuses page-level scrolling', () => {
    expect(declarationsOf(indexCss, 'html')).toMatch(/height:\s*100dvh/);
    expect(declarationsOf(indexCss, 'html')).toMatch(/overflow:\s*hidden/);
    expect(declarationsOf(indexCss, 'body')).toMatch(/overflow:\s*hidden/);
  });

  it('paints no decorative background behind the workspace', () => {
    const body = declarationsOf(indexCss, 'body');

    expect(body).toMatch(/background:\s*var\(--surface-base\)/);
    expect(body).not.toMatch(/linear-gradient|radial-gradient|background-image/);
  });

  it('lays the shell out as one grid with a compact header row', () => {
    const shell = declarationsOf(shellCss, '.shell');

    expect(shell).toMatch(/height:\s*100dvh/);
    expect(shell).toMatch(/overflow:\s*hidden/);
    expect(shell).toMatch(
      /grid-template-columns:\s*var\(--nav-width\) minmax\(0, 1fr\)/,
    );
    expect(shell).toMatch(/grid-template-rows:\s*var\(--row-height\)/);
    // Four named cells, so there is nowhere for a second header row to appear.
    expect(shell).toMatch(/grid-template-areas:/);
    expect(shell).toContain('brand');
    expect(shell).toContain('toprow');
    expect(shell).toContain('sidenav');
    expect(shell).toContain('workspace');
  });

  it('keeps the header row between 44 and 48 pixels', () => {
    const height = /--row-height:\s*(\d+)px/.exec(declarationsOf(tokensCss, ':root'));

    expect(height).not.toBeNull();
    expect(Number(height?.[1])).toBeGreaterThanOrEqual(44);
    expect(Number(height?.[1])).toBeLessThanOrEqual(48);
  });

  it('gives the workspace the space that is left, and its own overflow', () => {
    const workspace = declarationsOf(shellCss, '.shell-workspace');

    expect(workspace).toMatch(/min-height:\s*0/);
    expect(workspace).toMatch(/overflow:\s*hidden/);
    expect(declarationsOf(shellCss, '.page')).toMatch(
      /grid-template-rows:\s*auto minmax\(0, 1fr\)/,
    );
    expect(declarationsOf(shellCss, '.page-body')).toMatch(/overflow-y:\s*auto/);
  });

  it('scrolls the tab strip rather than wrapping the header', () => {
    const strip = declarationsOf(shellCss, '.tabstrip');

    expect(strip).toMatch(/overflow-x:\s*auto/);
    expect(strip).not.toMatch(/flex-wrap:\s*wrap/);
  });

  it('scrolls long listings inside their own compartments', () => {
    expect(declarationsOf(shellCss, '.inventory-list')).toMatch(/overflow:\s*auto/);
    expect(declarationsOf(shellCss, '.admin-list')).toMatch(/overflow:\s*auto/);
    expect(declarationsOf(shellCss, '.inventory')).toMatch(
      /grid-template-rows:\s*auto minmax\(0, 1fr\)/,
    );
  });

  it('narrows the navigation rather than clipping it on a small viewport', () => {
    const narrow = shellCss.slice(shellCss.indexOf('@media (width <= 60rem)'));

    expect(narrow).toMatch(/grid-template-columns:\s*var\(--nav-width-narrow\)/);
    expect(narrow).toContain('.nav-label');
    expect(narrow).not.toContain('display: none;\n  }\n\n  .sidenav');
  });
});

describe('one token set, three themes', () => {
  it('redefines every light token for dark and for the system dark branch', () => {
    const light = tokensIn(declarationsOf(tokensCss, ':root'));
    const dark = tokensIn(declarationsOf(tokensCss, ":root[data-theme='dark']"));
    const system = tokensIn(ruleAnywhere(tokensCss, ":root[data-theme='system']"));

    // The light root also carries the metrics and font stacks, which do not
    // change with the palette; every colour token must appear in all three.
    for (const token of dark) {
      expect(light).toContain(token);
      expect(system).toContain(token);
    }
    expect(dark).toEqual(system);
    expect(dark.length).toBeGreaterThan(15);
  });

  it('follows the device for the system theme in CSS, not in script', () => {
    expect(tokensCss).toMatch(
      /@media \(prefers-color-scheme: dark\)\s*\{\s*:root\[data-theme='system'\]/,
    );
  });

  it('declares no literal colour outside the token file', () => {
    // A component that hard-codes a colour is a second component tree waiting
    // to happen: it would be right in one theme and wrong in the other.
    const literals = shellCss.match(/#[0-9a-f]{3,8}\b|\brgb\(|\bhsl\(/gi) ?? [];
    const allowed = literals.filter((literal) =>
      literal.toLowerCase().startsWith('rgb('),
    );

    expect(literals.filter((literal) => literal.startsWith('#'))).toEqual([]);
    // The one permitted rgb() is the modal backdrop's black scrim, which is a
    // shadow rather than a surface and reads the same in both themes.
    expect(allowed.length).toBeLessThanOrEqual(1);
  });

  it('gives keyboard focus a visible ring in both themes', () => {
    expect(declarationsOf(shellCss, ':focus-visible')).toMatch(
      /outline:\s*2px solid var\(--focus-ring\)/,
    );
    expect(declarationsOf(tokensCss, ':root')).toMatch(/--focus-ring:/);
    expect(declarationsOf(tokensCss, ":root[data-theme='dark']")).toMatch(
      /--focus-ring:/,
    );
  });
});
