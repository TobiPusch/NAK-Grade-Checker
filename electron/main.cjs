const { app, BrowserWindow } = require('electron');
const path = require('path');
const { spawn, fork } = require('child_process');
const fs = require('fs');

// Handle creating/removing shortcuts on Windows when installing/uninstalling.
// if (require('electron-squirrel-startup')) {
//   app.quit();
// }

let mainWindow;
let botProcess;
let serverProcess;

function createWindow() {
    mainWindow = new BrowserWindow({
        width: 1200,
        height: 800,
        webPreferences: {
            nodeIntegration: false,
            contextIsolation: true,
        },
        autoHideMenuBar: true,
    });

    // Wait for server to start
    const checkServer = () => {
        const http = require('http');
        http.get('http://localhost:4321', (res) => {
            mainWindow.loadURL('http://localhost:4321');
        }).on('error', () => {
            setTimeout(checkServer, 1000);
        });
    };

    checkServer();
}

function startBot() {
    let botExecutable = 'gradechecker';
    if (process.platform === 'win32') {
        botExecutable += '.exe';
    }

    // In production (packaged), resources are in process.resourcesPath
    // In dev, they are in dist/ or root
    let botPath;
    if (app.isPackaged) {
        // e.g. resources/gradechecker-linux-amd64
        // We need to determine which binary to pick based on platform if we packaged all
        // But typically electron-builder packages only the relevant one if we configure it right.
        // Let's assume we put them in a 'bin' folder in resources
        const arch = process.arch === 'arm64' ? 'arm64' : 'amd64'; // Go arch names
        const platform = process.platform === 'win32' ? 'windows' : (process.platform === 'darwin' ? 'darwin' : 'linux');

        const binaryName = `gradechecker-${platform}-${arch}${process.platform === 'win32' ? '.exe' : ''}`;
        botPath = path.join(process.resourcesPath, 'bin', binaryName);
    } else {
        // Dev mode: use the one we just built
        const arch = process.arch === 'arm64' ? 'arm64' : 'amd64';
        const platform = process.platform === 'win32' ? 'windows' : (process.platform === 'darwin' ? 'darwin' : 'linux');
        const binaryName = `gradechecker-${platform}-${arch}${process.platform === 'win32' ? '.exe' : ''}`;
        botPath = path.join(__dirname, '..', 'dist', binaryName);
    }

    console.log(`Starting bot from: ${botPath}`);

    if (fs.existsSync(botPath)) {
        botProcess = spawn(botPath, [], {
            cwd: app.getPath('userData'),
            env: { ...process.env, ELECTRON_RUN: 'true' }
        });

        botProcess.stdout.on('data', (data) => console.log(`[BOT]: ${data}`));
        botProcess.stderr.on('data', (data) => console.error(`[BOT ERR]: ${data}`));
    } else {
        console.error(`Bot binary not found at ${botPath}`);
    }
}

function startServer() {
    const serverEntry = app.isPackaged
        ? path.join(app.getAppPath(), 'dist', 'server', 'entry.mjs')
        : path.join(__dirname, '..', 'dist', 'server', 'entry.mjs');

    console.log(`Starting server from: ${serverEntry}`);

    if (fs.existsSync(serverEntry)) {
        serverProcess = fork(serverEntry, [], {
            cwd: app.getPath('userData'),
            env: { ...process.env, HOST: 'localhost', PORT: '4321' }
        });

        serverProcess.on('message', (msg) => console.log(`[SERVER]: ${msg}`));
    } else {
        console.error(`Server entry not found at ${serverEntry}`);
    }
}

app.on('ready', () => {
    startBot();
    startServer();
    createWindow();
});

app.on('window-all-closed', () => {
    if (process.platform !== 'darwin') {
        app.quit();
    }
});

app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) {
        createWindow();
    }
});

app.on('will-quit', () => {
    if (botProcess) botProcess.kill();
    if (serverProcess) serverProcess.kill();
});
