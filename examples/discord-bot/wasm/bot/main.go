package main

import (
	"encoding/json"
	"strconv"

	"github.com/extism/go-pdk"
	"github.com/nodafw/noda-pdk-go/noda"
)

// Global state — persists across ticks in Wasm linear memory.
var (
	botToken     string
	lastSequence *int64 // nil until first dispatch received
	sessionID    string
	identified   bool
	rollCounter  int64 // monotonic counter for unique async labels

	// Pending async replies: label → pending roll info
	pendingRolls map[string]pendingRoll
)

type pendingRoll struct {
	channelID string
	message   string
}

//go:wasmexport initialize
func initialize() int32 {
	input, err := noda.GetInitInput()
	if err != nil {
		return noda.Fail(err)
	}

	botToken, _ = input.Config["token"].(string)
	if botToken == "" {
		return noda.FailMsg("DISCORD_BOT_TOKEN not configured")
	}

	pendingRolls = make(map[string]pendingRoll)

	noda.LogInfo("discord bot initialized", nil)

	// Connect to Discord gateway
	if err := noda.WSConnect("discord", "wss://gateway.discord.gg/?v=10&encoding=json", nil); err != nil {
		return noda.Fail(err)
	}

	noda.LogInfo("connecting to discord gateway", nil)
	return 0
}

// Discord gateway payload.
type gatewayPayload struct {
	Op       int             `json:"op"`
	Type     string          `json:"t,omitempty"`
	Sequence *int64          `json:"s,omitempty"`
	Data     json.RawMessage `json:"d,omitempty"`
}

//go:wasmexport tick
func tick() int32 {
	input, err := noda.GetTickInput()
	if err != nil {
		return 0
	}

	// Process async responses from previous tick
	for label, resp := range input.Responses {
		roll, ok := pendingRolls[label]
		if !ok {
			continue
		}
		delete(pendingRolls, label)

		if resp.OK() {
			// The async log succeeded — now send the dice roll reply to Discord.
			sendChannelMessage(roll.channelID, roll.message)
		} else {
			noda.LogError("async roll failed", map[string]any{
				"label": label,
				"error": resp.Error.Message,
			})
		}
	}

	for _, msg := range input.IncomingWS {
		if msg.Connection != "discord" {
			continue
		}

		var payload gatewayPayload
		if err := json.Unmarshal(msg.Data, &payload); err != nil {
			noda.LogError("failed to parse gateway payload", map[string]any{"error": err.Error()})
			continue
		}

		// Track sequence number
		if payload.Sequence != nil {
			lastSequence = payload.Sequence
		}

		switch payload.Op {
		case 10: // HELLO
			handleHello(payload.Data)
		case 0: // DISPATCH
			handleDispatch(payload.Type, payload.Data, input.Timestamp)
		case 1: // HEARTBEAT request — respond immediately
			sendHeartbeat()
		case 11: // HEARTBEAT_ACK
			// Expected, nothing to do
		case 7: // RECONNECT
			noda.LogWarn("discord requested reconnect", nil)
		case 9: // INVALID SESSION
			noda.LogError("invalid session", nil)
		}
	}

	return 0
}

func handleHello(data json.RawMessage) {
	var hello struct {
		HeartbeatInterval float64 `json:"heartbeat_interval"`
	}
	if err := json.Unmarshal(data, &hello); err != nil {
		noda.LogError("failed to parse HELLO", map[string]any{"error": err.Error()})
		return
	}

	noda.LogInfo("received HELLO", map[string]any{
		"heartbeat_interval_ms": hello.HeartbeatInterval,
	})

	// Configure automatic heartbeat via Noda gateway.
	noda.WSConfigure("discord", hello.HeartbeatInterval, map[string]any{"op": 1, "d": lastSequence})

	// Send IDENTIFY
	if !identified {
		sendIdentify()
	}
}

func sendIdentify() {
	identify := map[string]any{
		"op": 2,
		"d": map[string]any{
			"token":   botToken,
			"intents": 33281, // GUILDS | GUILD_MESSAGES | MESSAGE_CONTENT
			"properties": map[string]any{
				"os":      "linux",
				"browser": "noda",
				"device":  "noda",
			},
		},
	}

	if err := noda.WSSend("discord", identify); err != nil {
		noda.LogError("failed to send IDENTIFY", map[string]any{"error": err.Error()})
		return
	}

	identified = true
	noda.LogInfo("sent IDENTIFY", nil)
}

func sendHeartbeat() {
	hb := map[string]any{
		"op": 1,
		"d":  lastSequence,
	}
	noda.WSSend("discord", hb)
}

func handleDispatch(eventType string, data json.RawMessage, timestamp int64) {
	switch eventType {
	case "READY":
		var ready struct {
			SessionID string `json:"session_id"`
			User      struct {
				Username string `json:"username"`
			} `json:"user"`
		}
		if err := json.Unmarshal(data, &ready); err == nil {
			sessionID = ready.SessionID
			noda.LogInfo("READY received", map[string]any{
				"session_id": sessionID,
				"username":   ready.User.Username,
			})
		}

	case "MESSAGE_CREATE":
		var msg struct {
			Content   string `json:"content"`
			ChannelID string `json:"channel_id"`
			Author    struct {
				Bot      bool   `json:"bot"`
				Username string `json:"username"`
			} `json:"author"`
		}
		if err := json.Unmarshal(data, &msg); err != nil {
			return
		}

		// Ignore messages from bots (including ourselves)
		if msg.Author.Bot {
			return
		}

		switch msg.Content {
		case "!ping":
			noda.LogInfo("received !ping", map[string]any{
				"channel": msg.ChannelID,
				"user":    msg.Author.Username,
			})
			sendChannelMessage(msg.ChannelID, "Pong!")

		case "!roll":
			handleRoll(msg.ChannelID, msg.Author.Username, timestamp)
		}
	}
}

func handleRoll(channelID, username string, timestamp int64) {
	// Simple pseudo-random: use timestamp + counter to get a 1-6 result
	rollCounter++
	result := (timestamp+rollCounter)%6 + 1

	label := "roll-" + strconv.FormatInt(rollCounter, 10)
	message := "🎲 " + username + " rolled a **" + strconv.FormatInt(result, 10) + "**!"

	// Store the pending reply — we'll send it once the async response arrives
	pendingRolls[label] = pendingRoll{
		channelID: channelID,
		message:   message,
	}

	// Fire an async log call. The result arrives in the next tick's responses map.
	noda.CallAsync("", "log", label, map[string]any{
		"level":   "info",
		"message": "dice roll",
		"fields": map[string]any{
			"user":    username,
			"result":  result,
			"channel": channelID,
		},
	})
}

func sendChannelMessage(channelID, content string) {
	url := "https://discord.com/api/v10/channels/" + channelID + "/messages"

	body, _ := json.Marshal(map[string]string{
		"content": content,
	})

	req := pdk.NewHTTPRequest(pdk.MethodPost, url)
	req.SetHeader("Authorization", "Bot "+botToken)
	req.SetHeader("Content-Type", "application/json")
	req.SetBody(body)

	resp := req.Send()

	if resp.Status() >= 400 {
		noda.LogError("failed to send message", map[string]any{
			"status":  strconv.Itoa(int(resp.Status())),
			"channel": channelID,
			"body":    string(resp.Body()),
		})
	}
}

//go:wasmexport query
func query() int32 {
	result := map[string]any{
		"connected":  identified,
		"session_id": sessionID,
	}
	if lastSequence != nil {
		result["last_sequence"] = *lastSequence
	}
	return noda.Output(result)
}

//go:wasmexport shutdown
func shutdown() int32 {
	noda.LogInfo("discord bot shutting down", map[string]any{
		"session_id": sessionID,
	})

	// Close the gateway connection gracefully
	noda.WSClose("discord", 1000, "bot shutting down")
	return 0
}

func main() {}
