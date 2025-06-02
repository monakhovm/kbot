# kbot

A simple Telegram bot written in Go that generates images from text using the Imgbun API and allows users to customize text and background colors for the generated images. Built with Cobra for CLI structure and Telebot (v4) for Telegram API interaction.

**Talk to the bot:** [t.me/monakhovm_bot](https://t.me/monakhovm_bot)

## Features

*   Generates PNG images from user-provided text via Imgbun API.
*   Allows users to customize text color and background color for generated images.
*   Settings mode with interactive color input or direct command usage.
*   Reply keyboard for easy access to settings and saving changes.

## Prerequisites

*   **Go:** Version 1.21 or higher installed (check with `go version`).
*   **Telegram Bot Token:** Obtained from BotFather on Telegram.
*   **Imgbun API Key:** Obtained from [imgbun.com/api](https://imgbun.com/api).

## Setup & Installation

1.  **Clone the repository:**
    ```
    git clone https://github.com/monakhovm/kbot.git
    cd kbot
    ```

2.  **Set Environment Variables:**
    You need to provide your Telegram bot token and Imgbun API key as environment variables.

    *   On Linux/macOS:
        ```
        export TELE_TOKEN="YOUR_TELEGRAM_BOT_TOKEN"
        export IMGBUN_API_KEY="YOUR_IMGBUN_API_KEY"
        ```
    *   On Windows (Command Prompt):
        ```
        set TELE_TOKEN=YOUR_TELEGRAM_BOT_TOKEN
        set IMGBUN_API_KEY=YOUR_IMGBUN_API_KEY
        ```
    *   On Windows (PowerShell):
        ```
        $env:TELE_TOKEN="YOUR_TELEGRAM_BOT_TOKEN"
        $env:IMGBUN_API_KEY="YOUR_IMGBUN_API_KEY"
        ```
    *Alternatively, you can use a `.env` file and a library like `godotenv` if you prefer not to set system-wide variables (requires code modification).*

3.  **Install Dependencies:**
    ```
    go mod tidy
    ```

## Running the Bot

There are several ways to run the bot:

1.  **Using `go run` (for development):**
    ```
    go run main.go start
    # Or, as 'kbot' is the root command name:
    go run main.go
    # Or, using the alias defined in kbot.go:
    go run main.go kbot
    ```

2.  **Building and Running the Executable (for deployment):**
    *   Build the executable:
        ```
        go build -o kbot main.go
        ```
        *(This creates an executable file named `kbot` in the current directory)*
    *   Run the executable:
        ```
        ./kbot start
        # Or simply:
        ./kbot
        ```
3. Using helm:
    *   If you have Helm installed, you can deploy the bot using the provided Helm chart:
    - create a `values.yaml` file with your variables:
        ```yaml
        image:
        repository: ghcr.io/monakhovm
        tag: "<VERSION>"
        arch: <ARCH> # e.g., amd64, arm64
        
        secret:
        telegram:
            token: <YOUR_TELEGRAM_BOT_TOKEN>
        imgbun:
            token: <YOUR_IMGBUN_API_KEY>
        ```
    -   Then, install the bot using Helm:
        ```bash
        helm install kbot https://github.com/monakhovm/kbot/releases/download/<RELEASE_VERSION>/kbot-<VERSION>.tgz
        ```
Once the bot is running, find it on Telegram using the link provided above and start interacting with it.

## Usage

1.  **Start:**
    *   Send `/start` to the bot. You will receive a welcome message and the main menu keyboard.

2.  **Generate Image:**
    *   Simply send any text message to the bot (when not in settings mode).
    *   The bot will use the Imgbun API to generate an image with your text, using your currently saved (or default) text and background colors.
    *   The generated image will be sent back to you.

3.  **Enter Settings Mode:**
    *   Press the `⚙️ Settings` button on the keyboard.
    *   *Alternatively, send the `/settings` command.*
    *   The bot will reply confirming you are in settings mode, show current colors, and display the settings keyboard (`💾 Save Settings`, `◀️ Cancel & Exit`).

4.  **Change Colors (in Settings Mode):**
    *   **Method 1 (Command + Value):**
        *   Send `/tx_color <hex_value>` (e.g., `/tx_color FF0000` or `/tx_color #ff0000`) to set the text color.
        *   Send `/bg_color <hex_value>` (e.g., `/bg_color 0000FF` or `/bg_color #00f`) to set the background color.
        *(The bot expects 3 or 6 digit hex codes, '#' is optional).*
    *   **Method 2 (Command then Value):**
        *   Send just `/tx_color`. The bot will ask you to send the desired text color.
        *   Send the hex value (e.g., `FF0000`) in the next message.
        *   Send just `/bg_color`. The bot will ask you to send the desired background color.
        *   Send the hex value (e.g., `FFFFFF`) in the next message.
    *   After setting a color, the bot confirms the *temporary* change and reminds you to save.

5.  **Save Settings:**
    *   While in settings mode, press the `💾 Save Settings` button.
    *   *Alternatively, send the `/save_settings` command.*
    *   The bot will save the temporarily set colors, confirm the save, exit settings mode, and show the main menu keyboard.

6.  **Cancel Settings:**
    *   While in settings mode, press the `◀️ Cancel & Exit` button.
    *   *Alternatively, send the `/cancel_settings` command.*
    *   The bot will discard any temporary color changes, exit settings mode, and show the main menu keyboard.

## Environment Variables

*   `TELE_TOKEN` (Required): Your Telegram Bot API token.
*   `IMGBUN_API_KEY` (Required): Your API key for `imgbun.com`.

## Version

You can check the application version (if set during build or in `version.go`) using:

```
go run main.go version
```

or if built:
```
./kbot version
```