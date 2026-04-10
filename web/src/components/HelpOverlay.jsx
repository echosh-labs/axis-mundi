// Copyright (c) 2026 Justin Andrew Wood. All rights reserved.
// This software is licensed under the AGPL-3.0.
// Commercial licensing is available at echosh-labs.com.
const HelpOverlay = ({ isOpen, onClose }) => {
    if (!isOpen) return null;

    return (
        <div className="absolute inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-sm p-4">
            <div className="w-full max-w-3xl flex flex-col border border-cyan-900 bg-black/90 rounded shadow-[0_0_15px_rgba(0,255,255,0.1)] overflow-hidden">
                <div className="flex justify-between items-center border-b border-cyan-900 bg-cyan-950/30 p-3">
                    <h2 className="text-cyan-400 font-bold uppercase tracking-widest text-sm">System Operations & API Reference</h2>
                    <button onClick={onClose} className="text-cyan-500 hover:text-cyan-300 text-xs px-2 py-1 border border-cyan-800 rounded">ESC to Close</button>
                </div>
                <div className="p-4 overflow-y-auto max-h-[70vh] text-xs text-gray-300 flex flex-col gap-6">
                    <section>
                        <h3 className="text-cyan-500 border-b border-cyan-900/50 pb-1 mb-2 uppercase font-semibold">TUI Operations</h3>
                        <div className="grid grid-cols-2 gap-2 font-mono">
                            <div><span className="text-yellow-400 font-bold">A</span> : Auto mode (live stream)</div>
                            <div><span className="text-yellow-400 font-bold">M</span> : Manual mode (enables nav)</div>
                            <div><span className="text-yellow-400 font-bold">R</span> : Refresh registry (manual only)</div>
                            <div><span className="text-yellow-400 font-bold">H</span> : Toggle help overlay</div>
                            <div><span className="text-yellow-400 font-bold">Enter/Space</span> : Inspect highlighted item</div>
                            <div><span className="text-yellow-400 font-bold">Arrows</span> : Navigate list / scroll detail</div>
                            <div><span className="text-yellow-400 font-bold">Del/Bksp</span> : Purge selected item</div>
                            <div><span className="text-yellow-400 font-bold">PgUp/PgDn</span> : Cycle item status (fwd/back)</div>
                            <div><span className="text-yellow-400 font-bold">K/D/S/G/V</span> : Filter Keep/Docs/Sheets/Gmail/All</div>
                            <div><span className="text-yellow-400 font-bold">Esc</span> : Close detail or help (manual)</div>
                        </div>
                    </section>

                    <section>
                        <h3 className="text-cyan-500 border-b border-cyan-900/50 pb-1 mb-2 uppercase font-semibold">API Endpoints</h3>
                        <table className="w-full text-left border-collapse font-mono text-[10px]">
                            <thead>
                                <tr className="text-gray-500">
                                    <th className="pb-2">Endpoint</th>
                                    <th className="pb-2 w-20">Method</th>
                                    <th className="pb-2">Description</th>
                                </tr>
                            </thead>
                            <tbody>
                                <tr className="border-t border-gray-800 hover:bg-white/5"><td className="py-1 text-green-400">/api/registry</td><td>GET</td><td>Unified registry stream state</td></tr>
                                <tr className="border-t border-gray-800 hover:bg-white/5"><td className="py-1 text-green-400">/api/registry?refresh=1</td><td>GET</td><td>Manual fetch (R key)</td></tr>
                                <tr className="border-t border-gray-800 hover:bg-white/5"><td className="py-1 text-cyan-400">/api/{'{'}notes|docs|sheets|gmail{'}'}/detail?id=X</td><td>GET</td><td>Detail payload for selected item</td></tr>
                                <tr className="border-t border-gray-800 hover:bg-white/5"><td className="py-1 text-red-400">/api/{'{'}notes|docs|sheets|gmail{'}'}/delete?id=X</td><td>DELETE</td><td>Purge selected item</td></tr>
                                <tr className="border-t border-gray-800 hover:bg-white/5"><td className="py-1 text-yellow-400">/api/status?id=X&amp;status=Y</td><td>POST</td><td>Update item status (cycle keys)</td></tr>
                                <tr className="border-t border-gray-800 hover:bg-white/5"><td className="py-1 text-purple-400">/api/mode</td><td>GET</td><td>Current Auto/Manual state</td></tr>
                                <tr className="border-t border-gray-800 hover:bg-white/5"><td className="py-1 text-purple-400">/api/mode?set=X</td><td>GET</td><td>Switch to AUTO or MANUAL</td></tr>
                                <tr className="border-t border-gray-800 hover:bg-white/5"><td className="py-1 text-purple-400">/api/user</td><td>GET</td><td>Active user profile</td></tr>
                                <tr className="border-t border-gray-800 hover:bg-white/5"><td className="py-1 text-blue-400">/api/events</td><td>SSE</td><td>Live registry + tick/status events</td></tr>
                            </tbody>
                        </table>
                    </section>
                </div>
            </div>
        </div>
    );
};

export default HelpOverlay;
