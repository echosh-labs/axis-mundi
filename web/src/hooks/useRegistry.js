// Copyright (c) 2026 Justin Andrew Wood. All rights reserved.
// This software is licensed under the AGPL-3.0.
// Commercial licensing is available at echosh-labs.com.
import { useState, useEffect, useCallback } from 'react';
import {
    getMode,
    setMode as apiSetMode,
    getRegistry as apiGetRegistry,
    getDetail as apiGetDetail,
    deleteResource,
    setStatus as apiSetStatus,
    getUser,
    normalizeRegistry,
} from '../utils/apiClient';

const STATUS_CYCLE = ['Pending', 'Execute', 'Active', 'Blocked', 'Review', 'Complete', 'Error'];

export function useRegistry({ addLog, onRegistryChange } = {}) {
    const [mode, setMode] = useState('MANUAL');
    const [registry, setRegistry] = useState([]);
    const [user, setUser] = useState(null);
    const [connected, setConnected] = useState(false);
    const [secondsRemaining, setSecondsRemaining] = useState(null);

    const syncMode = useCallback(async (newMode) => {
        setMode(newMode);
        try {
            await apiSetMode(newMode);
        } catch {
            addLog?.('error', `Failed to sync mode ${newMode}`);
        }
    }, [addLog]);

    const fetchRegistry = useCallback(async () => {
        try {
            const normalized = await apiGetRegistry(true);
            setRegistry(normalized);
            onRegistryChange?.(normalized);
            addLog?.('success', 'Manual registry refresh.');
        } catch {
            addLog?.('error', 'Failed to retrieve registry.');
        }
    }, [addLog, onRegistryChange]);

    const fetchDetail = useCallback(async (item) => apiGetDetail(item), []);

    const deleteItem = useCallback(async (item) => {
        try {
            await deleteResource(item);
            addLog?.('success', `Object purged (${item?.type || 'item'}): ${item?.id || 'unknown'}`);
        } catch (err) {
            if (item?.type === 'doc' || item?.type === 'sheet') {
                addLog?.('error', `Purge failed for ${item.type} (${item.title || item.id}): Insufficient permissions (Drive is configured in READ-ONLY mode).`);
            } else {
                addLog?.('error', `Purge failed for ${item?.type || 'item'} (${item?.title || item?.id || 'unknown'}): ${err.message || 'Unknown error'}`);
            }
        }
    }, [addLog]);

    const updateStatus = useCallback(async (item, status) => {
        if (!item || !item.id) return;
        try {
            await apiSetStatus(item, status);
        } catch (err) {
            addLog?.('error', 'Failed to save status');
            throw err;
        }
    }, [addLog]);

    const nextStatus = useCallback((current, direction) => {
        const idx = STATUS_CYCLE.indexOf(current);
        const safeIdx = idx === -1 ? 0 : idx;
        if (direction === 'forward') {
            return STATUS_CYCLE[(safeIdx + 1) % STATUS_CYCLE.length];
        }
        return STATUS_CYCLE[(safeIdx - 1 + STATUS_CYCLE.length) % STATUS_CYCLE.length];
    }, []);

    useEffect(() => {
        const init = async () => {
            try {
                const userData = await getUser();
                setUser(userData);
            } catch {
                /* swallow init user errors */
            }

            try {
                const modeData = await getMode();
                if (modeData?.mode) {
                    setMode(modeData.mode);
                    addLog?.('system', `State asserted: ${modeData.mode}`);
                }
            } catch {
                /* swallow init mode errors */
            }
        };
        init();
    }, [addLog]);

    useEffect(() => {
        const es = new EventSource('/api/events');
        es.onopen = () => { setConnected(true); addLog?.('success', 'Uplink established (SSE).'); };
        es.onmessage = (e) => {
            try {
                const data = JSON.parse(e.data);
                const normalized = normalizeRegistry(data);
                setRegistry(normalized);
                onRegistryChange?.(normalized);
                setSecondsRemaining(60);
            } catch (err) { console.error('Stream parse error', err); }
        };

        es.addEventListener('tick', (e) => {
            try {
                const data = JSON.parse(e.data);
                if (data.seconds_remaining !== undefined) {
                    setSecondsRemaining(data.seconds_remaining);
                }
            } catch (err) { console.error('Tick parse error', err); }
        });

        es.addEventListener('status', (e) => {
            try {
                const data = JSON.parse(e.data);
                if (data.id && data.status) {
                    setRegistry(prev => prev.map(item => item.id === data.id ? { ...item, status: data.status } : item));
                    addLog?.(data.status, `Status → ${data.status}: ${data.title || data.id}`);
                }
            } catch (err) { console.error('Status event parse error', err); }
        });

        es.onerror = () => setConnected(false);
        return () => { es.close(); setConnected(false); };
    }, [addLog, onRegistryChange]);

    return {
        mode,
        registry,
        setRegistry,
        user,
        connected,
        secondsRemaining,
        syncMode,
        fetchRegistry,
        fetchDetail,
        deleteItem,
        updateStatus,
        nextStatus,
    };
}
