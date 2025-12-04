import { spawn, type ChildProcess } from 'node:child_process';
import path from 'node:path';

class BotManager {
    private process: ChildProcess | null = null;
    private logs: string[] = [];
    private readonly MAX_LOGS = 1000;

    constructor() {
        // Initial start if needed, or wait for manual start
    }

    public start() {
        if (this.process) {
            this.log('Bot is already running.');
            return;
        }

        const botPath = path.resolve('gradechecker');
        this.log(`Starting bot from: ${botPath}`);

        try {
            this.process = spawn(botPath, [], {
                cwd: process.cwd(),
                env: process.env // Inherit env (including .env loaded by Astro/Node)
            });

            this.process.stdout?.on('data', (data) => {
                this.log(data.toString());
            });

            this.process.stderr?.on('data', (data) => {
                this.log(data.toString());
            });

            this.process.on('close', (code) => {
                this.log(`Bot process exited with code ${code}`);
                this.process = null;
            });

            this.process.on('error', (err) => {
                this.log(`Failed to start bot: ${err.message}`);
                this.process = null;
            });

        } catch (error: any) {
            this.log(`Exception starting bot: ${error.message}`);
        }
    }

    public stop() {
        if (this.process) {
            this.log('Stopping bot...');
            this.process.kill();
            this.process = null;
        }
    }

    public restart() {
        this.stop();
        setTimeout(() => this.start(), 1000); // Give it a moment to cleanup
    }

    public getLogs() {
        return this.logs;
    }

    public isRunning() {
        return this.process !== null;
    }

    private log(message: string) {
        const timestamp = new Date().toLocaleTimeString();
        const lines = message.split('\n').filter(line => line.trim() !== '');

        lines.forEach(line => {
            this.logs.push(`[${timestamp}] ${line}`);
        });

        if (this.logs.length > this.MAX_LOGS) {
            this.logs = this.logs.slice(-this.MAX_LOGS);
        }
    }
}

// Singleton instance
const botManager = new BotManager();
export default botManager;
