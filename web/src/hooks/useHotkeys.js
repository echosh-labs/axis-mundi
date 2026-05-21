// Copyright (c) 2026 Justin Andrew Wood. All rights reserved.
// This software is licensed under the AGPL-3.0.
// Commercial licensing is available at echosh-labs.com.
import { useEffect } from 'react';

export function useHotkeys(options) {
    const {
        mode,
        showDetail,
        showHelp,
        setShowDetail,
        onToggleHelp,
        onSyncMode,
        onRefresh,
        onSelectNext,
        onSelectPrev,
        onInspect,
        onDelete,
        onCycleStatus,
        onChangeViewType,
        detailRef,
        detailScrollStep = 50,
    } = options;

    useEffect(() => {
        const handleKeyDown = (e) => {
            const key = e.key.toLowerCase();

            if (key === 'a') { onSyncMode('AUTO'); setShowDetail(false); return; }
            if (key === 'm') { onSyncMode('MANUAL'); return; }
            if (key === 'r') {
                onRefresh();
                return;
            }


            if ((showDetail || showHelp) && key === 'escape') {
                setShowDetail(false);
                if (onToggleHelp) onToggleHelp(false);
                return;
            }

            if (key === 'h' && onToggleHelp) {
                onToggleHelp();
                return;
            }

            if (onChangeViewType && !showDetail && !showHelp) {
                if (key === 'k') { onChangeViewType('keep'); return; }
                if (key === 'd') { onChangeViewType('doc'); return; }
                if (key === 'g') { onChangeViewType('gmail'); return; }
                if (key === 's') { onChangeViewType('sheet'); return; }
                if (key === 'c') { onChangeViewType('calendar'); return; }
                if (key === 'v') { onChangeViewType('all'); return; }
            }

            switch (e.key) {
                case 'ArrowDown':
                    e.preventDefault();
                    if (showDetail) {
                        if (detailRef?.current) detailRef.current.scrollTop += detailScrollStep;
                    } else {
                        onSelectNext();
                    }
                    break;
                case 'ArrowUp':
                    e.preventDefault();
                    if (showDetail) {
                        if (detailRef?.current) detailRef.current.scrollTop -= detailScrollStep;
                    } else {
                        onSelectPrev();
                    }
                    break;
                case 'Enter':
                case ' ':
                    e.preventDefault();
                    onInspect();
                    break;
                case 'Delete':
                case 'Backspace':
                    onDelete();
                    break;
                case 'PageUp':
                    e.preventDefault();
                    onCycleStatus('forward');
                    break;
                case 'PageDown':
                    e.preventDefault();
                    onCycleStatus('back');
                    break;
            }
        };

        window.addEventListener('keydown', handleKeyDown);
        return () => window.removeEventListener('keydown', handleKeyDown);
    }, [mode, showDetail, showHelp, setShowDetail, onToggleHelp, onSyncMode, onRefresh, onSelectNext, onSelectPrev, onInspect, onDelete, onCycleStatus, onChangeViewType, detailRef, detailScrollStep]);
}
