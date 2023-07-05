package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"log"
	"net/http"
	"sync"
	"time"
)

const BufferSize = 1024

var (
	websocketUpgrader = websocket.Upgrader{
		CheckOrigin:     checkOrigin,
		ReadBufferSize:  BufferSize,
		WriteBufferSize: BufferSize,
	}
)

type Manager struct {
	clients ClientList
	sync.RWMutex
	otps     RetentionMap
	handlers map[string]EventHandler
}

func NewManager(ctx context.Context) *Manager {
	m := &Manager{
		clients:  make(map[*Client]bool),
		handlers: make(map[string]EventHandler),
		otps:     NewRetentionMap(ctx, time.Second*5),
	}
	m.setupEventHandlers()
	return m
}

func (m *Manager) routeEvent(event Event, c *Client) error {
	if handler, ok := m.handlers[event.Type]; ok {
		if err := handler(event, c); err != nil {
			return err
		}
		return nil
	} else {
		return errors.New("not such event handler")
	}
}

func (m *Manager) setupEventHandlers() {
	m.handlers[EventSendMessage] = SendMessage
	m.handlers[EventChangeRoom] = ChatRoomHandler
}

func ChatRoomHandler(event Event, client *Client) error {
	var chatroomEvent ChatRoomEvent
	if err := json.Unmarshal(event.Payload, &chatroomEvent); err != nil {
		return fmt.Errorf("bad payload in request :%v", err)
	}
	client.chatroom = chatroomEvent.Name
	return nil
}

func SendMessage(event Event, client *Client) error {
	var chatEvent SendMessageEvent
	if err := json.Unmarshal(event.Payload, &chatEvent); err != nil {
		return fmt.Errorf("bad payload in req:%v", err)
	}
	var broadMessage NewMessageEvent
	broadMessage.Message = chatEvent.Message
	broadMessage.From = chatEvent.From
	broadMessage.Send = time.Now()
	data, err := json.Marshal(broadMessage)
	if err != nil {
		return fmt.Errorf("failed to marshal broad message:%v", err)
	}
	var outgoingEvent = Event{
		Payload: data,
		Type:    EventNewMessage,
	}
	for c := range client.manager.clients {
		if client.chatroom == c.chatroom {
			c.egress <- outgoingEvent
		}
	}
	return nil
}

func (m *Manager) loginHandler(w http.ResponseWriter, r *http.Request) {
	type userloginRequest struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	var req userloginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Username == "admin" && req.Password == "admin" {
		type response struct {
			OTP string `json:"otp"`
		}
		otp := m.otps.NewOTP()
		resp := response{
			OTP: otp.Key,
		}
		data, err := json.Marshal(resp)
		if err != nil {
			log.Println(err)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(data)
		return
	}
	w.WriteHeader(http.StatusUnauthorized)
}

func (m *Manager) serveWs(w http.ResponseWriter, req *http.Request) {
	otp := req.URL.Query().Get("otp")
	if otp == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if !m.otps.VerifyOTP(otp) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	log.Println("new connection")
	conn, err := websocketUpgrader.Upgrade(w, req, nil)
	if err != nil {
		log.Println(err)
		return
	}
	client := NewClient(conn, m)
	m.addClient(client)
	go client.readMessages()
	go client.writeMessages()
}

func (m *Manager) addClient(client *Client) {
	m.Lock()
	defer m.Unlock()
	m.clients[client] = true
}

func (m *Manager) removeClient(client *Client) {
	m.Lock()
	defer m.Unlock()
	delete(m.clients, client)
}

func checkOrigin(req *http.Request) bool {
	origin := req.Header.Get("Origin")
	switch origin {
	case "http://localhost:8080":
		return true
	default:
		return false
	}
}
