package control

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"
	"time"

	"kaffeinate/internal/power"
	"kaffeinate/internal/session"
)

type Client struct {
	SocketPath string
}

func (client Client) Start(ctx context.Context, request session.Request) (session.Snapshot, error) {
	if err := request.Validate(); err != nil {
		return session.Snapshot{}, err
	}
	connection, err := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "unix", client.SocketPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ECONNREFUSED) {
			return session.Snapshot{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		return session.Snapshot{}, fmt.Errorf("connect to Kaffeinate: %w", err)
	}
	defer connection.Close()
	stopCancellation := context.AfterFunc(ctx, func() { _ = connection.Close() })
	defer stopCancellation()
	deadline := time.Now().Add(ExchangeTimeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	_ = connection.SetDeadline(deadline)
	envelope := Request{Version: ProtocolVersion, ID: rand.Text(), Session: request}
	if err := json.NewEncoder(connection).Encode(envelope); err != nil {
		return session.Snapshot{}, unconfirmed(err)
	}
	var response Response
	if err := decodeMessage(connection, &response); err != nil {
		return session.Snapshot{}, unconfirmed(err)
	}
	if response.Version != ProtocolVersion || response.ID != envelope.ID {
		return session.Snapshot{}, unconfirmed(errors.New("unexpected application response"))
	}
	if response.Error != "" {
		return response.State, errors.New(response.Error)
	}
	if err := validateAcknowledgement(response.State); err != nil {
		return session.Snapshot{}, unconfirmed(err)
	}
	return response.State, nil
}

func validateAcknowledgement(snapshot session.Snapshot) error {
	if snapshot.Error != "" {
		return errors.New("application reported an unsuccessful session")
	}
	switch snapshot.Status {
	case session.Active:
		request := session.Request{PowerOptions: power.Options{Behaviors: snapshot.Behaviors}, WatchedPID: snapshot.WatchedPID}
		return request.Validate()
	case session.Inactive:
		if snapshot.Behaviors == 0 && snapshot.Deadline.IsZero() && snapshot.WatchedPID == 0 {
			return nil
		}
	}
	return errors.New("application returned an invalid session state")
}

func unconfirmed(err error) error {
	return fmt.Errorf("could not confirm the session; check the menu before retrying: %w", err)
}
