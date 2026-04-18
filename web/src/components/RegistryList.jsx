// Copyright (c) 2026 Justin Andrew Wood. All rights reserved.
// This software is licensed under the AGPL-3.0.
// Commercial licensing is available at echosh-labs.com.
const RegistryList = ({ registry, selectedIndex, mode, registryRef, getTagStyles }) => (
    <div ref={registryRef} className="flex-1 space-y-1 overflow-y-auto scrollbar-hide p-2 pb-2">
        {registry.map((item, i) => {
            const isSelected = i === selectedIndex && mode === 'MANUAL';
            let activeClass = 'border-transparent text-gray-600';

            if (isSelected) {
                if (item.type === 'keep') activeClass = 'bg-yellow-950/30 border-yellow-500 text-yellow-300';
                else if (item.type === 'doc') activeClass = 'bg-blue-950/30 border-blue-500 text-blue-300';
                else if (item.type === 'sheet') activeClass = 'bg-emerald-950/30 border-emerald-500 text-emerald-300';
                else if (item.type === 'gmail') activeClass = 'bg-gray-800/30 border-gray-400 text-gray-200';
                else if (item.type === 'calendar') activeClass = 'bg-red-950/30 border-red-500 text-red-300';
            }

            return (
                <div key={item.id} className={`p-2 border transition-all ${activeClass}`}>
                    <div className="flex justify-between items-center text-xs font-bold">
                        <span>{item.title}</span>
                        <div className="flex gap-2 items-center">
                            <span className={`text-[9px] uppercase px-1.5 py-0.5 rounded ${getTagStyles(item.status || 'Pending')}`}>{item.status || 'Pending'}</span>
                            <span className={`text-[9px] uppercase px-2 py-0.5 rounded-full border ${getTagStyles(item.type)}`}>{item.type}</span>
                        </div>
                    </div>
                    <div className="text-[10px] truncate italic mt-1">{item.snippet || 'No content preview.'}</div>
                </div>
            );
        })}
        <div className="h-2"></div>
    </div>
);

export default RegistryList;
