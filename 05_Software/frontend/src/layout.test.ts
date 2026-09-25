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
const indexHtml = readFileSync('index.html', 'utf8');

describe('product document metadata', () => {
  it('uses final NILO product identity in the browser tab and description', () => {
    expect(indexHtml).toContain('<title>NILO</title>');
    expect(indexHtml).toContain('content="NILO electronics bench appliance"');
  });
});

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

  it('places the About scroll rail at the workspace edge', () => {
    expect(declarationsOf(shellCss, '.about-page .page-body')).toMatch(
      /max-width:\s*none/,
    );
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

  it('keeps focused administration listings in natural page flow', () => {
    expect(declarationsOf(shellCss, '.inventory-list')).toMatch(/overflow:\s*auto/);
    expect(declarationsOf(shellCss, '.admin-list')).toMatch(/overflow:\s*visible/);
    expect(declarationsOf(shellCss, '.inventory')).toMatch(
      /grid-template-rows:\s*auto minmax\(0, 1fr\)/,
    );
  });

  it('narrows the navigation to a labeled compact rail on a field viewport', () => {
    const compact = mediaBlock(shellCss, '@media (width <= 72rem)');

    expect(compact).toMatch(/grid-template-columns:\s*var\(--nav-width-compact\)/);
    // The labels stay: a rail of bare icons is not navigation an operator can
    // learn on a panel used occasionally.
    expect(compact).not.toMatch(/\.nav-label[^{]*\{[^}]*display:\s*none/);
  });

  it('replaces the rail with a drawer on a constrained viewport', () => {
    const constrained = mediaBlock(
      shellCss,
      '@media (width <= 56rem), (height <= 520px)',
    );

    expect(constrained).toMatch(/grid-template-columns:\s*auto minmax\(0, 1fr\)/);
    expect(constrained).toMatch(/\.sidenav[^{]*\{[^}]*display:\s*none/);
    expect(constrained).toMatch(/\.shell-menu[^{]*\{[^}]*display:\s*(inline-)?flex/);
  });
});

/** One @media block, from its query to the matching closing brace. */
function mediaBlock(css: string, query: string): string {
  const start = css.indexOf(query);
  if (start === -1) {
    throw new Error(`no media block for ${query}`);
  }
  let depth = 0;
  for (let index = css.indexOf('{', start); index < css.length; index += 1) {
    if (css[index] === '{') depth += 1;
    if (css[index] === '}') {
      depth -= 1;
      if (depth === 0) return css.slice(start, index + 1);
    }
  }
  throw new Error(`unterminated media block for ${query}`);
}

/** Every font-size declaration in a stylesheet, as written. */
function fontSizes(css: string): string[] {
  return [...css.matchAll(/font-size:\s*([^;]+);/g)].map((match) => match[1].trim());
}

describe('field readability', () => {
  // REQ-UX-c8c16d33: one semantic type scale, nothing below 14px.
  it('declares the type scale as tokens with the body at 16px and nothing below 14px', () => {
    const root = declarationsOf(tokensCss, ':root');
    const sizes = Object.fromEntries(
      [...root.matchAll(/(--text-(?:title|section|body|dense|label)):\s*(\d+)px/g)].map(
        (match) => [match[1], Number(match[2])],
      ),
    );

    expect(Object.keys(sizes).sort()).toEqual([
      '--text-body',
      '--text-dense',
      '--text-label',
      '--text-section',
      '--text-title',
    ]);
    expect(sizes['--text-body']).toBe(16);
    expect(sizes['--text-dense']).toBeGreaterThanOrEqual(15);
    expect(sizes['--text-label']).toBeGreaterThanOrEqual(14);
    expect(Math.min(...Object.values(sizes))).toBeGreaterThanOrEqual(14);
    expect(declarationsOf(indexCss, 'body')).toMatch(/font-size:\s*var\(--text-body\)/);
  });

  it('sizes every component from the scale, never below 14px and never in capitals', () => {
    for (const size of fontSizes(shellCss)) {
      if (size.startsWith('var(--text-') || size === 'inherit') continue;
      const px = /^(\d+(?:\.\d+)?)px$/.exec(size);
      const rem = /^(\d+(?:\.\d+)?)rem$/.exec(size);
      if (px) {
        expect(Number(px[1]), size).toBeGreaterThanOrEqual(14);
      } else if (rem) {
        expect(Number(rem[1]) * 16, size).toBeGreaterThanOrEqual(14);
      } else {
        throw new Error(`font-size ${size} is not a token, px or rem value`);
      }
    }
    expect(shellCss).not.toMatch(/text-transform:\s*uppercase/);
  });

  // REQ-UX-c5f7c0cd: 44 pixel targets on field layouts.
  it('gives controls a 44 pixel target from one token', () => {
    const root = declarationsOf(tokensCss, ':root');
    expect(root).toMatch(/--control-height:\s*44px/);
    expect(root).toMatch(/--target-min:\s*44px/);

    expect(declarationsOf(shellCss, 'button')).toMatch(
      /min-height:\s*var\(--control-height\)/,
    );
    expect(declarationsOf(shellCss, '.field input')).toMatch(
      /min-height:\s*var\(--control-height\)/,
    );
    expect(declarationsOf(shellCss, '.nav-entry')).toMatch(
      /min-height:\s*var\(--control-height\)/,
    );
    expect(declarationsOf(shellCss, '.nav-group-toggle')).toMatch(
      /min-height:\s*var\(--target-min\)/,
    );
    expect(declarationsOf(shellCss, '.nav-group-toggle')).toMatch(
      /min-width:\s*var\(--target-min\)/,
    );
    expect(declarationsOf(shellCss, '.nav-entry-child')).toMatch(
      /min-height:\s*var\(--target-min\)/,
    );
    expect(declarationsOf(shellCss, '.tab-close')).toMatch(
      /min-width:\s*var\(--target-min\)/,
    );
    expect(declarationsOf(shellCss, '.help-trigger')).toMatch(
      /min-width:\s*var\(--target-min\)/,
    );
  });

  // REQ-UX-2de87216: dialogs fit and become sheets.
  it('bounds a dialog to the viewport with a scrolling body and sticky actions', () => {
    expect(declarationsOf(shellCss, '.modal')).toMatch(/max-height:\s*100dvh/);
    expect(declarationsOf(shellCss, '.modal-body')).toMatch(/overflow-y:\s*auto/);
    expect(declarationsOf(shellCss, '.modal-actions')).toMatch(/position:\s*sticky/);
    expect(declarationsOf(shellCss, '.modal-actions')).toMatch(/bottom:\s*0/);

    const sheet = mediaBlock(shellCss, '@media (width <= 56rem), (height <= 640px)');
    expect(sheet).toMatch(/\.modal[^{]*\{[^}]*height:\s*100dvh/);
  });

  // REQ-UX-ff989933: one markup, reflowed to labeled cards below 56rem.
  it('turns record tables into labeled cards below 56rem', () => {
    const cards = mediaBlock(shellCss, '@media (width <= 56rem) {');
    expect(cards).toMatch(/\.record-table thead[^{]*\{[^}]*display:\s*none/);
    expect(cards).toMatch(/content:\s*attr\(data-label\)/);
  });

  it('keeps each WireGuard readiness value next to its label', () => {
    const fact = declarationsOf(shellCss, '.network-readiness dl > div');
    expect(fact).toMatch(/grid-template-columns:\s*auto auto/);
    expect(fact).toMatch(/justify-content:\s*start/);
    expect(declarationsOf(shellCss, '.network-readiness dd')).toMatch(
      /text-align:\s*left/,
    );
  });

  // REQ-UX-ff989933 for the audit listing: the head, the filter panel and the
  // rows all reflow, so a seven-inch panel reads the history down the page
  // rather than sideways off it.
  // REQ-UX-f8c58c4c adds the filter line: scope, the fields and More filters
  // wrap as one line rather than taking a row each, and only the head stacks
  // below 56rem.
  it('wraps the activity filter line and stacks the head below 56rem', () => {
    // The chips scroll for the same reason the tab strip does: a head that
    // grew a second line would take the space the listing needs.
    expect(declarationsOf(shellCss, '.activity-chips')).toMatch(/overflow-x:\s*auto/);
    expect(declarationsOf(shellCss, '.activity-chips')).not.toMatch(
      /flex-wrap:\s*wrap/,
    );
    expect(declarationsOf(shellCss, '.activity-chip')).toMatch(
      /min-height:\s*var\(--target-min\)/,
    );
    // Where the filter panel collapses, the scope row is a flex item of the
    // scrolling page body. Its own `overflow-x` suppresses the automatic
    // minimum size a flex item would otherwise keep, so without this it absorbs
    // the column's shrink and clips the 44px chips it holds - the same trap
    // `.activity-page .table-scroll` is held out of below.
    expect(declarationsOf(shellCss, '.activity-chips')).toMatch(/flex:\s*none/);

    // `.table-scroll` declares `min-height: 0`, so as a flex item of the
    // scrolling page body it would absorb the whole shrink and clip every row
    // it holds. The listing scrolls with the page instead.
    expect(declarationsOf(shellCss, '.activity-page .table-scroll')).toMatch(
      /flex:\s*none/,
    );

    // One wrapping line, not a grid of rows: the fields share a basis they can
    // wrap on, and the disclosure keeps its own width on the same line.
    const line = declarationsOf(shellCss, '.activity-filters');
    expect(line).toMatch(/display:\s*flex/);
    expect(line).toMatch(/flex-wrap:\s*wrap/);
    expect(declarationsOf(shellCss, '.activity-filters .field')).toMatch(
      /flex:\s*1 1 12rem/,
    );
    expect(declarationsOf(shellCss, '.activity-filters-more')).toMatch(/flex:\s*none/);
    expect(
      declarationsOf(shellCss, '.activity-filters-more-fields:not([hidden])'),
    ).toMatch(/display:\s*contents/);

    // REQ-UX-3e01a5b6 asks for Export at the right of the title line at every
    // width, so the auto margin that puts it there is declared once and no
    // narrow-viewport block may take it back.
    expect(declarationsOf(shellCss, '.activity-head .menu')).toMatch(
      /margin-left:\s*auto/,
    );
    const cards = mediaBlock(shellCss, '@media (width <= 56rem) {');
    expect(cards).not.toMatch(/\.activity-head[^{]*\{[^}]*margin-left/);
  });

  // REQ-UX-f30891b4: an outcome is never carried by colour alone, so the badge
  // is a word inside a bordered shape and the tone only repeats it.
  it('draws an outcome badge from the semantic state tokens, never a literal', () => {
    const badge = declarationsOf(shellCss, '.status-badge');
    expect(badge).toMatch(/border:\s*1px solid var\(--border-strong\)/);
    expect(badge).toMatch(/font-size:\s*var\(--text-label\)/);

    for (const [tone, token] of [
      ['ok', 'ok'],
      ['warn', 'warn'],
      ['danger', 'danger'],
      ['pending', 'unknown'],
    ] as const) {
      expect(declarationsOf(shellCss, `.status-badge-${tone}`)).toMatch(
        new RegExp(`color:\\s*var\\(--state-${token}\\)`),
      );
    }
  });

  it('keeps row actions in a compact viewport-bounded dialog', () => {
    expect(declarationsOf(shellCss, '.modal-actions-dialog')).toMatch(
      /width:\s*min\(27rem, 100%\)/,
    );
    expect(declarationsOf(shellCss, '.action-dialog-action')).toMatch(/width:\s*100%/);
    expect(declarationsOf(shellCss, '.action-dialog-action')).toMatch(
      /min-height:\s*var\(--target-min\)/,
    );
    expect(declarationsOf(shellCss, '.action-dialog-danger-list')).toMatch(
      /border-top:/,
    );

    const shortFieldDisplay = mediaBlock(
      shellCss,
      '@media (width > 56rem) and (height <= 640px)',
    );
    expect(shortFieldDisplay).toMatch(
      /\.modal-actions-dialog[^{]*\{[^}]*max-height:\s*calc\(100dvh - 2 \* var\(--space-3\)\)/,
    );
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
