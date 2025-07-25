package websocket

import (
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// WSMessage représente un message WebSocket
type WSMessage struct {
	Type string                 `json:"type"`
	Data map[string]interface{} `json:"data"`
}

// handleWebSocket gère les connexions WebSocket
func (s *Server) handleWebSocket(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		s.logger.Error("Failed to upgrade to WebSocket", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}
	defer conn.Close()

	s.logger.Info("New WebSocket connection established")

	for {
		var msg WSMessage
		err := conn.ReadJSON(&msg)
		if err != nil {
			s.logger.Error("Failed to read WebSocket message", map[string]interface{}{
				"error": err.Error(),
			})
			break
		}

		// Traiter le message selon son type
		switch msg.Type {
		default:
			s.sendError(conn, "Unknown message type: "+msg.Type)
		}
	}
}

// sendError envoie un message d'erreur
func (s *Server) sendError(conn *websocket.Conn, message string) {
	errorMsg := WSMessage{
		Type: "Error",
		Data: map[string]interface{}{
			"message": message,
		},
	}
	if err := conn.WriteJSON(errorMsg); err != nil {
		s.logger.Error("Failed to send error response", map[string]interface{}{
			"error": err.Error(),
		})
	}
}
