package procdaemon

import (
	"context"
	"fmt"
	"io"
	"log"
	"strconv"
	"time"

	"github.com/goppydae/gapi/core/eventbus"
	pb "github.com/goppydae/gapi/internal/proto"
	"google.golang.org/grpc"
)

type ProcDaemon struct {
	id           string
	lang         string
	scope        string
	topicRoot    string
	conn         *grpc.ClientConn
	stream       pb.GoblinStream_CommunicateClient
	bus          *eventbus.EventBus
	status       string
	retries      int
	maxRetries   int
	restartCount int
	maxRestarts  int
	backoff      time.Duration
}

func NewProcDaemon(id, lang, scope, topicRoot, targetAddr string, bus *eventbus.EventBus) (*ProcDaemon, error) {
	conn, err := grpc.Dial(targetAddr, grpc.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	client := pb.NewGoblinStreamClient(conn)
	stream, err := client.Communicate(context.Background())
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to establish stream: %w", err)
	}

	pd := &ProcDaemon{
		id:          id,
		lang:        lang,
		scope:       scope,
		topicRoot:   topicRoot,
		conn:        conn,
		stream:      stream,
		bus:         bus,
		status:      "starting",
		maxRetries:  3,
		maxRestarts: 3,
		backoff:     2 * time.Second,
	}

	go pd.listen()
	return pd, nil
}

func (p *ProcDaemon) ID() string    { return p.id }
func (p *ProcDaemon) Scope() string { return p.scope }
func (p *ProcDaemon) Type() string  { return p.lang }

func (p *ProcDaemon) Start() error {
	p.status = "running"
	log.Printf("[%s] starting daemon via gRPC...", p.id)
	return p.sendLifecycle("start")
}

func (p *ProcDaemon) Stop() error {
	p.status = "stopped"
	return p.sendLifecycle("stop")
}

func (p *ProcDaemon) Reload() error {
	return p.sendLifecycle("reload")
}

func (p *ProcDaemon) Describe() map[string]string {
	return map[string]string{
		"id":     p.id,
		"scope":  p.scope,
		"type":   p.lang,
		"status": p.status,
	}
}

func (p *ProcDaemon) sendLifecycle(action string) error {
	return p.stream.Send(&pb.Envelope{
		Type:  "lifecycle",
		Topic: action,
	})
}

func (p *ProcDaemon) publishStatus(topic string, payload map[string]string) {
	_ = p.bus.Publish(eventbus.Event{
		Scope:     p.scope,
		Topic:     p.topicRoot + "/" + topic,
		Payload:   payload,
		Source:    p.id,
		Broadcast: false,
	})
}

func (p *ProcDaemon) listen() {
	for {
		msg, err := p.stream.Recv()
		if err == io.EOF {
			log.Printf("[%s] stream closed", p.id)
			break
		}
		if err != nil {
			log.Printf("[%s] stream error: %v", p.id, err)
			p.handleCrash(err)
			break
		}

		event := eventbus.Event{
			Scope:     p.scope,
			Topic:     p.topicRoot + "/" + msg.Topic,
			Payload:   msg.Payload,
			Source:    p.id,
			Broadcast: false,
		}
		_ = p.bus.Publish(event)
	}
}

func (p *ProcDaemon) handleCrash(err error) {
	p.status = "failed"
	p.restartCount++

	reason := err.Error()
	log.Printf("[%s] crashed (%d/%d): %s", p.id, p.restartCount, p.maxRestarts, reason)
	p.publishStatus("daemon.exit", map[string]string{"reason": reason})

	if p.restartCount <= p.maxRestarts {
		log.Printf("[%s] attempting restart...", p.id)
		p.publishStatus("daemon.restart", map[string]string{
			"attempt": strconv.Itoa(p.restartCount),
		})
		time.Sleep(p.backoff)

		// TODO: this should likely be coordinated by the caller with shared state
	} else {
		log.Printf("[%s] max restarts exceeded — permadeath", p.id)
		p.publishStatus("daemon.permadeath", map[string]string{"reason": "too many crashes"})
	}
}
