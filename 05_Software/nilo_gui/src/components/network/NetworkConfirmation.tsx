import { useEffect, useRef, type ReactNode } from 'react';

/** Native modality keeps keyboard focus inside the acknowledged change. */
export function NetworkConfirmation({
  children,
  onCancel,
}: {
  children: ReactNode;
  onCancel: () => void;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  const cancelRef = useRef(onCancel);
  useEffect(() => {
    cancelRef.current = onCancel;
  }, [onCancel]);
  useEffect(() => {
    const dialog = ref.current;
    const previous = document.activeElement;
    if (dialog === null) return;
    dialog.showModal();
    return () => {
      dialog.close();
      if (previous instanceof HTMLElement) previous.focus();
    };
  }, []);
  return (
    <dialog
      ref={ref}
      className="network-confirmation"
      aria-labelledby="network-proposal"
      onCancel={(event) => {
        event.preventDefault();
        cancelRef.current();
      }}
    >
      {children}
    </dialog>
  );
}
