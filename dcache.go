package dcache

import (
	"context"
	"fmt"
	"time"

	"github.com/mediocregopher/radix/v4"

	"github.com/coredns/coredns/request"

	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	lru "github.com/hashicorp/golang-lru"

	"github.com/miekg/dns"
)

const name = "dcache"

// Dcache is a plugin that distribute shard cache.
type Dcache struct {
	Addr string
	Next plugin.Handler
	log  clog.P

	cache *CacheRepository
	pool  radix.Client
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
	state := request.Request{Req: r, W: w}
	unix := time.Now().Unix()
	cr, hit := d.cache.Get(unix, r, state)

	if !hit {
		return plugin.NextOrFailure(d.Name(), d.Next, ctx, w, r)
	}

	a := dns.Msg{}
	a.SetReply(r)
	_ = w.WriteMsg(cr)

	return dns.RcodeSuccess, nil
}

func (d *Dcache) connect() error {
	ctx := context.Background()
	var err error

	d.pool, err = (radix.PoolConfig{}).New(ctx, "tcp", d.Addr)
	if err != nil {
		return err
	}

	return d.pool.Do(ctx, radix.Cmd(nil, "PING"))
}

// Name implements the Handler interface.
func (d *Dcache) Name() string {
	return name
}

func (d *Dcache) run() {

	//d.pool. pubsub here

}

func NewCacheRepository(size int) (*CacheRepository, error) {
	c, err := lru.New(size)
	if err != nil {
		return nil, err
	}

	return &CacheRepository{
		items: c,
	}, nil
}

type CacheRepository struct {
	items *lru.Cache
}

type AnswerCache struct {
	Response  *dns.Msg
	TimeToDie int64
}

const FormatCacheKey = "%s:%d:%t"

func (c *CacheRepository) key(name string, t uint16, r request.Request) string {
	return fmt.Sprintf(FormatCacheKey, name, t, r.Do())
}

func (c *CacheRepository) Get(now int64, q *dns.Msg, r request.Request) (*dns.Msg, bool) {
	key := c.key(q.Question[0].Name, q.Question[0].Qtype, r)

	v, ok := c.items.Get(key)
	if !ok {
		return nil, false
	}

	cn, ok := v.(AnswerCache)
	if !ok {
		return nil, false
	}

	expire := now-cn.TimeToDie > 0
	if expire {
		c.items.Remove(key)
		return nil, false
	}

	return cn.Response, true
}

func (c *CacheRepository) Set(q dns.Question, dns AnswerCache, r request.Request) error {
	_ = c.items.Add(c.key(q.Name, q.Qtype, r), dns)
	return nil
}
