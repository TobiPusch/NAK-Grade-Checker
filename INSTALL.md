# GradeChecker Installation Guide

This guide will help you set up the GradeChecker on your local machine (Linux/Raspberry Pi) and access it from your Android phone.

## Prerequisites

- **Go**: Version 1.21 or higher.
- **Node.js**: Version 18 or higher.
- **Git**: To clone the repository.
- **Discord Account**: Optional, for notifications.

## Installation

1.  **Clone the Repository**
    ```bash
    git clone https://github.com/yourusername/gradechecker.git
    cd gradechecker
    ```

2.  **Install Dependencies**
    ```bash
    npm install
    ```

3.  **Build the Backend**
    ```bash
    go build -o gradechecker cmd/bot/main.go
    ```

4.  **Create Environment File**
    Copy the example or create a new `.env` file:
    ```bash
    touch .env
    ```
    *Note: You can configure everything via the Web UI later.*

## Running the Application

1.  **Start the Development Server**
    ```bash
    npm run dev
    ```

2.  **Access the Web UI**
    - **Local**: Open `http://localhost:4321` in your browser.
    - **Mobile**: Look for the **Network** URL in the terminal (e.g., `http://192.168.1.100:4321`). Open this URL on your phone connected to the same WiFi.

## Configuration

1.  Open the Web UI.
2.  Click on **Settings** (gear icon).
3.  **Credentials**: Enter your CIS username (matriculation number) and password.
4.  **General**: Set the check interval (default: 60 minutes).
5.  **Notifications** (Optional):
    - Enable "Discord Notifications".
    - Paste your Discord Webhook URL.

## Discord Webhook Setup

To receive notifications on your phone:
1.  Create a text channel in your Discord server.
2.  Go to **Edit Channel** -> **Integrations** -> **Webhooks**.
3.  Create a new Webhook and copy the **Webhook URL**.
4.  Paste this URL in the GradeChecker Settings.

## Troubleshooting

-   **"Login failed"**: Double-check your CIS credentials.
-   **No Notifications**: Ensure "Discord Notifications" is enabled in Settings and the Webhook URL is correct.
-   **Cannot access from phone**: Ensure your firewall allows traffic on port 4321 and both devices are on the same network.
