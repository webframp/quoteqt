// ==UserScript==
// @name         Nightbot Command Exporter for QuoteQT
// @namespace    http://tampermonkey.net/
// @version      2.1
// @description  Export Nightbot commands to QuoteQT
// @match        https://nightbot.tv/*
// @grant        GM_xmlhttpRequest
// @connect      quoteqt.webframp.com
// @run-at       document-start
// ==/UserScript==

(function() {
    'use strict';

    // ============ CONFIGURATION ============
    const QUOTEQT_URL = 'https://quoteqt.webframp.com/admin/nightbot/snapshot/import';
    const IMPORT_TOKEN = 'YOUR_TOKEN_HERE';  // Get from QuoteQT admin
    // =======================================

    let capturedAuth = null;
    let capturedChannel = null;

    // Intercept fetch to capture headers
    const originalFetch = window.fetch;
    window.fetch = function(url, options) {
        if (options?.headers) {
            let auth = null;
            let channel = null;
            
            // Handle Headers object
            if (options.headers instanceof Headers) {
                auth = options.headers.get('Authorization');
                channel = options.headers.get('Nightbot-Channel');
            } 
            // Handle plain object
            else if (typeof options.headers === 'object') {
                auth = options.headers['Authorization'] || options.headers['authorization'];
                channel = options.headers['Nightbot-Channel'] || options.headers['nightbot-channel'];
            }
            
            if (auth && auth.startsWith('Session ')) {
                capturedAuth = auth;
                console.log('[QuoteQT] Captured auth');
            }
            if (channel) {
                capturedChannel = channel;
                console.log('[QuoteQT] Captured channel:', channel);
            }
        }
        return originalFetch.apply(this, arguments);
    };

    window.addEventListener('load', () => {
        setTimeout(() => {
            // Create button container
            const container = document.createElement('div');
            container.style.cssText = 'position:fixed;top:10px;right:10px;z-index:9999;display:flex;gap:8px;';
            document.body.appendChild(container);

            // Download button
            const downloadBtn = document.createElement('button');
            downloadBtn.textContent = '💾 Download';
            downloadBtn.style.cssText = 'padding:10px 16px;background:#6b7280;color:white;border:none;border-radius:8px;cursor:pointer;font-weight:bold;font-size:14px;';
            container.appendChild(downloadBtn);

            // Send to QuoteQT button
            const sendBtn = document.createElement('button');
            sendBtn.textContent = '📤 Send to QuoteQT';
            sendBtn.style.cssText = 'padding:10px 16px;background:#a855f7;color:white;border:none;border-radius:8px;cursor:pointer;font-weight:bold;font-size:14px;';
            container.appendChild(sendBtn);

            // Fetch commands helper
            async function fetchCommands() {
                if (!capturedAuth) {
                    throw new Error('No auth captured. Navigate around the dashboard first.');
                }

                const headers = { 'Authorization': capturedAuth };
                if (capturedChannel) {
                    headers['Nightbot-Channel'] = capturedChannel;
                }

                const resp = await fetch('https://api.nightbot.tv/1/commands', { headers });
                if (!resp.ok) throw new Error('Failed to fetch commands: ' + resp.status);
                const data = await resp.json();

                const channelResp = await fetch('https://api.nightbot.tv/1/channel', { headers });
                const channelData = await channelResp.json();

                return {
                    exportedAt: new Date().toISOString(),
                    channel: channelData.channel?.displayName || 'unknown',
                    commandCount: data.commands?.length || 0,
                    commands: data.commands || []
                };
            }

            // Download handler
            downloadBtn.addEventListener('click', async () => {
                try {
                    downloadBtn.textContent = '⏳ Fetching...';
                    const backup = await fetchCommands();

                    const blob = new Blob([JSON.stringify(backup, null, 2)], {type: 'application/json'});
                    const url = URL.createObjectURL(blob);
                    const a = document.createElement('a');
                    a.href = url;
                    a.download = `nightbot-${backup.channel}-${new Date().toISOString().split('T')[0]}.json`;
                    a.click();

                    downloadBtn.textContent = `✅ ${backup.commandCount} commands`;
                    setTimeout(() => downloadBtn.textContent = '💾 Download', 3000);
                } catch (err) {
                    downloadBtn.textContent = '❌ Error';
                    console.error('[QuoteQT] Download error:', err);
                    alert('Export failed: ' + err.message);
                    setTimeout(() => downloadBtn.textContent = '💾 Download', 3000);
                }
            });

            // Send to QuoteQT handler
            sendBtn.addEventListener('click', async () => {
                if (IMPORT_TOKEN === 'YOUR_TOKEN_HERE') {
                    alert('Please configure your IMPORT_TOKEN in the script settings.');
                    return;
                }

                try {
                    sendBtn.textContent = '⏳ Fetching...';
                    const backup = await fetchCommands();

                    sendBtn.textContent = '📡 Sending...';
                    
                    GM_xmlhttpRequest({
                        method: 'POST',
                        url: QUOTEQT_URL,
                        headers: {
                            'Content-Type': 'application/json',
                            'X-Import-Token': IMPORT_TOKEN
                        },
                        data: JSON.stringify(backup),
                        onload: function(response) {
                            if (response.status === 200) {
                                const result = JSON.parse(response.responseText);
                                sendBtn.textContent = `✅ Sent ${result.commands} commands`;
                                console.log('[QuoteQT] Import successful:', result);
                            } else {
                                sendBtn.textContent = '❌ Failed';
                                console.error('[QuoteQT] Import failed:', response.status, response.responseText);
                                alert('Import failed: ' + response.responseText);
                            }
                            setTimeout(() => sendBtn.textContent = '📤 Send to QuoteQT', 3000);
                        },
                        onerror: function(err) {
                            sendBtn.textContent = '❌ Error';
                            console.error('[QuoteQT] Request error:', err);
                            alert('Request failed. Check console for details.');
                            setTimeout(() => sendBtn.textContent = '📤 Send to QuoteQT', 3000);
                        }
                    });
                } catch (err) {
                    sendBtn.textContent = '❌ Error';
                    console.error('[QuoteQT] Error:', err);
                    alert('Export failed: ' + err.message);
                    setTimeout(() => sendBtn.textContent = '📤 Send to QuoteQT', 3000);
                }
            });
        }, 2000);
    });
})();
