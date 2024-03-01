package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/maaxorlov/kafka-pet-project/aggregator/pkg"
	"github.com/segmentio/kafka-go"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

var (
	brokers = ""
	topic   = ""
	groupID = ""
)

func init() {
	flag.StringVar(&brokers, "brokers", os.Getenv("KAFKA_BROKERS"), "Kafka bootstrap brokers to connect to, as a comma separated list")
	flag.StringVar(&topic, "aggregatedPaymentsTopic", os.Getenv("KAFKA_AGGREGATED_PAYMENTS_TOPIC"), "...")
	flag.StringVar(&groupID, "aggregatedPaymentsGroup", os.Getenv("KAFKA_AGGREGATED_PAYMENTS_GROUP"), "...")
	flag.Parse()

	if len(brokers) == 0 {
		panic("no Kafka bootstrap brokers defined, please set the -brokers flag")
	}

	if len(topic) == 0 {
		panic("no aggregatedPaymentsTopic given to be consumed, please set the -aggregatedPaymentsTopic flag")
	}

	if len(groupID) == 0 {
		panic("no Kafka aggregatedPaymentsGroup defined, please set the -aggregatedPaymentsGroup flag")
	}
}

func main() {
	addrs := strings.Split(brokers, ",")

	consumer := newConsumer(
		kafka.NewReader(kafka.ReaderConfig{
			Brokers:  addrs,
			Topic:    topic,
			GroupID:  groupID,
			MinBytes: 1e3,  // 1KB
			MaxBytes: 10e6, // 10MB
		}),
	)

	fmt.Println("starting consumer program...")
	fmt.Println("======== ENV ========")
	fmt.Printf("brokers: %s\ntopic: %s\nconsumer groupID: %s\n", brokers, topic, groupID)
	fmt.Println("=====================")

	consumer.run()

	// graceful shutdown
	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-quit:
		fmt.Println("shutdown os signal")
	case err := <-consumer.errNotify:
		fmt.Println("consumer error:", err)
	}

	if err := consumer.shutdown(); err != nil {
		fmt.Println("consumer shutdown error:", err)
	} else {
		fmt.Println("consumer gracefully stopped")
	}
}

type consumer struct {
	reader    *kafka.Reader
	errNotify chan error
}

func newConsumer(reader *kafka.Reader) *consumer {
	return &consumer{
		reader:    reader,
		errNotify: make(chan error, 1),
	}
}

func (c *consumer) run() {
	go func() {
		if err := c.consumeAggregatedPayments(); err != nil {
			c.errNotify <- err
			close(c.errNotify)
		}
	}()
}

func (c *consumer) consumeAggregatedPayments() error {
	for {
		message, err := c.reader.ReadMessage(context.Background())
		if err != nil {
			return fmt.Errorf("kafka read error: %v", err)
		}

		var payment pkg.AggregatedPayment
		if err = json.Unmarshal(message.Value, &payment); err != nil {
			return fmt.Errorf("json unmarshal error: %v", err)
		}

		fmt.Printf("{%s} payment: %+v (topic/offset: %v/%v)\n", message.Time.String(), payment, message.Topic, message.Offset)
	}
}

func (c *consumer) shutdown() error {
	if err := c.reader.Close(); err != nil {
		return fmt.Errorf("kafka close reader error: %v", err)
	}

	return nil
}
