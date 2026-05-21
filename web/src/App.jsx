// Copyright (c) 2026 Justin Andrew Wood. All rights reserved.
// This software is licensed under the AGPL-3.0.
// Commercial licensing is available at echosh-labs.com.
import { useState, useEffect, useRef, useMemo, useCallback } from 'react';
import HeaderBar from './components/HeaderBar.jsx';
import TelemetryPanel from './components/TelemetryPanel.jsx';
import RegistryList from './components/RegistryList.jsx';
import DetailPanel from './components/DetailPanel.jsx';
import ShortcutsFooter from './components/ShortcutsFooter.jsx';
import HelpOverlay from './components/HelpOverlay.jsx';
import ThreeDOverlay from './components/ThreeDOverlay.jsx';
import { useRegistry } from './hooks/useRegistry.js';
import { useHotkeys } from './hooks/useHotkeys.js';

const App = () => {
    const [selectedIndex, setSelectedIndex] = useState(0);
    const [viewType, setViewType] = useState('keep'); // 'keep', 'doc', 'sheet'
    const [showDetail, setShowDetail] = useState(false);
    const [showHelp, setShowHelp] = useState(false);
    const [logs, setLogs] = useState([
        { timestamp: new Date().toLocaleTimeString(), type: 'system', message: 'Axis TUI Initialized. Mode: MANUAL' }
    ]);
    const [detailItem, setDetailItem] = useState(null);
    const [detailLoading, setDetailLoading] = useState(false);
    const [detailError, setDetailError] = useState(null);
    const [deletingIds, setDeletingIds] = useState(new Set());
    const scrollRef = useRef(null);
    const registryRef = useRef(null);
    const detailRef = useRef(null);

    const addLog = useCallback((type, message) => {
        setLogs(prev => [...prev, { timestamp: new Date().toLocaleTimeString(), type, message }]);
    }, []);

    const handleRegistryChange = useCallback((list) => {
        setSelectedIndex((prev) => {
            if (list.length === 0) return 0;
            return Math.min(prev, list.length - 1);
        });
    }, []);

    const {
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
    } = useRegistry({ addLog, onRegistryChange: handleRegistryChange });

    const visibleRegistry = useMemo(() => {
        if (viewType === 'all') return registry;
        return registry.filter(item => item.type === viewType);
    }, [registry, viewType]);

    useEffect(() => {
        if (!registryRef.current || showDetail) return;
        const listContainer = registryRef.current;
        const selectedElement = listContainer.children[selectedIndex];
        if (!selectedElement) return;

        if (selectedIndex === 0) {
            listContainer.scrollTop = 0;
            return;
        }

        if (selectedIndex === visibleRegistry.length - 1) {
            listContainer.scrollTop = listContainer.scrollHeight;
            return;
        }

        selectedElement.scrollIntoView({ block: 'nearest' });
    }, [selectedIndex, visibleRegistry.length, showDetail]);

    useEffect(() => {
        if (scrollRef.current) {
            scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
        }
    }, [logs]);

    const closeDetail = useCallback(() => {
        setShowDetail(false);
        setDetailItem(null);
        setDetailError(null);
        setDetailLoading(false);
    }, []);

    const setDetailVisibility = useCallback((value) => {
        if (!value) {
            closeDetail();
        } else {
            setShowDetail(true);
        }
    }, [closeDetail]);

    const handleInspect = useCallback(async () => {
        if (visibleRegistry.length === 0) return;
        const target = visibleRegistry[selectedIndex];
        if (!target) return;

        setShowDetail(true);
        setDetailLoading(true);
        setDetailItem(null);
        setDetailError(null);

        try {
            const data = await fetchDetail(target);
            setDetailItem(data);
            addLog('success', `Detail pulled for ${target.type}: ${target.id}`);
        } catch (err) {
            setDetailError(err.message || `Failed to load detail for ${target?.type || 'item'}.`);
            addLog('error', `Detail retrieval failed for: ${target?.id || 'unknown'}`);
        } finally {
            setDetailLoading(false);
        }
    }, [visibleRegistry, selectedIndex, fetchDetail, addLog]);

    const handleDelete = useCallback(async () => {
        const target = visibleRegistry[selectedIndex];
        if (!target) return;

        if (mode === 'AUTO') {
            addLog('error', 'Operation Denied: Items can only be purged in MANUAL mode.');
            return;
        }

        setDeletingIds(prev => {
            const next = new Set(prev);
            next.add(target.id);
            return next;
        });

        if (showDetail) closeDetail();

        try {
            await deleteItem(target);
        } finally {
            setDeletingIds(prev => {
                const next = new Set(prev);
                next.delete(target.id);
                return next;
            });
        }
    }, [visibleRegistry, selectedIndex, deleteItem, showDetail, closeDetail, mode, addLog]);

    const handleCycleStatus = useCallback((direction) => {
        if (visibleRegistry.length === 0) return;
        const currentItem = visibleRegistry[selectedIndex];
        if (!currentItem) return;

        const newStatus = nextStatus(currentItem.status || 'Pending', direction === 'forward' ? 'forward' : 'back');
        setRegistry(prev => prev.map(item => item.id === currentItem.id ? { ...item, status: newStatus } : item));

        updateStatus(currentItem, newStatus).catch(() => {
            setRegistry(prev => prev.map(item => item.id === currentItem.id ? { ...item, status: currentItem.status || 'Pending' } : item));
        });
    }, [visibleRegistry, selectedIndex, nextStatus, setRegistry, updateStatus]);

    const handleSelectNext = useCallback(() => {
        setSelectedIndex((prev) => {
            if (visibleRegistry.length === 0) return 0;
            return (prev + 1) % visibleRegistry.length;
        });
    }, [visibleRegistry.length]);

    const handleSelectPrev = useCallback(() => {
        setSelectedIndex((prev) => {
            if (visibleRegistry.length === 0) return 0;
            return (prev - 1 + visibleRegistry.length) % visibleRegistry.length;
        });
    }, [visibleRegistry.length]);

    const handleChangeViewType = useCallback((type) => {
        setViewType(type);
        setSelectedIndex(0);
        setShowDetail(false);
    }, []);

    useHotkeys({
        mode,
        showDetail,
        showHelp,
        setShowDetail: setDetailVisibility,
        onToggleHelp: (state) => setShowHelp(prev => state !== undefined ? state : !prev),
        onSyncMode: syncMode,
        onRefresh: fetchRegistry,
        onSelectNext: handleSelectNext,
        onSelectPrev: handleSelectPrev,
        onInspect: handleInspect,
        onDelete: handleDelete,
        onCycleStatus: handleCycleStatus,
        onChangeViewType: handleChangeViewType,
        detailRef,
        detailScrollStep: 50,
    });

    const formatNoteContent = useMemo(() => {
        const firstDefined = (obj, keys) => {
            if (!obj) return undefined;
            for (const key of keys) {
                if (obj[key] !== undefined && obj[key] !== null) return obj[key];
            }
            return undefined;
        };
        const normalizeString = (value) => {
            if (typeof value === 'string') return value;
            if (!value) return '';
            if (typeof value.text === 'string') return value.text;
            if (typeof value.Text === 'string') return value.Text;
            if (typeof value.value === 'string') return value.value;
            return '';
        };

        return {
            fromNote(note) {
                const section = firstDefined(note, ['body', 'Body']);
                if (!section) return 'No body content.';

                const text = normalizeString(firstDefined(section, ['text', 'Text']));
                if (text.trim() !== '') return text;

                const list = firstDefined(section, ['list', 'List']);
                const itemsList = firstDefined(list, ['listItems', 'ListItems']);
                const items = Array.isArray(itemsList) ? itemsList : [];

                if (items.length > 0) {
                    const lines = [];
                    const walk = (entries, depth) => {
                        entries.forEach((entry) => {
                            const raw = normalizeString(firstDefined(entry, ['text', 'Text']));
                            const label = raw.trim() === '' ? '[Empty]' : raw;
                            const isChecked = firstDefined(entry, ['checked', 'Checked']);
                            const checkedMarker = isChecked ? ' [x]' : '';
                            lines.push(`${'  '.repeat(depth)}- ${label}${checkedMarker}`);
                            const children = firstDefined(entry, ['childListItems', 'ChildListItems']);
                            if (Array.isArray(children) && children.length > 0) walk(children, depth + 1);
                        });
                    };
                    walk(items, 0);
                    return lines.join('\n');
                }
                return 'No body content.';
            },
        };
    }, []);

    const getTagStyles = (tag) => {
        switch (tag) {
            case 'keep': return 'border-yellow-600/70 text-yellow-400';
            case 'doc': return 'border-blue-600/70 text-blue-400';
            case 'sheet': return 'border-emerald-600/70 text-emerald-400';
            case 'gmail': return 'border-gray-500 text-gray-300';
            case 'calendar': return 'border-red-600/70 text-red-400';

            case 'Pending': return 'bg-yellow-900/30 text-yellow-300';
            case 'Execute': return 'bg-purple-900/30 text-purple-300';
            case 'Active': return 'bg-cyan-900/30 text-cyan-300';
            case 'Blocked': return 'bg-orange-900/30 text-orange-300';
            case 'Review': return 'bg-magenta-900/30 text-magenta-300';
            case 'Complete': return 'bg-emerald-900/30 text-emerald-300';
            case 'Error': return 'bg-red-900/30 text-red-300';
            default: return 'border-gray-700/60 text-gray-300';
        }
    };

    return (
        <div className="flex flex-col h-screen p-4 select-text relative outline-none" tabIndex="0">
            <ThreeDOverlay />
            <HelpOverlay isOpen={showHelp} onClose={() => setShowHelp(false)} />
            <HeaderBar
                user={user}
                connected={connected}
                mode={mode}
                onSyncMode={syncMode}
                onRefresh={fetchRegistry}
            />

            <div className="flex flex-1 gap-4 overflow-hidden">
                <TelemetryPanel logs={logs} scrollRef={scrollRef} />

                <div className="w-1/2 flex flex-col border border-gray-900 bg-black/40 rounded overflow-hidden relative">
                    <div className="text-[9px] text-gray-600 uppercase border-b border-gray-900 p-2 flex justify-between bg-black/60 z-10">
                        <span>Unified Registry</span>
                        <span className="text-[8px] text-gray-700">{connected ? 'LIVE STREAM' : 'DISCONNECTED'}</span>
                    </div>
                    {!showDetail ? (
                        <RegistryList
                            registry={visibleRegistry}
                            selectedIndex={selectedIndex}
                            registryRef={registryRef}
                            getTagStyles={getTagStyles}
                            deletingIds={deletingIds}
                        />
                    ) : (
                        <DetailPanel
                            title={visibleRegistry[selectedIndex]?.title || 'Unknown'}
                            status={visibleRegistry[selectedIndex]?.status}
                            isKeep={visibleRegistry[selectedIndex]?.type === 'keep'}
                            isDoc={visibleRegistry[selectedIndex]?.type === 'doc'}
                            isSheet={visibleRegistry[selectedIndex]?.type === 'sheet'}
                            isGmail={visibleRegistry[selectedIndex]?.type === 'gmail'}
                            isCalendar={visibleRegistry[selectedIndex]?.type === 'calendar'}
                            detailContent={visibleRegistry[selectedIndex]?.type === 'keep' ? formatNoteContent.fromNote(detailItem) : (visibleRegistry[selectedIndex]?.type === 'doc' || visibleRegistry[selectedIndex]?.type === 'gmail') ? detailItem?.content : visibleRegistry[selectedIndex]?.type === 'calendar' ? detailItem?.description : null}
                            sheetValues={visibleRegistry[selectedIndex]?.type === 'sheet' ? detailItem?.values : null}
                            detailItem={detailItem}
                            detailLoading={detailLoading}
                            detailError={detailError}
                            detailRef={detailRef}
                            onExit={closeDetail}
                        />
                    )}
                </div>
            </div>

            <ShortcutsFooter mode={mode} secondsRemaining={secondsRemaining} viewType={viewType} />
        </div>
    );
};

export default App;
