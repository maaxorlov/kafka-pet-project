package pkg

import (
	"github.com/google/uuid"
	"time"
)

type AggregatedPayment struct {
	TimeInterval [2]time.Time `json:"time_interval"`
	TotalAmount  float64      `json:"total_amount"`
	PaymentIds   []uuid.UUID  `json:"payment_ids"`
}

func NewAggregatedPayment(t time.Time, d time.Duration) *AggregatedPayment {
	return &AggregatedPayment{
		TimeInterval: getTimeInterval(t, d),
	}
}

func getTimeInterval(t time.Time, d time.Duration) [2]time.Time {
	nsec := t.UnixNano() / int64(d) * int64(d)
	start := time.Unix(0, nsec).UTC()
	end := start.Add(d)

	return [2]time.Time{start, end}
}
