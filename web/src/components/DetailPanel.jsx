// Copyright (c) 2026 Justin Andrew Wood. All rights reserved.
// This software is licensed under the AGPL-3.0.
// Commercial licensing is available at echosh-labs.com.
const DetailPanel = ({
    title,
    status,
    isKeep,
    isDoc,
    isSheet,
    isGmail,
    isCalendar,
    detailContent,
    sheetValues,
    detailItem,
    detailLoading,
    detailError,
    detailRef,
    onExit,
}) => (
    <div className="flex-1 flex flex-col overflow-hidden bg-black/60 m-2 border border-blue-900/30 rounded p-2">
        <div className="flex justify-between items-start text-[10px] mb-2 font-bold uppercase">
            <div className="flex flex-col">
                <span className="text-blue-400">Detail: {title}</span>
                <span className={`text-[9px] mt-1 ${status === 'Execute' ? 'text-purple-300' : status === 'Complete' ? 'text-emerald-300' : 'text-yellow-300'}`}>
                    Status: {status || 'Pending'}
                </span>
            </div>
            <span className="cursor-pointer text-blue-400" onClick={onExit}>[ESC] EXIT</span>
        </div>
        {detailLoading && (
            <div className="flex-1 text-[10px] text-blue-300 overflow-auto scrollbar-hide bg-black/40 p-2">Loading detail...</div>
        )}
        {!detailLoading && detailError && (
            <div className="flex-1 text-[10px] text-red-400 overflow-auto scrollbar-hide bg-black/40 p-2">{detailError}</div>
        )}
        {!detailLoading && !detailError && detailItem && (
            <div ref={detailRef} className="flex-1 flex flex-col gap-2 overflow-auto scrollbar-hide">
                {isKeep && (
                    <div className="border border-emerald-900/40 bg-black/50 p-2 rounded">
                        <div className="text-[9px] uppercase text-emerald-500 mb-1">Body Content</div>
                        <div className="text-[11px] text-emerald-200 whitespace-pre-wrap leading-relaxed select-text">
                            {detailContent || 'No body content.'}
                        </div>
                    </div>
                )}
                {isDoc && (
                    <div className="border border-indigo-900/40 bg-black/50 p-2 rounded">
                        <div className="text-[9px] uppercase text-indigo-400 mb-1">Document Content</div>
                        <div className="text-[11px] text-indigo-200 whitespace-pre-wrap leading-relaxed select-text">
                            {detailContent || 'Document is blank or unreadable.'}
                        </div>
                    </div>
                )}
                {isGmail && (
                    <div className="border border-gray-700 bg-black/50 p-2 rounded">
                        <div className="text-[9px] uppercase text-gray-400 mb-1">Gmail Thread Content</div>
                        <div className="text-[11px] text-gray-200 whitespace-pre-wrap leading-relaxed select-text">
                            {detailContent || 'No thread content found.'}
                        </div>
                    </div>
                )}
                {isCalendar && (
                    <div className="border border-red-900/40 bg-black/50 p-2 rounded">
                        <div className="text-[9px] uppercase text-red-400 mb-1">Calendar Event Description</div>
                        <div className="text-[11px] text-red-200 whitespace-pre-wrap leading-relaxed select-text">
                            {detailContent || 'No description provided.'}
                        </div>
                    </div>
                )}
                {isSheet && (
                    <div className="border border-emerald-900/40 bg-black/50 p-2 rounded overflow-auto shrink-0 min-h-[250px] max-h-[60vh]">
                        <div className="text-[9px] uppercase text-emerald-400 mb-2">Grid Data</div>
                        <div className="select-text">
                            {(!sheetValues || sheetValues.length === 0) ? (
                                <div className="text-[10px] text-emerald-200/50 italic">Spreadsheet is empty or unreadable.</div>
                            ) : (
                                <table className="w-full text-left border-collapse border border-emerald-900/40 mt-1">
                                    <tbody>
                                        {sheetValues.map((row, rIdx) => (
                                            <tr key={rIdx} className="border-b border-emerald-900/30">
                                                {row.map((cell, cIdx) => (
                                                    <td key={cIdx} className="text-[10px] text-emerald-100 p-1 px-2 border-r border-emerald-900/30 whitespace-nowrap">
                                                        {cell !== null && cell !== undefined && cell !== '' ? String(cell) : <span className="text-emerald-900/50 italic">null</span>}
                                                    </td>
                                                ))}
                                            </tr>
                                        ))}
                                    </tbody>
                                </table>
                            )}
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
);

export default DetailPanel;
