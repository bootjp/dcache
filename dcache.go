package dcache

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/coredns/coredns/plugin/pkg/response"
	"github.com/go-redis/redis/v8"

	"github.com/coredns/coredns/request"

	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	lru "github.com/hashicorp/golang-lru"

	"github.com/miekg/dns"
)

const name = "dcache"

// Dcache is a plugin that distribute shard cache.
type Dcache struct {
	init bool
	Addr string
	Next plugin.Handler
	log  clog.P

	cache      *CacheRepository
	pubSubConn *redis.Client
	pool       *redis.Client
}

func New(host string) *Dcache {
	l, _ := NewCacheRepository(1000)

	return &Dcache{
		Addr:  host,
		cache: l,
	}
}

// ServeDNS implements the plugin.Handler interface.
func (d *Dcache) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	d.log.Info("serve")
	state := request.Request{Req: r, W: w}
	unix := time.Now().Unix()
	cr, hit := d.cache.Get(unix, r, state.Do())

	rw := NewResponsePrinter(w, d.log, d)

	if !hit {
		return plugin.NextOrFailure(d.Name(), d.Next, ctx, rw, r)
	}

	cr.SetReply(r)
	w.WriteMsg(cr)

	return dns.RcodeSuccess, nil
}

func (d *Dcache) connect() error {
	ctx := context.Background()

	d.pool = redis.NewClient(&redis.Options{
		Addr: d.Addr,
	})

	d.pubSubConn = redis.NewClient(&redis.Options{
		Addr: d.Addr,
	})

	if cmd := d.pubSubConn.Ping(ctx); cmd.Err() != nil {
		d.log.Error("failed connect redis", cmd.Err())
	}

	if cmd := d.pool.Ping(ctx); cmd.Err() != nil {
		d.log.Error("failed connect redis", cmd.Err())
	}

	return nil
}

// Name implements the Handler interface.
func (d *Dcache) Name() string {
	return name
}

func (d *Dcache) run() {

	d.log.Info("run")
	defer d.pubSubConn.Close()
	ctx := context.Background()

	d.log.Info("loop")
	sub := d.pubSubConn.Subscribe(ctx, d.Name())
	for {
		m, err := sub.ReceiveMessage(ctx)
		if err != nil {
			d.log.Errorf("failed receive %s", err)
		}

		b, err := base64.RawStdEncoding.DecodeString(m.Payload)
		if err != nil {
			d.log.Error("failed decode", err)
			continue
		}

		msg := &dns.Msg{}
		err = msg.Unpack(b)
		if err != nil {
			d.log.Error("failed unpack", err)
			continue
		}

		if err = d.cache.Set(*msg); err != nil {
			d.log.Errorf("cache set failed %s", m, err)
			d.log.Error(err)
			continue
		}
	}
}

func (d *Dcache) minTTL(msg *dns.Msg) uint32 {
	min := uint32(math.MaxUint32)
	for _, ans := range msg.Answer {
		if min > ans.Header().Ttl {
			min = ans.Header().Ttl
		}
	}

	return min
}

func (d *Dcache) publish(ans *AnswerCache) {
	ctx := context.Background()

	b, err := ans.Response.Pack()
	if err != nil {
		d.log.Errorf("pack error %s %s", err, ans)
		return
	}

	msgBase64 := base64.RawStdEncoding.EncodeToString(b)
	cmd := d.pool.Publish(ctx, d.Name(), msgBase64)
	if cmd.Err() != nil {
		d.log.Errorf("error publish", cmd.Err())
		return
	}
}

type ResponsePrinter struct {
	dns.ResponseWriter
	log   clog.P
	cache *Dcache
}

// NewResponsePrinter returns ResponseWriter.
func NewResponsePrinter(w dns.ResponseWriter, log clog.P, d *Dcache) *ResponsePrinter {
	return &ResponsePrinter{
		ResponseWriter: w,
		log:            log,
		cache:          d,
	}
}

// WriteMsg calls the underlying ResponseWriter's WriteMsg method and prints "example" to standard output.
func (r *ResponsePrinter) WriteMsg(res *dns.Msg) error {
	r.log.Info("writemgs")

	do := false
	mt, opt := response.Typify(res, time.Now().UTC())
	if opt != nil {
		do = opt.Do()
	}
	now := time.Now().UTC().Unix()

	ans := &AnswerCache{
		Type:      dns.Type(mt),
		Do:        do,
		Response:  *res,
		TimeToDie: now + int64(r.cache.minTTL(res)),
	}

	switch mt {
	case response.NoError, response.Delegation, response.NoData:
		r.cache.publish(ans)
	case response.NameError:
		// todo
	case response.OtherError:
		// todo
	default:
		r.log.Warningf("Redis called with unknown typification: %d", mt)
	}
	return r.ResponseWriter.WriteMsg(res)
}

func NewCacheRepository(size int) (*CacheRepository, error) {
	c, err := lru.New(size)
	if err != nil {
		return nil, err
	}

	return &CacheRepository{items: c}, nil
}

type CacheRepository struct {
	items *lru.Cache
}

type AnswerCache struct {
	Response  dns.Msg  `json:"response"`
	Type      dns.Type `json:"type"`
	Do        bool     `json:"do"`
	TimeToDie int64    `json:"time_to_die"`
}

const FormatCacheKey = "%s:%d:%t"

func (c *CacheRepository) key(name string, t uint16, r bool) string {
	return fmt.Sprintf(FormatCacheKey, name, t, r)
}

func (c *CacheRepository) Get(now int64, q *dns.Msg, do bool) (*dns.Msg, bool) {
	key := c.key(q.Question[0].Name, q.Question[0].Qtype, do)

	v, ok := c.items.Get(key)
	if !ok {
		return nil, false
	}

	cn, ok := v.(dns.Msg)
	if !ok {
		return nil, false
	}

	// todo calculate expire
	//
	//expire := now-cn.TimeToDie > 0
	//if expire {
	//	c.items.Remove(key)
	//	return nil, false
	//}

	return &cn, true
}

func (c *CacheRepository) Set(msg dns.Msg) error {
	if len(msg.Answer) == 0 {
		return errors.New("")
	}

	qtype := msg.Answer[0].Header().Rrtype
	name := msg.Answer[0].Header().Name
	do := msg.IsEdns0().Do()

	_ = c.items.Add(c.key(name, qtype, do), msg)
	return nil
}
