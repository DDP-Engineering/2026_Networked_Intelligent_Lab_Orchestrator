import { act, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { MessageBanner, NotificationProvider } from './MessageBanner';
import { LanguageProvider } from '../i18n/LanguageProvider';
import type { ReactNode } from 'react';

function feedback(children: ReactNode) {
  return (
    <LanguageProvider initial="en">
      <NotificationProvider>{children}</NotificationProvider>
    </LanguageProvider>
  );
}
afterEach(() => vi.useRealTimers());

describe('shared action feedback', () => {
  it('offers a labelled left-side dismiss action for general feedback', () => {
    render(
      <LanguageProvider initial="en">
        <MessageBanner tone="error">Connection failed</MessageBanner>
      </LanguageProvider>,
    );
    expect(screen.getByRole('alert').textContent).toContain('Error:');
    fireEvent.click(screen.getByRole('button', { name: 'Dismiss notification' }));
    expect(screen.queryByText('Connection failed')).toBeNull();
  });
  it('coalesces duplicates and keeps identical polling messages dismissed', () => {
    const notices = (text: string) =>
      feedback(
        <>
          <MessageBanner tone="warning">{text}</MessageBanner>
          <MessageBanner tone="warning">{text}</MessageBanner>
        </>,
      );
    const view = render(notices('Lost connection'));
    expect(screen.getAllByRole('status')).toHaveLength(1);
    fireEvent.click(screen.getByRole('button', { name: 'Dismiss notification' }));
    view.rerender(notices('Lost connection'));
    expect(screen.queryByRole('status')).toBeNull();
    view.rerender(notices('Connection restored'));
    expect(screen.getByRole('status').textContent).toContain('Connection restored');
    view.rerender(feedback(null));
    expect(screen.queryByRole('status')).toBeNull();
    view.rerender(notices('Lost connection'));
    expect(screen.getByRole('status').textContent).toContain('Lost connection');
  });
  it('expires info, pauses during hover or focus, and retains warnings/errors', () => {
    vi.useFakeTimers();
    render(
      feedback(
        <>
          <MessageBanner tone="info">Saved</MessageBanner>
          <MessageBanner tone="warning">Offline</MessageBanner>
          <MessageBanner tone="error">Failed</MessageBanner>
        </>,
      ),
    );
    const info = screen.getByText('Saved').closest('.notification-card')!;
    fireEvent.mouseEnter(info);
    act(() => vi.advanceTimersByTime(10000));
    expect(screen.getByText('Saved')).toBeTruthy();
    fireEvent.mouseLeave(info);
    fireEvent.focus(info.querySelector('button')!);
    act(() => vi.advanceTimersByTime(10000));
    expect(screen.getByText('Saved')).toBeTruthy();
    fireEvent.blur(info.querySelector('button')!);
    act(() => vi.advanceTimersByTime(5000));
    expect(screen.queryByText('Saved')).toBeNull();
    expect(screen.getByText('Offline')).toBeTruthy();
    expect(screen.getByRole('alert').textContent).toContain('Failed');
  });
  it('keeps approval buttons inline, visible and actionable', () => {
    vi.useFakeTimers();
    const approve = vi.fn();
    render(
      feedback(
        <MessageBanner tone="info" presentation="inline">
          Approval required<button onClick={approve}>Approve</button>
        </MessageBanner>,
      ),
    );
    act(() => vi.advanceTimersByTime(10000));
    const button = screen.getByRole('button', { name: 'Approve' });
    expect(button.closest('.notification-stack')).toBeNull();
    expect(screen.queryByRole('button', { name: 'Dismiss notification' })).toBeNull();
    fireEvent.click(button);
    expect(approve).toHaveBeenCalledOnce();
  });
  it('keeps the info deadline when another message arrives', () => {
    vi.useFakeTimers();
    const content = (extra: boolean) =>
      feedback(
        <>
          <MessageBanner tone="info">Saved</MessageBanner>
          {extra && <MessageBanner tone="error">Another failure</MessageBanner>}
        </>,
      );
    const view = render(content(false));
    act(() => vi.advanceTimersByTime(2000));
    view.rerender(content(true));
    act(() => vi.advanceTimersByTime(3000));
    expect(screen.queryByText('Saved')).toBeNull();
    expect(screen.getByRole('alert')).toBeTruthy();
  });
  it('resets notification scope without remounting the page', () => {
    const content = (scope: string) => (
      <LanguageProvider initial="en">
        <NotificationProvider scope={scope}>
          <input aria-label="Draft" defaultValue="" />
          <MessageBanner tone="error">Failed</MessageBanner>
        </NotificationProvider>
      </LanguageProvider>
    );
    const view = render(content('network'));
    fireEvent.change(screen.getByLabelText('Draft'), {
      target: { value: 'unsaved value' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Dismiss notification' }));
    view.rerender(content('device-access'));
    expect(screen.getByLabelText('Draft')).toHaveValue('unsaved value');
    expect(screen.getByRole('alert')).toBeTruthy();
  });
  it('places callouts inside the fullscreen element', () => {
    render(
      feedback(
        <div data-testid="fullscreen">
          <MessageBanner tone="error">Failed</MessageBanner>
        </div>,
      ),
    );
    const target = screen.getByTestId('fullscreen');
    Object.defineProperty(document, 'fullscreenElement', {
      configurable: true,
      value: target,
    });
    fireEvent(document, new Event('fullscreenchange'));
    expect(target.querySelector('.notification-stack')).toBeTruthy();
    Object.defineProperty(document, 'fullscreenElement', {
      configurable: true,
      value: null,
    });
    fireEvent(document, new Event('fullscreenchange'));
    expect(target.querySelector('.notification-stack')).toBeNull();
  });
});
