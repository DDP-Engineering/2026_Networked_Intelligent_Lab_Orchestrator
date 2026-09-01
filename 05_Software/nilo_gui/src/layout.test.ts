import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

/**
 * The primary screen must fit one monitor viewport. jsdom applies no CSS and
 * Vitest stubs out CSS imports, so the layout contract is asserted by reading
 * the shipped stylesheets from disk.
 */

const appCss = stylesheet('./App.css');
const indexCss = stylesheet('./index.css');

function stylesheet(name: string): string {
  // Comments are stripped so a documented rule is still found by its selector;
  // this crude reader would otherwise read the comment as part of the head.
  return readFileSync(new URL(name, import.meta.url), 'utf8').replace(
    /\/\*[\s\S]*?\*\//g,
    '',
  );
}

function declarationsOf(css: string, selector: string): string {
  for (const block of css.split('@media')[0].split('}')) {
    const [head, body] = block.split('{');
    if (head === undefined || body === undefined) {
      continue;
    }
    const selectors = head.split(',').map((entry) => entry.trim());
    if (selectors.includes(selector)) {
      return body;
    }
  }
  throw new Error(`no top-level rule for ${selector}`);
}

describe('single-viewport layout', () => {
  it('bounds the document to the viewport height and refuses page-level scrolling', () => {
    expect(declarationsOf(indexCss, 'html')).toMatch(/height:\s*100dvh/);
    expect(declarationsOf(indexCss, 'html')).toMatch(/overflow:\s*hidden/);
    expect(declarationsOf(indexCss, 'body')).toMatch(/height:\s*100dvh/);
    expect(declarationsOf(indexCss, 'body')).toMatch(/overflow:\s*hidden/);
  });

  it('sizes the application frame to the viewport rather than to its content', () => {
    const frame = declarationsOf(appCss, '.app-frame');

    expect(frame).toMatch(/height:\s*100dvh/);
    expect(frame).not.toMatch(/min-height/);
    expect(frame).toMatch(/overflow:\s*hidden/);
    expect(frame).toMatch(/grid-template-rows:\s*auto minmax\(0, 1fr\) auto/);
  });

  it('gives the workspace its own scroll region for content that does not fit', () => {
    const workspace = declarationsOf(appCss, '.workspace');

    expect(workspace).toMatch(/overflow-y:\s*auto/);
    expect(workspace).toMatch(/min-height:\s*0/);
  });

  it('scrolls a long inventory inside its own compartment, not down the page', () => {
    const list = declarationsOf(appCss, '.inventory-list');

    expect(list).toMatch(/overflow-y:\s*auto/);
    expect(list).toMatch(/min-height:\s*0/);
  });

  it('lets the connected screen give the inventory the space that is left', () => {
    const connected = declarationsOf(appCss, '.connected');

    expect(connected).toMatch(/min-height:\s*0/);
    expect(connected).toMatch(/grid-template-rows:\s*auto minmax\(0, 1fr\)/);
  });
});
