package httpx

// A minimal WebSocket client (RFC 6455), standard library only, for APIs
// that exist only over a WebSocket (Home Assistant). Copied from Sentinel's
// internal/httpx/ws.go (same code, same behaviour). Text messages
// only; ping is answered, fragmented messages are reassembled. One reader
// and one writer at a time.

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // mandated by RFC 6455 for the handshake, not for security
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const wsMaxMessage = 32 << 20

type WSConn struct {
	conn net.Conn
	r    *bufio.Reader
}

// DialWS opens ws:// or wss:// (http:// and https:// are accepted too).
func DialWS(ctx context.Context, rawURL string, insecure bool) (*WSConn, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	secure := u.Scheme == "wss" || u.Scheme == "https"
	host := u.Host
	if u.Port() == "" {
		if secure {
			host += ":443"
		} else {
			host += ":80"
		}
	}
	d := net.Dialer{Timeout: 10 * time.Second}
	var c net.Conn
	if secure {
		c, err = (&tls.Dialer{NetDialer: &d, Config: &tls.Config{ServerName: u.Hostname(), InsecureSkipVerify: insecure}}).DialContext(ctx, "tcp", host) //nolint:gosec // LAN only
	} else {
		c, err = d.DialContext(ctx, "tcp", host)
	}
	if err != nil {
		return nil, err
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = c.SetDeadline(dl)
	}
	key := make([]byte, 16)
	_, _ = rand.Read(key)
	k := base64.StdEncoding.EncodeToString(key)
	path := u.RequestURI()
	req := "GET " + path + " HTTP/1.1\r\nHost: " + u.Host + "\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + k + "\r\nSec-WebSocket-Version: 13\r\n\r\n"
	if _, err := io.WriteString(c, req); err != nil {
		c.Close()
		return nil, err
	}
	r := bufio.NewReader(c)
	resp, err := http.ReadResponse(r, &http.Request{Method: "GET"})
	if err != nil {
		c.Close()
		return nil, err
	}
	sum := sha1.Sum([]byte(k + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11")) //nolint:gosec
	if resp.StatusCode != http.StatusSwitchingProtocols || !strings.EqualFold(resp.Header.Get("Upgrade"), "websocket") ||
		resp.Header.Get("Sec-WebSocket-Accept") != base64.StdEncoding.EncodeToString(sum[:]) {
		c.Close()
		return nil, fmt.Errorf("websocket refusé : HTTP %d", resp.StatusCode)
	}
	// the ctx deadline was for the handshake only: a long-lived connection must not inherit it
	// (otherwise every read fails with "i/o timeout" once it passes)
	_ = c.SetDeadline(time.Time{})
	return &WSConn{conn: c, r: r}, nil
}

func (w *WSConn) Close() error {
	_ = w.writeFrame(8, nil)
	return w.conn.Close()
}

// writeFrame sends one masked frame (clients must mask, RFC 6455 §5.3).
func (w *WSConn) writeFrame(op byte, p []byte) error {
	h := []byte{0x80 | op}
	switch n := len(p); {
	case n < 126:
		h = append(h, 0x80|byte(n))
	case n <= 0xFFFF:
		h = append(h, 0x80|126, byte(n>>8), byte(n))
	default:
		h = append(h, 0x80|127)
		h = binary.BigEndian.AppendUint64(h, uint64(n))
	}
	mask := make([]byte, 4)
	_, _ = rand.Read(mask)
	h = append(h, mask...)
	m := make([]byte, len(p))
	for i := range p {
		m[i] = p[i] ^ mask[i%4]
	}
	_, err := w.conn.Write(append(h, m...))
	return err
}

func (w *WSConn) WriteJSON(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return w.writeFrame(1, b)
}

// ReadMessage returns the next text or binary message.
func (w *WSConn) ReadMessage() ([]byte, error) {
	var msg []byte
	for {
		var h [2]byte
		if _, err := io.ReadFull(w.r, h[:]); err != nil {
			return nil, err
		}
		fin, op := h[0]&0x80 != 0, h[0]&0x0F
		n := uint64(h[1] & 0x7F)
		switch n {
		case 126:
			var b [2]byte
			if _, err := io.ReadFull(w.r, b[:]); err != nil {
				return nil, err
			}
			n = uint64(binary.BigEndian.Uint16(b[:]))
		case 127:
			var b [8]byte
			if _, err := io.ReadFull(w.r, b[:]); err != nil {
				return nil, err
			}
			n = binary.BigEndian.Uint64(b[:])
		}
		var mask []byte
		if h[1]&0x80 != 0 {
			mask = make([]byte, 4)
			if _, err := io.ReadFull(w.r, mask); err != nil {
				return nil, err
			}
		}
		if n > wsMaxMessage || uint64(len(msg))+n > wsMaxMessage {
			return nil, errors.New("message websocket trop long")
		}
		p := make([]byte, n)
		if _, err := io.ReadFull(w.r, p); err != nil {
			return nil, err
		}
		if mask != nil {
			for i := range p {
				p[i] ^= mask[i%4]
			}
		}
		switch op {
		case 9: // ping
			if err := w.writeFrame(10, p); err != nil {
				return nil, err
			}
			continue
		case 10: // pong
			continue
		case 8:
			return nil, io.EOF
		}
		msg = append(msg, p...)
		if fin {
			return msg, nil
		}
	}
}

func (w *WSConn) ReadJSON(v any) error {
	b, err := w.ReadMessage()
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
