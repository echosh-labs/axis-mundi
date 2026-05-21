// Copyright (c) 2026 Justin Andrew Wood. All rights reserved.
// This software is licensed under the AGPL-3.0.
// Commercial licensing is available at echosh-labs.com.
import { fetchJson } from './fetchJson';

const DEFAULT_RETRY = 1;
const DEFAULT_TIMEOUT = 8000;

export const normalizeRegistryItem = (item) => {
    if (!item) return null;
    const title = item.title || item.name || 'Untitled';
    const status = item.status || 'Pending';
    const snippet = item.snippet || item.preview || '';
    return {
        id: item.id,
        type: item.type || 'keep',
        title,
        status,
        snippet,
        raw: item,
    };
};

export const normalizeRegistry = (list) => {
    if (!Array.isArray(list)) return [];
    return list
        .map(normalizeRegistryItem)
        .filter(Boolean);
};

export async function getMode() {
    return fetchJson('/api/mode', { timeout: DEFAULT_TIMEOUT, retry: DEFAULT_RETRY });
}

export async function setMode(mode) {
    return fetchJson(`/api/mode?set=${mode}`, { timeout: DEFAULT_TIMEOUT, retry: DEFAULT_RETRY });
}

export async function getUser() {
    return fetchJson('/api/user', { timeout: DEFAULT_TIMEOUT, retry: DEFAULT_RETRY });
}

export async function getRegistry(force = false) {
    const url = force ? '/api/registry?refresh=1' : '/api/registry';
    const data = await fetchJson(url, { timeout: DEFAULT_TIMEOUT, retry: DEFAULT_RETRY });
    return normalizeRegistry(data);
}

export async function getDetail(item) {
    if (!item || !item.id) throw new Error('Missing item identifier.');
    let url = '';
    switch (item.type) {
        case 'keep':
            url = `/api/notes/detail?id=${encodeURIComponent(item.id)}`;
            break;
        case 'doc':
            url = `/api/docs/detail?id=${encodeURIComponent(item.id)}`;
            break;
        case 'sheet':
            url = `/api/sheets/detail?id=${encodeURIComponent(item.id)}`;
            break;
        case 'gmail':
            url = `/api/gmail/detail?id=${encodeURIComponent(item.id)}`;
            break;
        case 'calendar':
            url = `/api/calendar/detail?id=${encodeURIComponent(item.id)}`;
            break;
        default:
            throw new Error(`Unknown item type: ${item.type}`);
    }
    return fetchJson(url, { timeout: DEFAULT_TIMEOUT, retry: DEFAULT_RETRY });
}

export async function deleteResource(item) {
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
        case 'gmail':
            url = `/api/gmail/delete?id=${encodeURIComponent(item.id)}`;
            break;
        case 'calendar':
            url = `/api/calendar/delete?id=${encodeURIComponent(item.id)}`;
            break;
        default:
            throw new Error(`Unknown item type for deletion: ${item?.type || 'unknown'}`);
    }
    const res = await fetch(url, { method: 'DELETE', timeout: DEFAULT_TIMEOUT });
    if (!res.ok) {
        const errMsg = await res.text().catch(() => '');
        throw new Error(errMsg || 'Purge request failed');
    }
}

export async function setStatus(item, status) {
    if (!item || !item.id) return;
    return fetch(`/api/status?id=${encodeURIComponent(item.id)}&status=${status}`, { method: 'POST' });
}
