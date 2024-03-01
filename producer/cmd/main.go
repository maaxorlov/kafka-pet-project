package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/brianvoe/gofakeit/v6"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/maaxorlov/kafka-pet-project/producer/pkg"
	"github.com/segmentio/kafka-go"
)

const (
	producerIterationMinSleep = int(1 * time.Second)
	producerIterationMaxSleep = int(5 * time.Second)
)

var (
	brokers = ""
	topic   = ""
)

func init() {
	flag.StringVar(&brokers, "brokers", os.Getenv("KAFKA_BROKERS"), "Kafka bootstrap brokers to connect to, as a comma separated list")
	flag.StringVar(&topic, "paymentsTopic", os.Getenv("KAFKA_PAYMENTS_TOPIC"), "...")
	flag.Parse()

	if len(brokers) == 0 {
		panic("no Kafka bootstrap brokers defined, please set the -brokers flag")
	}

	if len(topic) == 0 {
		panic("no paymentsTopic given to write, please set the -paymentsTopic flag")
	}
}

func main() {
	addrs := strings.Split(brokers, ",")

	producer := newProducer(
		&kafka.Writer{
			Addr:  kafka.TCP(addrs...),
			Topic: topic,
		},
	)

	fmt.Println("starting producer program...")
	fmt.Println("======== ENV ========")
	fmt.Printf("brokers: %s\ntopic: %s\n", brokers, topic)
	fmt.Println("=====================")

	producer.run()

	// graceful shutdown
	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-quit:
		fmt.Println("shutdown os signal")
	case err := <-producer.errNotify:
		fmt.Println("producer error:", err)
	}

	if err := producer.shutdown(); err != nil {
		fmt.Println("producer shutdown error:", err)
	} else {
		fmt.Println("producer gracefully stopped")
	}
}

type producer struct {
	writer    *kafka.Writer
	errNotify chan error
}

func newProducer(writer *kafka.Writer) *producer {
	return &producer{
		writer:    writer,
		errNotify: make(chan error, 1),
	}
}

func (p *producer) run() {
	go func() {
		if err := p.producePayments(); err != nil {
			p.errNotify <- err
			close(p.errNotify)
		}
	}()
}

func (p *producer) producePayments() error {
	for {
		payment := &pkg.Payment{
			ID:           uuid.New(),
			IsSuccessful: gofakeit.Bool(),
			Amount:       gofakeit.Float64Range(100, 5000),
		}
		payload, err := json.Marshal(payment)
		if err != nil {
			return fmt.Errorf("json marshal error: %v (payment: %+v)\n", err, *payment)
		}

		message := kafka.Message{
			Key:   []byte(payment.ID.String()),
			Value: payload,
		}
		if err = p.writer.WriteMessages(context.Background(), message); err != nil {
			return fmt.Errorf("kafka write error: %v\n", err)
		}

		fmt.Printf("kafka write success: topic: %s key: %s value: %s\n", p.writer.Topic, message.Key, message.Value)

		randomSleep := time.Second * 3 //time.Duration(gofakeit.Number(producerIterationMinSleep, producerIterationMaxSleep))
		time.Sleep(randomSleep)
	}
}

func (p *producer) shutdown() error {
	if err := p.writer.Close(); err != nil {
		return fmt.Errorf("kafka close writer error: %v", err)
	}

	return nil
}
