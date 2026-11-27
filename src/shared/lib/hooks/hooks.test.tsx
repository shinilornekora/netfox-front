import { act, renderHook } from '@testing-library/react';
import type { ReactNode } from 'react';
import { Provider } from 'react-redux';
import type { UnknownAction } from '@reduxjs/toolkit';
import { makeStore } from '@/app/store';
import { useDebounce } from './useDebounce';
import { usePageModel } from './usePageModel';

describe('useDebounce', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it('отдаёт новое значение только после паузы и сбрасывает таймер на каждом изменении', () => {
    const { result, rerender } = renderHook(({ v }) => useDebounce(v, 400), { initialProps: { v: 'м' } });
    expect(result.current).toBe('м');

    rerender({ v: 'ме' });
    act(() => vi.advanceTimersByTime(300));
    rerender({ v: 'мет' });
    act(() => vi.advanceTimersByTime(300));
    // 600 мс прошло, но с последнего изменения — только 300: значение ещё старое.
    expect(result.current).toBe('м');

    act(() => vi.advanceTimersByTime(100));
    expect(result.current).toBe('мет');
  });
});

describe('usePageModel', () => {
  const opened = (arg: { id: string; ep?: string }) => ({ type: 'test/opened', payload: arg });
  const closed = () => ({ type: 'test/closed' });

  function setup(initial: { id: string; ep?: string }) {
    const store = makeStore();
    const seen: string[] = [];
    const dispatch = store.dispatch;
    store.dispatch = ((a: UnknownAction) => {
      if (typeof a === 'object' && a.type.startsWith('test/'))
        seen.push(`${a.type} ${JSON.stringify(a.payload ?? '')}`);
      return dispatch(a);
    }) as typeof store.dispatch;
    const wrapper = ({ children }: { children: ReactNode }) => <Provider store={store}>{children}</Provider>;
    const hook = renderHook(({ arg }) => usePageModel(opened, closed, arg), {
      wrapper,
      initialProps: { arg: initial },
    });
    return { seen, ...hook };
  }

  it('открывает модель один раз, даже если объект аргумента пересоздаётся на каждом рендере', () => {
    const { seen, rerender } = setup({ id: 'a' });
    rerender({ arg: { id: 'a' } });
    rerender({ arg: { id: 'a' } });
    expect(seen).toEqual(['test/opened {"id":"a"}']);
  });

  it('при смене аргумента закрывает старую модель и открывает новую, при уходе — закрывает', () => {
    const { seen, rerender, unmount } = setup({ id: 'a' });
    rerender({ arg: { id: 'a', ep: '2' } });
    unmount();
    expect(seen).toEqual([
      'test/opened {"id":"a"}',
      'test/closed ""',
      'test/opened {"id":"a","ep":"2"}',
      'test/closed ""',
    ]);
  });
});
