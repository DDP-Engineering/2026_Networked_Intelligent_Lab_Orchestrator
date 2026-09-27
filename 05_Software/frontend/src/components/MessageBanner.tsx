import {
  Children,
  createContext,
  isValidElement,
  useCallback,
  useContext,
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
} from 'react';
import { createPortal } from 'react-dom';
import { useTranslate } from '../i18n/languages';

type Tone = 'info' | 'warning' | 'error';
type Notice = {
  id: string;
  tone: Tone;
  text: string;
  scope?: string;
  dismissed?: boolean;
  onDismiss?: () => void;
};
const NotificationContext = createContext<((notice: Notice) => () => void) | null>(
  null,
);

/** A page-scoped, bounded overlay shared by instrument and system pages. */
export function NotificationProvider({
  children,
  scope = '',
}: {
  children: ReactNode;
  scope?: string;
}) {
  const t = useTranslate();
  const [notices, setNotices] = useState<Notice[]>([]);
  const [fullscreen, setFullscreen] = useState<Element | null>(
    document.fullscreenElement,
  );
  const register = useCallback(
    (notice: Notice) => {
      setNotices((current) => [
        ...current.filter((item) => item.id !== notice.id),
        {
          ...notice,
          scope,
          dismissed: current.some(
            (item) =>
              item.scope === scope &&
              item.tone === notice.tone &&
              item.text === notice.text &&
              item.dismissed,
          ),
        },
      ]);
      return () =>
        setNotices((current) => current.filter((item) => item.id !== notice.id));
    },
    [scope],
  );
  useEffect(() => {
    const changed = () => setFullscreen(document.fullscreenElement);
    document.addEventListener('fullscreenchange', changed);
    return () => document.removeEventListener('fullscreenchange', changed);
  }, []);
  const unique = new Map(
    notices
      .filter((notice) => notice.scope === scope)
      .map((notice) => [`${scope}:${notice.tone}:${notice.text}`, notice]),
  );
  const stack = (
    <NotificationStack key={scope} label={t('message.notifications')}>
      {[...unique]
        .filter(([, notice]) => !notice.dismissed)
        .map(([key, notice]) => (
          <NotificationCard
            key={key}
            tone={notice.tone}
            text={notice.text}
            onDismiss={() => {
              setNotices((current) =>
                current.map((item) =>
                  item.tone === notice.tone && item.text === notice.text
                    ? { ...item, dismissed: true }
                    : item,
                ),
              );
              notices
                .filter(
                  (item) => item.tone === notice.tone && item.text === notice.text,
                )
                .forEach((item) => item.onDismiss?.());
            }}
          />
        ))}
    </NotificationStack>
  );
  return (
    <NotificationContext.Provider value={register}>
      {children}
      {fullscreen ? createPortal(stack, fullscreen) : stack}
    </NotificationContext.Provider>
  );
}

function NotificationStack({
  label,
  children,
}: {
  label: string;
  children: ReactNode;
}) {
  const region = useRef<HTMLElement>(null);
  useLayoutEffect(() => {
    const node = region.current;
    const workspace = node?.parentElement;
    const header = workspace?.querySelector('.page-head, .camera-view-heading');
    if (!node || !workspace || !header) return;
    const position = () => {
      node.style.insetBlockStart = `${Math.max(8, header.getBoundingClientRect().bottom - workspace.getBoundingClientRect().top + 8)}px`;
    };
    position();
    if (typeof ResizeObserver === 'undefined') return;
    const observer = new ResizeObserver(position);
    observer.observe(header);
    observer.observe(workspace);
    return () => observer.disconnect();
  }, []);
  return (
    <section ref={region} className="notification-stack" aria-label={label}>
      {children}
    </section>
  );
}

function NotificationCard({
  tone,
  text,
  onDismiss,
}: {
  tone: Tone;
  text: string;
  onDismiss?: () => void;
}) {
  const t = useTranslate();
  const [dismissed, setDismissed] = useState(false);
  const [hovered, setHovered] = useState(false);
  const [focused, setFocused] = useState(false);
  const dismissRef = useRef(onDismiss);
  useEffect(() => {
    dismissRef.current = onDismiss;
  }, [onDismiss]);
  const dismiss = useCallback(() => {
    setDismissed(true);
    dismissRef.current?.();
  }, []);
  useEffect(() => {
    if (tone !== 'info' || dismissed || hovered || focused) return;
    const timer = window.setTimeout(dismiss, 5000);
    return () => window.clearTimeout(timer);
  }, [tone, dismissed, hovered, focused, dismiss]);
  if (dismissed) return null;
  return (
    <div
      className={`message-banner message-banner-${tone} notification-card`}
      role={tone === 'error' ? 'alert' : 'status'}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      onFocus={() => setFocused(true)}
      onBlur={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget)) setFocused(false);
      }}
    >
      <button
        type="button"
        className="notification-dismiss"
        aria-label={t('message.dismiss')}
        onClick={dismiss}
      >
        ×
      </button>
      <div>
        <strong>{t(`message.${tone}`)}:</strong> <span>{text}</span>
      </div>
    </div>
  );
}

// Callouts carry plain feedback; contextual content retains its original markup.
function messageText(children: ReactNode): string {
  return Children.toArray(children)
    .map((child): string => {
      if (typeof child === 'string' || typeof child === 'number') return String(child);
      return isValidElement<{ children?: ReactNode }>(child)
        ? messageText(child.props.children)
        : '';
    })
    .join('');
}

/** Shared, explicitly labelled feedback for application actions. */
export function MessageBanner({
  tone,
  children,
  presentation = 'notification',
  onDismiss,
}: {
  tone: Tone;
  children: ReactNode;
  presentation?: 'notification' | 'inline';
  onDismiss?: () => void;
}) {
  const t = useTranslate();
  const register = useContext(NotificationContext);
  const id = useId();
  const text = messageText(children);
  const dismissRef = useRef(onDismiss);
  useEffect(() => {
    dismissRef.current = onDismiss;
  }, [onDismiss]);
  useEffect(() => {
    if (presentation === 'notification' && register)
      return register({ id, tone, text, onDismiss: () => dismissRef.current?.() });
  }, [id, tone, text, presentation, register]);
  if (presentation === 'notification') {
    return register ? null : (
      <NotificationCard
        key={`${tone}:${text}`}
        tone={tone}
        text={text}
        onDismiss={onDismiss}
      />
    );
  }
  return (
    <div
      className={`message-banner message-banner-${tone}`}
      role={tone === 'error' ? 'alert' : 'status'}
    >
      <strong>{t(`message.${tone}`)}:</strong> {children}
    </div>
  );
}
