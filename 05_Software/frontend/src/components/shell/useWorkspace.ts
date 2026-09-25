import { useCallback, useEffect, useRef, useState } from 'react';

/**
 * The open tabs and which one is active.
 *
 * Two rules make this predictable rather than merely working. Opening a page
 * that is already open activates it instead of duplicating it, so a tab list
 * never holds the same page twice. Closing the active tab activates its nearest
 * remaining neighbour, preferring the one to its left, because that is where the
 * eye already is; closing a tab that is not active leaves the active one alone.
 */
export interface Workspace {
  /** Open page identifiers, in the order their tabs appear. */
  readonly open: readonly string[];
  /** The active page, or null when every tab has been closed. */
  readonly active: string | null;
  open_: (id: string) => void;
  close: (id: string) => void;
  activate: (id: string) => void;
  /** Return to the opening tab set, for when a different person signs in. */
  reset: () => void;
}

export function useWorkspace(initial: readonly string[] = []): Workspace {
  const [open, setOpen] = useState<readonly string[]>(initial);
  const [active, setActive] = useState<string | null>(initial[0] ?? null);

  const openPage = useCallback((id: string) => {
    setOpen((current) => (current.includes(id) ? current : [...current, id]));
    setActive(id);
  }, []);

  const close = useCallback((id: string) => {
    setOpen((current) => {
      const index = current.indexOf(id);
      if (index === -1) {
        return current;
      }
      const remaining = current.filter((entry) => entry !== id);
      setActive((currentActive) => {
        if (currentActive !== id) {
          return currentActive;
        }
        // The left neighbour, or the right one when the first tab closed.
        return remaining[index - 1] ?? remaining[index] ?? null;
      });
      return remaining;
    });
  }, []);

  const activate = useCallback((id: string) => {
    setActive((current) => (current === id ? current : id));
  }, []);

  // `initial` is read through a ref-free closure on purpose: the opening tab set
  // is a constant of the shell, and treating it as a dependency would make a
  // caller's inline array reset the workspace on every render.
  const initialOpen = useRef(initial);
  const reset = useCallback(() => {
    setOpen(initialOpen.current);
    setActive(initialOpen.current[0] ?? null);
  }, []);

  return { open, active, open_: openPage, close, activate, reset };
}

/**
 * Drop tabs whose page is no longer available.
 *
 * A role change can take the Authentication page away while its tab is open. The
 * tab has to go with it rather than render a page the navigation no longer
 * offers; the same neighbour rule then chooses what becomes active.
 */
export function usePruneClosedPages(
  workspace: Workspace,
  availableIds: readonly string[],
): void {
  const { open, close } = workspace;
  useEffect(() => {
    for (const id of open) {
      if (!availableIds.includes(id)) {
        close(id);
      }
    }
  }, [open, availableIds, close]);
}
