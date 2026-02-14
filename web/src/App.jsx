/*
File: web/src/App.jsx
Description: React terminal interface for Axis Mundi. Handles keyboard navigation, 
real-time telemetry display, and unified registry management with lowercase key binding.
*/
import { useState, useEffect, useRef, useMemo } from 'react';

const App = () => {
    const [mode, setMode] = useState('MANUAL');
    const [selectedIndex, setSelectedIndex] = useState(0);
    const [showDetail, setShowDetail] = useState(false);
    const [logs, setLogs] = useState([
        { timestamp: new Date().toLocaleTimeString(), type: 'system', message: 'Axis TUI Initialized. Mode: MANUAL' }
    ]);
    const [registry, setRegistry] = useState([]);
    const [user, setUser] = useState(null);
    const [detailItem, setDetailItem] = useState(null);
    const [detailLoading, setDetailLoading] = useState(false);
    const [detailError, setDetailError] = useState(null);
    const [connected, setConnected] = useState(false);
    const [secondsRemaining, setSecondsRemaining] = useState(null);
    const scrollRef = useRef(null);
    const registryRef = useRef(null);
    const detailRef = useRef(null);

    const stateRef = useRef({ mode, selectedIndex, registry, showDetail });
    useEffect(() => {
        stateRef.current = { mode, selectedIndex, registry, showDetail };
    }, [mode, selectedIndex, registry, showDetail]);

    // Auto-scroll registry list when selectedIndex changes
    useEffect(() => {
        if (registryRef.current) {
            const listContainer = registryRef.current;
            const selectedElement = listContainer.children[selectedIndex];
            
            if (selectedElement) {
                // Explicitly handle top/bottom anchoring
                if (selectedIndex === 0) {
                    listContainer.scrollTop = 0;
                    return;
                }
                
                // For the last item, ensure we scroll to the very bottom
                if (selectedIndex === registry.length - 1) {
                    listContainer.scrollTop = listContainer.scrollHeight;
                    return;
                }

                const elementTop = selectedElement.offsetTop;
                const elementBottom = elementTop + selectedElement.clientHeight;
                const containerHeight = listContainer.clientHeight;
                const scrollTop = listContainer.scrollTop;
                
                // If element is above the visible area (with some buffer for padding)
                if (elementTop < scrollTop) {
                    listContainer.scrollTop = elementTop;
                }
                // If element is below the visible area
                else if (elementBottom > scrollTop + containerHeight) {
                    listContainer.scrollTop = elementBottom - containerHeight;
                }
            }
        }
    }, [selectedIndex, registry, showDetail]);

    const addLog = (type, message) => {
        setLogs(prev => [...prev, { timestamp: new Date().toLocaleTimeString(), type, message }]);
    };

    const syncMode = async (newMode) => {
        setMode(newMode);
        try {
            await fetch(`/api/mode?set=${newMode}`);
        } catch (err) {
            addLog('error', `Failed to sync mode ${newMode}`);
        }
    };

    const fetchRegistry = async () => {
        try {
            const res = await fetch('/api/registry');
            const data = await res.json();
            const list = Array.isArray(data) ? data : [];
            const filtered = list.filter(item => item.type === 'keep');
            setRegistry(filtered);
            addLog('success', 'Manual registry refresh.');
        } catch (err) {
            addLog('error', 'Failed to retrieve registry.');
        }
    };

    const loadItemDetail = async (item) => {
        if (!item || !item.id) {
            setDetailError('Missing item identifier.');
            return;
        }
        setDetailLoading(true);
        setDetailItem(null);
        setDetailError(null);

        let url = '';
        switch (item.type) {
            case 'keep':
                url = `/api/notes/detail?id=${encodeURIComponent(item.id)}`;
                break;
            case 'doc':
                url = `/api/docs?id=${encodeURIComponent(item.id)}`;
                break;
            case 'sheet':
                url = `/api/sheets?id=${encodeURIComponent(item.id)}`;
                break;
            default:
                setDetailError(`Unknown item type: ${item.type}`);
                setDetailLoading(false);
                return;
        }

        try {
            const res = await fetch(url);
            if (!res.ok) throw new Error(`detail fetch failed for ${item.type}`);
            const data = await res.json();
            setDetailItem(data);
            addLog('success', `Detail pulled for ${item.type}: ${item.id}`);
        } catch (err) {
            setDetailError(`Failed to load detail for ${item.type}.`);
            addLog('error', `Detail retrieval failed for: ${item.id}`);
        } finally {
            setDetailLoading(false);
        }
    };

    const deleteItem = async (item) => {
        if (!item || !item.id) return;

        let url = '';
        switch (item.type) {
            case 'keep':
                url = `/api/notes/delete?id=${encodeURIComponent(item.id)}`;
                break;
            case 'doc':
                url = `/api/docs/delete?id=${encodeURIComponent(item.id)}`;
                break;
            case 'sheet':
                url = `/api/sheets/delete?id=${encodeURIComponent(item.id)}`;
                break;
            default:
                addLog('error', `Unknown item type for deletion: ${item.type}`);
                return;
        }

        try {
            const res = await fetch(url, { method: 'DELETE' });
            if (res.ok) {
                addLog('success', `Object purged (${item.type}): ${item.id}`);
            } else {
                throw new Error('Purge request failed');
            }
        } catch (err) {
            addLog('error', `Purge failed for ${item.type}: ${item.id}`);
        }
    };

    useEffect(() => {
        const init = async () => {
            try {
                const userRes = await fetch('/api/user');
                if (userRes.ok) setUser(await userRes.json());
                
                const modeRes = await fetch('/api/mode');
                if (modeRes.ok) {
                    const modeData = await modeRes.json();
                    if (modeData.mode) {
                        setMode(modeData.mode);
                        addLog('system', `State asserted: ${modeData.mode}`);
                    }
                }
            } catch (e) {
                console.error("Init failed", e);
            }
        };
        init();
    }, []);

    useEffect(() => {
        const es = new EventSource('/api/events');
        es.onopen = () => { setConnected(true); addLog('success', 'Uplink established (SSE).'); };
        
        es.onmessage = (e) => {
            try {
                const data = JSON.parse(e.data);
                const list = Array.isArray(data) ? data : [];
                const filtered = list.filter(item => item.type === 'keep');
                setRegistry(filtered);
                setSecondsRemaining(60); // Reset indication on refresh
                setSelectedIndex(prev => {
                    if (filtered.length === 0) return 0;
                    return Math.min(prev, filtered.length - 1);
                });
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
                if (data.status && data.title) {
                    const logType = data.status === 'Execute' ? 'execute' : 'warning';
                    addLog(logType, `Status → ${data.status}: ${data.title}`);
                }
            } catch (err) { console.error('Status event parse error', err); }
        });

        es.onerror = () => setConnected(false);
        return () => { es.close(); setConnected(false); };
    }, []);

    useEffect(() => {
        const handleKeyDown = (e) => {
            const { mode, selectedIndex, registry, showDetail } = stateRef.current;
            const key = e.key.toLowerCase();

            if (key === 'a') { syncMode('AUTO'); setShowDetail(false); return; }
            if (key === 'm') { syncMode('MANUAL'); return; }
            if (key === 'r') { 
                if (mode === 'MANUAL') fetchRegistry(); 
                return; 
            }

            if (mode !== 'MANUAL') return;

            if (showDetail && key === 'escape') {
                setShowDetail(false);
                setDetailItem(null);
                setDetailError(null);
                setDetailLoading(false);
                return;
            }

            switch (e.key) {
                case 'ArrowDown':
                    e.preventDefault();
                    if (showDetail) {
                        if (detailRef.current) detailRef.current.scrollTop += 50;
                    } else if (registry.length > 0) {
                        setSelectedIndex((selectedIndex + 1) % registry.length);
                    }
                    break;
                case 'ArrowUp':
                    e.preventDefault();
                    if (showDetail) {
                        if (detailRef.current) detailRef.current.scrollTop -= 50;
                    } else if (registry.length > 0) {
                        setSelectedIndex((selectedIndex - 1 + registry.length) % registry.length);
                    }
                    break;
                case 'Enter':
                case ' ':
                    e.preventDefault();
                    if (registry.length === 0) break;
                    const target = registry[selectedIndex];
                    if (target) {
                        setShowDetail(true);
                        loadItemDetail(target);
                    }
                    break;
                case 'Delete':
                case 'Backspace':
                    if (registry[selectedIndex]) deleteItem(registry[selectedIndex]);
                    break;
                case 'PageUp':
                case 'PageDown':
                    e.preventDefault();
                    if (registry.length === 0) break;
                    const currentItem = registry[selectedIndex];
                    if (currentItem && currentItem.type === 'keep') {
                        const currentStatus = currentItem.status || 'Pending';
                        const cycle = ['Pending', 'Execute'];
                        let idx = cycle.indexOf(currentStatus);
                        if (idx === -1) idx = 0;
                        
                        if (e.key === 'PageUp') {
                            idx = (idx + 1) % cycle.length;
                        } else {
                            idx = (idx - 1 + cycle.length) % cycle.length;
                        }
                        
                        const newStatus = cycle[idx];
                        
                        // Optimistic update
                        setRegistry(prev => prev.map(item => 
                            item.id === currentItem.id ? { ...item, status: newStatus } : item
                        ));

                        fetch(`/api/status?id=${encodeURIComponent(currentItem.id)}&status=${newStatus}`, { method: 'POST' })
                            .catch(err => addLog('error', 'Failed to save status'));
                    }
                    break;
            }
        };

        window.addEventListener('keydown', handleKeyDown);
        return () => window.removeEventListener('keydown', handleKeyDown);
    }, []);

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

    const detailContent = useMemo(() => {
        if (!detailItem) return '';
        const selectedItem = registry[selectedIndex];
        if (selectedItem && selectedItem.type === 'keep') {
            return formatNoteContent.fromNote(detailItem);
        }
        return 'Detail view not applicable for this item type.';
    }, [detailItem, formatNoteContent, registry, selectedIndex]);

    const getTagStyles = (tag) => {
        switch (tag) {
            case 'keep':
            case 'Pending':
                return 'border-yellow-700/60 text-yellow-300';
            case 'Execute':
                return 'border-purple-700/60 text-purple-300';
            case 'doc': return 'border-blue-700/60 text-blue-300';
            case 'sheet': return 'border-green-700/60 text-green-300';
            default: return 'border-gray-700/60 text-gray-300';
        }
    };

    return (
        <div className="flex flex-col h-screen p-4 select-text relative outline-none" tabIndex="0">
             <div className="mb-4 border border-gray-900 bg-black/60 p-3 rounded flex justify-between items-center">
                <div className="text-lg font-bold tracking-[0.6em] lowercase bg-gradient-to-r from-violet-900 via-purple-700 to-emerald-700 text-transparent bg-clip-text shimmer-text drop-shadow-[0_0_8px_rgba(76,29,149,0.45)]">axis mundi</div>
                <div className="w-full max-w-sm">
                    <div className="flex items-center justify-between">
                        <div className="text-xs text-gray-500 font-bold">
                            {user ? `${user.name} (${user.email})` : 'Syncing user...'}
                        </div>
                        <div className={`w-2 h-2 rounded-full ${connected ? 'bg-emerald-500 shadow-[0_0_5px_rgba(16,185,129,0.5)]' : 'bg-red-500 animate-pulse'}`}></div>
                    </div>
                    <div className="text-[9px] text-gray-600 uppercase tracking-widest mt-1">
                        {user && user.id ? `USER PROFILE: ID#${user.id}` : 'USER PROFILE'}
                    </div>
                </div>
            </div>
            <div className="flex justify-between items-center border-b border-gray-900 pb-2 mb-4 text-[10px] tracking-widest uppercase">
                <div className="flex gap-8">
                    <span className={mode === 'AUTO' ? "text-emerald-500 font-bold" : "text-gray-600 cursor-pointer"} onClick={() => syncMode('AUTO')}>[A] AUTO</span>
                    <span className={mode === 'MANUAL' ? "text-yellow-600 font-bold" : "text-gray-600 cursor-pointer"} onClick={() => syncMode('MANUAL')}>[M] MANUAL</span>
                    <span className={mode === 'MANUAL' ? "text-blue-500 cursor-pointer" : "text-gray-700 cursor-not-allowed"} onClick={() => mode === 'MANUAL' && fetchRegistry()}>[R] REFRESH</span>
                </div>
                <div className={mode === 'AUTO' ? "text-emerald-400 animate-pulse" : "text-yellow-600"}>STATUS: {mode}</div>
            </div>

            <div className="flex flex-1 gap-4 overflow-hidden">
                <div className="w-1/2 flex flex-col border border-gray-900 bg-black/40 p-3 rounded">
                    <div className="text-[9px] text-gray-600 mb-2 uppercase border-b border-gray-900 pb-1">Telemetry Buffer</div>
                    <div ref={scrollRef} className="flex-1 overflow-y-auto space-y-1 text-[11px] scrollbar-hide">
                        {logs.map((log, i) => (
                            <div key={i} className="flex gap-2">
                                <span className="text-gray-700">[{log.timestamp}]</span>
                                <span className={
                                    log.type === 'error' ? 'text-red-500' :
                                    log.type === 'success' ? 'text-emerald-500' :
                                    log.type === 'warning' ? 'text-yellow-500' :
                                    log.type === 'execute' ? 'text-purple-300' :
                                    'text-gray-500'
                                }>
                                    {log.message}
                                </span>
                            </div>
                        ))}
                    </div>
                </div>

                <div className="w-1/2 flex flex-col border border-gray-900 bg-black/40 rounded overflow-hidden relative">
                    <div className="text-[9px] text-gray-600 uppercase border-b border-gray-900 p-2 flex justify-between bg-black/60 z-10">
                        <span>Unified Registry</span>
                        <span className="text-[8px] text-gray-700">{connected ? 'LIVE STREAM' : 'DISCONNECTED'}</span>
                    </div>
                    {!showDetail ? (
                        <div ref={registryRef} className="flex-1 space-y-1 overflow-y-auto scrollbar-hide p-2 pb-2">
                            {registry.map((item, i) => {
                                const tagLabel = (item.type === 'keep')
                                    ? (item.status || 'Pending')
                                    : item.type;
                                return (
                                <div key={item.id} className={`p-2 border transition-all ${i === selectedIndex && mode === 'MANUAL' ? 'bg-emerald-950/30 border-emerald-500 text-emerald-300' : 'border-transparent text-gray-600'}`}>
                                    <div className="flex justify-between text-xs font-bold">
                                        <span>{item.title}</span>
                                        <span className={`text-[9px] uppercase px-2 py-0.5 rounded-full border ${getTagStyles(tagLabel)}`}>{tagLabel}</span>
                                    </div>
                                    <div className="text-[10px] truncate italic">{item.snippet || 'No content preview.'}</div>
                                </div>
                                );
                            })}
                            <div className="h-2"></div>
                        </div>
                    ) : (
                        <div className="flex-1 flex flex-col overflow-hidden bg-black/60 m-2 border border-blue-900/30 rounded p-2">
                            <div className="flex justify-between items-start text-[10px] mb-2 font-bold uppercase">
                                <div className="flex flex-col">
                                    <span className="text-blue-400">Detail: {registry[selectedIndex]?.title || 'Unknown'}</span>
                                    {registry[selectedIndex]?.type === 'keep' && (
                                        <span className={`text-[9px] mt-1 ${registry[selectedIndex]?.status === 'Execute' ? 'text-purple-300' : 'text-yellow-300'}`}>
                                            Status: {registry[selectedIndex]?.status || 'Pending'}
                                        </span>
                                    )}
                                </div>
                                <span className="cursor-pointer text-blue-400" onClick={() => { setShowDetail(false); setDetailItem(null); setDetailError(null); setDetailLoading(false); }}>[ESC] EXIT</span>
                            </div>
                            {detailLoading && (
                                <div className="flex-1 text-[10px] text-blue-300 overflow-auto scrollbar-hide bg-black/40 p-2">Loading detail...</div>
                            )}
                            {!detailLoading && detailError && (
                                <div className="flex-1 text-[10px] text-red-400 overflow-auto scrollbar-hide bg-black/40 p-2">{detailError}</div>
                            )}
                            {!detailLoading && !detailError && detailItem && (
                                <div ref={detailRef} className="flex-1 flex flex-col gap-2 overflow-auto scrollbar-hide">
                                    {registry[selectedIndex]?.type === 'keep' && (
                                        <div className="border border-emerald-900/40 bg-black/50 p-2 rounded">
                                            <div className="text-[9px] uppercase text-emerald-500 mb-1">Body Content</div>
                                            <div className="text-[11px] text-emerald-200 whitespace-pre-wrap leading-relaxed select-text">
                                                {detailContent || 'No body content.'}
                                            </div>
                                        </div>
                                    )}
                                    <div className="border border-blue-900/40 bg-black/50 p-2 rounded">
                                        <div className="text-[9px] uppercase text-blue-400 mb-1">Raw Payload</div>
                                        <pre className="text-[10px] text-blue-300 overflow-auto scrollbar-hide bg-black/40 p-2 rounded select-text">
                                            {JSON.stringify(detailItem, null, 2)}
                                        </pre>
                                    </div>
                                </div>
                            )}
                            {!detailLoading && !detailError && !detailItem && (
                                <div className="flex-1 text-[10px] text-blue-300 overflow-auto scrollbar-hide bg-black/40 p-2">No data available.</div>
                            )}
                        </div>
                    )}
                </div>
            </div>
            <div className="mt-4 flex justify-between text-[9px] text-gray-600 border-t border-gray-900 pt-2 uppercase italic">
                <span>Arrows: Nav | Enter: Inspect | Delete: Kill</span>
                <span className="flex gap-4">
                    {mode === 'AUTO' && secondsRemaining !== null && (
                        <span className="text-emerald-500 font-bold">NEXT TICK: {secondsRemaining}s</span>
                    )}
                    <span>Postural Alignment: Neutral Axis</span>
                </span>
            </div>
        </div>
    );
};

export default App;