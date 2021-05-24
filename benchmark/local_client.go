package benchmark

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"golang.org/x/net/websocket"
)

var RemoteAddr struct {
	Addr   *net.TCPAddr
	Host   string
	Config *websocket.Config
	Secure bool
}

const (
	MsgServerEcho            = 'e'
	MsgServerBroadcast       = 'b'
	MsgServerBroadcastResult = 'r'
	MsgClientEcho            = 'e'
	MsgClientBroadcast       = 'b'
)

type localClient struct {
	conn           *websocket.Conn
	config         *websocket.Config
	laddr          *net.TCPAddr
	dest           string
	origin         string
	serverType     string
	serverAdapter  ServerAdapter
	rttResultChan  chan<- time.Duration
	errChan        chan<- error
	payloadPadding []byte

	rxBroadcastCountLock sync.Mutex
	rxBroadcastCount     int
}

type ServerAdapter interface {
	SendEcho(payload *Payload) error
	SendBroadcast(payload *Payload) error
	Receive() (*serverSentMsg, error)
}

type Payload struct {
	SendTime time.Time
	Padding  []byte
}

type jsonPayload struct {
	SendTime string      `json:"sendTime"`
	Padding  interface{} `json:"padding,omitempty"`
}

// serverSentMsg includes all fields that can be in server sent message
type serverSentMsg struct {
	Type          byte
	Payload       *Payload
	ListenerCount int
}

type jsonServerSentMsg struct {
	Type          string       `json:"type"`
	Payload       *jsonPayload `json:"payload"`
	ListenerCount int          `json:"listenerCount"`
}

func newLocalClient(
	laddr *net.TCPAddr,
	dest, origin, serverType string,
	rttResultChan chan<- time.Duration,
	errChan chan error,
	padding []byte,
) (*localClient, error) {
	if origin == "" {
		origin = dest
	}

	c := &localClient{
		laddr:          laddr,
		dest:           dest,
		origin:         origin,
		rttResultChan:  rttResultChan,
		errChan:        errChan,
		payloadPadding: padding,
	}

	tcpConn, err := net.DialTCP("tcp", c.laddr, RemoteAddr.Addr)
	if err != nil {
		panic(err)
	}

	var conn io.ReadWriteCloser

	if RemoteAddr.Secure {
		conn = tls.Client(tcpConn, &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         RemoteAddr.Host,
		})
	} else {
		conn = tcpConn
	}

	initTime := time.Now()

	c.conn, err = websocket.NewClient(RemoteAddr.Config, conn)
	if err != nil {
		return nil, err
	}

	c.conn.MaxPayloadBytes = 1000000

	switch serverType {
	case "json":
		c.serverAdapter = &StandardServerAdapter{conn: c.conn}
	case "binary":
		c.serverAdapter = &BinaryServerAdapter{conn: c.conn}
	case "actioncable":
		acsa := &ActionCableServerAdapter{conn: c.conn}
		err = acsa.Startup()
		if err != nil {
			return nil, err
		}
		c.serverAdapter = acsa
	case "actioncable-connect":
		acsa := &ActionCableServerConnectAdapter{conn: c.conn}
		err = acsa.Connected(initTime)
		if err != nil {
			return nil, err
		}
		c.serverAdapter = acsa
	case "phoenix":
		psa := &PhoenixServerAdapter{conn: c.conn}
		err = psa.Startup()
		if err != nil {
			return nil, err
		}
		c.serverAdapter = psa
	default:
		return nil, fmt.Errorf("Unknown server type: %v", serverType)
	}

	go c.rx()

	return c, nil
}

func (c *localClient) SendEcho() error {
	return c.serverAdapter.SendEcho(&Payload{SendTime: time.Now(), Padding: c.payloadPadding})
}

func (c *localClient) SendBroadcast() error {
	return c.serverAdapter.SendBroadcast(&Payload{SendTime: time.Now(), Padding: c.payloadPadding})
}

func (c *localClient) ResetRxBroadcastCount() (int, error) {
	c.rxBroadcastCountLock.Lock()
	count := c.rxBroadcastCount
	c.rxBroadcastCount = 0
	c.rxBroadcastCountLock.Unlock()
	return count, nil
}

func (c *localClient) rx() {
	for {
		msg, err := c.serverAdapter.Receive()
		if err != nil {
			c.errChan <- err
			return
		}

		switch msg.Type {
		case MsgServerEcho, MsgServerBroadcastResult:
			if msg.Payload != nil {
				rtt := time.Now().Sub(msg.Payload.SendTime)
				c.rttResultChan <- rtt
			} else {
				c.errChan <- fmt.Errorf("received unparsable %s payload: %v", msg.Type, msg.Payload)
				return
			}
		case MsgServerBroadcast:
			c.rxBroadcastCountLock.Lock()
			c.rxBroadcastCount++
			c.rxBroadcastCountLock.Unlock()
		default:
			c.errChan <- fmt.Errorf("received unknown message type: %v", msg.Type)
			return
		}
	}
}

type LocalClientPool struct {
	laddr   *net.TCPAddr
	clients map[int]*localClient
	mu      sync.Mutex
}

func NewLocalClientPool(laddr *net.TCPAddr) *LocalClientPool {
	return &LocalClientPool{
		laddr:   laddr,
		clients: make(map[int]*localClient),
	}
}

func (lcp *LocalClientPool) New(
	id int,
	dest, origin, serverType string,
	rttResultChan chan time.Duration,
	errChan chan error,
	padding []byte,
) (Client, error) {
	c, err := newLocalClient(lcp.laddr, dest, origin, serverType, rttResultChan, errChan, padding)
	if err != nil {
		return nil, err
	}

	lcp.mu.Lock()
	lcp.clients[id] = c
	lcp.mu.Unlock()

	return c, nil
}

func (lcp *LocalClientPool) Close() error {
	for _, c := range lcp.clients {
		if err := c.conn.Close(); err != nil {
			return err
		}
	}

	return nil
}
