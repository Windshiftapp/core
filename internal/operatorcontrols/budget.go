package operatorcontrols

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// BudgetStore must reserve atomically and persist reservations before effects.
// A crashed or ambiguous operation keeps its reservation, never free credit.
type BudgetStore interface {
	Reserve(context.Context, string, int64, int64, int64) error
	Refund(context.Context, string, int64, int64) error
	Used(context.Context, string, int64) (int64, error)
}

var ErrBudgetUnavailable = errors.New("operator budget store unavailable")

// Reserve charges a fixed UTC interval. Finish refunds only known unused units;
// omitting it leaves the reservation charged across restarts.
func (c *Controls) Reserve(ctx context.Context, resource string, amount, ceiling int64, interval time.Duration) (finish func(int64) error, err error) {
	if c == nil || ceiling == 0 {
		return func(int64) error { return nil }, ctx.Err()
	}
	if amount < 0 || interval < time.Second {
		return nil, errors.New("invalid operator reservation")
	}
	if amount > ceiling {
		return nil, &LimitError{Resource: resource}
	}
	if c.store == nil {
		return nil, ErrBudgetUnavailable
	}
	period := c.now().Unix() / int64(interval/time.Second)
	if err := c.store.Reserve(ctx, resource, period, amount, ceiling); err != nil {
		return nil, err
	}
	var once sync.Once
	var finishErr error
	return func(actual int64) error {
		once.Do(func() {
			if actual < 0 || actual > amount {
				finishErr = fmt.Errorf("invalid actual usage for %s", resource)
				return
			}
			if actual == amount {
				return
			}
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			finishErr = c.store.Refund(cleanupCtx, resource, period, amount-actual)
		})
		return finishErr
	}, nil
}

func (c *Controls) Consume(ctx context.Context, resource string, amount, ceiling int64, interval time.Duration) error {
	_, err := c.Reserve(ctx, resource, amount, ceiling, interval)
	return err
}

func (c *Controls) Used(ctx context.Context, resource string, interval time.Duration) (int64, error) {
	if c == nil || c.store == nil {
		return 0, nil
	}
	if interval < time.Second {
		return 0, errors.New("invalid operator budget interval")
	}
	return c.store.Used(ctx, resource, c.now().Unix()/int64(interval/time.Second))
}
