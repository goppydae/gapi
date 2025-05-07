package transport

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"io"
	"log"
	"sync"

	quicgo "github.com/quic-go/quic-go"
	"google.golang.org/protobuf/proto"

	"github.com/goppydae/gapi/internal/eventbus"
	pb "github.com/goppydae/gapi/internal/proto"
)

type QUICTransport struct {
	listener *quicgo.Listener
	conn     quicgo.Connection
	onEvent  func(eventbus.Event)
	lock     sync.Mutex
}

func NewQUICServer(addr string, cert tls.Certificate) (*QUICTransport, error) {
	tlsConf := &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"gapi-quic"},
	}

	listener, err := quicgo.ListenAddr(addr, tlsConf, nil)
	if err != nil {
		return nil, err
	}

	qt := &QUICTransport{listener: listener}
	go qt.acceptLoop()
	return qt, nil
}

func NewQUICClient(addr string, cert *tls.Certificate) (*QUICTransport, error) {
	tlsConf := &tls.Config{
		Certificates:       []tls.Certificate{*cert},
		InsecureSkipVerify: true,
		NextProtos:         []string{"gapi-quic"},
	}

	conn, err := quicgo.DialAddr(context.Background(), addr, tlsConf, nil)
	if err != nil {
		return nil, err
	}

	qt := &QUICTransport{
		conn: conn,
	}

	// Handle inbound streams from server
	go qt.handleConn(conn)

	return qt, nil
}

// Added back missing clientStreamLoop
func (qt *QUICTransport) clientStreamLoop() {
	for {
		stream, err := qt.conn.AcceptStream(context.Background())
		if err != nil {
			log.Println("client stream error:", err)
			return
		}
		go qt.handleStream(stream)
	}
}

func (qt *QUICTransport) acceptLoop() {
	for {
		conn, err := qt.listener.Accept(context.Background())
		if err != nil {
			if err.Error() == "listener closed" {
				log.Println("Listener closed gracefully.")
				return
			}
			log.Println("QUIC accept error:", err)
			continue
		}
		go qt.handleConn(conn)
	}
}

func (qt *QUICTransport) handleConn(conn quicgo.Connection) {
	qt.lock.Lock()
	qt.conn = conn
	qt.lock.Unlock()

	for {
		stream, err := conn.AcceptStream(context.Background())
		if err != nil {
			log.Println("AcceptStream error:", err)
			return
		}
		go qt.handleStream(stream)
	}
}

func (qt *QUICTransport) handleStream(stream quicgo.Stream) {
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(stream, lenBuf); err != nil {
		log.Println("Read length error:", err)
		return
	}
	length := binary.BigEndian.Uint32(lenBuf)

	data := make([]byte, length)
	if _, err := io.ReadFull(stream, data); err != nil {
		log.Println("Read data error:", err)
		return
	}

	var env pb.Envelope
	if err := proto.Unmarshal(data, &env); err != nil {
		log.Println("Unmarshal error:", err)
		return
	}

	e := eventbus.Event{
		ID:      env.Id,
		Scope:   "user",
		Topic:   env.Topic,
		Source:  env.Source,
		Payload: env.Payload,
	}

	if qt.onEvent != nil {
		log.Printf("Dispatching remote event id=%s topic=%s", e.ID, e.Topic)
		qt.onEvent(e)
	} else {
		log.Printf("No event handler registered. Dropping event id=%s", e.ID)
	}
}

func (qt *QUICTransport) PublishRemote(e eventbus.Event) error {
	qt.lock.Lock()
	defer qt.lock.Unlock()

	if qt.conn == nil {
		log.Println("QUICTransport: conn is nil in PublishRemote")
		return nil
	}

	stream, err := qt.conn.OpenStreamSync(context.Background())
	if err != nil {
		return err
	}
	defer stream.Close()

	env := &pb.Envelope{
		Id:      e.ID,
		Type:    "event", // Or e.Type if you separate types
		Topic:   e.Topic,
		Source:  e.Source,
		Payload: e.Payload,
	}

	data, err := proto.Marshal(env)
	if err != nil {
		return err
	}

	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(data)))

	if _, err := stream.Write(lenBuf); err != nil {
		return err
	}
	if _, err := stream.Write(data); err != nil {
		return err
	}

	log.Printf("QUICTransport: sent envelope id=%s topic=%s to peer", e.ID, e.Topic)
	return nil
}

func (qt *QUICTransport) Broadcast(e eventbus.Event) error {
	return qt.PublishRemote(e)
}

func (qt *QUICTransport) OnRemoteEvent(fn func(eventbus.Event)) {
	qt.onEvent = fn
}

// Close method for graceful shutdown
func (qt *QUICTransport) Close() error {
	qt.lock.Lock()
	defer qt.lock.Unlock()

	var err error

	if qt.listener != nil {
		err = qt.listener.Close()
		qt.listener = nil
	}

	if qt.conn != nil {
		closeErr := qt.conn.CloseWithError(0, "shutdown")
		if err == nil {
			err = closeErr
		}
		qt.conn = nil
	}

	return err
}
