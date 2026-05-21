// Copyright (c) 2026 Justin Andrew Wood. All rights reserved.
// This software is licensed under the AGPL-3.0.
// Commercial licensing is available at echosh-labs.com.
import { render, screen } from '@testing-library/react';
import { describe, it, expect } from 'vitest';
import RegistryList from '../components/RegistryList.jsx';
import DetailPanel from '../components/DetailPanel.jsx';
import HelpOverlay from '../components/HelpOverlay.jsx';
import HeaderBar from '../components/HeaderBar.jsx';
import ShortcutsFooter from '../components/ShortcutsFooter.jsx';

const sampleRegistry = [
    { id: '1', title: 'First', type: 'keep', status: 'Pending', snippet: 'alpha' },
    { id: '2', title: 'Second', type: 'keep', status: 'Execute', snippet: 'beta' },
    { id: '3', title: 'Third', type: 'keep', status: 'Complete', snippet: 'gamma' },
];

describe('Components rendering', () => {
    it('renders registry list with selection', () => {
        render(
            <RegistryList
                registry={sampleRegistry}
                selectedIndex={1}
                registryRef={{ current: null }}
                getTagStyles={() => 'border-yellow-700/60 text-yellow-300'}
                deletingIds={new Set()}
            />
        );
        expect(screen.getByText('Second')).toBeInTheDocument();
        expect(screen.getByText('First')).toBeInTheDocument();
        expect(screen.getByText('Third')).toBeInTheDocument();
    });

    it('renders detail panel for keep item', () => {
        render(
            <DetailPanel
                title="Sample"
                status="Pending"
                isKeep
                detailContent="Body text"
                detailItem={{ id: '1', foo: 'bar' }}
                detailLoading={false}
                detailError={null}
                detailRef={{ current: null }}
                onExit={() => { }}
            />
        );
        expect(screen.getByText('Detail: Sample')).toBeInTheDocument();
        expect(screen.getByText('Body text')).toBeInTheDocument();
    });

    it('renders terminal interrupt when item is deleting', () => {
        const deletingSet = new Set(['2']);
        render(
            <RegistryList
                registry={sampleRegistry}
                selectedIndex={1}
                registryRef={{ current: null }}
                getTagStyles={() => 'border-yellow-700/60 text-yellow-300'}
                deletingIds={deletingSet}
            />
        );
        expect(screen.getByText('DELETING: Second')).toBeInTheDocument();
        expect(screen.getByText('Please wait while the resource is being removed...')).toBeInTheDocument();
    });
});

describe('Additional Components', () => {
    it('renders HeaderBar in disconnected state', () => {
        render(
            <HeaderBar
                mode="AUTO"
                user={{ email: 'test@example.com' }}
                connected={false}
            />
        );
        expect(screen.getByText(/test@example.com/)).toBeInTheDocument();
    });

    it('renders ShortcutsFooter next tick in auto mode', () => {
        render(
            <ShortcutsFooter
                mode="AUTO"
                secondsRemaining={45}
            />
        );
        expect(screen.getByText(/NEXT TICK: 45s/)).toBeInTheDocument();
    });

    it('renders HelpOverlay when open', () => {
        render(
            <HelpOverlay
                isOpen={true}
                onClose={() => { }}
            />
        );
        expect(screen.getByText(/TUI Operations/)).toBeInTheDocument();
        expect(screen.getByText(/API Endpoints/)).toBeInTheDocument();
    });

    it('does not render HelpOverlay when closed', () => {
        const { container } = render(
            <HelpOverlay
                isOpen={false}
                onClose={() => { }}
            />
        );
        expect(container).toBeEmptyDOMElement();
    });
});
