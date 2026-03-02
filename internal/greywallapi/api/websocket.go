package api

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-gost/core/logger"
	"github.com/gorilla/websocket"
	greywallapi "github.com/go-gost/gost/internal/greywallapi"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type wsClient struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (c *wsClient) send(msg any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteJSON(msg)
}

func WebSocketHandler(s *Shared) gin.HandlerFunc {
	log := logger.Default().WithFields(map[string]any{"kind": "websocket"})

	return func(c *gin.Context) {
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			log.Errorf("websocket upgrade: %v", err)
			return
		}
		defer conn.Close()

		client := &wsClient{conn: conn}

		// Send connected message
		client.send(gin.H{
			"type":      "connected",
			"message":   "Connected to proxy event stream",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})

		// Subscribe to events
		ch := s.Bus.Subscribe(128)
		defer s.Bus.Unsubscribe(ch)

		// Read commands from client in a goroutine
		done := make(chan struct{})
		go func() {
			defer close(done)
			handleClientCommands(client, s, log)
		}()

		// Forward events to client
		for {
			select {
			case evt, ok := <-ch:
				if !ok {
					return
				}
				if err := client.send(evt); err != nil {
					log.Debugf("ws send error: %v", err)
					return
				}
			case <-done:
				return
			}
		}
	}
}

type wsCommand struct {
	Command   string `json:"command"`
	PendingID int64  `json:"pending_id"`
	Scope     string `json:"scope"`
	Duration  string `json:"duration"`
	Notes     string `json:"notes"`
}

func handleClientCommands(client *wsClient, s *Shared, log logger.Logger) {
	for {
		_, message, err := client.conn.ReadMessage()
		if err != nil {
			return
		}

		var cmd wsCommand
		if err := json.Unmarshal(message, &cmd); err != nil {
			client.send(gin.H{
				"type":      "error",
				"error":     "invalid JSON",
				"timestamp": time.Now().UTC().Format(time.RFC3339),
			})
			continue
		}

		switch cmd.Command {
		case "ping":
			client.send(gin.H{
				"type":      "pong",
				"timestamp": time.Now().UTC().Format(time.RFC3339),
			})

		case "allow":
			scope := cmd.Scope
			if scope == "" {
				scope = "exact"
			}
			duration := cmd.Duration
			if duration == "" {
				duration = "permanent"
			}
			var notes *string
			if cmd.Notes != "" {
				notes = &cmd.Notes
			}

			rule, err := greywallapi.AllowPending(s.DB, cmd.PendingID, scope, duration, notes)
			if err != nil {
				client.send(gin.H{
					"type":      "error",
					"error":     err.Error(),
					"timestamp": time.Now().UTC().Format(time.RFC3339),
				})
				continue
			}

			s.Bus.Publish(greywallapi.Event{
				Type: greywallapi.EventPendingAllowed,
				Data: gin.H{"pending_id": cmd.PendingID, "rule": rule.ToJSON()},
			})

			client.send(gin.H{
				"type":       "command_success",
				"command":    "allow",
				"pending_id": cmd.PendingID,
				"rule_id":    rule.ID,
				"timestamp":  time.Now().UTC().Format(time.RFC3339),
			})

		case "dismiss":
			ok, err := greywallapi.DeletePending(s.DB, cmd.PendingID)
			if err != nil || !ok {
				errMsg := "pending request not found"
				if err != nil {
					errMsg = err.Error()
				}
				client.send(gin.H{
					"type":      "error",
					"error":     errMsg,
					"timestamp": time.Now().UTC().Format(time.RFC3339),
				})
				continue
			}

			s.Bus.Publish(greywallapi.Event{
				Type: greywallapi.EventPendingDismissed,
				Data: gin.H{"pending_id": cmd.PendingID},
			})

			client.send(gin.H{
				"type":       "command_success",
				"command":    "dismiss",
				"pending_id": cmd.PendingID,
				"timestamp":  time.Now().UTC().Format(time.RFC3339),
			})

		default:
			client.send(gin.H{
				"type":      "error",
				"error":     "unknown command: " + cmd.Command,
				"timestamp": time.Now().UTC().Format(time.RFC3339),
			})
		}
	}
}
