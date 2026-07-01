// js/api.js - 处理所有与后端API的通信

const API_BASE = '/v0/api';

async function handleResponse(response) {
    // 如果响应是重定向(通常是session过期), 则让浏览器自动跳转
    if (response.redirected) {
        window.location.href = response.url;
        // 返回一个永远不会 resolve 的 Promise 来中断后续的 .then() 链
        return new Promise(() => {});
    }
    if (!response.ok) {
        const errorData = await response.json().catch(() => ({ error: `HTTP error! status: ${response.status}` }));
        throw new Error(errorData.error);
    }
    const text = await response.text();
    // 检查响应体是否为空, 避免解析空字符串时出错
    return text ? JSON.parse(text) : { success: true };
}

export const api = {
    get: (endpoint) => fetch(`${API_BASE}${endpoint}`).then(handleResponse),
    post: (endpoint, body = {}) => {
        let fetchBody, headers = { 'Content-Type': 'application/json' };
        if (body instanceof FormData || body instanceof URLSearchParams || body instanceof Blob || body instanceof URL) {
            fetchBody = body;
            if (body instanceof FormData || body instanceof URLSearchParams) headers['Content-Type'] = null;
            else if (body instanceof Blob) headers['Content-Type'] = null;
        } else {
            fetchBody = JSON.stringify(body);
        }
        // build options
        const opts = { method: 'POST' };
        if (fetchBody !== undefined) opts.body = fetchBody;
        if (headers['Content-Type'] !== null) opts.headers = headers;
        // if Content-Type was nulled, don't set it
        if (fetchBody !== undefined && (body instanceof FormData || body instanceof URLSearchParams || body instanceof Blob)) {
            // no Content-Type header needed
        }
        return fetch(`${API_BASE}${endpoint}`, opts).then(handleResponse);
    },
    put: (endpoint, body) => fetch(`${API_BASE}${endpoint}`, { method: 'PUT', body: JSON.stringify(body), headers: {'Content-Type': 'application/json'} }).then(handleResponse),
    delete: (endpoint) => fetch(`${API_BASE}${endpoint}`, { method: 'DELETE' }).then(handleResponse),
};