import { useState, useEffect, useRef, useMemo } from 'react';

const App = () => {
    const [mode, setMode] = useState('MANUAL');
    const [selectedIndex, setSelectedIndex] = useState(0);
    const [showDetail, setShowDetail] = useState(false);
    const [logs, setLogs] = useState([
        { timestamp: new Date().toLocaleTimeString(), type: 'system', message: 'Axis TUI Initialized. Mode: MANUAL' }
    ]);
    const [notes, setNotes] = useState([]);
    const [user, setUser] = useState(null);
    const [detailNote, setDetailNote] = useState(null);
    const [detailLoading, setDetailLoading] = useState(false);
    const [detailError, setDetailError] = useState(null);
    const [connected, setConnected] = useState(false);
    const scrollRef = useRef(null);

    // State Ref Pattern: Keeps the event listener stable while accessing fresh data
    const stateRef = useRef({ mode, selectedIndex, notes, showDetail });
    useEffect(() => {
        stateRef.current = { mode, selectedIndex, notes, showDetail };
    }, [mode, selectedIndex, notes, showDetail]);

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

    const fetchNotes = async () => {
        try {
            const res = await fetch('/api/notes');
            const data = await res.json();
            const list = Array.isArray(data) ? data : [];
            setNotes(list);
            addLog('success', 'Manual registry refresh.');
        } catch (err) {
            addLog('error', 'Failed to retrieve notes.');
        }
    };

    const loadNoteDetail = async (id) => {
        if (!id) {
            setDetailError('Missing note identifier.');
            return;
        }
        setDetailLoading(true);
        setDetailNote(null);
        setDetailError(null);
        try {
            const res = await fetch(`/api/notes/detail?id=${encodeURIComponent(id)}`);
            if (!res.ok) throw new Error('detail fetch failed');
            const data = await res.json();
            setDetailNote(data);
            addLog('success', `Detail pulled: ${id}`);
        } catch (err) {
            setDetailError('Failed to load note detail.');
            addLog('error', `Detail retrieval failed for: ${id}`);
        } finally {
            setDetailLoading(false);
        }
    };

    const deleteNote = async (id) => {
        try {
            const res = await fetch(`/api/notes/delete?id=${encodeURIComponent(id)}`, { method: 'DELETE' });
            if (res.ok) {
                addLog('success', `Object purged: ${id}`);
            }
        } catch (err) {
            addLog('error', `Purge failed for: ${id}`);
        }
    };

    // User & Mode Init
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

    // SSE Stream
    useEffect(() => {
        const es = new EventSource('/api/events');
        es.onopen = () => { setConnected(true); addLog('success', 'Uplink established (SSE).'); };
        es.onmessage = (e) => {
            try {
                const data = JSON.parse(e.data);
                const list = Array.isArray(data) ? data : [];
                setNotes(list);
                // Auto-clamp index if list shrinks
                setSelectedIndex(prev => {
                    if (list.length === 0) return 0;
                    return Math.min(prev, list.length - 1);
                });
            } catch (err) { console.error('Stream parse error', err); }
        };
        es.onerror = () => setConnected(false);
        return () => { es.close(); setConnected(false); };
    }, []);

    // Stable Keyboard Handler
    useEffect(() => {
        const handleKeyDown = (e) => {
            const { mode, selectedIndex, notes, showDetail } = stateRef.current;
            const key = e.key.toLowerCase();

            // Global Shortcuts
            if (key === 'a') { syncMode('AUTO'); setShowDetail(false); return; }
            if (key === 'm') { syncMode('MANUAL'); return; }
            if (key === 'r') { fetchNotes(); return; }

            // Manual Mode Only
            if (mode !== 'MANUAL') return;

            // Escape Context
            if (showDetail && key === 'escape') {
                setShowDetail(false);
                setDetailNote(null);
                setDetailError(null);
                setDetailLoading(false);
                return;
            }

            // Navigation
            switch (e.key) {
                case 'ArrowDown':
                    e.preventDefault();
                    if (notes.length > 0) {
                        setSelectedIndex((selectedIndex + 1) % notes.length);
                    }
                    break;
                case 'ArrowUp':
                    e.preventDefault();
                    if (notes.length > 0) {
                        setSelectedIndex((selectedIndex - 1 + notes.length) % notes.length);
                    }
                    break;
                case 'Enter':
                case ' ':
                    e.preventDefault();
                    if (notes.length === 0) break;
                    const target = notes[selectedIndex];
                    if (target) {
                        setShowDetail(true);
                        loadNoteDetail(target.ID);
                    }
                    break;
                case 'Delete':
                case 'Backspace':
                    if (notes[selectedIndex]) deleteNote(notes[selectedIndex].ID);
                    break;
            }
        };

        window.addEventListener('keydown', handleKeyDown);
        return () => window.removeEventListener('keydown', handleKeyDown);
    }, []); // Empty dependency array = binds once

    // Helper: Content Formatting
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
            if (typeof value.Text === 'string') return value.Text;
            if (typeof value.text === 'string') return value.text;
            if (typeof value.value === 'string') return value.value;
            return '';
        };

        return {
            fromNote(note) {
                const section = firstDefined(note, ['Body', 'body']);
                if (!section) return 'No body content.';

                const text = normalizeString(firstDefined(section, ['Text', 'text']));
                if (text.trim() !== '') return text;

                const list = firstDefined(section, ['List', 'list']);
                const items = Array.isArray(firstDefined(list, ['ListItems', 'listItems'])) ? firstDefined(list, ['ListItems', 'listItems']) : [];
                
                if (items.length > 0) {
                    const lines = [];
                    const walk = (entries, depth) => {
                        entries.forEach((entry) => {
                            const raw = normalizeString(firstDefined(entry, ['Text', 'text']));
                            const label = raw.trim() === '' ? '[Empty]' : raw;
                            const checked = entry?.Checked ? ' [x]' : '';
                            lines.push(`${'  '.repeat(depth)}- ${label}${checked}`);
                            const children = firstDefined(entry, ['ChildListItems', 'childListItems']);
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
        if (!detailNote) return '';
        return formatNoteContent.fromNote(detailNote);
    }, [detailNote, formatNoteContent]);

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
                    <span className={mode === 'AUTO' ? "text-emerald-500 font-bold" : "text-gray-600"}>[A] AUTO</span>
                    <span className={mode === 'MANUAL' ? "text-yellow-600 font-bold" : "text-gray-600"}>[M] MANUAL</span>
                    <span className="text-blue-500 cursor-pointer" onClick={fetchNotes}>[R] REFRESH</span>
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
                                <span className={log.type === 'error' ? 'text-red-500' : log.type === 'success' ? 'text-emerald-500' : 'text-gray-500'}>
                                    {log.message}
                                </span>
                            </div>
                        ))}
                    </div>
                </div>

                <div className="w-1/2 flex flex-col border border-gray-900 bg-black/40 p-3 rounded overflow-hidden">
                    <div className="text-[9px] text-gray-600 mb-2 uppercase border-b border-gray-900 pb-1 flex justify-between">
                        <span>Note Registry</span>
                        <span className="text-[8px] text-gray-700">{connected ? 'LIVE STREAM' : 'DISCONNECTED'}</span>
                    </div>
                    {!showDetail ? (
                        <div className="space-y-1 overflow-y-auto scrollbar-hide">
                            {notes.map((note, i) => (
                                <div key={note.ID} className={`p-2 border transition-all ${i === selectedIndex && mode === 'MANUAL' ? 'bg-emerald-950/30 border-emerald-500 text-emerald-300' : 'border-transparent text-gray-600'}`}>
                                    <div className="flex justify-between text-xs font-bold"><span>{note.Title}</span></div>
                                    <div className="text-[10px] truncate italic">{note.Snippet}</div>
                                </div>
                            ))}
                        </div>
                    ) : (
                        <div className="flex-1 flex flex-col overflow-hidden bg-black/60 p-2 border border-blue-900/30 rounded">
                            <div className="flex justify-between text-[10px] text-blue-400 mb-2 font-bold uppercase">
                                <span>Detail: {notes[selectedIndex]?.Title || 'Unknown'}</span>
                                <span onClick={() => { setShowDetail(false); setDetailNote(null); setDetailError(null); setDetailLoading(false); }}>[ESC] EXIT</span>
                            </div>
                            {detailLoading && (
                                <div className="flex-1 text-[10px] text-blue-300 overflow-auto scrollbar-hide bg-black/40 p-2">Loading note detail...</div>
                            )}
                            {!detailLoading && detailError && (
                                <div className="flex-1 text-[10px] text-red-400 overflow-auto scrollbar-hide bg-black/40 p-2">{detailError}</div>
                            )}
                            {!detailLoading && !detailError && detailNote && (
                                <div className="flex-1 flex flex-col gap-2 overflow-auto scrollbar-hide">
                                    <div className="border border-emerald-900/40 bg-black/50 p-2 rounded">
                                        <div className="text-[9px] uppercase text-emerald-500 mb-1">Body Content</div>
                                        <div className="text-[11px] text-emerald-200 whitespace-pre-wrap leading-relaxed select-text">
                                            {detailContent || 'No body content.'}
                                        </div>
                                    </div>
                                    <div className="border border-blue-900/40 bg-black/50 p-2 rounded">
                                        <div className="text-[9px] uppercase text-blue-400 mb-1">Raw Payload</div>
                                        <pre className="text-[10px] text-blue-300 overflow-auto scrollbar-hide bg-black/40 p-2 rounded select-text">
                                            {JSON.stringify(detailNote, null, 2)}
                                        </pre>
                                    </div>
                                </div>
                            )}
                            {!detailLoading && !detailError && !detailNote && (
                                <div className="flex-1 text-[10px] text-blue-300 overflow-auto scrollbar-hide bg-black/40 p-2">No data available.</div>
                            )}
                        </div>
                    )}
                </div>
            </div>
            <div className="mt-4 flex justify-between text-[9px] text-gray-600 border-t border-gray-900 pt-2 uppercase italic">
                <span>Arrows: Nav | Enter: Inspect | Delete: Kill</span>
                <span>Postural Alignment: Neutral Axis</span>
            </div>
        </div>
    );
};

export default App;