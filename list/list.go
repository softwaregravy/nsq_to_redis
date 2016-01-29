package list

import (
	"time"

	"github.com/segmentio/go-interpolate"
	"github.com/segmentio/go-log"
	"github.com/segmentio/go-stats"
	"github.com/segmentio/nsq_to_redis/broadcast"
	"github.com/statsd/client"
)

// Options for List.
type Options struct {
	Format  string         // Redis list key format
	Metrics *statsd.Client // Metrics
	Log     *log.Logger    // Logger
	Size    int64          // List size
}

// List writes messages to capped lists.
type List struct {
	template *interpolate.Template
	stats    *stats.Stats
	*Options
}

// New list with options.
func New(options *Options) (*List, error) {
	r := &List{
		Options: options,
		stats:   stats.New(),
	}

	tmpl, err := interpolate.New(r.Format)
	if err != nil {
		return nil, err
	}

	r.template = tmpl
	go r.stats.TickEvery(10 * time.Second)

	return r, nil
}

// HandleMessage parses json messages received from NSQ,
// applies them against the key template to produce a
// key name, and writes to the list.
func (l *List) Handle(c *broadcast.Conn, msg *broadcast.Message) error {
	start := time.Now()

	key, err := l.template.Eval(msg.JSON)
	if err != nil {
		l.Log.Error("evaluating template: %s", err)
		return nil
	}

	l.Log.Info("pushing %s to %s", msg.ID, key)
	l.Log.Debug("contents %s %s", msg.ID, msg.Body)

	err = c.Send("LPUSH", key, msg.Body)
	if err != nil {
		l.Log.Error("lpush: %s", err)
	}

	err = c.Send("LTRIM", key, 0, l.Size-1)
	if err != nil {
		l.Log.Error("ltrim: %s", err)
	}

	l.Metrics.Duration("timers.pushed", time.Since(start))
	l.Metrics.Incr("counts.pushed")
	l.stats.Incr("pushed")
	return nil
}
