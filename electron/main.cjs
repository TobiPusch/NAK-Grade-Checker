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

function startServer() {
    const serverEntry = app.isPackaged
        ? path.join(app.getAppPath(), 'dist', 'server', 'entry.mjs')
        : path.join(__dirname, '..', 'dist', 'server', 'entry.mjs');

    console.log(`Starting server from: ${serverEntry}`);

    // Determine bot path to pass to server
    let botExecutable = 'gradechecker';
    if (process.platform === 'win32') {
        botExecutable += '.exe';
    }

    let botPath;
    if (app.isPackaged) {
        const arch = process.arch === 'arm64' ? 'arm64' : 'amd64';
        const platform = process.platform === 'win32' ? 'windows' : (process.platform === 'darwin' ? 'darwin' : 'linux');
        const binaryName = `gradechecker-${platform}-${arch}${process.platform === 'win32' ? '.exe' : ''}`;
        botPath = path.join(process.resourcesPath, 'bin', binaryName);
    } else {
        const arch = process.arch === 'arm64' ? 'arm64' : 'amd64';
        const platform = process.platform === 'win32' ? 'windows' : (process.platform === 'darwin' ? 'darwin' : 'linux');
        const binaryName = `gradechecker-${platform}-${arch}${process.platform === 'win32' ? '.exe' : ''}`;
        botPath = path.join(__dirname, '..', 'bin', binaryName);
    }

    if (fs.existsSync(serverEntry)) {
        serverProcess = fork(serverEntry, [], {
            cwd: app.getPath('userData'),
            env: {
                ...process.env,
                HOST: 'localhost',
                PORT: '4321',
                BOT_BINARY_PATH: botPath
            }
        });

        serverProcess.on('message', (msg) => console.log(`[SERVER]: ${msg}`));
    } else {
        console.error(`Server entry not found at ${serverEntry}`);
    }
}

app.on('ready', () => {
    // startBot(); // Let Astro manage the bot
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
