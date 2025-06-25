package cmd

import (
	"context" // Додаємо context
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	tele "gopkg.in/telebot.v4" // Using v4

	// OpenTelemetry imports
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp" // Імпорт для інструментації HTTP клієнта
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes" // Додаємо імпорт codes
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// --- Global Variables ---
var (
	TeleToken    = os.Getenv("TELE_TOKEN")
	ImgbunAPIKey = os.Getenv("IMGBUN_API_KEY")

	// OpenTelemetry globals
	tracer trace.Tracer
	meter  metric.Meter

	// Metrics instruments
	startCmdCounter           metric.Int64Counter
	settingsEnterCounter      metric.Int64Counter
	settingsSaveCounter       metric.Int64Counter
	settingsCancelCounter     metric.Int64Counter
	imageGenRequestCounter    metric.Int64Counter
	imageGenSuccessCounter    metric.Int64Counter
	imageGenFailureCounter    metric.Int64Counter
	imageGenerationDuration   metric.Float64Histogram
	unrecognizedTextCounter   metric.Int64Counter
	waitingForInputCounter    metric.Int64Counter
	invalidColorFormatCounter metric.Int64Counter
)

// --- Structs ---

// UserSettings stores color preferences for a user
type UserSettings struct {
	TextColor string // Expects hex format without '#'
	BgColor   string // Expects hex format without '#'
}

// ImgbunResponse struct for parsing the response from the Imgbun API
type ImgbunResponse struct {
	Status     string `json:"status"` // Should be "OK" on success according to API v2 docs
	DirectLink string `json:"direct_link"`
	Message    string `json:"message"` // For potential error messages
}

// --- User State and Keyboards ---
var (
	// State storage (thread-safe)
	userSettingsStore     sync.Map // Key: int64 (UserID), Value: UserSettings
	tempUserSettingsStore sync.Map // Key: int64 (UserID), Value: UserSettings (for editing)
	userInSettingsMode    sync.Map // Key: int64 (UserID), Value: bool
	userWaitingFor        sync.Map // Key: int64 (UserID), Value: string ("tx_color", "bg_color", or "")

	// Keyboards and Buttons
	mainMenuMarkup     *tele.ReplyMarkup
	settingsMenuMarkup *tele.ReplyMarkup
	btnSettings        tele.Btn // Global var for the settings button
	btnSaveChanges     tele.Btn // Global var for the save button
	btnCancelSettings  tele.Btn // Global var for the cancel button
)

// --- Keyboard Initialization ---
func setupKeyboards() {
	// Main Menu Keyboard
	mainMenuMarkup = &tele.ReplyMarkup{ResizeKeyboard: true}
	btnSettings = mainMenuMarkup.Text("⚙️ Settings") // Assign button to global variable
	mainMenuMarkup.Reply(
		mainMenuMarkup.Row(btnSettings),
	)

	// Settings Menu Keyboard
	settingsMenuMarkup = &tele.ReplyMarkup{ResizeKeyboard: true}
	btnSaveChanges = settingsMenuMarkup.Text("💾 Save Settings")     // Assign button to global variable
	btnCancelSettings = settingsMenuMarkup.Text("◀️ Cancel & Exit") // Assign button to global variable
	settingsMenuMarkup.Reply(
		settingsMenuMarkup.Row(btnSaveChanges),
		settingsMenuMarkup.Row(btnCancelSettings),
	)
	log.Println("Keyboards initialized.")
}

// initMetrics initializes OpenTelemetry metric instruments
func initMetrics() {
	// Отримання Meter з глобального провайдера
	meter = otel.GetMeterProvider().Meter(serviceName, metric.WithInstrumentationVersion(appVersion))

	var err error

	// Лічильники (Counters)
	startCmdCounter, err = meter.Int64Counter("kbot.commands.start.total",
		metric.WithDescription("Total number of /start commands received."),
		metric.WithUnit("1"),
	)
	if err != nil {
		log.Fatalf("Failed to create startCmdCounter: %v", err)
	}

	settingsEnterCounter, err = meter.Int64Counter("kbot.settings.enter.total",
		metric.WithDescription("Total number of times users entered settings mode."),
		metric.WithUnit("1"),
	)
	if err != nil {
		log.Fatalf("Failed to create settingsEnterCounter: %v", err)
	}

	settingsSaveCounter, err = meter.Int64Counter("kbot.settings.save.total",
		metric.WithDescription("Total number of times users saved settings."),
		metric.WithUnit("1"),
	)
	if err != nil {
		log.Fatalf("Failed to create settingsSaveCounter: %v", err)
	}

	settingsCancelCounter, err = meter.Int64Counter("kbot.settings.cancel.total",
		metric.WithDescription("Total number of times users cancelled settings."),
		metric.WithUnit("1"),
	)
	if err != nil {
		log.Fatalf("Failed to create settingsCancelCounter: %v", err)
	}

	imageGenRequestCounter, err = meter.Int64Counter("kbot.image.requests.total",
		metric.WithDescription("Total number of image generation requests."),
		metric.WithUnit("1"),
	)
	if err != nil {
		log.Fatalf("Failed to create imageGenRequestCounter: %v", err)
	}

	imageGenSuccessCounter, err = meter.Int64Counter("kbot.image.success.total",
		metric.WithDescription("Total number of successful image generations."),
		metric.WithUnit("1"),
	)
	if err != nil {
		log.Fatalf("Failed to create imageGenSuccessCounter: %v", err)
	}

	imageGenFailureCounter, err = meter.Int64Counter("kbot.image.failure.total",
		metric.WithDescription("Total number of failed image generations."),
		metric.WithUnit("1"),
	)
	if err != nil {
		log.Fatalf("Failed to create imageGenFailureCounter: %v", err)
	}

	unrecognizedTextCounter, err = meter.Int64Counter("kbot.text.unrecognized.total",
		metric.WithDescription("Total number of unrecognized text inputs."),
		metric.WithUnit("1"),
	)
	if err != nil {
		log.Fatalf("Failed to create unrecognizedTextCounter: %v", err)
	}

	waitingForInputCounter, err = meter.Int64Counter("kbot.settings.waiting_for_input.total",
		metric.WithDescription("Total times user entered waiting for color input state."),
		metric.WithUnit("1"),
	)
	if err != nil {
		log.Fatalf("Failed to create waitingForInputCounter: %v", err)
	}

	invalidColorFormatCounter, err = meter.Int64Counter("kbot.settings.invalid_color_format.total",
		metric.WithDescription("Total times user provided invalid hex color format."),
		metric.WithUnit("1"),
	)
	if err != nil {
		log.Fatalf("Failed to create invalidColorFormatCounter: %v", err)
	}

	// Гістограма (Histogram)
	imageGenerationDuration, err = meter.Float64Histogram("kbot.image.generation.duration_seconds",
		metric.WithDescription("Duration of image generation requests to Imgbun API."),
		metric.WithUnit("s"),
	)
	if err != nil {
		log.Fatalf("Failed to create imageGenerationDuration: %v", err)
	}

	log.Println("OpenTelemetry metrics initialized.")
}

// kbotCmd represents the kbot command (entry point for the bot)
var kbotCmd = &cobra.Command{
	Use:     "kbot",
	Aliases: []string{"start"}, // Can be run via 'go run main.go kbot' or 'go run main.go start'
	Short:   "Starts the kbot Telegram bot",
	Long: `Starts the kbot Telegram bot, which generates images from text
using the Imgbun API and allows color customization.

Required environment variables:
  TELE_TOKEN: Your Telegram bot token.
  IMGBUN_API_KEY: Your API key for the Imgbun service.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Validate environment variables
		if TeleToken == "" {
			log.Fatal("Error: TELE_TOKEN environment variable not set!")
		}
		if ImgbunAPIKey == "" {
			log.Fatal("Error: IMGBUN_API_KEY environment variable not set!")
		}

		// Initialize OpenTelemetry
		// Це повинно бути викликано лише один раз на початку програми.
		shutdownTelemetry, err := InitTelemetry()
		if err != nil {
			log.Fatalf("Failed to initialize OpenTelemetry: %v", err)
		}
		defer shutdownTelemetry() // Забезпечуємо коректне завершення роботи OTel

		// Отримання глобального Tracer після ініціалізації OTel
		tracer = otel.Tracer(serviceName)
		initMetrics() // Ініціалізуємо метрики після ініціалізації MeterProvider

		// Initialize keyboards before creating the bot
		setupKeyboards()

		log.Printf("kbot %s starting...", appVersion) // appVersion should be defined in version.go

		// Bot settings
		pref := tele.Settings{
			Token:  TeleToken,
			Poller: &tele.LongPoller{Timeout: 10 * time.Second}, // Using Long Polling
		}

		// Create new bot instance
		kbot, err := tele.NewBot(pref)
		if err != nil {
			log.Fatalf("Error creating bot: %v", err)
			return
		}

		log.Printf("Authorized as %s (ID: %d)", kbot.Me.Username, kbot.Me.ID)

		// --- Register Handlers ---
		registerHandlers(kbot)

		// --- Start Bot ---
		log.Println("Starting bot's main loop...")
		kbot.Start()
	},
}

// registerHandlers sets up all the command, button, and text handlers
func registerHandlers(b *tele.Bot) {
	// Кожен обробник тепер створює свій власний кореневий спан.
	// Обгортка oteltelebotHandlerWrapper видалена, оскільки tele.Context не підтримує пряме вбудовування контексту.
	b.Handle("/start", handleStart)
	b.Handle(&btnSettings, handleSettingsEnter)
	b.Handle("/settings", handleSettingsEnter)
	b.Handle("/tx_color", handleSetColor)
	b.Handle("/bg_color", handleSetColor)
	b.Handle(&btnSaveChanges, handleSettingsSave)
	b.Handle("/save_settings", handleSettingsSave)
	b.Handle(&btnCancelSettings, handleSettingsCancel)
	b.Handle("/cancel_settings", handleSettingsCancel)
	b.Handle(tele.OnText, handleTextInput)

	log.Println("Handlers registered successfully.")
}

// --- Handler Functions ---

// handleStart handles the /start command
func handleStart(c tele.Context) error {
	// Створюємо кореневий спан для обробки цього Telegram-повідомлення
	ctx, span := tracer.Start(context.Background(), "handleStart",
		trace.WithAttributes(
			attribute.Int64("telegram.user.id", c.Sender().ID),
			attribute.String("telegram.user.username", c.Sender().Username),
			attribute.Int64("telegram.chat.id", c.Chat().ID),
			attribute.String("telegram.message.text", c.Message().Text),
		))
	defer span.End()

	startCmdCounter.Add(ctx, 1) // Метрика: лічильник команди /start
	senderID := c.Sender().ID
	log.Printf("Received /start from %d (%s)", senderID, c.Sender().Username)
	// Reset user state in case they were in settings mode
	exitSettingsMode(senderID) // Safely exits settings mode if user was in it
	// Send welcome message with the main keyboard
	msg := fmt.Sprintf("Hello, %s! I'm Kbot %s.\nSend me text to create an image, or press 'Settings' to customize colors.", c.Sender().FirstName, appVersion)
	return c.Send(msg, mainMenuMarkup)
}

// handleSettingsEnter handles entering the settings mode (via command or button)
func handleSettingsEnter(c tele.Context) error {
	// Створюємо кореневий спан для обробки цього Telegram-повідомлення
	ctx, span := tracer.Start(context.Background(), "handleSettingsEnter",
		trace.WithAttributes(
			attribute.Int64("telegram.user.id", c.Sender().ID),
			attribute.String("telegram.user.username", c.Sender().Username),
			attribute.Int64("telegram.chat.id", c.Chat().ID),
			attribute.String("telegram.message.text", c.Message().Text),
		))
	defer span.End()

	settingsEnterCounter.Add(ctx, 1) // Метрика: лічильник входу в налаштування
	senderID := c.Sender().ID
	log.Printf("User %d (%s) entering settings mode", senderID, c.Sender().Username)

	// Load current settings or store defaults (hex without '#')
	currentSettingsRaw, _ := userSettingsStore.LoadOrStore(senderID, UserSettings{TextColor: "000000", BgColor: "FFFFFF"})
	currentSettings := currentSettingsRaw.(UserSettings)
	tempUserSettingsStore.Store(senderID, currentSettings) // Copy settings for editing
	userInSettingsMode.Store(senderID, true)               // Set user state to 'in settings mode'
	userWaitingFor.Store(senderID, "")                     // Reset waiting state

	msg := fmt.Sprintf(`You are now in settings mode.
Current colors: Text=#%s, Background=#%s

Use commands or send the value after them:
/tx_color [<value>] - text color (hex)
/bg_color [<value>] - background color (hex)`,
		currentSettings.TextColor, currentSettings.BgColor) // Show current colors

	// Send message with the settings keyboard
	return c.Send(msg, settingsMenuMarkup)
}

// handleSetColor handles /tx_color and /bg_color commands
func handleSetColor(c tele.Context) error {
	// Створюємо кореневий спан для обробки цього Telegram-повідомлення
	ctx, span := tracer.Start(context.Background(), "handleSetColor",
		trace.WithAttributes(
			attribute.Int64("telegram.user.id", c.Sender().ID),
			attribute.String("telegram.user.username", c.Sender().Username),
			attribute.Int64("telegram.chat.id", c.Chat().ID),
			attribute.String("telegram.message.text", c.Message().Text),
		))
	defer span.End()

	senderID := c.Sender().ID

	// Check if user is in settings mode
	if !isUserInSettingsMode(senderID) {
		log.Printf("User %d (%s) tried to set color outside settings mode.", senderID, c.Sender().Username)
		span.AddEvent("Attempted to set color outside settings mode")
		span.SetStatus(codes.Error, "Not in settings mode") // Виправлено: codes.Error
		return c.Send("This command is only available in settings mode (use '⚙️ Settings' button).", mainMenuMarkup)
	}

	command := c.Message().Text
	parts := strings.Fields(command)
	commandName := parts[0] // e.g., /tx_color or /bg_color

	var settingType string
	var promptMsg string
	// Determine which color is being set and prepare the prompt message
	if strings.HasPrefix(commandName, "/tx_color") {
		settingType = "tx_color"
		promptMsg = "Please send the desired text color (hex, e.g., `FF0000`):"
		span.SetAttributes(attribute.String("settings.color_type", "text_color"))
	} else if strings.HasPrefix(commandName, "/bg_color") {
		settingType = "bg_color"
		promptMsg = "Please send the desired background color (hex, e.g., `FFFFFF`):"
		span.SetAttributes(attribute.String("settings.color_type", "background_color"))
	} else {
		log.Printf("Unknown command '%s' received from user %d", commandName, senderID)
		span.AddEvent("Unknown command for color setting")
		span.SetStatus(codes.Error, "Unknown command") // Виправлено: codes.Error
		return nil                                     // Ignore unknown command
	}

	// Check if color value was provided with the command
	if len(parts) >= 2 {
		colorValue := strings.TrimPrefix(parts[1], "#") // Remove '#' if present
		span.SetAttributes(attribute.String("settings.color_value_provided", colorValue))
		log.Printf("User %d (%s) sent command %s with value %s", senderID, c.Sender().Username, commandName, colorValue)

		// Validate the hex color format
		if !isValidHexColor(colorValue) {
			invalidColorFormatCounter.Add(ctx, 1) // Метрика: невірний формат кольору
			span.AddEvent("Invalid hex color format", trace.WithAttributes(attribute.String("color.value", colorValue)))
			span.SetStatus(codes.Error, "Invalid hex color format") // Виправлено: codes.Error
			return c.Send(fmt.Sprintf("'%s' doesn't look like a valid HEX color (3 or 6 chars, 0-9, A-F). Please try again.", colorValue), settingsMenuMarkup)
		}

		// Load temporary settings
		tempSettingsRaw, ok := tempUserSettingsStore.Load(senderID)
		if !ok { // Should exist if we are in settings mode
			log.Printf("Critical Error: Temporary settings not found for user %d in handleSetColor!", senderID)
			span.RecordError(fmt.Errorf("temporary settings missing"))
			span.SetStatus(codes.Error, "Internal state error") // Виправлено: codes.Error
			exitSettingsMode(senderID)                          // Exit mode on state error
			return c.Send("An internal state error occurred. You have been exited from settings mode.", mainMenuMarkup)
		}
		tempSettings := tempSettingsRaw.(UserSettings)

		// Update the corresponding color field
		if settingType == "tx_color" {
			tempSettings.TextColor = colorValue
		} else {
			tempSettings.BgColor = colorValue
		}

		// Save updated temporary settings
		tempUserSettingsStore.Store(senderID, tempSettings)
		userWaitingFor.Store(senderID, "") // Reset waiting state, as value was provided
		span.AddEvent("Color value updated in temporary settings",
			trace.WithAttributes(attribute.String("settings.new_value", colorValue)))

		return c.Send(fmt.Sprintf("Temporarily set %s: #%s. Save changes with '💾 Save Settings'.", settingType, colorValue), settingsMenuMarkup)

	} else {
		// If color value was NOT provided - enter waiting state
		log.Printf("User %d (%s) sent command %s without value. Waiting for input.", senderID, c.Sender().Username, commandName)
		waitingForInputCounter.Add(ctx, 1)          // Метрика: очікування вводу
		userWaitingFor.Store(senderID, settingType) // Store which color we are waiting for
		span.AddEvent("Waiting for color input from user")
		return c.Send(promptMsg, settingsMenuMarkup) // Send prompt message
	}
}

// handleSettingsSave handles saving the settings (via command or button)
func handleSettingsSave(c tele.Context) error {
	// Створюємо кореневий спан для обробки цього Telegram-повідомлення
	ctx, span := tracer.Start(context.Background(), "handleSettingsSave",
		trace.WithAttributes(
			attribute.Int64("telegram.user.id", c.Sender().ID),
			attribute.String("telegram.user.username", c.Sender().Username),
			attribute.Int64("telegram.chat.id", c.Chat().ID),
			attribute.String("telegram.message.text", c.Message().Text),
		))
	defer span.End()

	settingsSaveCounter.Add(ctx, 1) // Метрика: лічильник збереження налаштувань
	senderID := c.Sender().ID

	// Check if user is in settings mode
	if !isUserInSettingsMode(senderID) {
		log.Printf("User %d (%s) tried to save settings while not in settings mode.", senderID, c.Sender().Username)
		span.AddEvent("Attempted to save settings outside settings mode")
		span.SetStatus(codes.Error, "Not in settings mode") // Виправлено: codes.Error
		return c.Send("You are not in settings mode.", mainMenuMarkup)
	}

	// Load temporary settings
	tempSettingsRaw, ok := tempUserSettingsStore.Load(senderID)
	if !ok {
		log.Printf("Error: Temporary settings not found for user %d during save.", senderID)
		span.RecordError(fmt.Errorf("temporary settings missing during save"))
		span.SetStatus(codes.Error, "Internal state error on save") // Виправлено: codes.Error
		exitSettingsMode(senderID)                                  // Exit mode anyway
		return c.Send("An internal error occurred while saving. You have been exited from settings mode.", mainMenuMarkup)
	}

	// Save temporary settings as permanent
	savedSettings := tempSettingsRaw.(UserSettings)
	userSettingsStore.Store(senderID, savedSettings)

	// Exit settings mode
	exitSettingsMode(senderID)

	span.SetAttributes(
		attribute.String("settings.text_color.saved", savedSettings.TextColor),
		attribute.String("settings.background_color.saved", savedSettings.BgColor),
	)
	log.Printf("User %d (%s) saved settings: Text=#%s, BG=#%s", senderID, c.Sender().Username, savedSettings.TextColor, savedSettings.BgColor)
	// Send confirmation with the main keyboard
	return c.Send("Settings saved successfully!", mainMenuMarkup)
}

// handleSettingsCancel handles cancelling the settings mode (via command or button)
func handleSettingsCancel(c tele.Context) error {
	// Створюємо кореневий спан для обробки цього Telegram-повідомлення
	ctx, span := tracer.Start(context.Background(), "handleSettingsCancel",
		trace.WithAttributes(
			attribute.Int64("telegram.user.id", c.Sender().ID),
			attribute.String("telegram.user.username", c.Sender().Username),
			attribute.Int64("telegram.chat.id", c.Chat().ID),
			attribute.String("telegram.message.text", c.Message().Text),
		))
	defer span.End()

	settingsCancelCounter.Add(ctx, 1) // Метрика: лічильник скасування налаштувань
	senderID := c.Sender().ID

	if !isUserInSettingsMode(senderID) {
		log.Printf("User %d (%s) tried to cancel settings while not in settings mode.", senderID, c.Sender().Username)
		span.AddEvent("Attempted to cancel settings outside settings mode")
		span.SetStatus(codes.Error, "Not in settings mode") // Виправлено: codes.Error
		return c.Send("You are not currently in settings mode.", mainMenuMarkup)
	}

	log.Printf("User %d (%s) cancelled settings mode.", senderID, c.Sender().Username)
	exitSettingsMode(senderID) // Exit mode and discard temporary changes
	span.AddEvent("Settings mode cancelled")
	return c.Send("Settings mode cancelled. Temporary changes have been discarded.", mainMenuMarkup)
}

// handleTextInput is the main handler for text messages
func handleTextInput(c tele.Context) error {
	// Створюємо кореневий спан для обробки цього Telegram-повідомлення
	ctx, span := tracer.Start(context.Background(), "handleTextInput",
		trace.WithAttributes(
			attribute.Int64("telegram.user.id", c.Sender().ID),
			attribute.String("telegram.user.username", c.Sender().Username),
			attribute.Int64("telegram.chat.id", c.Chat().ID),
			attribute.String("telegram.message.text", c.Message().Text),
		))
	defer span.End()

	senderID := c.Sender().ID
	text := c.Text()
	username := c.Sender().Username

	span.SetAttributes(attribute.String("telegram.input_text", text))

	// --- 1. Check if waiting for color input ---
	waitingForRaw, userIsWaiting := userWaitingFor.Load(senderID)
	if userIsWaiting {
		if waitingFor, isString := waitingForRaw.(string); isString && waitingFor != "" {
			span.AddEvent("User is in waiting state for color input")
			log.Printf("User %d (%s) sent value '%s', expecting input for %s", senderID, username, text, waitingFor)
			colorValue := strings.TrimPrefix(text, "#") // Get color value, remove '#'
			span.SetAttributes(attribute.String("settings.color_input_value", colorValue))

			// Validate hex color
			if !isValidHexColor(colorValue) {
				invalidColorFormatCounter.Add(ctx, 1) // Метрика: невірний формат кольору
				span.AddEvent("Invalid hex color format in waiting state", trace.WithAttributes(attribute.String("color.value", colorValue)))
				span.SetStatus(codes.Error, "Invalid hex color format") // Виправлено: codes.Error
				return c.Send(fmt.Sprintf("'%s' doesn't look like a valid HEX color (3 or 6 chars, 0-9, A-F). Please send a correct color value for %s:", text, waitingFor), settingsMenuMarkup)
			}

			// Load temporary settings
			tempSettingsRaw, ok := tempUserSettingsStore.Load(senderID)
			if !ok {
				log.Printf("Critical Error: User %d was waiting for input, but temporary settings are missing!", senderID)
				span.RecordError(fmt.Errorf("temporary settings missing in waiting state"))
				span.SetStatus(codes.Error, "Internal state error") // Виправлено: codes.Error
				exitSettingsMode(senderID)                          // Exit mode on state error
				return c.Send("A state error occurred. You have been exited from settings mode.", mainMenuMarkup)
			}
			tempSettings := tempSettingsRaw.(UserSettings)

			// Update the correct color field
			settingType := waitingFor // "tx_color" or "bg_color"
			if settingType == "tx_color" {
				tempSettings.TextColor = colorValue
			} else if settingType == "bg_color" {
				tempSettings.BgColor = colorValue
			}

			// Save updated temporary settings
			tempUserSettingsStore.Store(senderID, tempSettings)
			userWaitingFor.Store(senderID, "") // Reset waiting state
			span.AddEvent("Color value updated in temporary settings from input",
				trace.WithAttributes(attribute.String("settings.new_value", colorValue)))

			log.Printf("Temporarily set %s: #%s for user %d (%s)", settingType, colorValue, senderID, username)
			return c.Send(fmt.Sprintf("Temporarily set %s: #%s. Save changes with '💾 Save Settings'.", settingType, colorValue), settingsMenuMarkup)
		}
	}

	// --- 2. Check if in settings mode (but not waiting for input) ---
	if isUserInSettingsMode(senderID) {
		unrecognizedTextCounter.Add(ctx, 1) // Метрика: нерозпізнаний текст
		span.AddEvent("Unrecognized text while in settings mode")
		log.Printf("User %d (%s) sent unrecognized text '%s' while in settings mode", senderID, username, text)
		// Ignore unrecognized text or prompt user
		return c.Send("Please use the commands /tx_color, /bg_color or the 'Save Settings' / 'Cancel & Exit' buttons.", settingsMenuMarkup)
	}

	// --- 3. If not in settings mode and not waiting for input - generate image ---
	log.Printf("User %d (%s) sent text '%s' for image generation", senderID, username, text)
	return generateAndSendImage(ctx, c) // Викликаємо generateAndSendImage, передаючи контекст
}

// generateAndSendImage generates image via Imgbun and sends it to the user
func generateAndSendImage(ctx context.Context, c tele.Context) error { // Приймаємо контекст
	// Ця функція вже викликається з контекстом, що містить батьківський спан.
	// Тут створюємо дочірній спан для операції генерації зображення.
	ctx, span := tracer.Start(ctx, "generateAndSendImage")
	defer span.End()

	imageGenRequestCounter.Add(ctx, 1) // Метрика: запит на генерацію зображення
	startTime := time.Now()            // Початок вимірювання тривалості

	senderID := c.Sender().ID
	text := c.Text()
	username := c.Sender().Username

	span.SetAttributes(
		attribute.String("image.text_input", text),
	)

	// Load user settings (or defaults)
	settingsRaw, _ := userSettingsStore.LoadOrStore(senderID, UserSettings{TextColor: "000000", BgColor: "FFFFFF"})
	currentSettings := settingsRaw.(UserSettings)
	// Ensure colors don't have '#' (they shouldn't if saved correctly)
	textColorHex := strings.TrimPrefix(currentSettings.TextColor, "#")
	bgColorHex := strings.TrimPrefix(currentSettings.BgColor, "#")

	span.SetAttributes(
		attribute.String("image.text_color", textColorHex),
		attribute.String("image.background_color", bgColorHex),
	)

	// Construct the Imgbun API URL
	// Reference: https://api.imgbun.com/png?key={API Key}&text=some_text&color=tx_color&background=bg_color&size=16&format=json
	apiURL := fmt.Sprintf("https://api.imgbun.com/png?key=%s&text=%s&color=%s&background=%s&size=%s&format=json",
		url.QueryEscape(ImgbunAPIKey), // API Key
		url.QueryEscape(text),         // Text from user
		url.QueryEscape(textColorHex), // Text color from settings
		url.QueryEscape(bgColorHex),   // Background color from settings
		"16",                          // Font size (fixed)
	)

	log.Printf("Forming Imgbun API request for user %d (%s)...", senderID, username)
	span.AddEvent("Imgbun API request formed")

	// Create HTTP request with OpenTelemetry transport for automatic tracing
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil) // Використовуємо контекст з span
	if err != nil {
		log.Printf("Error creating Imgbun HTTP request for user %d: %v", senderID, err)
		imageGenFailureCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("error.type", "request_creation"))) // Метрика: помилка
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to create HTTP request") // Виправлено: codes.Error
		return c.Send("Failed to generate image: could not create request.", mainMenuMarkup)
	}
	req.Header.Set("User-Agent", fmt.Sprintf("kbot/%s", appVersion)) // Set User-Agent

	// Wrap the default HTTP client with otelhttp transport
	client := &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport), Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error executing Imgbun HTTP request for user %d: %v", senderID, err)
		imageGenFailureCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("error.type", "network_error"))) // Метрика: помилка
		span.RecordError(err)
		span.SetStatus(codes.Error, "Network error or service unavailable") // Виправлено: codes.Error
		return c.Send("Failed to generate image: network error or service unavailable.", mainMenuMarkup)
	}
	defer resp.Body.Close() // Ensure body is closed

	// Check HTTP status code
	if resp.StatusCode != http.StatusOK {
		log.Printf("Imgbun API returned non-OK status (%d) for user %d", resp.StatusCode, senderID)
		imageGenFailureCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("error.type", "api_http_error"), attribute.Int("http.status_code", resp.StatusCode))) // Метрика: помилка
		span.SetStatus(codes.Error, fmt.Sprintf("Imgbun API returned non-OK status: %d", resp.StatusCode))                                                              // Виправлено: codes.Error
		return c.Send(fmt.Sprintf("Failed to generate image: service returned error %d.", resp.StatusCode), mainMenuMarkup)
	}

	// Decode JSON response
	var imgbunResp ImgbunResponse
	if err := json.NewDecoder(resp.Body).Decode(&imgbunResp); err != nil {
		log.Printf("Error decoding Imgbun JSON response for user %d: %v", senderID, err)
		imageGenFailureCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("error.type", "json_decode_error"))) // Метрика: помилка
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to decode JSON response") // Виправлено: codes.Error
		return c.Send("Failed to process response from image service.", mainMenuMarkup)
	}

	// Check 'status' field in JSON response (should be "OK")
	if imgbunResp.Status != "OK" {
		log.Printf("Error in Imgbun JSON response for user %d: status=%s, message=%s", senderID, imgbunResp.Status, imgbunResp.Message)
		imageGenFailureCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("error.type", "api_logic_error"), attribute.String("api.message", imgbunResp.Message))) // Метрика: помилка
		errMsg := "Failed to generate image."
		if imgbunResp.Message != "" {
			errMsg += fmt.Sprintf(" Service message: %s", imgbunResp.Message)
		}
		span.SetStatus(codes.Error, fmt.Sprintf("Imgbun API status not OK: %s", imgbunResp.Message)) // Виправлено: codes.Error
		return c.Send(errMsg, mainMenuMarkup)
	}

	// Check if direct link is present
	if imgbunResp.DirectLink == "" {
		log.Printf("Error: Imgbun API returned OK but no direct link for user %d", senderID)
		imageGenFailureCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("error.type", "no_image_link"))) // Метрика: помилка
		span.SetStatus(codes.Error, "Imgbun API returned no direct link")                                          // Виправлено: codes.Error
		return c.Send("Image service returned success but did not provide an image link.", mainMenuMarkup)
	}

	// Метрика: тривалість генерації зображення
	// Виправлено: Додаємо attributes як окремий аргумент
	imageGenerationDuration.Record(ctx, time.Since(startTime).Seconds(),
		metric.WithAttributes(attribute.Bool("success", true)),
	)
	imageGenSuccessCounter.Add(ctx, 1) // Метрика: успішна генерація

	// Create Photo object to send
	photoToSend := &tele.Photo{
		File:    tele.FromURL(imgbunResp.DirectLink),
		Caption: fmt.Sprintf("Image for: '%s'", text), // Add caption
	}
	// Trim caption if too long (Telegram limit is 1024)
	if len(photoToSend.Caption) > 1024 {
		photoToSend.Caption = photoToSend.Caption[:1020] + "..."
	}

	span.SetAttributes(attribute.String("image.direct_link", imgbunResp.DirectLink))
	log.Printf("Sending generated image %s to user %d (%s)", imgbunResp.DirectLink, senderID, username)

	// Send the photo with the main keyboard
	if err := c.Send(photoToSend, mainMenuMarkup); err != nil {
		log.Printf("Error sending photo to user %d: %v", senderID, err)
		imageGenFailureCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("error.type", "telegram_send_error"))) // Метрика: помилка
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to send photo to Telegram") // Виправлено: codes.Error
		// Attempt to send a text message if photo sending fails
		return c.Send("Failed to send the generated image.", mainMenuMarkup)
	}
	return nil // Return nil on successful send
}

// isUserInSettingsMode checks if a user is currently in settings mode
func isUserInSettingsMode(userID int64) bool {
	inSettingsRaw, ok := userInSettingsMode.Load(userID)
	if !ok {
		return false // Not in map means not in settings mode
	}
	// Safely assert type to bool
	inSettings, isBool := inSettingsRaw.(bool)
	return isBool && inSettings
}

// exitSettingsMode safely transitions a user out of settings mode
func exitSettingsMode(userID int64) {
	userInSettingsMode.Store(userID, false) // Set mode to false
	userWaitingFor.Store(userID, "")        // Clear waiting state
	tempUserSettingsStore.Delete(userID)    // Remove temporary settings data
	log.Printf("User %d exited settings mode.", userID)
}

// isValidHexColor performs basic validation for 3 or 6 character hex colors
func isValidHexColor(hex string) bool {
	hex = strings.ToLower(strings.TrimPrefix(hex, "#")) // Normalize: lowercase, no '#'
	length := len(hex)
	if length != 3 && length != 6 {
		return false // Must be 3 or 6 characters
	}
	// Check if all characters are valid hex digits
	for _, r := range hex {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}

// --- Cobra Initialization ---
func init() {
	rootCmd.AddCommand(kbotCmd)
	// Define flags and configuration settings for the kbot command here, if needed.
	// Example: add a flag for a log file
	// kbotCmd.Flags().StringP("log-file", "l", "", "Path to log file (optional)")
}
