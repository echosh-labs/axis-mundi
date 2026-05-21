// Copyright (c) 2026 Justin Andrew Wood. All rights reserved.
// This software is licensed under the AGPL-3.0.
// Commercial licensing is available at echosh-labs.com.
import { renderHook } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { useHotkeys } from '../hooks/useHotkeys.js';

const makeDetailRef = () => ({ current: { scrollTop: 0 } });

describe('useHotkeys', () => {
    let callbacks;
    beforeEach(() => {
        callbacks = {
            onSyncMode: vi.fn(),
            onRefresh: vi.fn(),
            onSelectNext: vi.fn(),
            onSelectPrev: vi.fn(),
            onInspect: vi.fn(),
            onDelete: vi.fn(),
            onCycleStatus: vi.fn(),
            setShowDetail: vi.fn(),
        };
    });

    afterEach(() => {
        vi.clearAllMocks();
    });

    it('navigates list when not showing detail', () => {
        renderHook(() => useHotkeys({
            mode: 'MANUAL',
            showDetail: false,
            detailRef: makeDetailRef(),
            detailScrollStep: 50,
            ...callbacks,
        }));

        window.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown' }));
        window.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowUp' }));

        expect(callbacks.onSelectNext).toHaveBeenCalledTimes(1);
        expect(callbacks.onSelectPrev).toHaveBeenCalledTimes(1);
    });

    it('scrolls detail when showing detail', () => {
        const detailRef = makeDetailRef();
        renderHook(() => useHotkeys({
            mode: 'MANUAL',
            showDetail: true,
            detailRef,
            detailScrollStep: 25,
            ...callbacks,
        }));

        window.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown' }));
        window.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowUp' }));

        expect(detailRef.current.scrollTop).toBe(0);
        // ensures list callbacks not triggered in detail mode
        expect(callbacks.onSelectNext).not.toHaveBeenCalled();
        expect(callbacks.onSelectPrev).not.toHaveBeenCalled();
    });

    it('navigates list in AUTO mode', () => {
        renderHook(() => useHotkeys({
            mode: 'AUTO',
            showDetail: false,
            detailRef: makeDetailRef(),
            detailScrollStep: 50,
            ...callbacks,
        }));

        window.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown' }));
        window.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowUp' }));

        expect(callbacks.onSelectNext).toHaveBeenCalledTimes(1);
        expect(callbacks.onSelectPrev).toHaveBeenCalledTimes(1);
    });
});
