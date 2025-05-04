package transport

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"log"
	"sync"

	"github.com/goppydae/gapi/core/eventbus"
	pb "github.com/goppydae/gapi/internal/proto"
	quicgo "github.com/quic-go/quic-go"
	"google.golang.org/protobuf/proto"
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

	go qt.clientStreamLoop()
	return qt, nil
}

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
	if _, err := stream.Read(lenBuf); err != nil {
		log.Println("Read length error:", err)
		return
	}
	length := binary.BigEndian.Uint32(lenBuf)

	data := make([]byte, length)
	if _, err := stream.Read(data); err != nil {
		log.Println("Read data error:", err)
		return
	}

	var env pb.Envelope
	if err := proto.Unmarshal(data, &env); err != nil {
		log.Println("Unmarshal error:", err)
		return
	}

	e := eventbus.NewEvent("user", env.Topic, "quic-peer", env.Payload, false)
	if qt.onEvent != nil {
		qt.onEvent(e)
	}
}

func (qt *QUICTransport) PublishRemote(e eventbus.Event) error {
	qt.lock.Lock()
	defer qt.lock.Unlock()

	if qt.conn == nil {
		return nil
	}

	stream, err := qt.conn.OpenStreamSync(context.Background())
	if err != nil {
		return err
	}
	defer stream.Close()

	env := &pb.Envelope{
		Type:    e.Topic,
		Topic:   e.Topic,
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

	return nil
}

func (qt *QUICTransport) Broadcast(e eventbus.Event) error {
	return qt.PublishRemote(e)
}

func (qt *QUICTransport) OnRemoteEvent(fn func(eventbus.Event)) {
	qt.onEvent = fn
}
