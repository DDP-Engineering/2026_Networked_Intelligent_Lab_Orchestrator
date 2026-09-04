import { useCallback, useEffect, useRef } from 'react';

/**
 * The stack of dismissible layers: dialogs, menus and help popovers.
 *
 * Escape closes the top layer and nothing underneath it. Every layer listens
 * on the document, so without this a help popover over an editor would take
 * the editor down with it; with it, each listener asks whether it is on top
 * before acting.
 */
const stack: symbol[] = [];

/** Register one open layer for as long as `open` holds; true while it is on top. */
export function useLayer(open: boolean): () => boolean {
  const id = useRef(Symbol('layer'));
  useEffect(() => {
    if (!open) return;
    const mine = id.current;
    stack.push(mine);
    return () => {
      const at = stack.lastIndexOf(mine);
      if (at !== -1) stack.splice(at, 1);
    };
  }, [open]);
  return useCallback(() => stack[stack.length - 1] === id.current, []);
}
