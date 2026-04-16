package ws

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"

	"njk_go/internal/bot"
	"njk_go/internal/napcat"
)

const (
	wsOpcodeText  = 0x1
	wsOpcodeClose = 0x8
	wsOpcodePing  = 0x9
	wsOpcodePong  = 0xA
)

type Server struct {
	addr    string
	service *bot.Service
}

func NewServer(addr string, service *bot.Service) *Server {
	return &Server{addr: addr, service: service}
}

func (s *Server) ListenAndServe() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleWebSocket)

	log.Printf("WebSocket 服务器启动，监听 %s", s.addr)

	server := &http.Server{
		Addr:    s.addr,
		Handler: mux,
	}

	return server.ListenAndServe()
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	if !isWebSocketUpgrade(r) {
		http.Error(w, "websocket upgrade required", http.StatusUpgradeRequired)
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijack not supported", http.StatusInternalServerError)
		return
	}

	conn, rw, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := writeHandshakeResponse(rw, r.Header.Get("Sec-WebSocket-Key")); err != nil {
		_ = conn.Close()
		log.Printf("WebSocket 握手失败: %v", err)
		return
	}

	clientAddr := conn.RemoteAddr().String()
	log.Printf("【新连接】客户端地址: %s", clientAddr)

	wsConn := &Conn{
		conn:   conn,
		reader: rw.Reader,
	}
	connCtx, cancel := context.WithCancel(context.Background())

	defer func() {
		cancel()
		_ = wsConn.Close()
		log.Printf("【连接关闭】客户端地址: %s", clientAddr)
	}()

	for {
		payload, err := wsConn.ReadText()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				log.Printf("【连接异常】%s - %v", clientAddr, err)
			}
			return
		}

		log.Printf("【收到消息】%s - %s", clientAddr, string(payload))

		parsed, err := napcat.ParseInboundMessage(payload)
		if err != nil {
			log.Printf("【解析失败】%s - %v", clientAddr, err)
			continue
		}

		switch parsed.Kind {
		case napcat.EventKindNotice:
			go s.service.HandleNotice(connCtx, wsConn, clientAddr, parsed.Notice)
		case napcat.EventKindGroupMessage:
			go s.service.HandleGroupMessage(connCtx, wsConn, clientAddr, parsed.GroupMessage)
		case napcat.EventKindActionResponse:
			if parsed.Action != nil {
				log.Printf("【收到回执】%s - status=%s retcode=%d", clientAddr, parsed.Action.Status, parsed.Action.Retcode)
				go s.service.HandleActionResponse(connCtx, parsed.Action)
			}
		default:
			log.Printf("【收到其他事件】%s - kind=%s", clientAddr, parsed.Kind)
		}
	}
}

type Conn struct {
	conn    net.Conn
	reader  *bufio.Reader
	writeMu sync.Mutex
}

func (c *Conn) Close() error {
	return c.conn.Close()
}

func (c *Conn) ReadText() ([]byte, error) {
	for {
		finOpcode := make([]byte, 2)
		if _, err := io.ReadFull(c.reader, finOpcode); err != nil {
			return nil, err
		}

		opcode := finOpcode[0] & 0x0F
		masked := finOpcode[1]&0x80 != 0
		payloadLen := uint64(finOpcode[1] & 0x7F)

		if payloadLen == 126 {
			var ext uint16
			if err := binary.Read(c.reader, binary.BigEndian, &ext); err != nil {
				return nil, err
			}
			payloadLen = uint64(ext)
		} else if payloadLen == 127 {
			var ext uint64
			if err := binary.Read(c.reader, binary.BigEndian, &ext); err != nil {
				return nil, err
			}
			payloadLen = ext
		}

		var maskKey [4]byte
		if masked {
			if _, err := io.ReadFull(c.reader, maskKey[:]); err != nil {
				return nil, err
			}
		}

		payload := make([]byte, payloadLen)
		if _, err := io.ReadFull(c.reader, payload); err != nil {
			return nil, err
		}

		if masked {
			for i := range payload {
				payload[i] ^= maskKey[i%4]
			}
		}

		switch opcode {
		case wsOpcodeText:
			return payload, nil
		case wsOpcodePing:
			if err := c.writeFrame(wsOpcodePong, payload); err != nil {
				return nil, err
			}
		case wsOpcodePong:
		case wsOpcodeClose:
			_ = c.writeFrame(wsOpcodeClose, nil)
			return nil, io.EOF
		default:
			if finOpcode[0]&0x80 == 0 {
				return nil, fmt.Errorf("unsupported fragmented frame opcode=%d", opcode)
			}
		}
	}
}

func (c *Conn) WriteText(payload []byte) error {
	return c.writeFrame(wsOpcodeText, payload)
}

func (c *Conn) writeFrame(opcode byte, payload []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	header := []byte{0x80 | opcode}
	length := len(payload)

	switch {
	case length < 126:
		header = append(header, byte(length))
	case length <= 65535:
		header = append(header, 126, byte(length>>8), byte(length))
	default:
		header = append(header, 127)
		ext := make([]byte, 8)
		binary.BigEndian.PutUint64(ext, uint64(length))
		header = append(header, ext...)
	}

	if _, err := c.conn.Write(header); err != nil {
		return err
	}

	if length == 0 {
		return nil
	}

	_, err := c.conn.Write(payload)
	return err
}

func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket") &&
		strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade") &&
		r.Header.Get("Sec-WebSocket-Key") != ""
}

func writeHandshakeResponse(rw *bufio.ReadWriter, key string) error {
	acceptKey := computeAcceptKey(key)
	response := fmt.Sprintf(
		"HTTP/1.1 101 Switching Protocols\r\n"+
			"Upgrade: websocket\r\n"+
			"Connection: Upgrade\r\n"+
			"Sec-WebSocket-Accept: %s\r\n\r\n",
		acceptKey,
	)

	if _, err := rw.WriteString(response); err != nil {
		return err
	}
	return rw.Flush()
}

func computeAcceptKey(key string) string {
	hash := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	return base64.StdEncoding.EncodeToString(hash[:])
}
