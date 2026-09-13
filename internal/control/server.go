package control

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"kaffeinate/internal/session"
)

type SessionStarter interface {
	Start(context.Context, session.Request) (session.Snapshot, error)
}

type Server struct {
	listener       *net.UnixListener
	lockFile       *os.File
	sessionStarter SessionStarter
	mu             sync.Mutex
	connections    map[net.Conn]struct{}
	closing        bool
	workers        sync.WaitGroup
	closeOnce      sync.Once
	closeErr       error
	stopOnce       sync.Once
	stopErr        error
}

func Listen(ctx context.Context, directory string, sessionStarter SessionStarter) (*Server, error) {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	listener, lockFile, err := awaitInstance(ctx, directory)
	if err != nil {
		return nil, err
	}
	server := &Server{listener: listener, lockFile: lockFile, sessionStarter: sessionStarter, connections: make(map[net.Conn]struct{})}
	server.workers.Go(server.serve)
	return server, nil
}

// StopRequests drains clients while retaining instance ownership until Close.
func (server *Server) StopRequests() error {
	server.stopOnce.Do(func() {
		server.mu.Lock()
		server.closing = true
		server.stopErr = server.listener.Close()
		for connection := range server.connections {
			_ = connection.Close()
		}
		server.mu.Unlock()
		server.workers.Wait()
	})
	return server.stopErr
}

func (server *Server) Close() error {
	server.closeOnce.Do(func() {
		server.closeErr = server.StopRequests()
		if err := server.lockFile.Close(); server.closeErr == nil {
			server.closeErr = err
		}
	})
	return server.closeErr
}

func (server *Server) serve() {
	for {
		connection, err := server.listener.AcceptUnix()
		if err != nil {
			return
		}
		server.mu.Lock()
		if server.closing || len(server.connections) >= 32 {
			server.mu.Unlock()
			_ = connection.Close()
			continue
		}
		server.connections[connection] = struct{}{}
		server.workers.Go(func() { server.handle(connection) })
		server.mu.Unlock()
	}
}

func (server *Server) handle(connection net.Conn) {
	defer func() {
		_ = connection.Close()
		server.mu.Lock()
		delete(server.connections, connection)
		server.mu.Unlock()
	}()
	_ = connection.SetDeadline(time.Now().Add(ExchangeTimeout))
	var request Request
	response := Response{Version: ProtocolVersion}
	if err := decodeMessage(connection, &request); err != nil {
		response.Error = err.Error()
	} else {
		response.ID = request.ID
		switch {
		case request.Version != ProtocolVersion:
			response.Error = "incompatible control protocol; rebuild and reopen Kaffeinate"
		case request.ID == "" || len(request.ID) > 128:
			response.Error = "invalid request identifier"
		default:
			if err := request.Session.Validate(); err != nil {
				response.Error = err.Error()
			} else {
				ctx, cancel := context.WithTimeout(context.Background(), ExchangeTimeout)
				var err error
				response.State, err = server.sessionStarter.Start(ctx, request.Session)
				cancel()
				if err != nil {
					response.Error = fmt.Sprintf("start session: %v", err)
				} else if response.State.Error != "" {
					response.Error = response.State.Error
				}
			}
		}
	}
	_ = json.NewEncoder(connection).Encode(response)
}
