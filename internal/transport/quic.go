package transport

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"io"
	"log"
	"reflect"
	"strings"
	"sync"

	"github.com/goppydae/gapi/internal/eventbus"
	protopkg "github.com/goppydae/gapi/internal/proto"
	quicgo "github.com/quic-go/quic-go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

type QUIC[T proto.Message] struct {
	listener *quicgo.Listener
	conn     quicgo.Connection

	onRemote func(eventbus.Event[T])
	mu       sync.Mutex
}

// Server: listens and accepts inbound connections/streams.
func NewQUICServer[T proto.Message](addr string, cert tls.Certificate) (*QUIC[T], error) {
	tlsConf := &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"gapi-quic"},
	}
	ln, err := quicgo.ListenAddr(addr, tlsConf, nil)
	if err != nil {
		return nil, err
	}
	q := &QUIC[T]{listener: ln}
	go q.acceptLoop()
	return q, nil
}

// Client: dials a remote QUIC endpoint.
func NewQUICClient[T proto.Message](addr string, cert *tls.Certificate) (*QUIC[T], error) {
	tlsConf := &tls.Config{
		Certificates:       []tls.Certificate{*cert},
		InsecureSkipVerify: true, // tune per your trust model
		NextProtos:         []string{"gapi-quic"},
	}
	conn, err := quicgo.DialAddr(context.Background(), addr, tlsConf, nil)
	if err != nil {
		return nil, err
	}
	q := &QUIC[T]{conn: conn}
	go q.handleConn(conn)
	return q, nil
}

func (q *QUIC[T]) acceptLoop() {
	for {
		conn, err := q.listener.Accept(context.Background())
		if err != nil {
			log.Println("QUIC accept:", err)
			return
		}
		go q.handleConn(conn)
	}
}

func (q *QUIC[T]) handleConn(conn quicgo.Connection) {
	q.mu.Lock()
	q.conn = conn
	q.mu.Unlock()
	for {
		s, err := conn.AcceptStream(context.Background())
		if err != nil {
			log.Println("AcceptStream:", err)
			return
		}
		go q.handleStream(s)
	}
}

func (q *QUIC[T]) handleStream(s quicgo.Stream) {
	defer s.Close()

	var lenBuf [4]byte
	if _, err := io.ReadFull(s, lenBuf[:]); err != nil {
		log.Println("read len:", err)
		return
	}
	n := binary.BigEndian.Uint32(lenBuf[:])

	data := make([]byte, n)
	if _, err := io.ReadFull(s, data); err != nil {
		log.Println("read payload:", err)
		return
	}

	var env protopkg.Envelope
	if err := proto.Unmarshal(data, &env); err != nil {
		log.Println("unmarshal:", err)
		return
	}

	msg := allocT[T]()
	if env.Payload != nil {
		if err := env.Payload.UnmarshalTo(msg); err != nil {
			log.Println("unmarshal payload:", err)
			return
		}
	}

	sc := ""
	tp := env.Topic
	if i := strings.IndexByte(env.Topic, '/'); i > 0 {
		sc = env.Topic[:i]
		tp = env.Topic[i+1:]
	}

	e := eventbus.Event[T]{
		ID:      env.Id,
		Scope:   sc, // <- restore scope
		Topic:   tp, // <- topic without scope prefix
		Source:  env.Source,
		Payload: msg,
	}

	if q.onRemote != nil {
		q.onRemote(e)
	}
}

func allocT[T proto.Message]() T {
	var zero T
	typ := reflect.TypeOf(zero)
	if typ.Kind() == reflect.Ptr {
		return reflect.New(typ.Elem()).Interface().(T)
	}
	return zero
}

func (q *QUIC[T]) PublishRemote(e eventbus.Event[T]) error {
	q.mu.Lock()
	conn := q.conn
	q.mu.Unlock()
	if conn == nil {
		return io.ErrUnexpectedEOF
	}

	s, err := conn.OpenStreamSync(context.Background())
	if err != nil {
		return err
	}
	defer s.Close()

	anyPayload, err := anypb.New(e.Payload)
	if err != nil {
		return err
	}
	wireTopic := e.Topic
	if e.Scope != "" {
		wireTopic = e.Scope + "/" + e.Topic
	}
	env := &protopkg.Envelope{
		Id:      e.ID,
		Topic:   wireTopic,
		Source:  e.Source,
		Type:    "event",
		Payload: anyPayload,
	}

	b, err := proto.Marshal(env)
	if err != nil {
		return err
	}

	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(b)))

	if _, err := s.Write(lenBuf[:]); err != nil {
		return err
	}
	if _, err := s.Write(b); err != nil {
		return err
	}
	return nil
}

func (q *QUIC[T]) Broadcast(e eventbus.Event[T]) error {
	// Single-conn implementation; if you maintain multiple peers, iterate them here.
	return q.PublishRemote(e)
}

func (q *QUIC[T]) OnRemoteEvent(fn func(eventbus.Event[T])) { q.onRemote = fn }

func (q *QUIC[T]) Close() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	var err error
	if q.listener != nil {
		err = q.listener.Close()
		q.listener = nil
	}
	if q.conn != nil {
		_ = q.conn.CloseWithError(0, "shutdown")
		q.conn = nil
	}
	return err
}
