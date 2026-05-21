// Copyright (c) 2026 Justin Andrew Wood. All rights reserved.
// This software is licensed under the AGPL-3.0.
// Commercial licensing is available at echosh-labs.com.
const HeaderBar = ({ user, connected, mode, onSyncMode, onRefresh }) => {
    return (
        <>
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
                    <span className={mode === 'AUTO' ? "text-emerald-500 font-bold" : "text-gray-600 cursor-pointer"} onClick={() => onSyncMode('AUTO')}>[A] AUTO</span>
                    <span className={mode === 'MANUAL' ? "text-yellow-600 font-bold" : "text-gray-600 cursor-pointer"} onClick={() => onSyncMode('MANUAL')}>[M] MANUAL</span>
                    <span className="text-blue-500 cursor-pointer" onClick={onRefresh}>[R] REFRESH</span>
                </div>
                <div className={mode === 'AUTO' ? "text-emerald-400 animate-pulse" : "text-yellow-600"}>STATUS: {mode}</div>
            </div>
        </>
    );
};

export default HeaderBar;
