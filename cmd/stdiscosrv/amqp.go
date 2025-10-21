// Copyright (C) 2024 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"context"
	"fmt"
	"io"
	"log"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/thejerf/suture/v4"
	"google.golang.org/protobuf/proto"

	"github.com/syncthing/syncthing/internal/gen/discosrv"
	"github.com/syncthing/syncthing/internal/protoutil"
	"github.com/syncthing/syncthing/lib/protocol"
)

type amqpReplicator struct {
	suture.Service

	broker   string
	sender   *amqpSender
	receiver *amqpReceiver
	outbox   chan *discosrv.ReplicationRecord
}

func newAMQPReplicator(broker, clientID string, db database) *amqpReplicator {
	svc := suture.New("amqpReplicator", suture.Spec{PassThroughPanics: true})

	sender := &amqpSender{
		broker:   broker,
		clientID: clientID,
		outbox:   make(chan *discosrv.ReplicationRecord, replicationOutboxSize),
	}
	svc.Add(sender)

	receiver := &amqpReceiver{
		broker:   broker,
		clientID: clientID,
		db:       db,
	}
	svc.Add(receiver)

	return &amqpReplicator{
		Service:  svc,
		broker:   broker,
		sender:   sender,
		receiver: receiver,
		outbox:   make(chan *discosrv.ReplicationRecord, replicationOutboxSize),
	}
}

func (s *amqpReplicator) send(key *protocol.DeviceID, ps []*discosrv.DatabaseAddress, seen int64) {
	s.sender.send(key, ps, seen)
}

type amqpSender struct {
	broker   string
	clientID string
	outbox   chan *discosrv.ReplicationRecord
}

func (s *amqpSender) Serve(ctx context.Context) error {
	conn, ch, err := amqpChannel(s.broker)
	if err != nil {
		return err
	}
	defer ch.Close()
	defer conn.Close()

	buf := make([]byte, 1024)
	for {
		select {
		case rec := <-s.outbox:
			size := proto.Size(rec)
			if len(buf) < size {
				buf = make([]byte, size)
			}

			n, err := protoutil.MarshalTo(buf, rec)
			if err != nil {
				replicationSendsTotal.WithLabelValues("error").Inc()
				return fmt.Errorf("replication marshal: %w", err)
			}

			err = ch.PublishWithContext(ctx,
				"discovery", // exchange
				"",          // routing key
				false,       // mandatory
				false,       // immediate
				amqp.Publishing{
					ContentType: "application/protobuf",
					Body:        buf[:n],
					AppId:       s.clientID,
				})
			if err != nil {
				replicationSendsTotal.WithLabelValues("error").Inc()
				return fmt.Errorf("replication publish: %w", err)
			}

			replicationSendsTotal.WithLabelValues("success").Inc()

		case <-ctx.Done():
			return nil
		}
	}
}

func (s *amqpSender) String() string {
	return fmt.Sprintf("amqpSender(%q)", s.broker)
}

func (s *amqpSender) send(key *protocol.DeviceID, ps []*discosrv.DatabaseAddress, seen int64) {
	item := &discosrv.ReplicationRecord{
		Key:       key[:],
		Addresses: ps,
		Seen:      seen,
	}

	// The send should never block. The inbox is suitably buffered for at
	// least a few seconds of stalls, which shouldn't happen in practice.
	select {
	case s.outbox <- item:
	default:
		replicationSendsTotal.WithLabelValues("drop").Inc()
	}
}

type amqpReceiver struct {
	broker   string
	clientID string
	db       database
}

func (s *amqpReceiver) Serve(ctx context.Context) error {
	conn, ch, err := amqpChannel(s.broker)
	if err != nil {
		return err
	}
	defer ch.Close()
	defer conn.Close()

	msgs, err := amqpConsume(ch)
	if err != nil {
		return err
	}

	for {
		select {
		case msg, ok := <-msgs:
			if !ok {
				return fmt.Errorf("subscription closed: %w", io.EOF)
			}

			// ignore messages from ourself
			if msg.AppId == s.clientID {
				continue
			}

			var rec discosrv.ReplicationRecord
			if err := proto.Unmarshal(msg.Body, &rec); err != nil {
				replicationRecvsTotal.WithLabelValues("error").Inc()
				return fmt.Errorf("replication unmarshal: %w", err)
			}
			id, err := protocol.DeviceIDFromBytes(rec.Key)
			if err != nil {
				id, err = protocol.DeviceIDFromString(string(rec.Key))
			}
			if err != nil {
				log.Println("Replication device ID:", err)
				replicationRecvsTotal.WithLabelValues("error").Inc()
				continue
			}

			if err := s.db.merge(&id, rec.Addresses, rec.Seen); err != nil {
				return fmt.Errorf("replication database merge: %w", err)
			}

			replicationRecvsTotal.WithLabelValues("success").Inc()

		case <-ctx.Done():
			return nil
		}
	}
}

func (s *amqpReceiver) String() string {
	return fmt.Sprintf("amqpReceiver(%q)", s.broker)
}

func amqpChannel(dst string) (*amqp.Connection, *amqp.Channel, error) {
	conn, err := amqp.Dial(dst)
	if err != nil {
		return nil, nil, fmt.Errorf("AMQP dial: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		return nil, nil, fmt.Errorf("AMQP channel: %w", err)
	}

	err = ch.ExchangeDeclare(
		"discovery", // name
		"fanout",    // type
		false,       // durable
		false,       // auto-deleted
		false,       // internal
		false,       // no-wait
		nil,         // arguments
	)
	if err != nil {
		return nil, nil, fmt.Errorf("AMQP declare exchange: %w", err)
	}

	return conn, ch, nil
}

func amqpConsume(ch *amqp.Channel) (<-chan amqp.Delivery, error) {
	q, err := ch.QueueDeclare(
		"",    // name
		false, // durable
		false, // delete when unused
		true,  // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		return nil, fmt.Errorf("AMQP declare queue: %w", err)
	}

	err = ch.QueueBind(
		q.Name,      // queue name
		"",          // routing key
		"discovery", // exchange
		false,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("AMQP bind queue: %w", err)
	}

	msgs, err := ch.Consume(
		q.Name, // queue
		"",     // consumer
		true,   // auto-ack
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	if err != nil {
		return nil, fmt.Errorf("AMQP consume: %w", err)
	}

	return msgs, nil
}
