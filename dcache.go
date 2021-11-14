package dcache

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/coredns/coredns/plugin/metrics"

	"github.com/goccy/go-json"

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
	state := &request.Request{Req: r, W: w}
	unix := time.Now().Unix()
	cr, hit := d.cache.Get(unix, state)

	d.log.Debugf("cache hit %t", hit)

	rw := NewResponsePrinter(w, d.log, d)
	s := metrics.WithServer(ctx)

	if !hit {
		cacheMisses.WithLabelValues(s).Inc()
		return plugin.NextOrFailure(d.Name(), d.Next, ctx, rw, r)
	}
	cacheHits.WithLabelValues(s).Inc()

	cr.Response.SetReply(r)
	_ = w.WriteMsg(cr.Response)

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
	d.log.Info("start distribute cache receive routine")
	defer func() {
		_ = d.pubSubConn.Close()
	}()
	ctx := context.Background()

	sub := d.pubSubConn.Subscribe(ctx, d.Name())
	for {
		m, err := sub.ReceiveMessage(ctx)
		if err != nil {
			d.log.Errorf("failed receive %s", err)
		}

		ans := &AnswerCache{}
		if err := json.Unmarshal([]byte(m.Payload), ans); err != nil {
			d.log.Errorf("error unmarshal %s got %v", err, ans)
		}

		if err = d.cache.Set(ans); err != nil {
			d.log.Errorf("cache set failed got %v err %s", m, err)
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

	// truncated data not cache.
	if ans.Response.Truncated {
		return
	}

	ctx := context.Background()

	b, err := ans.MarshalJSON()
	if err != nil {
		d.log.Errorf("failed marshal %s %v", err, ans)
		return
	}

	cmd := d.pool.Publish(ctx, d.Name(), string(b))
	if cmd.Err() != nil {
		s := metrics.WithServer(ctx)
		redisErr.WithLabelValues(s).Inc()
		d.log.Errorf("error publish err %s", cmd.Err())
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
	do := false

	mt, opt := response.Typify(res, time.Now().UTC())
	if opt != nil {
		do = opt.Do()
	}
	now := time.Now().UTC().Unix()

	ans := &AnswerCache{
		Type:      dns.Type(mt),
		Do:        do,
		Response:  res,
		TimeToDie: now + int64(r.cache.minTTL(res)),
	}

	switch mt {
	case response.NoError, response.Delegation, response.NoData:
		go r.cache.publish(ans)
	case response.NameError:
		// todo
	case response.OtherError:
		// todo
	default:
		r.log.Warningf("unknown type %#v", mt)
	}
	res.Answer = filterRRSlice(res.Answer, do)
	res.Ns = filterRRSlice(res.Ns, do)
	res.Extra = filterRRSlice(res.Extra, do)

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
	Response  *dns.Msg `json:"response"`
	Type      dns.Type `json:"type"`
	Do        bool     `json:"do"`
	TimeToDie int64    `json:"time_to_die"`
}

func (a *AnswerCache) MarshalJSON() ([]byte, error) {
	if a == nil {
		return nil, nil
	}

	b, err := a.Response.Pack()
	if err != nil {
		return nil, err
	}

	return json.Marshal(&struct {
		Response  []byte
		Type      dns.Type
		Do        bool
		TimeToDie int64
	}{
		Response:  b,
		Type:      a.Type,
		Do:        a.Do,
		TimeToDie: a.TimeToDie,
	})
}
func (a *AnswerCache) UnmarshalJSON(data []byte) error {
	if a == nil {
		return nil
	}

	ans := &struct {
		Type      dns.Type
		Do        bool
		TimeToDie int64
		Response  []byte
	}{
		Type:      a.Type,
		Do:        a.Do,
		TimeToDie: a.TimeToDie,
	}

	if err := json.Unmarshal(data, &ans); err != nil {
		return err
	}

	a.Type = ans.Type
	a.Do = ans.Do
	a.TimeToDie = ans.TimeToDie
	a.Response = &dns.Msg{}
	return a.Response.Unpack(ans.Response)
}

const FormatCacheKey = "%s:%d:%t"

func (c *CacheRepository) key(name string, t uint16, r bool) string {
	return fmt.Sprintf(FormatCacheKey, name, t, r)
}

func (c *CacheRepository) Get(now int64, r *request.Request) (*AnswerCache, bool) {
	key := c.key(r.QName(), r.QType(), r.Do())
	v, ok := c.items.Get(key)

	if !ok {
		return nil, false
	}

	cn, ok := v.(*AnswerCache)
	if !ok {
		s := metrics.WithServer(context.Background())
		corruptedCache.WithLabelValues(s).Inc()
		return nil, false
	}

	expire := now-cn.TimeToDie > 0
	if expire {
		c.items.Remove(key)
		return nil, false
	}

	return cn, true
}

var ErrNotEnoughAnswer = fmt.Errorf("answer not enough length")

func (c *CacheRepository) Set(msg *AnswerCache) error {
	if len(msg.Response.Answer) == 0 {
		return ErrNotEnoughAnswer
	}

	qtype := msg.Response.Answer[0].Header().Rrtype
	name := msg.Response.Answer[0].Header().Name
	do := msg.Do

	newExtra := make([]dns.RR, len(msg.Response.Extra))

	j := 0
	for _, e := range msg.Response.Extra {
		if e.Header().Rrtype == dns.TypeOPT {
			continue
		}
		newExtra[j] = e
		j++
	}
	msg.Response.Extra = newExtra[:j]

	_ = c.items.Add(c.key(name, qtype, do), msg)
	return nil
}

//https://github.com/coredns/coredns/blob/002b748ccd6b7cc2e3a65f1bd71509f80b95d342/plugin/cache/dnssec.go#L24-L46
func filterRRSlice(rrs []dns.RR, do bool) []dns.RR {
	j := 0
	rs := make([]dns.RR, len(rrs), len(rrs))
	for _, r := range rrs {
		if !do && isDNSSEC(r) {
			continue
		}
		if r.Header().Rrtype == dns.TypeOPT {
			continue
		}

		rs[j] = r
		j++
	}
	return rs[:j]
}

// client explicitly asked for it.
//https://github.com/coredns/coredns/blob/002b748ccd6b7cc2e3a65f1bd71509f80b95d342/plugin/cache/dnssec.go#L5-L22
func isDNSSEC(r dns.RR) bool {
	switch r.Header().Rrtype {
	case dns.TypeNSEC:
		return true
	case dns.TypeNSEC3:
		return true
	case dns.TypeDS:
		return true
	case dns.TypeRRSIG:
		return true
	case dns.TypeSIG:
		return true
	}
	return false
}
