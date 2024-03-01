package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	. "github.com/maaxorlov/kafka-pet-project/aggregator/pkg"
	. "github.com/maaxorlov/kafka-pet-project/producer/pkg"
	"github.com/segmentio/kafka-go"
)

var (
	brokers                 = ""
	paymentsTopic           = ""
	aggregatedPaymentsTopic = ""
	aggregatorsGroupID      = ""
	tumblingWindowSize      = time.Duration(0)
)

func init() {
	flag.StringVar(&brokers, "brokers", os.Getenv("KAFKA_BROKERS"), "Kafka bootstrap brokers to connect to, as a comma separated list")
	flag.StringVar(&paymentsTopic, "paymentsTopic", os.Getenv("KAFKA_PAYMENTS_TOPIC"), "...")
	flag.StringVar(&aggregatedPaymentsTopic, "aggregatedPaymentsTopic", os.Getenv("KAFKA_AGGREGATED_PAYMENTS_TOPIC"), "...")
	flag.StringVar(&aggregatorsGroupID, "aggregatorsGroupID", os.Getenv("KAFKA_AGGREGATORS_GROUP"), "...")
	tumblingWindowSizeStr := ""
	flag.StringVar(&tumblingWindowSizeStr, "tumblingWindowSize", os.Getenv("KAFKA_TUMBLING_WINDOW_SIZE"), "...")
	flag.Parse()

	if len(brokers) == 0 {
		panic("no Kafka bootstrap brokers defined, please set the -brokers flag")
	}

	if len(paymentsTopic) == 0 {
		panic("no paymentsTopic given to be consumed, please set the -paymentsTopic flag")
	}

	if len(aggregatedPaymentsTopic) == 0 {
		panic("no aggregatedPaymentsTopic given to be consumed, please set the -aggregatedPaymentsTopic flag")
	}

	if len(aggregatorsGroupID) == 0 {
		panic("no Kafka consumer aggregatorsGroupID defined, please set the -aggregatorsGroupID flag")
	}

	if len(tumblingWindowSizeStr) == 0 {
		panic("no tumblingWindowSize, please set the -tumblingWindowSize flag")
	}
	size, err := strconv.Atoi(tumblingWindowSizeStr)
	if err != nil {
		panic("not correct tumblingWindowSize format, please set the -tumblingWindowSize flag as number")
	}
	tumblingWindowSize = time.Duration(size)
}

func main() {
	addrs := strings.Split(brokers, ",")

	aggregator := newAggregator(
		kafka.NewReader(kafka.ReaderConfig{
			Brokers:  addrs,
			GroupID:  aggregatorsGroupID,
			Topic:    paymentsTopic,
			MinBytes: 1e3,  // 1KB
			MaxBytes: 10e6, // 10MB
		}),
		&kafka.Writer{
			Addr:     kafka.TCP(addrs...),
			Topic:    aggregatedPaymentsTopic,
			Balancer: &kafka.RoundRobin{},
		},
	)

	fmt.Println("starting aggregator program...")
	fmt.Println("======== ENV ========")
	fmt.Printf("brokers: %s\npaymentsTopic: %s\naggregatedPaymentsTopic: %s\naggregatorsGroupID: %s\ntumblingWindowSize: %v\n", brokers, paymentsTopic, aggregatedPaymentsTopic, aggregatorsGroupID, tumblingWindowSize.String())
	fmt.Println("=====================")

	aggregator.run()

	// graceful shutdown
	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-quit:
		fmt.Println("shutdown os signal")
	case err := <-aggregator.errNotify:
		fmt.Println("aggregator error:", err)
	}

	if err := aggregator.shutdown(); err != nil {
		fmt.Println("aggregator shutdown error:", err)
	} else {
		fmt.Println("aggregator gracefully stopped")
	}
}

type aggregator struct {
	reader    *kafka.Reader
	writer    *kafka.Writer
	payment   *AggregatedPayment
	errNotify chan error
}

func newAggregator(reader *kafka.Reader, writer *kafka.Writer) *aggregator {
	a := &aggregator{
		reader:    reader,
		writer:    writer,
		errNotify: make(chan error, 1),
	}

	a.payment = NewAggregatedPayment(time.Now().UTC(), tumblingWindowSize)

	return a
}

func (a *aggregator) run() {
	go func() {
		if err := a.aggregatePayments(); err != nil {
			a.errNotify <- err
			close(a.errNotify)
		}
	}()
}

func (a *aggregator) aggregatePayments() error {
	for {
		ctx, cancelCtx := context.WithTimeout(context.Background(), tumblingWindowSize)
		message, err := a.reader.ReadMessage(ctx)
		cancelCtx()
		if errors.Is(err, context.DeadlineExceeded) {
			if err = a.writeAggregatedPayment(); err != nil {
				return fmt.Errorf("write aggregated payment error: %v\n", err)
			}

			a.payment = NewAggregatedPayment(a.payment.TimeInterval[1], tumblingWindowSize)

			continue
		} else if err != nil {
			return fmt.Errorf("kafka read error: %v", err)
		}

		var payment Payment
		if err = json.Unmarshal(message.Value, &payment); err != nil {
			return fmt.Errorf("json unmarshal error: %v", err)
		}

		fmt.Printf("{%s} payment: %+v\n", message.Time.String(), payment)

		if message.Time.Before(a.payment.TimeInterval[0]) {
			a.payment = NewAggregatedPayment(message.Time, tumblingWindowSize)
		}

		if message.Time.After(a.payment.TimeInterval[1]) {
			newTime := message.Time

			if err = a.writeAggregatedPayment(); err != nil {
				return fmt.Errorf("write aggregated payment error: %v\n", err)
			}

			a.payment = NewAggregatedPayment(newTime, tumblingWindowSize)
		}

		a.addPayment(&payment)
	}
}

func (a *aggregator) writeAggregatedPayment() (err error) {
	message := kafka.Message{}
	if message.Value, err = json.Marshal(a.payment); err != nil {
		return fmt.Errorf("json marshal error: %v (payment: %+v)\n", err, a.payment)
	}

	if err = a.writer.WriteMessages(context.Background(), message); err != nil {
		return fmt.Errorf("kafka write error: %v", err)
	}

	return nil
}

func (a *aggregator) addPayment(payment *Payment) {
	if payment.IsSuccessful {
		a.payment.PaymentIds = append(a.payment.PaymentIds, payment.ID)
		a.payment.TotalAmount += payment.Amount
	}
}

func (a *aggregator) shutdown() error {
	readerCloseErr := a.reader.Close()
	writerCloseErr := a.writer.Close()
	if readerCloseErr != nil || writerCloseErr != nil {
		return fmt.Errorf("kafka close reader error: %v kafka close writer error: %v", readerCloseErr, writerCloseErr)
	}

	return nil
}
