/*
FILE: internal/ws/conn.go

DESCRIPTION:
Managing wrapper over a single Gate WebSocket connection. One Conn serves both
public and private channels on the same Gate endpoint (Gate authenticates
per-subscribe via the auth object, so there is no separate login step).

RESPONSIBILITIES:
  - connect / reconnect with backoff + jitter;
  - heartbeat (application-level ping on cfg.PingChannel);
  - subscribe with buffering (registered even while disconnected, applied on connect);
  - resubscribe after reconnect, fully transparent to the caller;
  - dispatch incoming data pushes to subscription handlers;
  - graceful shutdown via ctx / Close.

SUBSCRIPTION MODEL:
  Multiple handlers may share one server subscription key (channel|payload): e.g.
  WatchMarkPrice and WatchLastPrice both ride the tickers channel for one contract.
  The Conn sends ONE subscribe per unique key and fans every push out to all
  handlers registered on that channel; each handler filters by contract itself.

LIFETIME:
  Start(ctx) wins once — the connection lives until that ctx is cancelled or
  Close() is called. Individual Watch* calls cannot be cancelled independently
  (mirrors the sibling SDKs); cancel the shared ctx or Close the section.

ERROR STRATEGY:
  - transient socket errors → reconnect;
  - subscribe acks with an error object → logged, the reconnect cycle continues;
  - a closed Conn ignores further Subscribe calls.
*/

package ws

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/tonymontanov/go-gate/v2/internal/auth"
	"github.com/tonymontanov/go-gate/v2/internal/codec"
	"github.com/tonymontanov/go-gate/v2/internal/gatelog"
)

// Subscription describes one subscription. Channel/Payload identify the server
// subscription; Handler receives every push on Channel (filter by contract in
// the handler). Reset, if set, runs before each resubscribe (after reconnect).
type Subscription struct {
	Channel string
	Payload []string
	Private bool
	Handler func(event string, result []byte)
	Reset   func()
}

func (s *Subscription) key() string {
	return s.Channel + "|" + strings.Join(s.Payload, ",")
}

// Config — parameters for a single WS connection (derived from gate.WsConfig).
type Config struct {
	URL                     string
	PingChannel             string
	HandshakeTimeout        time.Duration
	ReadTimeout             time.Duration
	WriteTimeout            time.Duration
	PingInterval            time.Duration
	ReconnectInitialBackoff time.Duration
	ReconnectMaxBackoff     time.Duration
	ReconnectJitter         float64
	ReadBufferSize          int
	WriteBufferSize         int
}

// Conn — managing wrapper over a single Gate WS connection.
type Conn struct {
	cfg    Config
	signer *auth.Signer
	logger gatelog.Logger

	mu        sync.RWMutex
	subsByKey map[string][]*Subscription
	byChannel map[string][]*Subscription
	socket    *websocket.Conn
	closed    bool
	cancel    context.CancelFunc

	writeMu   sync.Mutex
	startOnce sync.Once
}

// NewConn creates a Conn. No network activity until Start/Subscribe.
func NewConn(cfg Config, signer *auth.Signer, log gatelog.Logger) *Conn {
	if log == nil {
		log = gatelog.Noop()
	}
	return &Conn{
		cfg:       cfg,
		signer:    signer,
		logger:    log,
		subsByKey: make(map[string][]*Subscription, 16),
		byChannel: make(map[string][]*Subscription, 16),
	}
}

// Start launches the supervisor loop (connect→read→reconnect). Idempotent;
// the first ctx wins. Stops on ctx cancellation or Close.
func (c *Conn) Start(ctx context.Context) {
	c.startOnce.Do(func() {
		var supCtx context.Context
		supCtx, c.cancel = context.WithCancel(ctx)
		go c.supervise(supCtx)
	})
}

// Subscribe registers a handler and, if the socket is up and the key is new,
// sends the subscribe command. The subscription is restored after reconnect.
func (c *Conn) Subscribe(sub *Subscription) error {
	if sub == nil || sub.Channel == "" || sub.Handler == nil {
		return errors.New("ws: invalid subscription")
	}
	var key string = sub.key()

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrConnClosed
	}
	var isNewKey bool = len(c.subsByKey[key]) == 0
	c.subsByKey[key] = append(c.subsByKey[key], sub)
	c.byChannel[sub.Channel] = append(c.byChannel[sub.Channel], sub)
	var socket *websocket.Conn = c.socket
	c.mu.Unlock()

	if socket == nil || !isNewKey {
		return nil // applied on (re)connect, or key already subscribed
	}
	return c.sendSubscribe(socket, sub)
}

// ErrConnClosed is returned by Subscribe after Close.
var ErrConnClosed = errors.New("ws: connection closed")

// Close shuts down the connection. Safe to call multiple times.
func (c *Conn) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	if c.cancel != nil {
		c.cancel()
	}
	var s *websocket.Conn = c.socket
	c.socket = nil
	c.mu.Unlock()
	if s != nil {
		_ = s.Close()
	}
	return nil
}

// supervise — connect→read→backoff loop. Stops on ctx.Done.
func (c *Conn) supervise(ctx context.Context) {
	var backoff time.Duration = c.cfg.ReconnectInitialBackoff
	var attempt int
	for {
		if ctx.Err() != nil {
			return
		}
		var err error = c.connectAndRun(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			c.logger.Warn("ws: connection error, will reconnect",
				gatelog.Str("url", c.cfg.URL), gatelog.Int("attempt", int64(attempt)), gatelog.Err(err))
		}
		attempt++
		var sleep time.Duration = applyJitter(backoff, c.cfg.ReconnectJitter)
		select {
		case <-ctx.Done():
			return
		case <-time.After(sleep):
		}
		backoff = nextBackoff(backoff, c.cfg.ReconnectMaxBackoff)
	}
}

// connectAndRun dials, resubscribes, and runs read+ping loops until one fails.
func (c *Conn) connectAndRun(ctx context.Context) error {
	var dialer *websocket.Dialer = &websocket.Dialer{
		HandshakeTimeout: c.cfg.HandshakeTimeout,
		ReadBufferSize:   c.cfg.ReadBufferSize,
		WriteBufferSize:  c.cfg.WriteBufferSize,
	}
	var socket *websocket.Conn
	var err error
	socket, _, err = dialer.DialContext(ctx, c.cfg.URL, nil)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	c.logger.Info("ws: connected", gatelog.Str("url", c.cfg.URL))

	// Snapshot one subscription per unique key (call Reset before resubscribe).
	c.mu.Lock()
	c.socket = socket
	var toSubscribe []*Subscription = make([]*Subscription, 0, len(c.subsByKey))
	for _, subs := range c.subsByKey {
		if len(subs) == 0 {
			continue
		}
		var i int
		for i = 0; i < len(subs); i++ {
			if subs[i].Reset != nil {
				subs[i].Reset()
			}
		}
		toSubscribe = append(toSubscribe, subs[0])
	}
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		if c.socket == socket {
			c.socket = nil
		}
		c.mu.Unlock()
		_ = socket.Close()
	}()

	var i int
	for i = 0; i < len(toSubscribe); i++ {
		if err = c.sendSubscribe(socket, toSubscribe[i]); err != nil {
			c.logger.Warn("ws: resubscribe failed", gatelog.Str("channel", toSubscribe[i].Channel), gatelog.Err(err))
		}
	}

	var loopCtx context.Context
	var loopCancel context.CancelFunc
	loopCtx, loopCancel = context.WithCancel(ctx)
	defer loopCancel()

	var wg sync.WaitGroup
	wg.Add(2)
	var readErr error
	go func() {
		defer wg.Done()
		defer loopCancel()
		readErr = c.readLoop(loopCtx, socket)
	}()
	go func() {
		defer wg.Done()
		c.pingLoop(loopCtx, socket)
	}()
	wg.Wait()

	if readErr != nil {
		return readErr
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

// readLoop reads frames and dispatches data pushes to handlers.
func (c *Conn) readLoop(ctx context.Context, socket *websocket.Conn) error {
	for {
		if ctx.Err() != nil {
			return nil
		}
		_ = socket.SetReadDeadline(time.Now().Add(c.cfg.ReadTimeout))
		var msgType int
		var message []byte
		var err error
		msgType, message, err = socket.ReadMessage()
		if err != nil {
			return err
		}
		if msgType != websocket.TextMessage {
			continue
		}

		var env response
		if err = codec.Unmarshal(message, &env); err != nil {
			c.logger.Warn("ws: failed to parse message", gatelog.Err(err))
			continue
		}

		// Heartbeat pong (channel "<prefix>.pong").
		if strings.HasSuffix(env.Channel, ".pong") {
			continue
		}

		switch env.Event {
		case eventSubscribe, eventUnsubscribe:
			if env.Error != nil {
				c.logger.Warn("ws: subscribe error",
					gatelog.Str("channel", env.Channel),
					gatelog.Int("code", int64(env.Error.Code)),
					gatelog.Str("msg", env.Error.Message))
			} else {
				c.logger.Debug("ws: ack", gatelog.Str("channel", env.Channel), gatelog.Str("event", env.Event))
			}
			continue
		}

		if env.Error != nil {
			c.logger.Warn("ws: server error",
				gatelog.Str("channel", env.Channel),
				gatelog.Int("code", int64(env.Error.Code)),
				gatelog.Str("msg", env.Error.Message))
			continue
		}
		c.dispatch(env.Channel, env.Event, env.Result)
	}
}

// dispatch fans a push out to every handler registered on the channel.
func (c *Conn) dispatch(channel, event string, result []byte) {
	c.mu.RLock()
	var subs []*Subscription = c.byChannel[channel]
	c.mu.RUnlock()
	var i int
	for i = 0; i < len(subs); i++ {
		subs[i].Handler(event, result)
	}
}

// pingLoop sends an application-level ping on cfg.PingChannel.
func (c *Conn) pingLoop(ctx context.Context, socket *websocket.Conn) {
	if c.cfg.PingInterval <= 0 || c.cfg.PingChannel == "" {
		<-ctx.Done()
		return
	}
	var ticker *time.Ticker = time.NewTicker(c.cfg.PingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var req request = request{Time: time.Now().Unix(), Channel: c.cfg.PingChannel}
			var raw []byte
			var err error
			raw, err = codec.Marshal(req)
			if err != nil {
				return
			}
			if err = c.writeMessage(socket, raw); err != nil {
				c.logger.Debug("ws: ping failed", gatelog.Err(err))
				return
			}
		}
	}
}

// sendSubscribe builds and writes a subscribe command (with auth for private).
func (c *Conn) sendSubscribe(socket *websocket.Conn, sub *Subscription) error {
	var ts int64 = time.Now().Unix()
	var req request = request{
		Time:    ts,
		Channel: sub.Channel,
		Event:   eventSubscribe,
		Payload: sub.Payload,
	}
	if sub.Private {
		if c.signer == nil || !c.signer.Enabled() {
			return errors.New("ws: private channel requires signer with credentials")
		}
		var sign string
		var err error
		sign, err = c.signer.SignWS(sub.Channel, eventSubscribe, ts)
		if err != nil {
			return err
		}
		req.Auth = &authBlock{Method: "api_key", Key: c.signer.APIKey(), Sign: sign}
	}
	var raw []byte
	var err error
	raw, err = codec.Marshal(req)
	if err != nil {
		return err
	}
	return c.writeMessage(socket, raw)
}

// writeMessage — thread-safe write (gorilla requires exclusive writes).
func (c *Conn) writeMessage(socket *websocket.Conn, data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_ = socket.SetWriteDeadline(time.Now().Add(c.cfg.WriteTimeout))
	return socket.WriteMessage(websocket.TextMessage, data)
}

// nextBackoff doubles backoff without exceeding max.
func nextBackoff(cur, max time.Duration) time.Duration {
	cur *= 2
	if cur > max {
		cur = max
	}
	return cur
}

// applyJitter adds a random multiplier [1-j, 1+j] to d.
func applyJitter(d time.Duration, jitter float64) time.Duration {
	if jitter <= 0 {
		return d
	}
	var f float64 = 1.0 + (rand.Float64()*2.0-1.0)*jitter
	return time.Duration(float64(d) * f)
}
