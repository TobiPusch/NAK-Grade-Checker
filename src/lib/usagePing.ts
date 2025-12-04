import db from './db';
import versionConfig from '../../version.json';
import dotenv from 'dotenv';
import path from 'path';
import fs from 'fs';

const PING_URL_BASE64 = "aHR0cHM6Ly9yYWluZGFuY2VyMTE4LmRlL2FwaS91c2FnZS9waW5n"; // https://raindancer118.de/api/usage/ping

export async function checkAndSendPing() {
    // Load env dynamically to pick up changes
    const envPath = path.resolve('.env');
    let envConfig: Record<string, string> = {};
    try {
        envConfig = dotenv.parse(fs.readFileSync(envPath));
    } catch (e) {
        // ignore if .env missing
    }

    // Check if enabled (default true)
    if (envConfig.USAGE_PING_ENABLED === 'false') {
        return;
    }

    const today = new Date().toISOString().split('T')[0];

    // Check last ping date
    try {
        const row = db.prepare('SELECT value FROM app_state WHERE key = ?').get('last_ping_date') as { value: string } | undefined;

        if (row && row.value === today) {
            // Already pinged today
            return;
        }

        // Send ping
        const url = Buffer.from(PING_URL_BASE64, 'base64').toString('utf-8');

        // Check integrity status
        let isFork = true;
        try {
            const integrityRow = db.prepare('SELECT value FROM system_status WHERE key = ?').get('integrity_status') as { value: string } | undefined;
            if (integrityRow && integrityRow.value === 'OFFICIAL') {
                isFork = false;
            }
        } catch (e) {
            console.error('Failed to read integrity status:', e);
        }

        const body = {
            app_name: "GradeChecker",
            app_version: versionConfig.version,
            is_fork: isFork
        };

        const response = await fetch(url, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(body)
        });

        if (response.ok) {
            // Update last ping date
            const stmt = db.prepare('INSERT OR REPLACE INTO app_state (key, value) VALUES (?, ?)');
            stmt.run('last_ping_date', today);
            console.log('Daily usage ping sent successfully.');
        } else {
            console.error('Failed to send usage ping:', response.statusText);
        }

    } catch (error) {
        console.error('Error in usage ping service:', error);
    }
}
